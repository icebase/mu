package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/icebase/mu"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
	"google.golang.org/protobuf/proto"
)

type nodeMetricsReceiver struct {
	attempts atomic.Int32
	reports  chan *pb.ReportNodeMetricsRequest
}

func (r *nodeMetricsReceiver) ReportNodeMetrics(_ context.Context, req *pb.ReportNodeMetricsRequest) (*pb.ReportNodeMetricsResponse, error) {
	r.reports <- req
	if r.attempts.Add(1) == 1 {
		return nil, twirp.NewError(twirp.Unavailable, "test retry")
	}
	return &pb.ReportNodeMetricsResponse{}, nil
}

func TestReportNodeMetricsWithoutPrometheus(t *testing.T) {
	healthy, _, stats := reviewRealVMess(t)
	failing, _, _ := reviewRealVMess(t, true)
	counter, err := stats.RegisterCounter("inbound>>>proxy>>>traffic>>>uplink")
	if err != nil {
		t.Fatal(err)
	}
	counter.Set(123)
	if _, err := stats.RegisterCounter("user>>>private@example.test>>>traffic>>>uplink"); err != nil {
		t.Fatal(err)
	}
	receiver := &nodeMetricsReceiver{reports: make(chan *pb.ReportNodeMetricsRequest, 2)}
	handler := pb.NewNodeMetricsServiceServer(receiver, twirp.WithServerPathPrefix(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(mu.TokenHeader) != "fixture-token" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()

	address := func(s *v2flySync) string { return strings.TrimSuffix(strings.TrimPrefix(s.key, "v2fly:"), ":"+tag) }
	t.Setenv("MU_NODE_METRICS_REPORTING_ENABLED", "true")
	t.Setenv("MU_METRICS_ENABLED", "false")
	cfg := ConfigFromEnv()
	cfg.MuNodeID = "metrics-test"
	cfg.MuAPIBaseURL = server.URL
	cfg.MuAPIToken = "fixture-token"
	cfg.StateFile = filepath.Join(t.TempDir(), "state.json")
	cfg.V2flyAddrs = []string{address(healthy)}
	cfg.VlessAddrs = []string{address(healthy), address(failing)}
	a := New(cfg)
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	if !cfg.NodeMetricsReportingEnabled || a.metricsServer != nil {
		t.Fatal("expected reporting enabled and the scrape server disabled")
	}
	if err := a.reportNodeMetricsOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	first, retry := <-receiver.reports, <-receiver.reports
	if first.NodeId != cfg.MuNodeID || first.BatchId == "" || !proto.Equal(first, retry) {
		t.Fatal("report did not preserve node identity and the same payload across retries")
	}
	if len(first.Backends) != 2 || counter.Value() != 123 {
		t.Fatal("backends were not deduplicated or the counter changed")
	}
	for _, sample := range first.Backends {
		if sample.SampledAtUnixMillis <= 0 {
			t.Fatal("missing sample timestamp")
		}
		switch sample.Backend {
		case address(healthy):
			expected := &pb.V2FlyTrafficCounter{Dimension: "inbound", Tag: "proxy", Direction: "uplink", Bytes: 123}
			if !sample.Up || sample.Runtime == nil || len(sample.Traffic) != 1 || !proto.Equal(sample.Traffic[0], expected) {
				t.Fatalf("unexpected healthy backend sample: %v", sample)
			}
		case address(failing):
			if sample.Up || sample.Runtime != nil || len(sample.Traffic) != 0 {
				t.Fatalf("unexpected failed backend sample: %v", sample)
			}
		default:
			t.Fatalf("unexpected backend: %s", sample.Backend)
		}
	}
}
