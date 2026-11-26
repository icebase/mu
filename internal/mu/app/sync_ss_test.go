package app

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"net"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/icebase/mu/internal/ssserver"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/sagernet/sing-shadowsocks/shadowaead_2022"
	M "github.com/sagernet/sing/common/metadata"
)

const ssTestToken = "test-token"

func ssKey(t *testing.T) ([]byte, string) {
	t.Helper()
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key, base64.StdEncoding.EncodeToString(key)
}

type ssFixture struct {
	server *ssserver.Server
	iPSK   []byte
	syncer *ssSync
}

// newSSFixture runs a real ss-server with its management API, so Sync is exercised end to end.
func newSSFixture(t *testing.T, token string) *ssFixture {
	t.Helper()
	iPSK, encoded := ssKey(t)
	server, err := ssserver.New(&ssserver.Config{
		ListenAddr: "127.0.0.1:0", Method: ssserver.MethodAES128, ServerPSK: encoded, APIAddr: "127.0.0.1:0",
		APIToken: ssTestToken, UDPEnabled: false, AllowPrivate: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Close() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	api := ssserver.NewAPIServer(server, ssTestToken)
	go func() { _ = ssserver.ServeAPI(api, listener) }()
	t.Cleanup(api.Stop)
	syncer, err := newSSSync(listener.Addr().String(), token, newMemoryState())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syncer.Close() })
	return &ssFixture{server: server, iPSK: iPSK, syncer: syncer}
}

func (f *ssFixture) names() []string {
	var names []string
	for _, u := range f.server.Users() {
		names = append(names, u.Name)
	}
	return names
}

func ssUser(id int64, enable int32, psk string) *pb.User {
	return &pb.User{Id: id, Enable: enable, SsUser: &pb.SSUser{Psk: psk}}
}

func TestSSSyncReconcilesUsers(t *testing.T) {
	f := newSSFixture(t, ssTestToken)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, pskA := ssKey(t)
	_, pskB := ssKey(t)
	_, pskC := ssKey(t)

	sync := func(users ...*pb.User) {
		t.Helper()
		// Repeated syncs model successive timer ticks and must be idempotent.
		for range 2 {
			if err := f.syncer.Sync(ctx, users); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := func(names ...string) {
		t.Helper()
		if got := f.names(); !slices.Equal(got, names) {
			t.Fatalf("server users = %v, want %v", got, names)
		}
	}

	sync(ssUser(1, 1, pskA), ssUser(2, 1, pskB), ssUser(3, 0, pskC))
	want("1", "2") // the disabled account is never installed

	sync(ssUser(1, 1, pskA), ssUser(2, 0, pskB))
	want("1") // disabling revokes

	sync(ssUser(1, 1, pskA), ssUser(3, 1, pskC))
	want("1", "3") // an account absent from the snapshot is gone, a new one arrives

	sync()
	want()
}

func TestSSSyncRotatesKeysInPlace(t *testing.T) {
	f := newSSFixture(t, ssTestToken)
	ctx := context.Background()
	_, oldPSK := ssKey(t)
	newKey, newPSK := ssKey(t)
	if err := f.syncer.Sync(ctx, []*pb.User{ssUser(1, 1, oldPSK)}); err != nil {
		t.Fatal(err)
	}
	before := f.server.Users()[0].PSKFingerprint
	if err := f.syncer.Sync(ctx, []*pb.User{ssUser(1, 1, newPSK)}); err != nil {
		t.Fatal(err)
	}
	after := f.server.Users()
	if len(after) != 1 || after[0].PSKFingerprint == before {
		t.Fatalf("key was not rotated: %+v", after)
	}
	if want := fingerprintOf(newPSK); after[0].PSKFingerprint != want {
		t.Fatalf("server holds the wrong key: %s, want %s", after[0].PSKFingerprint, want)
	}
	_ = newKey
}

func TestSSSyncReportsPerUserTraffic(t *testing.T) {
	f := newSSFixture(t, ssTestToken)
	ctx := context.Background()
	aliceKey, alicePSK := ssKey(t)
	_, bobPSK := ssKey(t)
	users := []*pb.User{ssUser(11, 1, alicePSK), ssUser(22, 1, bobPSK)}
	if err := f.syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}

	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = echo.Close() })
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = conn.Close() }(); _, _ = io.Copy(conn, conn) }()
		}
	}()
	client, err := shadowaead_2022.New(ssserver.MethodAES128, [][]byte{f.iPSK, aliceKey}, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := net.Dial("tcp", f.server.TCPAddr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	conn, err := client.DialConn(raw, M.ParseSocksaddr(echo.Addr().String()))
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(conn, make([]byte, 10)); err != nil {
		t.Fatal(err)
	}

	var logs []*pb.UserTrafficLog
	deadline := time.Now().Add(5 * time.Second)
	for len(logs) == 0 && time.Now().Before(deadline) {
		if logs, err = f.syncer.GetTraffic(ctx, users); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %v, want exactly one (only alice moved data)", logs)
	}
	got := logs[0]
	if got.UserId != 11 || got.U != 10 || got.D != 10 || got.TrafficType != int32(pb.TrafficType_TRAFFIC_TYPE_SHADOWSOCKS) {
		t.Fatalf("unexpected traffic log: %v", got)
	}
	again, err := f.syncer.GetTraffic(ctx, users)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("delta was reported twice: %v", again)
	}

	// The outbox must survive a restart, which requires state validation to know the new traffic type.
	if len(f.syncer.state.snapshot().Pending) == 0 {
		t.Fatal("traffic was not queued durably")
	}
	path := filepath.Join(t.TempDir(), "state.json")
	state, err := openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	if err := state.update(func(data *stateData) error {
		data.Pending = f.syncer.state.snapshot().Pending
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_ = state.Close()
	reopened, err := openSyncState(path, "node")
	if err != nil {
		t.Fatalf("state with Shadowsocks logs was rejected on restart: %v", err)
	}
	_ = reopened.Close()
}

func TestSSSyncWrongTokenChangesNothing(t *testing.T) {
	f := newSSFixture(t, "wrong-token")
	_, psk := ssKey(t)
	if err := f.syncer.Sync(context.Background(), []*pb.User{ssUser(1, 1, psk)}); err == nil {
		t.Fatal("sync with a wrong token must fail")
	}
	if got := f.names(); len(got) != 0 {
		t.Fatalf("unauthenticated call changed users: %v", got)
	}
}

func TestDesiredSSUsersValidation(t *testing.T) {
	_, valid := ssKey(t)
	_, other := ssKey(t)
	short := base64.StdEncoding.EncodeToString([]byte("short"))
	tests := []struct {
		name    string
		users   []*pb.User
		wantErr bool
		wantAll int
		wantOn  int
	}{
		{name: "skips accounts without Shadowsocks credentials", users: []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "u"}}, ssUser(2, 1, valid)}, wantAll: 1, wantOn: 1},
		{name: "disabled counts as known but not enabled", users: []*pb.User{ssUser(2, 0, valid)}, wantAll: 1},
		{name: "bad base64", users: []*pb.User{ssUser(1, 1, "***")}, wantErr: true},
		{name: "wrong key length", users: []*pb.User{ssUser(1, 1, short)}, wantErr: true},
		{name: "non-positive id", users: []*pb.User{ssUser(0, 1, valid)}, wantErr: true},
		{name: "duplicate id", users: []*pb.User{ssUser(1, 1, valid), ssUser(1, 1, other)}, wantErr: true},
		{name: "duplicate key", users: []*pb.User{ssUser(1, 1, valid), ssUser(2, 1, valid)}, wantErr: true},
		{name: "invalid enable flag", users: []*pb.User{ssUser(1, 2, valid)}, wantErr: true},
		{name: "nil record", users: []*pb.User{nil}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			all, enabled, err := desiredUsers(tt.users, protocolSS)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (len(all) != tt.wantAll || len(enabled) != tt.wantOn) {
				t.Fatalf("all/enabled = %d/%d, want %d/%d", len(all), len(enabled), tt.wantAll, tt.wantOn)
			}
		})
	}
}

// Shadowsocks-only accounts must not poison the other backends' snapshot validation.
func TestOtherProtocolsIgnoreShadowsocksOnlyUsers(t *testing.T) {
	_, psk := ssKey(t)
	users := []*pb.User{
		{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "11111111-1111-1111-1111-111111111111", Email: "a@example.com"}},
		ssUser(2, 1, psk),
	}
	for _, protocol := range []string{protocolTrojan, protocolV2Fly} {
		all, _, err := desiredUsers(users, protocol)
		if err != nil || len(all) != 1 {
			t.Fatalf("%s: all=%d err=%v, want the single V2Ray account", protocol, len(all), err)
		}
	}
	// A record with neither identity is still rejected.
	if _, _, err := desiredUsers([]*pb.User{{Id: 3, Enable: 1}}, protocolTrojan); err == nil {
		t.Fatal("record without any credentials must be rejected")
	}
}
