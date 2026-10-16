package trojan

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/p4gefau1t/trojan-go/api/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type reviewInterruptedListServer struct {
	service.UnimplementedTrojanServerServiceServer
}

func (s *reviewInterruptedListServer) ListUsers(_ *service.ListUsersRequest, stream service.TrojanServerService_ListUsersServer) error {
	if err := stream.Send(&service.ListUsersResponse{Status: &service.UserStatus{User: &service.User{Hash: "fixture-hash"}, TrafficTotal: &service.Traffic{}}}); err != nil {
		return err
	}
	return status.Error(codes.Unavailable, "injected interrupted snapshot")
}
func TestReviewTrojanInterruptedListIsNotSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service.RegisterTrojanServerServiceServer(server, &reviewInterruptedListServer{})
	t.Cleanup(server.Stop)
	go func() { _ = server.Serve(listener) }()
	manager, err := NewManager(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	users, err := manager.ListUsers(ctx)
	if err == nil || len(users) != 0 || !strings.Contains(err.Error(), "Unavailable") {
		t.Errorf("interrupted snapshot was not rejected atomically: users=%d, error=%v", len(users), err)
	}
}
