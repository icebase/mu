package mu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

type observed struct {
	rpc     string
	code    string
	err     error
	elapsed time.Duration
}

func TestClientEmitsSpanPropagatesTraceAndNotifiesObserver(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator())
	})

	var mu sync.Mutex
	var traceparents []string
	service := &clientTestService{}
	handler := pb.NewMUServiceServer(service, twirp.WithServerPathPrefix(""))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		traceparents = append(traceparents, r.Header.Get("traceparent"))
		mu.Unlock()
		w.Header().Set("X-Request-ID", "req-1")
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)

	var calls []observed
	client := NewClient(server.URL, "fixture-token", "node", WithRequestObserver(func(rpc string, elapsed time.Duration, code string, err error) {
		calls = append(calls, observed{rpc, code, err, elapsed})
	}))

	if _, err := client.Ping(context.Background(), &pb.PingRequest{}); err != nil {
		t.Fatal(err)
	}
	// The fixture fails the first upload with a Twirp "internal" error.
	_, uploadErr := client.UploadTrafficLog(context.Background(), &pb.UploadTrafficLogRequest{})
	if uploadErr == nil {
		t.Fatal("expected upload failure")
	}

	if len(calls) != 2 || calls[0].rpc != "Ping" || calls[0].err != nil || calls[0].code != "" || calls[0].elapsed <= 0 {
		t.Fatalf("unexpected success observation: %+v", calls)
	}
	if calls[1].rpc != "UploadTrafficLog" || calls[1].code != string(twirp.Internal) || calls[1].err == nil ||
		strings.Contains(calls[1].err.Error(), "fixture-credential") {
		t.Fatalf("unexpected failure observation (must be sanitized): %+v", calls[1])
	}

	spans := recorder.Ended()
	if len(spans) != 2 {
		t.Fatalf("expected 2 client spans, got %d", len(spans))
	}
	for _, span := range spans {
		if !strings.HasPrefix(span.Name(), "MU.") {
			t.Errorf("unexpected span name %q", span.Name())
		}
	}
	failed := spans[1]
	if failed.Status().Code.String() != "Error" || strings.Contains(failed.Status().Description, "fixture-credential") {
		t.Errorf("failed span status = %+v", failed.Status())
	}
	attrs := map[attribute.Key]attribute.Value{}
	for _, kv := range failed.Attributes() {
		attrs[kv.Key] = kv.Value
	}
	if attrs["rpc.method"].AsString() != "UploadTrafficLog" || attrs["mu.node_id"].AsString() != "node" ||
		attrs["http.response.status_code"].AsInt64() != http.StatusInternalServerError ||
		attrs["rpc.twirp.error_code"].AsString() != "internal" || attrs["mu.request_id"].AsString() != "req-1" {
		t.Errorf("missing span attributes: %v", attrs)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(traceparents) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(traceparents))
	}
	for i, header := range traceparents {
		want := "00-" + spans[i].SpanContext().TraceID().String() + "-" + spans[i].SpanContext().SpanID().String() + "-01"
		if header != want {
			t.Errorf("request %d traceparent = %q, want %q", i, header, want)
		}
	}
}
