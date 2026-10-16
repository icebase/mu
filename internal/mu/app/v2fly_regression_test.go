package app

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/app/proxyman"
	handlercmd "github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	inboundmanager "github.com/v2fly/v2ray-core/v5/app/proxyman/inbound"
	statsmanager "github.com/v2fly/v2ray-core/v5/app/stats"
	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/serial"
	"github.com/v2fly/v2ray-core/v5/proxy/vmess"
	vmessinbound "github.com/v2fly/v2ray-core/v5/proxy/vmess/inbound"
	"google.golang.org/grpc"
)

type reviewRealVMessServer struct {
	handlercmd.UnimplementedHandlerServiceServer
	handler *vmessinbound.Handler
}

func (s *reviewRealVMessServer) AlterInbound(ctx context.Context, req *handlercmd.AlterInboundRequest) (*handlercmd.AlterInboundResponse, error) {
	op, err := serial.GetInstanceOf(req.Operation)
	if err != nil {
		return nil, err
	}
	switch v := op.(type) {
	case *handlercmd.AddUserOperation:
		user, err := v.User.ToMemoryUser()
		if err != nil {
			return nil, err
		}
		err = s.handler.AddUser(ctx, user)
		return &handlercmd.AlterInboundResponse{}, err
	case *handlercmd.RemoveUserOperation:
		return &handlercmd.AlterInboundResponse{}, s.handler.RemoveUser(ctx, v.Email)
	}
	return &handlercmd.AlterInboundResponse{}, nil
}
func reviewRealVMess(t *testing.T, failStats ...bool) (*v2flySync, *vmessinbound.Handler, *statsmanager.Manager) {
	t.Helper()
	instance, err := core.New(&core.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = instance.Close() })
	inboundManager, err := inboundmanager.New(context.Background(), &proxyman.InboundConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if err := instance.AddFeature(inboundManager); err != nil {
		t.Fatal(err)
	}
	raw, err := core.CreateObject(instance, &vmessinbound.Config{Default: &vmessinbound.DefaultConfig{}})
	if err != nil {
		t.Fatal(err)
	}
	handler := raw.(*vmessinbound.Handler)
	t.Cleanup(func() { _ = handler.Close() })
	stats, err := statsmanager.NewManager(context.Background(), &statsmanager.Config{})
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	handlercmd.RegisterHandlerServiceServer(server, &reviewRealVMessServer{handler: handler})
	statsService := statscmd.NewStatsServer(stats)
	if len(failStats) > 0 && failStats[0] {
		statsService = &unavailableStatsServer{StatsServiceServer: statsService}
	}
	statscmd.RegisterStatsServiceServer(server, statsService)
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()
	syncer, err := newV2flySync(listener.Addr().String(), newMemoryState())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syncer.Close() })
	return syncer, handler, stats
}
func reviewMemoryUser(t *testing.T, email, uuid string, level uint32) *protocol.MemoryUser {
	t.Helper()
	raw := &protocol.User{Email: email, Level: level, Account: serial.ToTypedMessage(&vmess.Account{Id: uuid})}
	user, err := raw.ToMemoryUser()
	if err != nil {
		t.Fatal(err)
	}
	return user
}
func TestReviewV2flyRevokesUserWithoutTrafficCounters(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "missing"}[missing], func(t *testing.T) {
			syncer, handler, _ := reviewRealVMess(t)
			memoryUser := reviewMemoryUser(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa@example.test", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 0)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := handler.AddUser(ctx, memoryUser); err != nil {
				t.Fatal(err)
			}
			var users []*pb.User
			if missing {
				// A remembered account can be revoked without counters, including after restarting MU.
				if err := syncer.Sync(ctx, []*pb.User{{Enable: 1, V2RayUser: &pb.VUser{Uuid: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: memoryUser.Email}}}); err != nil {
					t.Fatal(err)
				}
			}
			if !missing {
				users = []*pb.User{{Enable: 0, V2RayUser: &pb.VUser{Uuid: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", Email: memoryUser.Email}}}
			}
			if err := syncer.Sync(ctx, users); err != nil {
				t.Fatal(err)
			}
			if err := handler.AddUser(ctx, memoryUser); err != nil {
				t.Errorf("user still authorized after Sync: %v", err)
			}
		})
	}
}
func TestReviewV2flyUpdatesExistingUserLevel(t *testing.T) {
	syncer, handler, stats := reviewRealVMess(t)
	email := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa@example.test"
	uuid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := handler.AddUser(ctx, reviewMemoryUser(t, email, uuid, 0)); err != nil {
		t.Fatal(err)
	}
	for _, direction := range []string{"uplink", "downlink"} {
		counter, err := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>" + direction)
		if err != nil {
			t.Fatal(err)
		}
		counter.Set(0)
	}
	if err := syncer.Sync(ctx, []*pb.User{{Enable: 1, V2RayUser: &pb.VUser{Uuid: uuid, Email: email, Level: 7}}}); err != nil {
		t.Fatal(err)
	}
	if got := handler.GetUser(email).Level; got != 7 {
		t.Errorf("user policy was not updated: level=%d, want 7", got)
	}
}
func TestReviewV2flyDoesNotDeleteEnabledUserWithNormalEmail(t *testing.T) {
	syncer, handler, stats := reviewRealVMess(t)
	email := "alice@example.test"
	uuid := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	original := reviewMemoryUser(t, email, uuid, 0)
	if err := handler.AddUser(ctx, original); err != nil {
		t.Fatal(err)
	}
	counter, err := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(100)
	if err := syncer.Sync(ctx, []*pb.User{{Enable: 1, V2RayUser: &pb.VUser{Uuid: uuid, Email: email}}}); err != nil {
		t.Fatal(err)
	}
	// Bootstrap reconciles an unknown pre-existing account once because its credentials cannot be queried.
	original = handler.GetUser(email)
	for range 3 {
		if err := syncer.Sync(ctx, []*pb.User{{Enable: 1, V2RayUser: &pb.VUser{Uuid: uuid, Email: email}}}); err != nil {
			t.Fatal(err)
		}
		if handler.GetUser(email) != original {
			t.Error("enabled user was repeatedly deleted because email local part differs from UUID")
		}
	}

}
