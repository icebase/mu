package trojan

import (
	"context"
	"io"

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
