package app

import (
	"context"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type unavailableStatsServer struct{ statscmd.StatsServiceServer }

func (s *unavailableStatsServer) QueryStats(context.Context, *statscmd.QueryStatsRequest) (*statscmd.QueryStatsResponse, error) {
	return nil, status.Error(codes.Unavailable, "injected stats outage")
}

func TestStatsOutageDoesNotPreventAuthorizationChanges(t *testing.T) {
	syncer, handler, _ := reviewRealVMess(t, true)
	email := "alice@example.test"
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(context.Background(), users); err == nil {
		t.Fatal("stats outage was not reported")
	}
	if err := handler.AddUser(context.Background(), reviewMemoryUser(t, email, fixtureUUID, 0)); err == nil {
		t.Fatal("stats outage prevented authorization from being installed")
	}
	if err := syncer.Sync(context.Background(), nil); err == nil {
		t.Fatal("stats outage was not reported on revocation")
	}
	if err := handler.AddUser(context.Background(), reviewMemoryUser(t, email, fixtureUUID, 0)); err != nil {
		t.Fatal("stats outage prevented a remembered account from being revoked")
	}
}

func TestRecreatedCounterCanExceedOldCursorWithoutLosingFirstInterval(t *testing.T) {
	syncer, _, stats := reviewRealVMess(t)
	email := "alice@example.test"
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	name := "user>>>" + email + ">>>traffic>>>uplink"
	counter, err := stats.RegisterCounter(name)
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(100)
	if _, err := syncer.GetTraffic(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	if err := stats.UnregisterCounter(name); err != nil {
		t.Fatal(err)
	}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	counter, err = stats.RegisterCounter(name)
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(200)
	logs, err := syncer.GetTraffic(context.Background(), users)
	if err != nil || len(logs) != 1 || logs[0].U != 200 {
		t.Fatalf("recreated counter reused an obsolete cursor: %v, %v", logs, err)
	}
}
