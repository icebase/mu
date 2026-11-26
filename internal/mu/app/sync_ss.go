package app

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"

	"github.com/icebase/mu/pkg/ss"
	pb "github.com/icebase/mu/proto/v1"
)

type ssSync struct {
	manager *ss.Manager
	state   *syncState
	key     string
}

func newSSSync(addr, token string, state *syncState) (*ssSync, error) {
	if state == nil {
		return nil, errors.New("synchronization state is required")
	}
	manager, err := ss.NewManager(addr, token)
	if err != nil {
		return nil, err
	}
	return &ssSync{manager: manager, state: state, key: "ss:" + addr}, nil
}

func (s *ssSync) Name() string { return protocolSS }
func (s *ssSync) Close() error { return s.manager.Close() }

// ssName is the stable ss-server user name. Credentials never serve as identifiers.
func ssName(id int64) string { return strconv.FormatInt(id, 10) }

// desiredSSUsers keeps only accounts that carry Shadowsocks credentials.
// Like the other protocols, a malformed record rejects the whole snapshot so no valid account is revoked by mistake.
func desiredSSUsers(users []*pb.User) (map[string]managedUser, map[string]managedUser, error) {
	all := make(map[string]managedUser)
	enabled := make(map[string]managedUser)
	psks := make(map[string]bool)
	for index, user := range users {
		if user == nil || (user.Enable != 0 && user.Enable != 1) {
			return nil, nil, fmt.Errorf("invalid user record at index %d", index)
		}
		if user.SsUser == nil {
			continue
		}
		if user.Id <= 0 {
			return nil, nil, fmt.Errorf("invalid Shadowsocks user ID at index %d", index)
		}
		raw, err := base64.StdEncoding.DecodeString(user.SsUser.Psk)
		if err != nil || (len(raw) != 16 && len(raw) != 32) {
			return nil, nil, fmt.Errorf("invalid Shadowsocks key at index %d", index)
		}
		id := ssName(user.Id)
		if _, exists := all[id]; exists {
			return nil, nil, errors.New("duplicate Shadowsocks user ID in snapshot")
		}
		// Identical keys would be indistinguishable to the server and mix two accounts' traffic.
		if psks[user.SsUser.Psk] {
			return nil, nil, errors.New("duplicate Shadowsocks key in snapshot")
		}
		psks[user.SsUser.Psk] = true
		record := managedUser{ID: user.Id, UUID: id, PSK: user.SsUser.Psk}
		all[id] = record
		if user.Enable == 1 {
			enabled[id] = record
		}
	}
	return all, enabled, nil
}

func (s *ssSync) capture(users map[string]managedUser, snapshot []ss.UserStatus) ([]*pb.UserTrafficLog, error) {
	samples := make(map[string]trafficSample, len(snapshot))
	for _, entry := range snapshot {
		samples[entry.Name] = trafficSample{Up: entry.Upload, Down: entry.Download, HasUp: true, HasDown: true}
	}
	return s.state.capture(s.key, pb.TrafficType_TRAFFIC_TYPE_SHADOWSOCKS, users, samples)
}

func fingerprintOf(psk string) string {
	raw, err := base64.StdEncoding.DecodeString(psk)
	if err != nil {
		return ""
	}
	return ss.Fingerprint(raw)
}

func (s *ssSync) Sync(ctx context.Context, users []*pb.User) error {
	all, enabled, err := desiredUsers(users, s.Name())
	if err != nil {
		return err
	}
	snapshot, err := s.manager.ListUsers(ctx)
	if err != nil {
		return err
	}
	// Includes remembered users absent from the new MU snapshot, before their counters are destroyed.
	if _, err := s.capture(all, snapshot); err != nil {
		return err
	}
	present := make(map[string]ss.UserStatus, len(snapshot))
	for _, entry := range snapshot {
		present[entry.Name] = entry
	}
	reset := make(map[string]bool)
	for id := range enabled {
		if _, ok := present[id]; !ok {
			reset[id] = true
		}
	}
	// Write ahead of remote additions so a crash cannot leave an untracked authorized account.
	if err := s.state.prepare(s.key, all, reset); err != nil {
		return err
	}
	var errs []error
	removed := make([]string, 0, len(present))
	for id := range present {
		if _, keep := enabled[id]; keep {
			continue
		}
		if err := s.manager.RemoveUser(ctx, id); err != nil {
			errs = append(errs, fmt.Errorf("remove Shadowsocks user ID %s: %w", id, err))
			continue
		}
		removed = append(removed, id)
	}
	applied := make(map[string]managedUser)
	for id, user := range enabled {
		// A key rotation keeps the name, so a fingerprint mismatch replaces the key in place.
		if current, ok := present[id]; !ok || current.PSKFingerprint != fingerprintOf(user.PSK) {
			if err := s.manager.SetUser(ctx, id, user.PSK); err != nil {
				errs = append(errs, fmt.Errorf("set Shadowsocks user ID %s: %w", id, err))
				continue
			}
		}
		applied[id] = user
	}
	if err := s.state.complete(s.key, applied, removed); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (s *ssSync) GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error) {
	all, _, err := desiredUsers(users, s.Name())
	if err != nil {
		return nil, err
	}
	snapshot, err := s.manager.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	return s.capture(all, snapshot)
}
