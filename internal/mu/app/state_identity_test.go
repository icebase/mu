package app

import (
	"testing"

	pb "github.com/icebase/mu/proto/v1"
)

func TestPendingConfigurationDoesNotReassignBillingBeforeAcknowledgement(t *testing.T) {
	state := newMemoryState()
	old := map[string]managedUser{"email": {ID: 1, UUID: fixtureUUID}}
	next := map[string]managedUser{"email": {ID: 2, UUID: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"}}
	if err := state.prepare("backend", old, map[string]bool{"email": true}); err != nil {
		t.Fatal(err)
	}
	if err := state.complete("backend", old, nil); err != nil {
		t.Fatal(err)
	}
	if err := state.prepare("backend", next, nil); err != nil {
		t.Fatal(err)
	}
	if _, exists := state.snapshot().Backends["backend"].Applied["email"]; exists {
		t.Fatal("pending credential replacement retained a stale applied identity")
	}

	logs, err := state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_V2FLY, next, map[string]trafficSample{"email": {Up: 10, HasUp: true}})
	if err != nil || len(logs) != 1 || logs[0].UserId != 1 {
		t.Fatalf("unapplied configuration reassigned old traffic: %v, %v", logs, err)
	}
	if err := state.complete("backend", next, nil); err != nil {
		t.Fatal(err)
	}
	logs, err = state.capture("backend", pb.TrafficType_TRAFFIC_TYPE_V2FLY, next, map[string]trafficSample{"email": {Up: 20, HasUp: true}})
	if err != nil || len(logs) != 1 || logs[0].UserId != 2 || logs[0].U != 10 {
		t.Fatalf("acknowledged configuration did not take ownership: %v, %v", logs, err)
	}
}
