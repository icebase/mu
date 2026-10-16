package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/icebase/mu/internal/mu/app/metrics"
	pb "github.com/icebase/mu/proto/v1"
	dto "github.com/prometheus/client_model/go"
)

func TestInvalidSnapshotsAreRejectedBeforeBackendCalls(t *testing.T) {
	valid := func(uuid, email string) *pb.User {
		return &pb.User{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: uuid, Email: email}}
	}
	tests := []struct {
		name   string
		users  []*pb.User
		v2Only bool
	}{
		{name: "nil user", users: []*pb.User{nil}},
		{name: "missing account", users: []*pb.User{{Id: 1, Enable: 1}}},
		{name: "empty UUID", users: []*pb.User{valid("", "alice@example.test")}},
		{name: "invalid enable", users: []*pb.User{{Enable: 2, V2RayUser: &pb.VUser{Uuid: fixtureUUID, Email: "alice@example.test"}}}},
		{name: "duplicate UUID", users: []*pb.User{valid(fixtureUUID, "alice@example.test"), valid(fixtureUUID, "bob@example.test")}},
		{name: "invalid VMess UUID", users: []*pb.User{valid("invalid", "alice@example.test")}, v2Only: true},
		{name: "duplicate canonical VMess UUID", users: []*pb.User{valid(fixtureUUID, "alice@example.test"), valid("AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", "bob@example.test")}, v2Only: true},
		{name: "empty email", users: []*pb.User{valid(fixtureUUID, "")}, v2Only: true},
		{name: "counter delimiter in email", users: []*pb.User{valid(fixtureUUID, "alice>>>traffic")}, v2Only: true},
		{name: "duplicate case-insensitive email", users: []*pb.User{valid(fixtureUUID, "Alice@example.test"), valid("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "alice@example.test")}, v2Only: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backends := []UserSync{&v2flySync{}}
			if !tt.v2Only {
				backends = append(backends, &trojanSync{})
			}
			for _, backend := range backends {
				if err := backend.Sync(context.Background(), tt.users); err == nil {
					t.Errorf("%s Sync accepted invalid snapshot", backend.Name())
				}
				if _, err := backend.GetTraffic(context.Background(), tt.users); err == nil {
					t.Errorf("%s GetTraffic accepted invalid snapshot", backend.Name())
				}
			}
		})
	}
}

func TestTrafficFailureDoesNotSkipHealthyBackendOrReportSuccess(t *testing.T) {
	first := &reviewSync{name: "trojan", trafficErr: errors.New("injected traffic failure")}
	second := &reviewSync{name: "v2fly"}
	app := reviewApp(t, &reviewMUService{}, first, second)
	counter := metrics.TrafficSyncCounter.WithLabelValues(syncFailure)
	before := &dto.Metric{}
	if err := counter.Write(before); err != nil {
		t.Fatal(err)
	}
	if err := app.syncTraffic(); err == nil {
		t.Fatal("partial collection failure reported success")
	}
	if second.trafficCalled != 1 {
		t.Fatal("healthy backend was skipped")
	}
	after := &dto.Metric{}
	if err := counter.Write(after); err != nil {
		t.Fatal(err)
	}
	if after.GetCounter().GetValue() != before.GetCounter().GetValue()+1 {
		t.Fatal("partial failure was not reflected in failure metrics")
	}
}

func TestPendingUploadIsRetriedEvenWhenUserSnapshotFails(t *testing.T) {
	state := newMemoryState()
	users := map[string]managedUser{"user": {ID: 1, UUID: fixtureUUID}}
	if err := state.prepare("source", users, map[string]bool{"user": true}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.capture("source", pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, map[string]trafficSample{"user": {Up: 7, HasUp: true}}); err != nil {
		t.Fatal(err)
	}
	service := &reviewMUService{failGetUsers: true}
	app := reviewApp(t, service)
	app.state = state
	if err := app.syncTraffic(); err == nil {
		t.Fatal("expected user snapshot error")
	}
	if service.received != 7 || len(state.snapshot().Pending) != 0 {
		t.Fatal("source API failure blocked a pending upload")
	}
}

func TestStorageFailurePreventsInitializationBeforeExternalClients(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	app := New(&Config{MuNodeID: "node", StateFile: filepath.Join(file, "state.json")})
	if err := app.Init(); err == nil {
		_ = app.Close()
		t.Fatal("unwritable storage was accepted")
	}
	if app.muClient != nil || len(app.userSync) != 0 {
		t.Fatal("external clients were initialized before durable storage")
	}
}

func TestAddressNormalizationAndStateConfiguration(t *testing.T) {
	t.Setenv("MU_STATE_FILE", "/var/lib/mu/test-state.json")
	t.Setenv("MU_TROJAN_ADDRS", " a:1, ,a:1,b:2 ")
	config := ConfigFromEnv()
	if config.StateFile != "/var/lib/mu/test-state.json" || len(config.TrojanAddrs) != 2 || config.TrojanAddrs[0] != "a:1" || config.TrojanAddrs[1] != "b:2" {
		t.Fatal("state configuration or endpoint normalization failed")
	}
}
