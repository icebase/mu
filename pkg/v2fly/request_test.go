package v2fly

import (
	"context"
	"strings"
	"testing"
	"time"

	handlercmd "github.com/v2fly/v2ray-core/v5/app/proxyman/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *reviewStatsClient) inspectRequest(ctx context.Context, reset bool) {
	if reset {
		s.resetRequests++
	}
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > requestTimeout+time.Second {
		s.missingDeadline = true
	}
}
func TestStatisticsRequestsAreBoundedAndNonDestructive(t *testing.T) {
	client := &reviewStatsClient{up: 100, down: 200}
	manager := &Manager{statsClient: client}
	if _, err := manager.GetUserList(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.GetTraffic(context.Background(), NewUser("alice@example.test", "fixture", 0, 0)); err != nil {
		t.Fatal(err)
	}
	if client.missingDeadline || client.resetRequests != 0 || client.up != 100 || client.down != 200 {
		t.Fatal("request lacked a deadline or reset counters")
	}
}

type failingHandlerClient struct {
	handlercmd.HandlerServiceClient
	err error
}

func (c *failingHandlerClient) AlterInbound(context.Context, *handlercmd.AlterInboundRequest, ...grpc.CallOption) (*handlercmd.AlterInboundResponse, error) {
	return nil, c.err
}
func TestRemoveDistinguishesMissingInboundAndMissingUser(t *testing.T) {
	client := &failingHandlerClient{err: status.Error(codes.Unknown, "failed to get handler: proxy > handler not found.")}
	manager := &Manager{client: client}
	user := NewUser("alice@example.test", "fixture", 0, 0)
	if err := manager.RemoveUser(context.Background(), user); err == nil {
		t.Fatal("missing inbound handler was silently accepted")
	}
	client.err = status.Error(codes.NotFound, "handler not found.")
	if err := manager.RemoveUser(context.Background(), user); err == nil {
		t.Fatal("ambiguous NotFound status hid a missing inbound")
	}
	client.err = status.Error(codes.Unknown, "proxy/vmess/inbound: User alice@example.test not found.")
	if err := manager.RemoveUser(context.Background(), user); err != nil {
		t.Fatalf("already removed user was not idempotent: %v", err)
	}
	client.err = status.Error(codes.PermissionDenied, "fixture-credential")
	if err := manager.RemoveUser(context.Background(), user); err == nil || strings.Contains(err.Error(), "fixture-credential") {
		t.Fatal("remote error was swallowed or leaked authentication material")
	}
}
