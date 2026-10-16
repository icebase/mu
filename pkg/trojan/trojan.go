package trojan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/p4gefau1t/trojan-go/api/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const requestTimeout = 5 * time.Second

var ErrUserNotFound = errors.New("trojan user not found")

type Manager struct {
	conn   *grpc.ClientConn
	client service.TrojanServerServiceClient
}

func NewManager(addr string) (*Manager, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, errors.New("invalid Trojan API address")
	}
	return &Manager{conn: conn, client: service.NewTrojanServerServiceClient(conn)}, nil
}

func (t *Manager) Close() error { return t.conn.Close() }

// ListUsers returns a complete snapshot. Trojan-Go exposes hashes, never passwords.
func (t *Manager) ListUsers(ctx context.Context) ([]*service.UserStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	stream, err := t.client.ListUsers(ctx, &service.ListUsersRequest{})
	if err != nil {
		return nil, rpcError("list Trojan users", err)
	}
	var out []*service.UserStatus
	for {
		reply, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, rpcError("receive Trojan users", err)
		}
		if reply.Status == nil || reply.Status.User == nil || reply.Status.User.Hash == "" || reply.Status.TrafficTotal == nil {
			return nil, errors.New("invalid Trojan user snapshot")
		}
		out = append(out, reply.Status)
	}
}

func (t *Manager) GetUser(ctx context.Context, password string) (*service.GetUsersResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	stream, err := t.client.GetUsers(ctx)
	if err != nil {
		return nil, rpcError("get Trojan user", err)
	}
	if err := stream.Send(&service.GetUsersRequest{User: &service.User{Password: password}}); err != nil {
		return nil, rpcError("send Trojan user lookup", err)
	}
	if err := stream.CloseSend(); err != nil {
		return nil, rpcError("close Trojan user lookup", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, rpcError("receive Trojan user", err)
	}
	if !resp.Success {
		return nil, ErrUserNotFound
	}
	if resp.Status == nil || resp.Status.TrafficTotal == nil {
		return nil, errors.New("invalid Trojan traffic response")
	}
	return resp, nil
}

// RemoveUser accepts either a cleartext password or a hash. Prefer the hash returned by ListUsers.
func (t *Manager) RemoveUser(ctx context.Context, password, hash string) error {
	return t.setUser(ctx, service.SetUsersRequest_Delete, &service.User{Password: password, Hash: hash})
}
func (t *Manager) AddUser(ctx context.Context, password string) error {
	return t.setUser(ctx, service.SetUsersRequest_Add, &service.User{Password: password})
}
func (t *Manager) setUser(ctx context.Context, operation service.SetUsersRequest_Operation, user *service.User) error {
	if user.Password == "" && user.Hash == "" {
		return errors.New("trojan user identity is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	stream, err := t.client.SetUsers(ctx)
	if err != nil {
		return rpcError("set Trojan user", err)
	}
	if err := stream.Send(&service.SetUsersRequest{Operation: operation, Status: &service.UserStatus{User: user}}); err != nil {
		return rpcError("send Trojan user change", err)
	}
	if err := stream.CloseSend(); err != nil {
		return rpcError("close Trojan user change", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		return rpcError("receive Trojan user change", err)
	}
	// Info may contain passwords or authentication hashes; never return or log it.
	if !resp.Success {
		return errors.New("trojan API rejected user change")
	}
	return nil
}
func rpcError(operation string, err error) error {
	return fmt.Errorf("%s: %s", operation, status.Code(err))
}
