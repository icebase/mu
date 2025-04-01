package trojan

import (
	"context"
	"io"
	"log/slog"
	"time"

	"github.com/p4gefau1t/trojan-go/api/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Manager struct {
	client        service.TrojanServerServiceClient
	userStatusMap map[int64]*service.UserStatus
}

func NewManager(addr string) (*Manager, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	client := service.NewTrojanServerServiceClient(conn)
	return &Manager{
		client:        client,
		userStatusMap: make(map[int64]*service.UserStatus),
	}, nil
}

func (t *Manager) ListUsers(ctx context.Context) ([]*service.UserStatus, error) {
	stream, err := t.client.ListUsers(ctx, &service.ListUsersRequest{})
	if err != nil {
		return nil, err
	}
	var out = make([]*service.UserStatus, 0)
	for {
		reply, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out, nil
		}
		out = append(out, reply.Status)
	}
	return out, nil
}

func (t *Manager) GetUser(ctx context.Context, password string) (*service.GetUsersResponse, error) {
	logger := slog.Default()
	var err error

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	stream, err := t.client.GetUsers(ctx)
	if err != nil {
		return nil, err
	}

	err = stream.Send(&service.GetUsersRequest{
		User: &service.User{
			Password: password,
		},
	})

	if err != nil {
		logger.Error("[trojan] get user fail ",
			"error", err,
		)
		return nil, err
	}

	resp, err := stream.Recv()
	if err != nil {
		logger.Error("[trojan] get user fail ",
			"error", err,
		)
		return nil, err
	}
	return resp, nil
}

func (t *Manager) RemoveUser(ctx context.Context, password, hash string) error {
	logger := slog.Default()
	var err error

	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	stream, err := t.client.SetUsers(ctx)
	if err != nil {
		return err
	}

	err = stream.Send(&service.SetUsersRequest{
		Operation: service.SetUsersRequest_Delete,
		Status: &service.UserStatus{
			User: &service.User{
				Password: password,
			},
		},
	})

	if err != nil {
		logger.Error("[trojan] remove user fail ",
			"error", err,
		)
		return err
	}

	resp, err := stream.Recv()
	if err != nil {
		logger.Error("[trojan] remove user fail ",
			"error", err,
		)
		return err
	}
	logger.Info("[trojan] remove user success ",
		"resp", resp,
	)
	return nil
}

func (t *Manager) AddUser(ctx context.Context, password string) error {
	logger := slog.Default()
	var err error
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()

	stream, err := t.client.SetUsers(ctx)
	if err != nil {
		return err
	}

	err = stream.Send(&service.SetUsersRequest{
		Operation: service.SetUsersRequest_Add,
		Status: &service.UserStatus{
			User: &service.User{
				Password: password,
			},
		},
	})

	if err != nil {
		logger.Error("[trojan] add user fail ",
			"error", err,
		)
		return err
	}

	resp, err := stream.Recv()
	if err != nil {
		logger.Error("[trojan] add user fail ",
			"error", err,
		)
		return err
	}
	logger.Info("[trojan] add user success ",
		"resp", resp,
	)
	return nil
}
