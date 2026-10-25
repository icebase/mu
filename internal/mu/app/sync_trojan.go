package app

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/icebase/mu/internal/mu/app/metrics"
	"github.com/icebase/mu/internal/mu/app/tracing"
	"github.com/icebase/mu/pkg/trojan"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/api/service"
	"go.opentelemetry.io/otel/attribute"
)

type trojanSync struct {
	tjManager *trojan.Manager
	state     *syncState
	key       string
}

func newTrojanSync(addr string, state *syncState) (*trojanSync, error) {
	if state == nil {
		return nil, errors.New("synchronization state is required")
	}
	manager, err := trojan.NewManager(addr)
	if err != nil {
		return nil, err
	}
	return &trojanSync{tjManager: manager, state: state, key: "trojan:" + addr}, nil
}
func (t *trojanSync) Name() string { return protocolTrojan }
func (t *trojanSync) Close() error { return t.tjManager.Close() }
func (t *trojanSync) capture(users map[string]managedUser, snapshot []*service.UserStatus) ([]*pb.UserTrafficLog, error) {
	samples := make(map[string]trafficSample, len(snapshot))
	for _, entry := range snapshot {
		if entry.TrafficTotal.UploadTraffic > math.MaxInt64 || entry.TrafficTotal.DownloadTraffic > math.MaxInt64 {
			return nil, errors.New("trojan traffic counter exceeds supported range")
		}
		samples[entry.User.Hash] = trafficSample{
			Up: int64(entry.TrafficTotal.UploadTraffic), Down: int64(entry.TrafficTotal.DownloadTraffic),
			HasUp: true, HasDown: true,
		}
	}
	return t.state.capture(t.key, pb.TrafficType_TRAFFIC_TYPE_TROJAN, users, samples)
}
func (t *trojanSync) Sync(ctx context.Context, users []*pb.User) error {
	all, enabled, err := desiredUsers(users, t.Name())
	if err != nil {
		return err
	}
	snapshot, err := t.tjManager.ListUsers(ctx)
	if err != nil {
		return err
	}
	// Includes remembered users absent from the new MU snapshot, before their counters are destroyed.
	if _, err := t.capture(all, snapshot); err != nil {
		return err
	}
	present := make(map[string]bool, len(snapshot))
	for _, entry := range snapshot {
		present[entry.User.Hash] = true
	}
	reset := make(map[string]bool)
	for id := range enabled {
		if !present[id] {
			reset[id] = true
		}
	}
	// Write ahead of remote additions so a crash cannot leave an untracked authorized account.
	if err := t.state.prepare(t.key, all, reset); err != nil {
		return err
	}
	var errs []error
	removed := make([]string, 0, len(present))
	for id := range present {
		if _, keep := enabled[id]; keep {
			continue
		}
		err := t.tjManager.RemoveUser(ctx, "", id)
		metrics.ObserveBackendUserOperation(t.Name(), operationRemove, err)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, id)
	}
	applied := make(map[string]managedUser)
	for id, user := range enabled {
		if !present[id] {
			err := t.tjManager.AddUser(ctx, user.UUID)
			metrics.ObserveBackendUserOperation(t.Name(), operationAdd, err)
			if err != nil {
				errs = append(errs, fmt.Errorf("add Trojan user ID %d: %w", user.ID, err))
				continue
			}
		}
		applied[id] = user
	}
	tracing.SetAttributes(ctx,
		attribute.Int("sync.users_desired", len(all)), attribute.Int("sync.users_enabled", len(enabled)),
		attribute.Int("sync.users_present", len(present)), attribute.Int("sync.users_removed", len(removed)),
		attribute.Int("sync.users_applied", len(applied)),
	)
	if err := t.state.complete(t.key, applied, removed); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
func (t *trojanSync) GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error) {
	all, _, err := desiredUsers(users, t.Name())
	if err != nil {
		return nil, err
	}
	snapshot, err := t.tjManager.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return t.capture(all, snapshot)
}
