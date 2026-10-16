package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/common"
)

func TestDurableTrafficOutboxSurvivesRestart(t *testing.T) {
	syncer, auth := realTrojanForReview(t)
	path := filepath.Join(t.TempDir(), "state.json")
	state, err := openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	syncer.state = state
	users := []*pb.User{{Id: 7, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	_, meter := auth.AuthUser(common.SHA224String("active"))
	meter.AddTraffic(100, 200)
	logs, err := syncer.GetTraffic(context.Background(), users)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].U+logs[0].D != 300 {
		t.Fatalf("unexpected captured traffic: %v", logs)
	}
	// Callers cannot mutate persisted data through returned protobuf pointers.
	logs[0].U = 9999
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	state, err = openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = state.Close() })
	syncer.state = state
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("state permissions: %v", info.Mode())
	}
	logs, err = syncer.GetTraffic(context.Background(), users)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 {
		t.Fatal("restarting repeated an already captured delta")
	}
	snapshot := state.snapshot()
	if len(snapshot.Pending) != 1 || snapshot.Pending[0].Logs[0].U+snapshot.Pending[0].Logs[0].D != 300 {
		t.Fatal("pending traffic did not survive restart intact")
	}
	service := &reviewMUService{users: users, failUpload: true}
	app := reviewApp(t, service, syncer)
	if err := app.syncTraffic(); err == nil {
		t.Fatal("expected upload failure")
	}
	if len(state.snapshot().Pending) != 1 {
		t.Fatal("failed upload removed pending traffic")
	}
	if err := app.syncTraffic(); err != nil {
		t.Fatal(err)
	}
	if service.received != 300 || len(state.snapshot().Pending) != 0 {
		t.Fatalf("traffic retry mismatch: received=%d", service.received)
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	restored, err := openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close() }()
	if len(restored.snapshot().Pending) != 0 {
		t.Fatal("acknowledged batch reappeared after restart")
	}
}

func TestCaptureAndCursorAreOneTransaction(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"user": {ID: 1, UUID: "fixture"}}
	if err := state.prepare("backend", users, map[string]bool{"user": true}); err != nil {
		t.Fatal(err)
	}
	state.save = func([]byte) (bool, error) { return false, errors.New("injected disk failure") }
	sample := map[string]trafficSample{"user": {Up: 100, HasUp: true}}
	if _, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, sample); err == nil {
		t.Fatal("expected disk error")
	}
	snapshot := state.snapshot()
	if snapshot.Backends["backend"].Cursors["user"].Up != 0 || len(snapshot.Pending) != 0 {
		t.Fatal("failed disk write advanced billing state")
	}
	state.save = nil
	logs, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].U != 100 {
		t.Fatal("traffic could not be recaptured after disk failure")
	}
}

func TestRenamedStateRemainsConsistentAfterDirectorySyncError(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"user": {ID: 1, UUID: "fixture"}}
	if err := state.prepare("backend", users, map[string]bool{"user": true}); err != nil {
		t.Fatal(err)
	}
	state.save = func([]byte) (bool, error) { return true, errors.New("injected directory fsync failure after rename") }
	sample := map[string]trafficSample{"user": {Up: 100, HasUp: true}}
	if _, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, sample); err == nil {
		t.Fatal("expected directory error")
	}
	state.save = nil
	logs, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, sample)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 0 || len(state.snapshot().Pending) != 1 {
		t.Fatal("rename commit was lost or counted twice")
	}
}

func TestStateOwnershipLockAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state, err := openSyncState(path, "node")
	if err != nil {
		t.Fatal(err)
	}
	if other, err := openSyncState(path, "node"); err == nil {
		_ = other.Close()
		t.Fatal("two writers acquired the state file")
	}
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	if other, err := openSyncState(path, "other-node"); err == nil {
		_ = other.Close()
		t.Fatal("state reused for another node")
	}
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if other, err := openSyncState(path, "node"); err == nil {
		_ = other.Close()
		t.Fatal("corrupt state was silently reset")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{broken" {
		t.Fatal("corrupt state was overwritten")
	}
	for _, bad := range []stateData{
		{Version: stateVersion, NodeID: "node", Backends: map[string]*backendState{"bad": nil}},
		{Version: stateVersion, NodeID: "node", Backends: map[string]*backendState{}, Pending: []trafficBatch{{UploadAt: 1, Logs: []*pb.UserTrafficLog{nil}}}},
	} {
		data, err := json.Marshal(bad)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if other, err := openSyncState(path, "node"); err == nil {
			_ = other.Close()
			t.Fatal("semantically corrupt state was accepted")
		}
	}
}

func TestBootstrapDoesNotRebillPreexistingLifetimeTraffic(t *testing.T) {
	syncer, auth := realTrojanForReview(t, "active")
	_, meter := auth.AuthUser(common.SHA224String("active"))
	meter.SetTraffic(1000, 2000)
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	if logs, err := syncer.GetTraffic(context.Background(), users); err != nil || len(logs) != 0 {
		t.Fatalf("unknown historical traffic must be baselined: %v, %v", logs, err)
	}
	meter.AddTraffic(10, 20)
	logs, err := syncer.GetTraffic(context.Background(), users)
	if err != nil || len(logs) != 1 || logs[0].U+logs[0].D != 30 {
		t.Fatalf("new interval missing: %v, %v", logs, err)
	}
}

func TestTrojanRemovalQueuesFinalTrafficForMissingUser(t *testing.T) {
	syncer, auth := realTrojanForReview(t)
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	if err := syncer.Sync(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	_, meter := auth.AuthUser(common.SHA224String("active"))
	meter.AddTraffic(10, 20)
	if err := syncer.Sync(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if valid, _ := auth.AuthUser(common.SHA224String("active")); valid {
		t.Fatal("removed user is still authorized")
	}
	snapshot := syncer.state.snapshot()
	if len(snapshot.Pending) != 1 || snapshot.Pending[0].Logs[0].U+snapshot.Pending[0].Logs[0].D != 30 {
		t.Fatal("final traffic was not queued before deleting the account")
	}
}

func TestMissingCounterDirectionDoesNotEraseOtherDirection(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"user": {ID: 1, UUID: "fixture"}}
	if err := state.prepare("backend", users, map[string]bool{"user": true}); err != nil {
		t.Fatal(err)
	}
	for _, sample := range []trafficSample{{Up: 100, HasUp: true}, {Down: 200, HasDown: true}} {
		if _, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_V2FLY, users, map[string]trafficSample{"user": sample}); err != nil {
			t.Fatal(err)
		}
	}
	cursor := state.snapshot().Backends["backend"].Cursors["user"]
	if cursor.Up != 100 || cursor.Down != 200 {
		t.Fatalf("missing direction erased previous cursor: %+v", cursor)
	}
}
