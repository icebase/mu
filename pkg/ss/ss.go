// Package ss is the client for the ss-server management API.
package ss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const requestTimeout = 5 * time.Second

// TokenMetadataKey carries the shared management token on every call.
const TokenMetadataKey = "x-ss-token"

// Fingerprint lets both sides compare keys without exchanging them back.
// raw is the decoded uPSK, not its base64 text.
func Fingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:16])
}

// UserStatus is one ss-server user with cumulative counters since the server process started.
type UserStatus struct {
	Name           string
	PSKFingerprint string
	Upload         int64
	Download       int64
}

type Manager struct {
	conn   *grpc.ClientConn
	client pb.SSManagerServiceClient
}

func NewManager(addr, token string) (*Manager, error) {
	if token == "" {
		return nil, errors.New("ss-server API token is required")
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(tokenInterceptor(token)),
	)
	if err != nil {
		return nil, errors.New("invalid ss-server API address")
	}
	return &Manager{conn: conn, client: pb.NewSSManagerServiceClient(conn)}, nil
}

func tokenInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(metadata.AppendToOutgoingContext(ctx, TokenMetadataKey, token), method, req, reply, cc, opts...)
	}
}

func (m *Manager) Close() error { return m.conn.Close() }

func (m *Manager) ListUsers(ctx context.Context) ([]UserStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	resp, err := m.client.ListUsers(ctx, &pb.SSListUsersRequest{})
	if err != nil {
		return nil, rpcError("list ss-server users", err)
	}
	out := make([]UserStatus, 0, len(resp.GetUsers()))
	for _, user := range resp.GetUsers() {
		if user == nil || user.Name == "" || user.Upload < 0 || user.Download < 0 {
			return nil, errors.New("invalid ss-server user snapshot")
		}
		out = append(out, UserStatus{Name: user.Name, PSKFingerprint: user.PskFingerprint, Upload: user.Upload, Download: user.Download})
	}
	return out, nil
}

// SetUser creates the user or replaces its key.
func (m *Manager) SetUser(ctx context.Context, name, psk string) error {
	if name == "" || psk == "" {
		return errors.New("ss-server user identity is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if _, err := m.client.SetUser(ctx, &pb.SSSetUserRequest{Name: name, Psk: psk}); err != nil {
		return rpcError("set ss-server user", err)
	}
	return nil
}

// RemoveUser is idempotent and also terminates the user's active sessions.
func (m *Manager) RemoveUser(ctx context.Context, name string) error {
	if name == "" {
		return errors.New("ss-server user identity is empty")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	if _, err := m.client.RemoveUser(ctx, &pb.SSRemoveUserRequest{Name: name}); err != nil {
		return rpcError("remove ss-server user", err)
	}
	return nil
}

// rpcError reports only the status code; messages could echo credentials.
func rpcError(operation string, err error) error {
	return fmt.Errorf("%s: %s", operation, status.Code(err))
}
