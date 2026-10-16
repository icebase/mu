package app

import (
	"context"
	"path/filepath"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/v2fly/v2ray-core/v5/proxy/vmess"
)

const fixtureUUID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"

func TestV2RayRegistryRevokesMissingUserAfterStateRestart(t *testing.T) {
	syncer, handler, _ := reviewRealVMess(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state, err := openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	syncer.state = state
	email := "alice@example.test"
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = state.Close() }()
	syncer.state = state
	if err := syncer.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	// A fresh Add succeeds only if the previous authorization really was revoked.
	if err := handler.AddUser(context.Background(), reviewMemoryUser(t, email, fixtureUUID, 0)); err != nil {
		t.Fatalf("remembered user still exists without any counters: %v", err)
	}
}

func TestV2RayConfigRotationUpdatesCredentialsAndKeepsBillingCursor(t *testing.T) {
	syncer, handler, stats := reviewRealVMess(t)
	email := "alice@example.test"
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	counter, err := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(100)
	if _, err := syncer.GetTraffic(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	nextUUID := "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	users = []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: nextUUID, Email: email, Level: 7}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	actual := handler.GetUser(email)
	actualUUID := actual.Account.(*vmess.MemoryAccount).ID.UUID()
	if actual.Level != 7 || actualUUID.String() != nextUUID {
		t.Fatal("credential or policy change was ignored")
	}
	counter.Add(5)
	logs, err := syncer.GetTraffic(context.Background(), users)
	if err != nil || len(logs) != 1 || logs[0].U != 5 || logs[0].Uuid != nextUUID {
		t.Fatalf("rotation reset or misattributed billing cursor: %v, %v", logs, err)
	}
}

func TestV2RayCounterHistoryDoesNotPreventReenable(t *testing.T) {
	syncer, handler, stats := reviewRealVMess(t)
	email := "alice@example.test"
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: email}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	counter, err := stats.RegisterCounter("user>>>" + email + ">>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(100)
	if err := syncer.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	// V2Ray retains counters after RemoveUser; a positive counter is not evidence of authorization.
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	if err := handler.AddUser(context.Background(), reviewMemoryUser(t, email, fixtureUUID, 0)); err == nil {
		t.Fatal("positive old counters prevented the account from being re-added")
	}
	if counter.Value() != 100 {
		t.Fatal("collection reset the backend counter")
	}
	counter.Add(5)
	logs, err := syncer.GetTraffic(context.Background(), users)
	if err != nil || len(logs) != 1 || logs[0].U != 5 {
		t.Fatalf("old traffic was rebilled after re-enable: %v, %v", logs, err)
	}
}

func TestV2RayCountersWithCaseVariantsKeepIndependentCursors(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"alice@example.test": {ID: 1, UUID: fixtureUUID, Email: "alice@example.test"}}
	first := map[string]trafficSample{
		"Alice@example.test": {Up: 100, HasUp: true, UserKey: "alice@example.test"},
		"alice@example.test": {Up: 200, HasUp: true, UserKey: "alice@example.test"},
	}
	if _, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_V2FLY, users, first); err != nil {
		t.Fatal(err)
	}
	second := map[string]trafficSample{
		"Alice@example.test": {Up: 110, HasUp: true, UserKey: "alice@example.test"},
		"alice@example.test": {Up: 220, HasUp: true, UserKey: "alice@example.test"},
	}
	logs, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_V2FLY, users, second)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, log := range logs {
		total += log.U
	}
	if total != 30 {
		t.Fatalf("email case variants overwrote or duplicated a counter: %d", total)
	}
}
