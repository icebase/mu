package v2fly

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"github.com/v2fly/v2ray-core/v5/common/protocol"
	"github.com/v2fly/v2ray-core/v5/common/serial"
	"github.com/v2fly/v2ray-core/v5/proxy/vmess"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Manager struct {
	client      command.HandlerServiceClient
	statsClient statscmd.StatsServiceClient

	inBoundTag string
}

const (
	UplinkFormat   = "user>>>%s>>>traffic>>>uplink"
	DownlinkFormat = "user>>>%s>>>traffic>>>downlink"
)

type TrafficInfo struct {
	Up, Down int64
}

func NewManager(addr, tag string) (*Manager, error) {
	slog.Info("create new mgr", "addr", addr)
	cc, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		slog.Error("grpc dail failed", "addr", addr)
		return nil, err
	}
	client := command.NewHandlerServiceClient(cc)
	statsClient := statscmd.NewStatsServiceClient(cc)
	m := &Manager{
		client:      client,
		statsClient: statsClient,
		inBoundTag:  tag,
	}

	return m, nil
}

// return is exist,and error
func (m *Manager) AddUser(ctx context.Context, u User) (bool, error) {
	logger := slog.Default()
	resp, err := m.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag: m.inBoundTag,
		Operation: serial.ToTypedMessage(&command.AddUserOperation{
			User: &protocol.User{
				Level: u.GetLevel(),
				Email: u.GetEmail(),
				Account: serial.ToTypedMessage(&vmess.Account{
					Id:               u.GetUUID(),
					AlterId:          u.GetAlterID(),
					SecuritySettings: &protocol.SecurityConfig{Type: protocol.SecurityType_AUTO},
				}),
			},
		}),
	})
	if err != nil && !IsAlreadyExistsError(err) {
		logger.Error("failed to call add user",
			"resp", resp,
			"error", err,
		)
		return false, err
	}
	return IsAlreadyExistsError(err), nil
}

func (m *Manager) RemoveUser(ctx context.Context, u User) error {
	logger := slog.Default()
	resp, err := m.client.AlterInbound(ctx, &command.AlterInboundRequest{
		Tag: m.inBoundTag,
		Operation: serial.ToTypedMessage(&command.RemoveUserOperation{
			Email: u.GetEmail(),
		}),
	})
	if err != nil {
		logger.Error("failed to call remove user : ", "error", err)
		return TODOErr
	}
	logger.Debug("call remove user resp: ", "resp", resp)

	return nil
}

func (m *Manager) GetTrafficAndReset(ctx context.Context, u User) (TrafficInfo, error) {
	logger := slog.Default()
	ti := TrafficInfo{}
	up, err := m.statsClient.GetStats(ctx, &statscmd.GetStatsRequest{
		Name:   fmt.Sprintf(UplinkFormat, u.GetEmail()),
		Reset_: true,
	})
	if err != nil && !IsNotFoundError(err) {
		logger.Error("get traffic user ", "u", u, "error", err)
		return ti, err
	}

	down, err := m.statsClient.GetStats(ctx, &statscmd.GetStatsRequest{
		Name:   fmt.Sprintf(DownlinkFormat, u.GetEmail()),
		Reset_: true,
	})
	if err != nil && !IsNotFoundError(err) {
		logger.Error("get traffic user fail",
			"user", u,
			"error", err)
		return ti, nil
	}

	if up != nil {
		ti.Up = up.Stat.Value
	}
	if down != nil {
		ti.Down = down.Stat.Value
	}
	return ti, nil
}

type UserData struct {
	User        User
	TrafficInfo TrafficInfo
}

func (m *Manager) GetUserList(ctx context.Context, reset bool) ([]UserData, error) {
	logger := slog.Default().With("method", "v2fly_get_user_list")
	resp, err := m.statsClient.QueryStats(ctx, &statscmd.QueryStatsRequest{
		Reset_: reset,
	})
	if err != nil {
		logger.Error("failed to call query stats", "error", err)
		return nil, err
	}
	logger.Info("query stats resp", "resp.stat.len", len(resp.Stat))

	var users = make(map[string]UserData)

	for _, v := range resp.Stat {

		email := getEmailFromStatName(v.GetName())
		uuid := getUUDIFromEmail(email)

		if _, ok := users[uuid]; !ok {
			users[uuid] = UserData{
				TrafficInfo: TrafficInfo{},
			}
		}

		u := user{
			email: email,
			uuid:  uuid,
		}
		ti := users[uuid].TrafficInfo

		if strings.Contains(v.GetName(), "downlink") {
			ti.Down = v.Value
		} else {
			ti.Up = v.Value
		}

		users[uuid] = UserData{
			User:        u,
			TrafficInfo: ti,
		}

	}

	var data = make([]UserData, 0, len(users))
	for _, v := range users {
		data = append(data, v)
	}

	return data, nil
}

func getEmailFromStatName(s string) string {
	arr := strings.Split(s, ">>>")
	if len(arr) > 1 {
		return arr[1]
	}
	return s
}

func getUUDIFromEmail(s string) string {
	arr := strings.Split(s, "@")
	if len(arr) > 0 {
		return arr[0]
	}
	return s
}
