package ssserver

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/icebase/mu/pkg/ss"
	"github.com/sagernet/sing-shadowsocks/shadowaead"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

const (
	handshakeTimeout = 10 * time.Second
	maxNameLength    = 64
	dialTimeout      = 10 * time.Second
)

var (
	ErrInvalidUser = errors.New("invalid user")
	ErrKeyInUse    = errors.New("key already belongs to another user")
)

type sessionRef struct{ cancel context.CancelFunc }

// user holds the authentication key and the cumulative payload counters of one account.
// Counters are payload bytes after decryption: upload is client to destination, download is the reverse.
type user struct {
	name        string
	key         []byte
	fingerprint string
	up, down    atomic.Int64
	sessions    map[*sessionRef]struct{}
}

// Server is a SIP023 multi-user Shadowsocks server.
type Server struct {
	cfg     *Config
	service *shadowaead_2022.MultiService[string]
	dialer  net.Dialer

	// authMu serializes UpdateUsers against the code paths that read the service's user tables.
	// sing-shadowsocks does not synchronize them, so handshakes hold a read lock until the user is identified.
	authMu sync.RWMutex

	mu    sync.Mutex // guards users, byFingerprint and every user's sessions
	users map[string]*user
	byFP  map[string]string

	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	tcp      net.Listener
	udp      net.PacketConn
	udpWrite N.NetPacketConn
	closeMu  sync.Once
}

func New(cfg *Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	psk, err := DecodeKey(cfg.ServerPSK, cfg.KeySize())
	if err != nil {
		return nil, err
	}
	s := &Server{cfg: cfg, users: make(map[string]*user), byFP: make(map[string]string)}
	s.dialer = net.Dialer{Timeout: dialTimeout, Control: s.controlDial}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.service, err = shadowaead_2022.NewMultiService[string](cfg.Method, psk, int64(cfg.UDPTimeout/time.Second), &relayHandler{s: s}, nil)
	if err != nil {
		s.cancel()
		return nil, fmt.Errorf("create shadowsocks service: %w", err)
	}
	return s, nil
}

// Start binds the sockets and begins serving. It returns once the listeners are ready.
func (s *Server) Start() error {
	tcp, err := net.Listen("tcp", s.cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen TCP: %w", err)
	}
	s.tcp = tcp
	if s.cfg.UDPEnabled {
		// Bind UDP on the port TCP actually received, so ":0" configurations stay consistent.
		udpAddr := s.cfg.ListenAddr
		if addr, ok := tcp.Addr().(*net.TCPAddr); ok {
			udpAddr = (&net.UDPAddr{IP: addr.IP, Port: addr.Port, Zone: addr.Zone}).String()
		}
		udp, err := net.ListenPacket("udp", udpAddr)
		if err != nil {
			_ = tcp.Close()
			return fmt.Errorf("listen UDP: %w", err)
		}
		s.udp = udp
		s.udpWrite = bufio.NewPacketConn(udp)
		s.wg.Add(1)
		go s.serveUDP()
	}
	s.wg.Add(1)
	go s.acceptTCP()
	return nil
}

func (s *Server) TCPAddr() net.Addr {
	if s.tcp == nil {
		return nil
	}
	return s.tcp.Addr()
}

func (s *Server) UDPAddr() net.Addr {
	if s.udp == nil {
		return nil
	}
	return s.udp.LocalAddr()
}

// Close stops the listeners and terminates every session.
func (s *Server) Close() error {
	var errs []error
	s.closeMu.Do(func() {
		s.cancel()
		if s.tcp != nil {
			errs = append(errs, s.tcp.Close())
		}
		if s.udp != nil {
			errs = append(errs, s.udp.Close())
		}
		s.wg.Wait()
	})
	return errors.Join(errs...)
}

func validName(name string) bool {
	if name == "" || len(name) > maxNameLength {
		return false
	}
	for _, r := range name {
		if r < 0x21 || r > 0x7e {
			return false
		}
	}
	return true
}

// SetUser creates a user or replaces its key. Counters survive a replacement, and sessions
// authenticated with the previous key are terminated.
func (s *Server) SetUser(name, psk string) error {
	if !validName(name) {
		return fmt.Errorf("%w: name", ErrInvalidUser)
	}
	key, err := DecodeKey(psk, s.cfg.KeySize())
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidUser, err)
	}
	fingerprint := ss.Fingerprint(key)

	s.mu.Lock()
	if owner, exists := s.byFP[fingerprint]; exists && owner != name {
		s.mu.Unlock()
		return ErrKeyInUse
	}
	existing := s.users[name]
	if existing != nil && existing.fingerprint == fingerprint {
		s.mu.Unlock()
		return nil
	}
	names := make([]string, 0, len(s.users)+1)
	keys := make([][]byte, 0, len(s.users)+1)
	for other, u := range s.users {
		if other != name {
			names = append(names, other)
			keys = append(keys, u.key)
		}
	}
	names = append(names, name)
	keys = append(keys, key)
	if err := s.updateAuth(names, keys); err != nil {
		s.mu.Unlock()
		return err
	}
	var evicted []*sessionRef
	if existing != nil {
		delete(s.byFP, existing.fingerprint)
		existing.key, existing.fingerprint = key, fingerprint
		evicted = detachAll(existing)
	} else {
		s.users[name] = &user{name: name, key: key, fingerprint: fingerprint, sessions: make(map[*sessionRef]struct{})}
	}
	s.byFP[fingerprint] = name
	usersGauge.Set(float64(len(s.users)))
	s.mu.Unlock()
	cancelAll(evicted)
	return nil
}

// RemoveUser deletes the user and terminates its sessions. Removing an unknown user succeeds.
func (s *Server) RemoveUser(name string) error {
	s.mu.Lock()
	existing := s.users[name]
	if existing == nil {
		s.mu.Unlock()
		return nil
	}
	names := make([]string, 0, len(s.users))
	keys := make([][]byte, 0, len(s.users))
	for other, u := range s.users {
		if other != name {
			names = append(names, other)
			keys = append(keys, u.key)
		}
	}
	if err := s.updateAuth(names, keys); err != nil {
		s.mu.Unlock()
		return err
	}
	delete(s.users, name)
	delete(s.byFP, existing.fingerprint)
	evicted := detachAll(existing)
	usersGauge.Set(float64(len(s.users)))
	s.mu.Unlock()
	cancelAll(evicted)
	return nil
}

func (s *Server) updateAuth(names []string, keys [][]byte) error {
	s.authMu.Lock()
	defer s.authMu.Unlock()
	return s.service.UpdateUsers(names, keys)
}

func detachAll(u *user) []*sessionRef {
	refs := make([]*sessionRef, 0, len(u.sessions))
	for ref := range u.sessions {
		refs = append(refs, ref)
	}
	return refs
}

func cancelAll(refs []*sessionRef) {
	for _, ref := range refs {
		ref.cancel()
	}
}

// Users returns a snapshot sorted by name. Keys are never exposed.
func (s *Server) Users() []ss.UserStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ss.UserStatus, 0, len(s.users))
	for _, u := range s.users {
		out = append(out, ss.UserStatus{Name: u.name, PSKFingerprint: u.fingerprint, Upload: u.up.Load(), Download: u.down.Load()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// attach registers a live session. It fails when the user was removed after authentication.
func (s *Server) attach(name string, cancel context.CancelFunc) (*user, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u := s.users[name]
	if u == nil {
		return nil, nil
	}
	ref := &sessionRef{cancel: cancel}
	u.sessions[ref] = struct{}{}
	sessionsGauge.Inc()
	return u, func() {
		s.mu.Lock()
		delete(u.sessions, ref)
		s.mu.Unlock()
		sessionsGauge.Dec()
	}
}

func (s *Server) acceptTCP() {
	defer s.wg.Done()
	for {
		conn, err := s.tcp.Accept()
		if err != nil {
			if s.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Warn("accept failed", "error", err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveTCP(conn)
		}()
	}
}

func (s *Server) serveTCP(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	// Read the fixed-size request prefix outside authMu, so a slow client can never stall user updates.
	prefix := make([]byte, s.cfg.KeySize()+16+shadowaead.Overhead+shadowaead_2022.RequestHeaderFixedChunkLength)
	_ = conn.SetDeadline(time.Now().Add(handshakeTimeout))
	if _, err := io.ReadFull(conn, prefix); err != nil {
		return
	}
	s.authMu.RLock()
	var once sync.Once
	release := func() { once.Do(s.authMu.RUnlock) }
	defer release()
	// release runs as soon as the user is identified; the long-lived relay then holds no lock.
	ctx := context.WithValue(s.ctx, rawConnKey{}, conn)
	//nolint:staticcheck // sing-shadowsocks only accepts M.Metadata.
	metadata := M.Metadata{Source: M.SocksaddrFromNet(conn.RemoteAddr())}
	err := s.service.NewConnection0(ctx, conn, metadata, bytes.NewReader(prefix), release)
	if err != nil {
		authFailures.Inc()
		slog.Debug("connection ended", "error", err)
	}
}

func (s *Server) serveUDP() {
	defer s.wg.Done()
	for {
		buffer := buf.NewPacket()
		source, err := s.udpWrite.ReadPacket(buffer)
		if err != nil {
			buffer.Release()
			if s.ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			slog.Warn("read UDP failed", "error", err)
			continue
		}
		s.authMu.RLock()
		//nolint:staticcheck // sing-shadowsocks only accepts M.Metadata.
		err = s.service.NewPacket(s.ctx, s.udpWrite, buffer, M.Metadata{Source: source})
		s.authMu.RUnlock()
		if err != nil {
			buffer.Release()
			authFailures.Inc()
			slog.Debug("packet dropped", "error", err)
		}
	}
}

// tokenEqual compares secrets without leaking the matching prefix length.
func tokenEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
