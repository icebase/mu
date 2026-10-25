package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/common"
	"github.com/v2fly/v2ray-core/v5/common/uuid"
)

const protocolTrojan = "trojan"
const protocolV2Fly = "v2fly"
const protocolVLESS = "vless"

// Backend user operation labels for mu_backend_user_operations_total.
const (
	operationAdd    = "add"
	operationRemove = "remove"
)

// GetTraffic durably queues deltas in the shared state before returning them for metrics.
// Sync also takes a final snapshot before changing authorization. Neither operation resets counters.
type UserSync interface {
	Sync(context.Context, []*pb.User) error
	GetTraffic(context.Context, []*pb.User) ([]*pb.UserTrafficLog, error)
	Name() string
}

func desiredUsers(users []*pb.User, protocol string) (map[string]managedUser, map[string]managedUser, error) {
	all := make(map[string]managedUser)
	enabled := make(map[string]managedUser)
	uuids := make(map[string]bool)
	emails := make(map[string]bool)
	for index, user := range users {
		// Reject the whole snapshot before removals; silently dropping malformed users could revoke valid accounts.
		if user == nil || user.V2RayUser == nil || strings.TrimSpace(user.V2RayUser.Uuid) == "" || (user.Enable != 0 && user.Enable != 1) {
			return nil, nil, fmt.Errorf("invalid user record at index %d", index)
		}
		v := user.V2RayUser
		uuidKey := v.Uuid
		id := common.SHA224String(v.Uuid)
		backendEmail := v.Email
		if protocol == protocolV2Fly || protocol == protocolVLESS {
			parsed, err := uuid.ParseString(v.Uuid)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid V2Ray UUID at index %d", index)
			}
			uuidKey = parsed.String()
			if strings.TrimSpace(v.Email) == "" || strings.TrimSpace(v.Email) != v.Email || strings.Contains(v.Email, ">>>") {
				return nil, nil, fmt.Errorf("invalid V2Ray email at index %d", index)
			}
			// The suffix is reserved for VLESS backend identities; reject it so protocols cannot collide.
			if hasVlessSuffix(v.Email) {
				return nil, nil, fmt.Errorf("V2Ray email at index %d uses reserved suffix %q", index, vlessEmailSuffix)
			}
			if protocol == protocolVLESS {
				backendEmail += vlessEmailSuffix
			}
			id = strings.ToLower(backendEmail)
			if emails[id] {
				return nil, nil, errors.New("duplicate V2Ray email in snapshot")
			}
			emails[id] = true
		}
		if uuids[uuidKey] {
			return nil, nil, errors.New("duplicate user UUID in snapshot")
		}
		uuids[uuidKey] = true
		record := managedUser{ID: user.Id, UUID: v.Uuid, Email: backendEmail, Level: v.Level, AlterID: v.AlterId}
		all[id] = record
		if user.Enable == 1 {
			enabled[id] = record
		}
	}
	return all, enabled, nil
}
