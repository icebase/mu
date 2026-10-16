package app

import (
	"context"
	"errors"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
)

func TestAcknowledgementDiskFailureRetainsBatchForAtLeastOnceRetry(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"user": {ID: 1, UUID: fixtureUUID}}
	if err := state.prepare("source", users, map[string]bool{"user": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.capture("source", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, map[string]trafficSample{"user": {Up: 7, HasUp: true}}); err != nil {
		t.Fatal(err)
	}
	service := &reviewMUService{}
	app := reviewApp(t, service)
	app.state = state
	state.save = func([]byte) (bool, error) { return false, errors.New("injected acknowledgement disk failure") }
	if err := app.flushTraffic(context.Background()); err == nil {
		t.Fatal("acknowledgement write failure was hidden")
	}
	if len(state.snapshot().Pending) != 1 || service.received != 7 {
		t.Fatal("accepted batch was dropped before a durable acknowledgement")
	}
	state.save = nil
	if err := app.flushTraffic(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The current server API has no idempotency key. This unavoidable duplicate is documented, not hidden.
	if len(state.snapshot().Pending) != 0 || service.received != 14 {
		t.Fatal("at-least-once retry behavior changed")
	}
}
