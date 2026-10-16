package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"google.golang.org/protobuf/proto"
)

const stateVersion = 1
const maxPendingLogs = 100000
const maxStateBytes = 64 << 20

// State contains credentials. It must live on a private, persistent volume, never in logs.
type managedUser struct {
	ID             int64
	UUID, Email    string
	Level, AlterID uint32
}

func (u managedUser) v2User() *v2flyUser {
	return &v2flyUser{uuid: u.UUID, email: u.Email, level: u.Level, alterID: u.AlterID}
}

type trafficCursor struct {
	Up, Down       int64
	HasUp, HasDown bool
}
type trafficSample struct {
	Up, Down       int64
	HasUp, HasDown bool
	UserKey        string // Optional owner key when a counter identity differs from the authorization key.
}
type backendState struct {
	Users   map[string]managedUser
	Applied map[string]managedUser
	Cursors map[string]trafficCursor
}
type trafficBatch struct {
	UploadAt int64
	Logs     []*pb.UserTrafficLog
}
type stateData struct {
	Version  int
	NodeID   string
	Backends map[string]*backendState
	Pending  []trafficBatch
}
type syncState struct {
	mu     sync.Mutex
	data   stateData
	save   func([]byte) (bool, error)
	lock   *os.File
	closed bool
}

func newMemoryState() *syncState {
	return &syncState{data: stateData{Version: stateVersion, Backends: make(map[string]*backendState)}}
}
func openSyncState(path, nodeID string) (*syncState, error) {
	if path == "" {
		return nil, errors.New("state file path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0700); err != nil {
		return nil, err
	}
	lock, err := lockStateFile(absolute + ".lock")
	if err != nil {
		return nil, err
	}
	state := newMemoryState()
	state.lock = lock
	state.save = func(data []byte) (bool, error) { return writeStateFile(absolute, data) }
	if info, err := os.Stat(absolute); err == nil && info.Size() > maxStateBytes {
		_ = state.Close()
		return nil, errors.New("synchronization state file exceeds size limit")
	}
	data, err := os.ReadFile(absolute)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = state.Close()
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, &state.data); err != nil {
			_ = state.Close()
			return nil, errors.New("invalid synchronization state file")
		}
		if state.data.Version != stateVersion || state.data.NodeID != nodeID || state.data.Backends == nil {
			_ = state.Close()
			return nil, errors.New("synchronization state version or node identity mismatch")
		}
		if err := validateState(state.data); err != nil {
			_ = state.Close()
			return nil, err
		}
		if err := os.Chmod(absolute, 0600); err != nil {
			_ = state.Close()
			return nil, err
		}
	}
	state.data.NodeID = nodeID
	// Verify durable storage before contacting either authorization API.
	if err := state.update(func(*stateData) error { return nil }); err != nil {
		_ = state.Close()
		return nil, err
	}
	return state, nil
}
func (s *syncState) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock != nil {
		err := s.lock.Close()
		s.lock = nil
		return err
	}
	return nil
}
func (s *syncState) update(change func(*stateData) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("synchronization state is closed")
	}
	encoded, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	var next stateData
	if err := json.Unmarshal(encoded, &next); err != nil {
		return err
	}
	if err := change(&next); err != nil {
		return err
	}
	encoded, err = json.Marshal(next)
	if err != nil {
		return err
	}
	if len(encoded) > maxStateBytes {
		return errors.New("synchronization state exceeds size limit")
	}
	if s.save != nil {
		committed, err := s.save(encoded)
		if committed {
			s.data = next
		}
		return err
	}
	s.data = next
	return nil
}
func backend(data *stateData, key string) *backendState {
	b := data.Backends[key]
	if b == nil {
		b = &backendState{}
		data.Backends[key] = b
	}
	if b.Users == nil {
		b.Users = make(map[string]managedUser)
	}
	if b.Applied == nil {
		b.Applied = make(map[string]managedUser)
	}
	if b.Cursors == nil {
		b.Cursors = make(map[string]trafficCursor)
	}
	return b
}
func (s *syncState) snapshot() stateData {
	s.mu.Lock()
	defer s.mu.Unlock()
	encoded, _ := json.Marshal(s.data)
	var data stateData
	_ = json.Unmarshal(encoded, &data)
	return data
}
func (s *syncState) prepare(key string, users map[string]managedUser, reset map[string]bool) error {
	cursors := make(map[string]trafficCursor, len(reset))
	for id, shouldReset := range reset {
		if shouldReset {
			cursors[id] = trafficCursor{HasUp: true, HasDown: true}
		}
	}
	return s.prepareCursors(key, users, cursors)
}
func (s *syncState) prepareCursors(key string, users map[string]managedUser, cursors map[string]trafficCursor) error {
	return s.update(func(data *stateData) error {
		b := backend(data, key)
		// Existing identities keep their billing owner until the remote change is acknowledged.
		// For a new identity, write ahead so an interrupted installation can still be revoked.
		for id, user := range users {
			if applied, exists := b.Applied[id]; exists && applied != user {
				delete(b.Applied, id)
			}
			if _, exists := b.Users[id]; !exists {
				b.Users[id] = user
			}
		}
		maps.Copy(b.Cursors, cursors)
		return nil
	})
}
func (s *syncState) complete(key string, applied map[string]managedUser, removed []string) error {
	return s.update(func(data *stateData) error {
		b := backend(data, key)
		for _, id := range removed {
			delete(b.Applied, id)
		}
		maps.Copy(b.Users, applied)
		maps.Copy(b.Applied, applied)
		return nil
	})
}
func counterDelta(current, previous int64) int64 {
	if current < previous {
		return current
	} // An observed backend counter reset starts a new epoch.
	return current - previous
}

// capture commits counter cursors and pending uploads together. Failed disk writes never advance a cursor.
// Unknown, pre-existing accounts are baselined once to avoid rebilling lifetime traffic on first migration.
// Newly installed accounts are seeded at zero by prepare, so their first interval is counted.
func (s *syncState) capture(key string, kind pb.TrafficType, users map[string]managedUser, samples map[string]trafficSample) ([]*pb.UserTrafficLog, error) {
	var logs []*pb.UserTrafficLog
	err := s.update(func(data *stateData) error {
		b := backend(data, key)
		for id, user := range users {
			if _, exists := b.Users[id]; !exists {
				b.Users[id] = user
			}
		}
		for id, sample := range samples {
			owner := sample.UserKey
			if owner == "" {
				owner = id
			}
			user, known := b.Users[owner]
			if !known {
				continue
			}
			cursor := b.Cursors[id]
			var up, down int64
			if sample.HasUp {
				if cursor.HasUp {
					up = counterDelta(sample.Up, cursor.Up)
				}
				cursor.Up = sample.Up
				cursor.HasUp = true
			}
			if sample.HasDown {
				if cursor.HasDown {
					down = counterDelta(sample.Down, cursor.Down)
				}
				cursor.Down = sample.Down
				cursor.HasDown = true
			}
			b.Cursors[id] = cursor
			if up > 0 || down > 0 {
				logs = append(logs, &pb.UserTrafficLog{UserId: user.ID, Uuid: user.UUID, U: up, D: down, TrafficType: int32(kind)})
			}
		}
		if len(logs) == 0 {
			return nil
		}
		count := len(logs)
		for _, batch := range data.Pending {
			count += len(batch.Logs)
		}
		if count > maxPendingLogs {
			return errors.New("traffic outbox is full; counters have not been advanced")
		}
		data.Pending = append(data.Pending, trafficBatch{UploadAt: time.Now().Unix(), Logs: logs})
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Returned protobuf messages must not alias the durable outbox.
	result := make([]*pb.UserTrafficLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, proto.Clone(log).(*pb.UserTrafficLog))
	}
	return result, nil
}
func (s *syncState) acknowledge() error {
	return s.update(func(data *stateData) error {
		if len(data.Pending) == 0 {
			return errors.New("traffic outbox is empty")
		}
		data.Pending = data.Pending[1:]
		return nil
	})
}
func writeStateFile(path string, data []byte) (bool, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".mu-state-*")
	if err != nil {
		return false, err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return false, err
	}
	if err := file.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(name, path); err != nil {
		return false, err
	}
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return true, err
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Sync(); err != nil {
		return true, fmt.Errorf("sync state directory: %w", err)
	}
	return true, nil
}
