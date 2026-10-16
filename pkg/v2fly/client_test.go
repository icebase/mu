package v2fly

import (
	"context"
	"strings"
	"testing"

	statscmd "github.com/v2fly/v2ray-core/v5/app/stats/command"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type reviewStatsClient struct {
	statscmd.StatsServiceClient
	counters        []*statscmd.Stat
	up              int64
	down            int64
	failDown        bool
	resetRequests   int
	missingDeadline bool
}

func (s *reviewStatsClient) QueryStats(ctx context.Context, req *statscmd.QueryStatsRequest, _ ...grpc.CallOption) (*statscmd.QueryStatsResponse, error) {
	s.inspectRequest(ctx, req.Reset_)
	return &statscmd.QueryStatsResponse{Stat: s.counters}, nil
}
func (s *reviewStatsClient) GetStats(ctx context.Context, req *statscmd.GetStatsRequest, _ ...grpc.CallOption) (*statscmd.GetStatsResponse, error) {
	s.inspectRequest(ctx, req.Reset_)
	value := s.up
	if strings.HasSuffix(req.Name, "downlink") {
		if s.failDown {
			return nil, status.Error(codes.Unavailable, "injected downlink failure")
		}
		value = s.down
		if req.Reset_ {
			s.down = 0
		}
	} else if req.Reset_ {
		s.up = 0
	}
	return &statscmd.GetStatsResponse{Stat: &statscmd.Stat{Name: req.Name, Value: value}}, nil
}
func TestReviewPartialTrafficReadFailure(t *testing.T) {
	stats := &reviewStatsClient{up: 100, down: 200, failDown: true}
	manager := &Manager{statsClient: stats}
	traffic, err := manager.GetTrafficAndReset(context.Background(), NewUser("test@example.test", "test", 0, 0))
	if err == nil {
		t.Error("downlink RPC failed but error was swallowed")
	}
	if traffic.Up != 100 || stats.up != 100 || !traffic.HasUp {
		t.Errorf("successful uplink read was reset and lost: returned=%d, counter=%d", traffic.Up, stats.up)
	}
}
func TestReviewStatsAreFilteredBeforeBecomingUsers(t *testing.T) {
	manager := &Manager{statsClient: &reviewStatsClient{counters: []*statscmd.Stat{
		{Name: "inbound>>>proxy>>>traffic>>>uplink", Value: 100},
		{Name: "outbound>>>direct>>>traffic>>>downlink", Value: 200},
		{Name: "user>>>uuid@example.test>>>traffic>>>uplink", Value: 300},
	}}}
	users, err := manager.GetUserList(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].User.GetEmail() != "uuid@example.test" {
		t.Errorf("non-user counters treated as users: got %d users", len(users))
	}
}
