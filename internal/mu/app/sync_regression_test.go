package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/icebase/mu"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/api/service"
	"github.com/p4gefau1t/trojan-go/common"
	"github.com/p4gefau1t/trojan-go/config"
	"github.com/p4gefau1t/trojan-go/statistic"
	"github.com/p4gefau1t/trojan-go/statistic/memory"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel"
)

func realTrojanForReview(t *testing.T, passwords ...string) (*trojanSync, statistic.Authenticator) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctx = config.WithConfig(ctx, memory.Name, &memory.Config{Passwords: passwords})
	ctx = config.WithConfig(ctx, service.Name, &service.Config{API: service.APIConfig{Enabled: true, APIHost: "127.0.0.1", APIPort: port}})
	auth, err := memory.NewAuthenticator(ctx)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- service.RunServerAPI(ctx, auth) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(time.Second):
			t.Error("server did not stop")
		}
	})
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 10*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start")
		}
		time.Sleep(time.Millisecond)
	}
	syncer, err := newTrojanSync(addr, newMemoryState())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syncer.Close() })
	return syncer, auth
}

func TestReviewTrojanRealAPIRevokesUsers(t *testing.T) {
	syncer, auth := realTrojanForReview(t, "active", "disabled", "removed")
	users := []*pb.User{{Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}, {Enable: 0, V2RayUser: &pb.VUser{Uuid: "disabled"}}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	listed, err := syncer.tjManager.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range listed {
		if user.User.Password != "" || len(user.User.Hash) != 56 {
			t.Fatal("unexpected actual API identity shape")
		}
	}
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	for _, password := range []string{"disabled", "removed"} {
		valid, _ := auth.AuthUser(common.SHA224String(password))
		if valid {
			t.Errorf("%s user still authorized after successful Sync against real Trojan API", password)
		}
	}
	valid, _ := auth.AuthUser(common.SHA224String("active"))
	if !valid {
		t.Error("active user was removed")
	}
}

func TestReviewTrojanRejectsFailedAcknowledgement(t *testing.T) {
	syncer, _ := realTrojanForReview(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := syncer.tjManager.RemoveUser(ctx, "nonexistent", ""); err == nil {
		t.Error("RemoveUser accepted Success=false as success")
	}
	if err := syncer.tjManager.AddUser(ctx, "new"); err != nil {
		t.Fatal(err)
	}
	if err := syncer.tjManager.AddUser(ctx, "new"); err == nil {
		t.Error("AddUser accepted Success=false as success")
	}
}

func TestReviewTrojanFirstSampleAndCounterReset(t *testing.T) {
	for _, reset := range []bool{false, true} {
		t.Run(fmt.Sprint("reset=", reset), func(t *testing.T) {
			syncer, auth := realTrojanForReview(t)
			users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := syncer.Sync(ctx, users); err != nil {
				t.Fatal(err)
			}
			_, meter := auth.AuthUser(common.SHA224String("active"))
			if reset {
				meter.SetTraffic(1000, 2000)
				if _, err := syncer.GetTraffic(ctx, users); err != nil {
					t.Fatal(err)
				}
			}
			meter.SetTraffic(10, 20)
			logs, err := syncer.GetTraffic(ctx, users)
			if err != nil {
				t.Fatal(err)
			}
			if len(logs) != 1 || logs[0].D != 10 || logs[0].U != 20 {
				t.Errorf("traffic lost: got %v, want down=10 up=20", logs)
			}
		})
	}
}

type reviewMUService struct {
	users        []*pb.User
	failUpload   bool
	uploads      int
	received     int64
	failGetUsers bool
}

func (s *reviewMUService) Ping(context.Context, *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{}, nil
}
func (s *reviewMUService) GetUsers(context.Context, *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	if s.failGetUsers {
		return nil, twirp.InternalError("injected user snapshot failure")
	}
	return &pb.GetUsersResponse{Users: s.users}, nil
}
func (s *reviewMUService) UploadTrafficLog(_ context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	s.uploads++
	if s.failUpload {
		s.failUpload = false
		return nil, twirp.InternalError("injected upload failure")
	}
	for _, log := range req.Logs {
		s.received += log.U + log.D
	}
	return &pb.UploadTrafficLogResponse{}, nil
}

type reviewSync struct {
	name          string
	err           error
	called        int
	trafficErr    error
	trafficCalled int
}

func (s *reviewSync) Name() string                           { return s.name }
func (s *reviewSync) Sync(context.Context, []*pb.User) error { s.called++; return s.err }
func (s *reviewSync) GetTraffic(context.Context, []*pb.User) ([]*pb.UserTrafficLog, error) {
	s.trafficCalled++
	return nil, s.trafficErr
}
func reviewApp(t *testing.T, service *reviewMUService, syncers ...UserSync) *App {
	t.Helper()
	server := httptest.NewServer(pb.NewMUServiceServer(service, twirp.WithServerPathPrefix("")))
	t.Cleanup(server.Close)
	state := newMemoryState()
	for _, syncer := range syncers {
		switch s := syncer.(type) {
		case *trojanSync:
			state = s.state
		case *v2flySync:
			state = s.state
		}
	}
	for _, syncer := range syncers {
		switch s := syncer.(type) {
		case *trojanSync:
			s.state = state
		case *v2flySync:
			s.state = state
		}
	}
	return &App{config: &Config{MuNodeID: "test-node"}, muClient: mu.NewClient(server.URL, "test-token", "test-node"), userSync: syncers, state: state, ctx: context.Background(), tracer: otel.Tracer("review")}
}
func TestReviewBackendFailureIsolation(t *testing.T) {
	first := &reviewSync{name: "trojan", err: errors.New("injected backend failure")}
	second := &reviewSync{name: "v2fly"}
	app := reviewApp(t, &reviewMUService{}, first, second)
	if err := app.syncUser(); err == nil {
		t.Fatal("expected backend error")
	}
	if second.called != 1 {
		t.Errorf("healthy v2fly backend skipped: calls=%d", second.called)
	}
}
func TestReviewTrojanUploadFailureRetainsTraffic(t *testing.T) {
	syncer, auth := realTrojanForReview(t, "active")
	_, meter := auth.AuthUser(common.SHA224String("active"))
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	if _, err := syncer.GetTraffic(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	service := &reviewMUService{users: users, failUpload: true}
	app := reviewApp(t, service, syncer)
	meter.AddTraffic(100, 200)
	if err := app.syncTraffic(); err == nil {
		t.Fatal("expected upload error")
	}
	if err := app.syncTraffic(); err != nil {
		t.Fatal(err)
	}
	if service.received != 300 {
		t.Errorf("failed upload permanently lost traffic: received=%d, want=300 (upload attempts=%d)", service.received, service.uploads)
	}
}
func TestReviewMissingV2RayUserDoesNotPanic(t *testing.T) {
	users := []*pb.User{{Id: 1, Enable: 1}}
	for name, fn := range map[string]func(){
		"trojan traffic": func() { s := &trojanSync{}; _, _ = s.GetTraffic(context.Background(), users) },
		"v2fly traffic":  func() { s := &v2flySync{}; _, _ = s.GetTraffic(context.Background(), users) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() != nil {
					t.Error("missing v2ray_user panics instead of rejecting malformed input")
				}
			}()
			fn()
		})
	}
}
