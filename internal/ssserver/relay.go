package ssserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"github.com/sagernet/sing/common/auth"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const maxResolvedCache = 256

var (
	errUserGone     = errors.New("user removed during handshake")
	errMissingUser  = errors.New("authenticated user missing from context")
	errBlockedRoute = errors.New("destination is not permitted")

	cgnat = netip.MustParsePrefix("100.64.0.0/10")
	zero4 = netip.MustParsePrefix("0.0.0.0/8")
)

func (u *user) addUp(n int64) {
	u.up.Add(n)
	uploadBytes.Add(float64(n))
}

func (u *user) addDown(n int64) {
	u.down.Add(n)
	downloadBytes.Add(float64(n))
}

// blocked reports addresses that must not be reachable through the proxy unless explicitly allowed.
func blocked(addr netip.Addr) bool {
	addr = addr.Unmap()
	return !addr.IsValid() || !addr.IsGlobalUnicast() || addr.IsPrivate() || cgnat.Contains(addr) || zero4.Contains(addr)
}

func (s *Server) permitted(addr netip.Addr) bool { return s.cfg.AllowPrivate || !blocked(addr) }

// controlDial runs after name resolution, so it also covers destinations given as domain names.
func (s *Server) controlDial(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errBlockedRoute
	}
	addr, err := netip.ParseAddr(host)
	if err != nil || !s.permitted(addr) {
		return errBlockedRoute
	}
	return nil
}

// rawConnKey carries the accepted socket to the relay. sing's wrapper must not be closed while another
// goroutine writes to it, whereas the raw socket can be closed from anywhere to abort the session.
type rawConnKey struct{}

type relayHandler struct{ s *Server }

func (h *relayHandler) NewError(_ context.Context, err error) {
	slog.Debug("shadowsocks error", "error", err)
}

//nolint:staticcheck // the handler interface is defined with M.Metadata.
func (h *relayHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	s := h.s
	_ = conn.SetDeadline(time.Time{}) // the handshake deadline no longer applies
	name, ok := auth.UserFromContext[string](ctx)
	if !ok {
		return errMissingUser
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	u, detach := s.attach(name, cancel)
	if u == nil {
		return errUserGone
	}
	defer detach()
	raw, ok := ctx.Value(rawConnKey{}).(net.Conn)
	if !ok {
		raw = conn
	}

	target, err := s.dialer.DialContext(ctx, "tcp", metadata.Destination.String())
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()
	// A removed user or a replaced key cancels ctx, which tears both sides down.
	stop := context.AfterFunc(ctx, func() {
		_ = raw.Close()
		_ = target.Close()
	})
	defer stop()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := copyCounted(target, conn, u.addUp)
		if closer, ok := target.(interface{ CloseWrite() error }); ok && err == nil {
			_ = closer.CloseWrite() // the client finished sending; the response may still be on its way
			return
		}
		_ = raw.Close()
		_ = target.Close()
	}()
	go func() {
		defer wg.Done()
		_ = copyCounted(conn, target, u.addDown)
		_ = raw.Close()
		_ = target.Close()
	}()
	wg.Wait()
	_ = conn.Close() // both copiers are done, so releasing sing's buffers is now safe
	return nil
}

var copyBuffers = sync.Pool{New: func() any { b := make([]byte, 32<<10); return &b }}

// copyCounted counts only bytes that were successfully delivered.
func copyCounted(dst io.Writer, src io.Reader, add func(int64)) error {
	pooled := copyBuffers.Get().(*[]byte)
	defer copyBuffers.Put(pooled)
	data := *pooled
	for {
		n, readErr := src.Read(data)
		if n > 0 {
			written, writeErr := dst.Write(data[:n])
			if written > 0 {
				add(int64(written))
			}
			if writeErr != nil {
				return writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
	}
}

//nolint:staticcheck // the handler interface is defined with M.Metadata.
func (h *relayHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, _ M.Metadata) error {
	s := h.s
	name, ok := auth.UserFromContext[string](ctx)
	if !ok {
		return errMissingUser
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	u, detach := s.attach(name, cancel)
	if u == nil {
		return errUserGone
	}
	defer detach()

	remote, err := net.ListenPacket("udp", ":0")
	if err != nil {
		return err
	}
	defer func() { _ = remote.Close() }()
	stop := context.AfterFunc(ctx, func() {
		_ = conn.Close()
		_ = remote.Close()
	})
	defer stop()

	front, rear := N.CalculateFrontHeadroom(conn), N.CalculateRearHeadroom(conn)
	go func() {
		defer cancel()
		for {
			buffer := buf.NewPacket()
			buffer.Resize(front, 0)
			buffer.Reserve(rear)
			n, from, err := remote.ReadFrom(buffer.FreeBytes())
			if err != nil {
				buffer.Release()
				return
			}
			buffer.Extend(n)
			if err := conn.WritePacket(buffer, M.SocksaddrFromNet(from).Unwrap()); err != nil {
				return
			}
			u.addDown(int64(n))
		}
	}()

	resolved := make(map[string]netip.Addr)
	for {
		buffer := buf.NewPacket()
		destination, err := conn.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			return nil // the association ended or idled out
		}
		size := buffer.Len()
		addr, err := s.resolveUDP(ctx, destination, resolved)
		if err != nil {
			buffer.Release()
			continue // unreachable or forbidden destinations drop the datagram only
		}
		_, err = remote.WriteTo(buffer.Bytes(), net.UDPAddrFromAddrPort(netip.AddrPortFrom(addr, destination.Port)))
		buffer.Release()
		if err == nil {
			u.addUp(int64(size))
		} else if errors.Is(err, net.ErrClosed) {
			return nil
		}
	}
}

// resolveUDP applies the same destination policy as TCP. Lookups are cached per association.
func (s *Server) resolveUDP(ctx context.Context, destination M.Socksaddr, cache map[string]netip.Addr) (netip.Addr, error) {
	if !destination.IsFqdn() {
		addr := destination.Addr.Unmap()
		if !s.permitted(addr) {
			return netip.Addr{}, errBlockedRoute
		}
		return addr, nil
	}
	if addr, ok := cache[destination.Fqdn]; ok {
		return addr, nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	candidates, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", destination.Fqdn)
	if err != nil {
		return netip.Addr{}, err
	}
	for _, candidate := range candidates {
		if candidate = candidate.Unmap(); s.permitted(candidate) {
			if len(cache) >= maxResolvedCache {
				clear(cache)
			}
			cache[destination.Fqdn] = candidate
			return candidate, nil
		}
	}
	return netip.Addr{}, fmt.Errorf("%w: %s", errBlockedRoute, destination.Fqdn)
}
