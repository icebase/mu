package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strings"

	"github.com/icebase/mu/internal/mu/app/metrics"
	"github.com/icebase/mu/internal/mu/app/tracing"
	"github.com/icebase/mu/pkg/v2fly"
	pb "github.com/icebase/mu/proto/v1"
	"go.opentelemetry.io/otel/attribute"
)

const tag = "proxy"

// vlessEmailSuffix is appended to the MU email to form the V2Ray backend email of VLESS users.
// V2Ray statistics are keyed by email only, so this keeps VLESS and VMess counters of one
// V2Ray instance disjoint and prevents the same traffic from being billed by both syncers.
const vlessEmailSuffix = "#vless"

type v2flySync struct {
	manager *v2fly.Manager
	state   *syncState
	key     string
	name    string
}

func newV2flySync(addr string, state *syncState) (*v2flySync, error) {
	if state == nil {
		return nil, errors.New("synchronization state is required")
	}
	manager, err := v2fly.NewManager(addr, tag)
	if err != nil {
		return nil, err
	}
	return &v2flySync{manager: manager, state: state, key: "v2fly:" + addr + ":" + tag, name: protocolV2Fly}, nil
}

// newVlessSync manages a VLESS inbound of a v2fly instance. Backend emails carry vlessEmailSuffix.
func newVlessSync(addr, inboundTag string, state *syncState) (*v2flySync, error) {
	if state == nil {
		return nil, errors.New("synchronization state is required")
	}
	if strings.TrimSpace(inboundTag) == "" {
		return nil, errors.New("VLESS inbound tag is required")
	}
	manager, err := v2fly.NewManager(addr, inboundTag, v2fly.WithProtocol(v2fly.ProtocolVLESS))
	if err != nil {
		return nil, err
	}
	return &v2flySync{manager: manager, state: state, key: "vless:" + addr + ":" + inboundTag, name: protocolVLESS}, nil
}
func (v *v2flySync) Name() string {
	if v.name == "" {
		return protocolV2Fly
	}
	return v.name
}

// userList returns the traffic identities that belong to this syncer's protocol. Counters of the
// other protocol sharing the same V2Ray instance are ignored.
func (v *v2flySync) userList(ctx context.Context) ([]v2fly.UserData, error) {
	list, err := v.manager.GetUserList(ctx, false)
	if err != nil {
		return nil, err
	}
	own := list[:0:0]
	for _, entry := range list {
		if hasVlessSuffix(entry.User.GetEmail()) == (v.Name() == protocolVLESS) {
			own = append(own, entry)
		}
	}
	return own, nil
}
func hasVlessSuffix(email string) bool {
	return strings.HasSuffix(strings.ToLower(email), vlessEmailSuffix)
}
func (v *v2flySync) Close() error { return v.manager.Close() }
func (v *v2flySync) capture(users map[string]managedUser, snapshot []v2fly.UserData) ([]*pb.UserTrafficLog, error) {
	samples := make(map[string]trafficSample, len(snapshot))
	for _, entry := range snapshot {
		info := entry.TrafficInfo
		email := entry.User.GetEmail()
		samples[email] = trafficSample{Up: info.Up, Down: info.Down, HasUp: info.HasUp, HasDown: info.HasDown, UserKey: strings.ToLower(email)}
	}
	return v.state.capture(v.key, pb.TrafficType_TRAFFIC_TYPE_V2FLY, users, samples)
}
func (v *v2flySync) Sync(ctx context.Context, users []*pb.User) error {
	all, enabled, err := desiredUsers(users, v.Name())
	if err != nil {
		return err
	}
	var errs []error
	snapshot, statsErr := v.userList(ctx)
	if statsErr != nil {
		// Authorization must not depend on the stats service. Non-reset counters can be collected later.
		errs = append(errs, statsErr)
	} else if _, err := v.capture(all, snapshot); err != nil {
		return err
	}
	data := v.state.snapshot()
	previous := backend(&data, v.key)
	known := maps.Clone(previous.Users)
	for id, user := range all {
		if _, exists := known[id]; !exists {
			known[id] = user
		}
	}
	untracked := 0
	for _, entry := range snapshot {
		email := entry.User.GetEmail()
		id := strings.ToLower(email)
		if _, exists := known[id]; !exists {
			known[id] = managedUser{Email: email}
			untracked++
		}
	}
	if untracked > 0 {
		slog.Warn("untracked V2Ray statistics identities discovered; historical billing identity is unavailable", "count", untracked)
	}
	observed := make(map[string]v2fly.TrafficInfo, len(snapshot))
	for _, entry := range snapshot {
		observed[entry.User.GetEmail()] = entry.TrafficInfo
	}
	cursors := make(map[string]trafficCursor)
	if statsErr == nil {
		for _, user := range enabled {
			cursor := previous.Cursors[user.Email]
			sample := observed[user.Email]
			// A complete snapshot proves absent directions have no surviving counter (e.g. after restart).
			if !sample.HasUp {
				cursor.Up = 0
				cursor.HasUp = true
			}
			if !sample.HasDown {
				cursor.Down = 0
				cursor.HasDown = true
			}
			cursors[user.Email] = cursor
		}
	}
	if err := v.state.prepareCursors(v.key, all, cursors); err != nil {
		return err
	}
	removed := make([]string, 0, len(known))
	for id, user := range known {
		if _, keep := enabled[id]; keep {
			continue
		}
		err := v.manager.RemoveUser(ctx, user.v2User())
		metrics.ObserveBackendUserOperation(v.Name(), operationRemove, err)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		removed = append(removed, id)
	}
	applied := make(map[string]managedUser)
	for id, user := range enabled {
		old, exists := previous.Applied[id]
		// Add is not an update. Replace unknown/changed configurations once; persist the applied identity.
		if !exists || !sameV2Account(old, user) {
			err := v.manager.RemoveUser(ctx, user.v2User())
			metrics.ObserveBackendUserOperation(v.Name(), operationRemove, err)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			removed = append(removed, id)
		}
		// Idempotent Add also restores users after a backend restart, regardless of traffic-counter existence.
		_, err := v.manager.AddUser(ctx, user.v2User())
		metrics.ObserveBackendUserOperation(v.Name(), operationAdd, err)
		if err != nil {
			errs = append(errs, fmt.Errorf("add V2Ray user ID %d: %w", user.ID, err))
			continue
		}
		applied[id] = user
	}
	tracing.SetAttributes(ctx,
		attribute.Int("sync.users_desired", len(all)), attribute.Int("sync.users_enabled", len(enabled)),
		attribute.Int("sync.users_observed", len(snapshot)), attribute.Int("sync.users_removed", len(removed)),
		attribute.Int("sync.users_applied", len(applied)), attribute.Bool("sync.stats_available", statsErr == nil),
	)
	if err := v.state.complete(v.key, applied, removed); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
func sameV2Account(a, b managedUser) bool {
	return a.UUID == b.UUID && a.Email == b.Email && a.Level == b.Level && a.AlterID == b.AlterID
}
func (v *v2flySync) GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error) {
	all, _, err := desiredUsers(users, v.Name())
	if err != nil {
		return nil, err
	}
	snapshot, err := v.userList(ctx)
	if err != nil {
		return nil, err
	}
	return v.capture(all, snapshot)
}

type v2flyUser struct {
	uuid, email    string
	level, alterID uint32
}

func (u *v2flyUser) GetUUID() string    { return u.uuid }
func (u *v2flyUser) GetEmail() string   { return u.email }
func (u *v2flyUser) GetLevel() uint32   { return u.level }
func (u *v2flyUser) GetAlterID() uint32 { return u.alterID }
