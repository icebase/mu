package trojan

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/p4gefau1t/trojan-go/api/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type deadlineClient struct {
	service.TrojanServerServiceClient
	calls           int
	missingDeadline bool
}

func (c *deadlineClient) inspect(ctx context.Context) error {
	c.calls++
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > requestTimeout+time.Second {
		c.missingDeadline = true
	}
	return status.Error(codes.Unavailable, "fixture-private-credential")
}
func (c *deadlineClient) ListUsers(ctx context.Context, _ *service.ListUsersRequest, _ ...grpc.CallOption) (service.TrojanServerService_ListUsersClient, error) {
	return nil, c.inspect(ctx)
}
func (c *deadlineClient) GetUsers(ctx context.Context, _ ...grpc.CallOption) (service.TrojanServerService_GetUsersClient, error) {
	return nil, c.inspect(ctx)
}
func (c *deadlineClient) SetUsers(ctx context.Context, _ ...grpc.CallOption) (service.TrojanServerService_SetUsersClient, error) {
	return nil, c.inspect(ctx)
}
func TestAllTrojanRequestsHaveDeadlinesAndSafeErrors(t *testing.T) {
	client := &deadlineClient{}
	manager := &Manager{client: client}
	_, listErr := manager.ListUsers(context.Background())
	_, getErr := manager.GetUser(context.Background(), "fixture-password")
	addErr := manager.AddUser(context.Background(), "fixture-password")
	removeErr := manager.RemoveUser(context.Background(), "", "fixture-hash")
	for _, err := range []error{listErr, getErr, addErr, removeErr} {
		if err == nil || strings.Contains(err.Error(), "fixture-private-credential") {
			t.Fatal("request failure was swallowed or leaked credential material")
		}
	}
	if client.calls != 4 || client.missingDeadline {
		t.Fatal("Trojan API request lacked a bounded deadline")
	}
}
