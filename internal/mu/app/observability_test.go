package app

import (
	"context"
	"testing"

	"github.com/icebase/mu/internal/mu/app/metrics"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/common"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"go.opentelemetry.io/otel/attribute"
	codes "go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func metricValue(t *testing.T, collector prometheus.Collector) float64 {
	t.Helper()
	metric, ok := collector.(prometheus.Metric)
	if !ok {
		t.Fatalf("%T is not a single metric", collector)
	}
	out := &dto.Metric{}
	if err := metric.Write(out); err != nil {
		t.Fatal(err)
	}
	if out.Counter != nil {
		return out.Counter.GetValue()
	}
	return out.Gauge.GetValue()
}

func spanAttrs(span sdktrace.ReadOnlySpan) map[attribute.Key]attribute.Value {
	attrs := map[attribute.Key]attribute.Value{}
	for _, kv := range span.Attributes() {
		attrs[kv.Key] = kv.Value
	}
	return attrs
}

func TestSyncTrafficEmitsSpansAndOutboxMetrics(t *testing.T) {
	syncer, auth := realTrojanForReview(t, "active")
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	if _, err := syncer.GetTraffic(context.Background(), users); err != nil {
		t.Fatal(err)
	}
	_, meter := auth.AuthUser(common.SHA224String("active"))
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	app := reviewApp(t, &reviewMUService{users: users, failUpload: true}, syncer)
	app.tracer = provider.Tracer("test")

	meter.AddTraffic(100, 200)
	if err := app.syncTraffic(); err == nil {
		t.Fatal("expected upload failure")
	}
	if got := metricValue(t, metrics.OutboxBatches); got != 1 {
		t.Errorf("outbox batches after failed upload = %v, want 1", got)
	}
	if got := metricValue(t, metrics.OutboxLogs); got != 1 {
		t.Errorf("outbox logs after failed upload = %v, want 1", got)
	}
	failed := recorder.Ended()
	byName := map[string]sdktrace.ReadOnlySpan{}
	for _, span := range failed {
		byName[span.Name()] = span
	}
	if span := byName["App.flushTraffic"]; span == nil || span.Status().Code != codes.Error {
		t.Errorf("failed flush span missing or not marked as error: %v", span)
	}
	if span := byName["App.syncTraffic"]; span == nil || span.Status().Code != codes.Error ||
		spanAttrs(span)["traffic.outbox_batches"].AsInt64() != 1 {
		t.Errorf("syncTraffic span missing error status/outbox attributes: %v", span)
	}

	if err := app.syncTraffic(); err != nil {
		t.Fatal(err)
	}
	if got := metricValue(t, metrics.OutboxBatches); got != 0 {
		t.Errorf("outbox batches after successful upload = %v, want 0", got)
	}
	if got := metricValue(t, metrics.JobLastSuccess.WithLabelValues(metrics.JobSyncTraffic)); got <= 0 {
		t.Error("job success timestamp was not recorded")
	}
	var uploaded sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == "App.flushTraffic" && span.Status().Code != codes.Error && spanAttrs(span)["traffic.logs_uploaded"].AsInt64() == 1 {
			uploaded = span
		}
	}
	if uploaded == nil {
		t.Error("no successful flush span with uploaded log count")
	}
}

func TestSyncUserRecordsBackendOperationsAndSpanAttributes(t *testing.T) {
	syncer, _ := realTrojanForReview(t, "removed")
	users := []*pb.User{{Id: 1, Enable: 1, V2RayUser: &pb.VUser{Uuid: "active"}}}
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	app := reviewApp(t, &reviewMUService{users: users}, syncer)
	app.tracer = provider.Tracer("test")

	adds := metrics.BackendUserOperations.WithLabelValues(protocolTrojan, operationAdd, syncSuccess)
	removes := metrics.BackendUserOperations.WithLabelValues(protocolTrojan, operationRemove, syncSuccess)
	addsBefore, removesBefore := metricValue(t, adds), metricValue(t, removes)
	if err := app.syncUser(); err != nil {
		t.Fatal(err)
	}
	if metricValue(t, adds) != addsBefore+1 || metricValue(t, removes) != removesBefore+1 {
		t.Error("backend add/remove operations were not counted")
	}
	var found bool
	for _, span := range recorder.Ended() {
		if span.Name() == "App.syncUser.trojan" {
			found = true
			attrs := spanAttrs(span)
			if attrs["sync.users_desired"].AsInt64() != 1 || attrs["sync.users_removed"].AsInt64() != 1 || attrs["sync.users_applied"].AsInt64() != 1 {
				t.Errorf("unexpected backend span attributes: %v", attrs)
			}
		}
	}
	if !found {
		t.Error("backend sync span was not recorded")
	}
	if got := metricValue(t, metrics.JobLastSuccess.WithLabelValues(metrics.JobSyncUser)); got <= 0 {
		t.Error("sync_user success timestamp was not recorded")
	}
}
