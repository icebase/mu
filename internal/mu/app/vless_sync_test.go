package app

import (
	"context"
	"net"
	"strings"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
	core "github.com/v2fly/v2ray-core/v5"
	"github.com/v2fly/v2ray-core/v5/app/proxyman"
	handlercmd "github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	inboundmanager "github.com/v2fly/v2ray-core/v5/app/proxyman/inbound"
	statsmanager "github.com/v2fly/v2ray-core/v5/app/stats"
	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/serial"
	"github.com/v2fly/v2ray-core/v5/proxy/vless"
	vlessinbound "github.com/v2fly/v2ray-core/v5/proxy/vless/inbound"
	"google.golang.org/grpc"
)

// vlessRPCServer forwards AlterInbound user operations to a real VLESS inbound handler.
type vlessRPCServer struct {
	handlercmd.UnimplementedHandlerServiceServer
	handler *vlessinbound.Handler
}

func (s *vlessRPCServer) AlterInbound(ctx context.Context, req *handlercmd.AlterInboundRequest) (*handlercmd.AlterInboundResponse, error) {
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
		return &handlercmd.AlterInboundResponse{}, s.handler.AddUser(ctx, user)
	case *handlercmd.RemoveUserOperation:
		return &handlercmd.AlterInboundResponse{}, s.handler.RemoveUser(ctx, v.Email)
	}
	return &handlercmd.AlterInboundResponse{}, nil
}

func realVLESS(t *testing.T) (*v2flySync, *vlessinbound.Handler, *statsmanager.Manager) {
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
	raw, err := core.CreateObject(instance, &vlessinbound.Config{})
	if err != nil {
		t.Fatal(err)
	}
	handler := raw.(*vlessinbound.Handler)
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
	handlercmd.RegisterHandlerServiceServer(server, &vlessRPCServer{handler: handler})
	statscmd.RegisterStatsServiceServer(server, statscmd.NewStatsServer(stats))
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()
	syncer, err := newVlessSync(listener.Addr().String(), "vless", newMemoryState())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syncer.Close() })
	return syncer, handler, stats
}

func vlessMemoryUser(t *testing.T, email, uuid string) *protocol.MemoryUser {
	t.Helper()
	raw := &protocol.User{Email: email, Account: serial.ToTypedMessage(&vless.Account{Id: uuid, Encryption: "none"})}
	user, err := raw.ToMemoryUser()
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func TestVlessSyncInstallsSuffixedUserAndRevokesIt(t *testing.T) {
	syncer, handler, _ := realVLESS(t)
	ctx := context.Background()
	email := "alice@example.test"
	users := []*pb.User{{Id: 7, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	// The backend identity carries the suffix; the bare MU email must not exist in the inbound.
	if err := handler.AddUser(ctx, vlessMemoryUser(t, email+vlessEmailSuffix, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")); err == nil || !strings.HasSuffix(err.Error(), " already exists.") {
		t.Fatalf("suffixed user was not installed: %v", err)
	}
	if err := handler.AddUser(ctx, vlessMemoryUser(t, email, "cccccccc-cccc-4ccc-8ccc-cccccccccccc")); err != nil {
		t.Fatalf("unsuffixed email was installed: %v", err)
	}
	if err := handler.RemoveUser(ctx, email); err != nil {
		t.Fatal(err)
	}
	// Idempotent re-sync, then disabling revokes the account.
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	users[0].Enable = 0
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	if err := handler.AddUser(ctx, vlessMemoryUser(t, email+vlessEmailSuffix, fixtureUUID)); err != nil {
		t.Fatalf("user still authorized after being disabled: %v", err)
	}
}

func TestVlessSyncBillsOnlySuffixedCountersAsV2FlyTraffic(t *testing.T) {
	syncer, _, stats := realVLESS(t)
	ctx := context.Background()
	email := "alice@example.test"
	users := []*pb.User{{Id: 7, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	vlessUp, err := stats.RegisterCounter("user>>>" + email + vlessEmailSuffix + ">>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	// A VMess counter for the same MU email in the same V2Ray instance must not be billed by VLESS.
	vmessUp, err := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	vlessUp.Set(100)
	vmessUp.Set(5000)
	if _, err := syncer.GetTraffic(ctx, users); err != nil { // establishes the baseline
		t.Fatal(err)
	}
	vlessUp.Set(160)
	vmessUp.Set(9000)
	logs, err := syncer.GetTraffic(ctx, users)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].UserId != 7 || logs[0].U != 60 || logs[0].Uuid != fixtureUUID ||
		logs[0].TrafficType != int32(pb.TrafficType_TRAFFIC_TYPE_V2FLY) {
		t.Fatalf("unexpected traffic logs: %v", logs)
	}
}

func TestV2FlySyncIgnoresVlessCounters(t *testing.T) {
	syncer, _, stats := reviewRealVMess(t)
	ctx := context.Background()
	email := "alice@example.test"
	users := []*pb.User{{Id: 7, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(ctx, users); err != nil {
		t.Fatal(err)
	}
	vmessUp, _ := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>uplink")
	vlessUp, _ := stats.RegisterCounter("user>>>" + email + vlessEmailSuffix + ">>>traffic>>>uplink")
	vmessUp.Set(10)
	vlessUp.Set(1000)
	if _, err := syncer.GetTraffic(ctx, users); err != nil {
		t.Fatal(err)
	}
	vmessUp.Set(30)
	vlessUp.Set(3000)
	logs, err := syncer.GetTraffic(ctx, users)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].U != 20 {
		t.Fatalf("VMess syncer billed VLESS counters: %v", logs)
	}
}

func TestDesiredUsersRejectsReservedVlessSuffix(t *testing.T) {
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: "alice" + strings.ToUpper(vlessEmailSuffix)}}}
	for _, protocol := range []string{protocolV2Fly, protocolVLESS} {
		if _, _, err := desiredUsers(users, protocol); err == nil {
			t.Fatalf("%s accepted an email using the reserved suffix", protocol)
		}
	}
}
