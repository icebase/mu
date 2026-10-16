package v2fly

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/serial"
	"github.com/v2fly/v2ray-core/v5/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const requestTimeout = 5 * time.Second
const uplinkDirection = "uplink"
const downlinkDirection = "downlink"
const (
	UplinkFormat   = "user>>>%s>>>traffic>>>uplink"
	DownlinkFormat = "user>>>%s>>>traffic>>>downlink"
)

type Manager struct {
	conn        *grpc.ClientConn
	client      command.HandlerServiceClient
	statsClient statscmd.StatsServiceClient
	inBoundTag  string
}
type TrafficInfo struct {
	Up, Down       int64
	HasUp, HasDown bool
}

func NewManager(addr, tag string) (*Manager, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.New("invalid V2Ray API address")
	}
	return &Manager{conn: conn, client: command.NewHandlerServiceClient(conn), statsClient: statscmd.NewStatsServiceClient(conn), inBoundTag: tag}, nil
}
func (m *Manager) Close() error { return m.conn.Close() }
func (m *Manager) AddUser(ctx context.Context, u User) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	_, err := m.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag: m.inBoundTag,
		Operation: serial.ToTypedMessage(&command.AddUserOperation{User: &protocol.User{
			Level: u.GetLevel(), Email: u.GetEmail(),
			Account: serial.ToTypedMessage(&vmess.Account{
				Id: u.GetUUID(), AlterId: u.GetAlterID(),
				SecuritySettings: &protocol.SecurityConfig{Type: protocol.SecurityType_AUTO},
			}),
		}}),
	})
	if IsAlreadyExistsError(err) {
		return true, nil
	}
	if err != nil {
		return false, rpcError("add V2Ray user", err)
	}
	return false, nil
}
func (m *Manager) RemoveUser(ctx context.Context, u User) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	_, err := m.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag:       m.inBoundTag,
		Operation: serial.ToTypedMessage(&command.RemoveUserOperation{Email: u.GetEmail()}),
	})
	if err == nil {
		return nil
	}
	// Do not mistake a missing inbound handler for an already-removed user.
	message := status.Convert(err).Message()
	if strings.Contains(message, "User ") && strings.HasSuffix(message, " not found.") {
		return nil
	}
	return rpcError("remove V2Ray user", err)
}

// GetTraffic reads cumulative counters without resetting them. Persist deltas before advancing a cursor.
func (m *Manager) GetTraffic(ctx context.Context, u User) (TrafficInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var traffic TrafficInfo
	var errs []error
	for _, direction := range []string{uplinkDirection, downlinkDirection} {
		response, err := m.statsClient.GetStats(ctx, &statscmd.GetStatsRequest{Name: "user>>>" + u.GetEmail() + ">>>traffic>>>" + direction})
		if IsNotFoundError(err) {
			continue
		}
		if err != nil {
			errs = append(errs, rpcError("read V2Ray "+direction, err))
			continue
		}
		if response == nil || response.Stat == nil || response.Stat.Value < 0 {
			errs = append(errs, errors.New("invalid V2Ray traffic counter"))
			continue
		}
		if direction == uplinkDirection {
			traffic.Up = response.Stat.Value
			traffic.HasUp = true
		} else {
			traffic.Down = response.Stat.Value
			traffic.HasDown = true
		}
	}
	return traffic, errors.Join(errs...)
}

// GetTrafficAndReset is retained for source compatibility. Deprecated: counters are no longer reset.
func (m *Manager) GetTrafficAndReset(ctx context.Context, u User) (TrafficInfo, error) {
	return m.GetTraffic(ctx, u)
}

type UserData struct {
	User        User
	TrafficInfo TrafficInfo
}

// GetUserList returns traffic identities, NOT an authorization roster. UUIDs cannot be inferred from emails.
func (m *Manager) GetUserList(ctx context.Context, reset bool) ([]UserData, error) {
	if reset {
		return nil, errors.New("destructive V2Ray counter resets are not supported")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	response, err := m.statsClient.QueryStats(ctx, &statscmd.QueryStatsRequest{Pattern: "user>>>"})
	if err != nil {
		return nil, rpcError("query V2Ray traffic", err)
	}
	if response == nil {
		return nil, errors.New("invalid V2Ray statistics response")
	}
	users := make(map[string]UserData)
	for _, counter := range response.Stat {
		if counter == nil {
			continue
		}
		parts := strings.Split(counter.Name, ">>>")
		if len(parts) != 4 || parts[0] != "user" || parts[1] == "" || parts[2] != "traffic" || (parts[3] != uplinkDirection && parts[3] != downlinkDirection) {
			continue
		}
		if counter.Value < 0 {
			return nil, errors.New("negative V2Ray traffic counter")
		}
		email := parts[1]
		entry := users[email]
		entry.User = NewUser(email, "", 0, 0)
		if parts[3] == uplinkDirection {
			entry.TrafficInfo.Up = counter.Value
			entry.TrafficInfo.HasUp = true
		} else {
			entry.TrafficInfo.Down = counter.Value
			entry.TrafficInfo.HasDown = true
		}
		users[email] = entry
	}
	return slices.Collect(maps.Values(users)), nil
}
func rpcError(operation string, err error) error {
	return fmt.Errorf("%s: %s", operation, status.Code(err))
}
