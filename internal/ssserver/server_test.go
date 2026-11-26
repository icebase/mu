package ssserver

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/icebase/mu/pkg/ss"
	"github.com/sagernet/sing-shadowsocks"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	M "github.com/sagernet/sing/common/metadata"
)

func randomKey(t *testing.T, size int) ([]byte, string) {
	t.Helper()
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key, base64.StdEncoding.EncodeToString(key)
}

type testServer struct {
	*Server
	iPSK   []byte
	method shadowsocks.Method
}

func newTestServer(t *testing.T, method string, allowPrivate bool) *testServer {
	t.Helper()
	size := 16
	if method == MethodAES256 {
		size = 32
	}
	iPSK, encoded := randomKey(t, size)
	server, err := New(&Config{
		ListenAddr: "127.0.0.1:0", Method: method, ServerPSK: encoded, APIAddr: "127.0.0.1:0", APIToken: "token",
		UDPEnabled: true, UDPTimeout: 5 * time.Second, AllowPrivate: allowPrivate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	return &testServer{Server: server, iPSK: iPSK}
}

func (s *testServer) client(t *testing.T, uPSK []byte) shadowsocks.Method {
	t.Helper()
	client, err := shadowaead_2022.New(s.cfg.Method, [][]byte{s.iPSK, uPSK}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func (s *testServer) dial(t *testing.T, uPSK []byte, destination string) net.Conn {
	t.Helper()
	raw, err := net.Dial("tcp", s.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	conn, err := s.client(t, uPSK).DialConn(raw, M.ParseSocksaddr(destination))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

func startEcho(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return listener.Addr().String()
}

func startUDPEcho(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		data := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFrom(data)
			if err != nil {
				return
			}
			_, _ = conn.WriteTo(data[:n], addr)
		}
	}()
	return conn.LocalAddr().String()
}

func roundTrip(t *testing.T, conn net.Conn, payload string) {
	t.Helper()
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(got) != payload {
		t.Fatalf("echo = %q, want %q", got, payload)
	}
}

func usage(t *testing.T, s *Server, name string) ss.UserStatus {
	t.Helper()
	for _, u := range s.Users() {
		if u.Name == name {
			return u
		}
	}
	t.Fatalf("user %s not found", name)
	return ss.UserStatus{}
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func expectClosed(t *testing.T, conn net.Conn, what string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatalf("%s: connection unexpectedly stayed readable", what)
	} else if ne, ok := errors.AsType[net.Error](err); ok && ne.Timeout() {
		t.Fatalf("%s: connection was not closed", what)
	}
}

func TestTwoUsersShareOnePortWithSeparateAccounting(t *testing.T) {
	for _, method := range []string{MethodAES128, MethodAES256} {
		t.Run(method, func(t *testing.T) {
			server := newTestServer(t, method, true)
			echo := startEcho(t)
			size := server.cfg.KeySize()
			aliceKey, alicePSK := randomKey(t, size)
			bobKey, bobPSK := randomKey(t, size)
			if err := server.SetUser("alice", alicePSK); err != nil {
				t.Fatal(err)
			}
			if err := server.SetUser("bob", bobPSK); err != nil {
				t.Fatal(err)
			}

			alice := server.dial(t, aliceKey, echo)
			bob := server.dial(t, bobKey, echo)
			roundTrip(t, alice, "hello")
			roundTrip(t, bob, "a longer payload")
			_ = alice.Close()
			_ = bob.Close()

			waitFor(t, "alice counters", func() bool {
				u := usage(t, server.Server, "alice")
				return u.Upload == 5 && u.Download == 5
			})
			waitFor(t, "bob counters", func() bool {
				u := usage(t, server.Server, "bob")
				return u.Upload == 16 && u.Download == 16
			})
		})
	}
}

func TestWrongKeyIsRejected(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	echo := startEcho(t)
	_, valid := randomKey(t, 16)
	if err := server.SetUser("alice", valid); err != nil {
		t.Fatal(err)
	}
	stranger, _ := randomKey(t, 16)
	conn := server.dial(t, stranger, echo)
	_, _ = conn.Write([]byte("hello"))
	expectClosed(t, conn, "unknown key")
	if u := usage(t, server.Server, "alice"); u.Upload != 0 || u.Download != 0 {
		t.Fatalf("rejected traffic was billed: %+v", u)
	}
}

func TestDynamicUpdatesWithoutRestart(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	echo := startEcho(t)
	aliceKey, alicePSK := randomKey(t, 16)
	if err := server.SetUser("alice", alicePSK); err != nil {
		t.Fatal(err)
	}
	alice := server.dial(t, aliceKey, echo)
	roundTrip(t, alice, "one")

	// Adding a user must not disturb existing sessions.
	carolKey, carolPSK := randomKey(t, 16)
	if err := server.SetUser("carol", carolPSK); err != nil {
		t.Fatal(err)
	}
	roundTrip(t, alice, "two")
	roundTrip(t, server.dial(t, carolKey, echo), "carol")

	// Removal terminates the active session and blocks new ones.
	if err := server.RemoveUser("alice"); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, alice, "removed user's session")
	again := server.dial(t, aliceKey, echo)
	_, _ = again.Write([]byte("x"))
	expectClosed(t, again, "removed user's new connection")
	if err := server.RemoveUser("alice"); err != nil {
		t.Fatalf("removal must be idempotent: %v", err)
	}
}

func TestKeyRotationKeepsCountersAndKicksOldSessions(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	echo := startEcho(t)
	oldKey, oldPSK := randomKey(t, 16)
	if err := server.SetUser("alice", oldPSK); err != nil {
		t.Fatal(err)
	}
	old := server.dial(t, oldKey, echo)
	roundTrip(t, old, "12345")
	waitFor(t, "initial counters", func() bool { return usage(t, server.Server, "alice").Upload == 5 })
	before := usage(t, server.Server, "alice").PSKFingerprint

	newKey, newPSK := randomKey(t, 16)
	if err := server.SetUser("alice", newPSK); err != nil {
		t.Fatal(err)
	}
	expectClosed(t, old, "session using the rotated-out key")
	after := usage(t, server.Server, "alice")
	if after.PSKFingerprint == before || after.Upload != 5 {
		t.Fatalf("rotation must change the fingerprint and keep counters: %+v", after)
	}
	roundTrip(t, server.dial(t, newKey, echo), "6789")
	stale := server.dial(t, oldKey, echo)
	_, _ = stale.Write([]byte("x"))
	expectClosed(t, stale, "old key after rotation")
}

func TestUserInputValidation(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	_, valid := randomKey(t, 16)
	_, wrongSize := randomKey(t, 32)
	for name, args := range map[string][2]string{
		"empty name":    {"", valid},
		"bad key":       {"alice", "not base64!"},
		"wrong length":  {"alice", wrongSize},
		"control chars": {"al\nice", valid},
	} {
		if err := server.SetUser(args[0], args[1]); !errors.Is(err, ErrInvalidUser) {
			t.Errorf("%s: err = %v, want ErrInvalidUser", name, err)
		}
	}
	if err := server.SetUser("alice", valid); err != nil {
		t.Fatal(err)
	}
	if err := server.SetUser("bob", valid); !errors.Is(err, ErrKeyInUse) {
		t.Fatalf("shared key err = %v, want ErrKeyInUse", err)
	}
	if err := server.SetUser("alice", valid); err != nil {
		t.Fatalf("re-applying the same key must be a no-op: %v", err)
	}
}

func TestPrivateDestinationsAreBlockedByDefault(t *testing.T) {
	server := newTestServer(t, MethodAES128, false)
	echo := startEcho(t)
	key, psk := randomKey(t, 16)
	if err := server.SetUser("alice", psk); err != nil {
		t.Fatal(err)
	}
	conn := server.dial(t, key, echo) // loopback
	_, _ = conn.Write([]byte("probe"))
	expectClosed(t, conn, "loopback destination")
	if u := usage(t, server.Server, "alice"); u.Upload != 0 {
		t.Fatalf("blocked traffic was billed: %+v", u)
	}
}

func TestUDPRelayCountsPerUser(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	echo := startUDPEcho(t)
	key, psk := randomKey(t, 16)
	if err := server.SetUser("alice", psk); err != nil {
		t.Fatal(err)
	}
	raw, err := net.Dial("udp", server.UDPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	client := server.client(t, key).DialPacketConn(raw)
	_ = raw.SetDeadline(time.Now().Add(5 * time.Second))
	destination := M.ParseSocksaddr(echo)
	if _, err := client.WriteTo([]byte("datagram"), destination); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 2048) // must hold the protocol header and tag, not only the payload
	n, _, err := client.ReadFrom(got)
	if err != nil {
		t.Fatalf("read UDP reply: %v", err)
	}
	if string(got[:n]) != "datagram" {
		t.Fatalf("reply = %q", got[:n])
	}
	waitFor(t, "UDP counters", func() bool {
		u := usage(t, server.Server, "alice")
		return u.Upload == 8 && u.Download == 8
	})
}

// Run with -race: user updates must be safe while handshakes and relays are in flight.
func TestConcurrentUpdatesAndConnections(t *testing.T) {
	server := newTestServer(t, MethodAES128, true)
	echo := startEcho(t)
	stableKey, stablePSK := randomKey(t, 16)
	if err := server.SetUser("stable", stablePSK); err != nil {
		t.Fatal(err)
	}
	udpEcho := startUDPEcho(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	wg.Add(2)
	go func() { // datagrams keep hitting the UDP path while users change
		defer wg.Done()
		raw, err := net.Dial("udp", server.UDPAddr().String())
		if err != nil {
			return
		}
		defer func() { _ = raw.Close() }()
		client := server.client(t, stableKey).DialPacketConn(raw)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, _ = client.WriteTo([]byte("ping"), M.ParseSocksaddr(udpEcho))
			time.Sleep(time.Millisecond)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_, psk := randomKey(t, 16)
			name := "churn"
			if i%2 == 0 {
				_ = server.SetUser(name, psk)
			} else {
				_ = server.RemoveUser(name)
			}
		}
	}()
	for range 20 {
		roundTrip(t, server.dial(t, stableKey, echo), "ping")
	}
	close(stop)
	wg.Wait()
}

func TestConfigValidation(t *testing.T) {
	_, psk := randomKey(t, 16)
	_, long := randomKey(t, 32)
	base := Config{ListenAddr: ":0", Method: MethodAES128, ServerPSK: psk, APIAddr: "127.0.0.1:0", APIToken: "t", UDPEnabled: true, UDPTimeout: time.Minute}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Config){
		"unsupported method": func(c *Config) { c.Method = "2022-blake3-chacha20-poly1305" },
		"missing psk":        func(c *Config) { c.ServerPSK = "" },
		"psk length":         func(c *Config) { c.ServerPSK = long },
		"missing token":      func(c *Config) { c.APIToken = "" },
		"udp timeout":        func(c *Config) { c.UDPTimeout = 0 },
	} {
		config := base
		mutate(&config)
		if err := config.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}
