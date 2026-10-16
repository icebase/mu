package mu

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
)

type clientTestService struct {
	mu      sync.Mutex
	uploads []int64
	nodes   []string
	slow    bool
}

func (s *clientTestService) Ping(context.Context, *pb.PingRequest) (*pb.PingResponse, error) {
	return &pb.PingResponse{}, nil
}
func (s *clientTestService) GetUsers(ctx context.Context, _ *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	if s.slow {
		<-ctx.Done()
		return nil, twirp.NewError(twirp.Canceled, "cancelled")
	}
	return &pb.GetUsersResponse{}, nil
}
func (s *clientTestService) UploadTrafficLog(_ context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploads = append(s.uploads, req.UploadAt)
	s.nodes = append(s.nodes, req.NodeId)
	if len(s.uploads) == 1 {
		return nil, twirp.InternalError("fixture-credential")
	}
	return &pb.UploadTrafficLogResponse{}, nil
}
func newTestClient(t *testing.T, service *clientTestService) *Client {
	t.Helper()
	server := httptest.NewServer(pb.NewMUServiceServer(service, twirp.WithServerPathPrefix("")))
	t.Cleanup(server.Close)
	return NewClient(server.URL, "fixture-token", "node")
}
func TestUploadPreservesTimestampAcrossRetriesAndRedactsErrors(t *testing.T) {
	service := &clientTestService{}
	client := newTestClient(t, service)
	request := &pb.UploadTrafficLogRequest{UploadAt: 123}
	if _, err := client.UploadTrafficLog(context.Background(), request); err == nil || strings.Contains(err.Error(), "fixture-credential") {
		t.Fatal("remote failure was ignored or leaked private response text")
	}
	if _, err := client.UploadTrafficLog(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.uploads) != 2 || service.uploads[0] != 123 || service.uploads[1] != 123 || service.nodes[0] != "node" {
		t.Fatal("retry changed the batch timestamp or node identity")
	}
}
func TestClientHonorsCallerDeadline(t *testing.T) {
	client := newTestClient(t, &clientTestService{slow: true})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := client.GetUsers(ctx, &pb.GetUsersRequest{}); err == nil {
		t.Fatal("hanging request ignored caller cancellation")
	}
	if time.Since(start) > time.Second {
		t.Fatal("request exceeded its deadline budget")
	}
}
