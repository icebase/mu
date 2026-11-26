package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/icebase/mu"
	"github.com/icebase/mu/internal/mu/app/metrics"
	"github.com/icebase/mu/internal/mu/app/tracing"
	"github.com/icebase/mu/pkg/v2fly"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const version = "v0.0.1"
const backendTimeout = 30 * time.Second
const syncSuccess = "success"
const syncFailure = "failure"

type App struct {
	config         *Config
	muClient       *mu.Client
	userSync       []UserSync
	state          *syncState
	ctx            context.Context
	cancel         context.CancelFunc
	tracer         trace.Tracer
	tracerProvider *sdktrace.TracerProvider
	metricsServer  *http.Server
	v2flyBackends  map[string]*v2fly.Manager
}

func New(config *Config) *App { return &App{config: config} }
func (a *App) Init() (err error) {
	a.ctx, a.cancel = context.WithCancel(context.Background())
	defer func() {
		if err != nil {
			_ = a.Close()
		}
	}()
	if a.config.MuNodeID == "" {
		return errors.New("MU_NODE_ID is required for durable synchronization state")
	}
	path := a.config.StateFile
	if path == "" {
		path = "data/mu-state.json"
	}
	a.state, err = openSyncState(path, a.config.MuNodeID)
	if err != nil {
		return fmt.Errorf("open synchronization state: %w", err)
	}
	metrics.SetOutbox(a.state.pendingStats())
	if err := a.initTracing(); err != nil {
		return fmt.Errorf("init tracing: %w", err)
	}
	a.muClient = mu.NewClient(a.config.MuAPIBaseURL, a.config.MuAPIToken, a.config.MuNodeID,
		mu.WithAPIDebug(a.config.MuAPIDebug), mu.WithRequestObserver(metrics.ObserveAPIRequest))
	for _, addr := range a.config.TrojanAddrs {
		syncer, err := newTrojanSync(addr, a.state)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, syncer)
	}
	v2flyBackends := make(map[string]*v2fly.Manager)
	for _, addr := range a.config.V2flyAddrs {
		syncer, err := newV2flySync(addr, a.state)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, syncer)
		v2flyBackends[addr] = syncer.manager
	}
	for _, addr := range a.config.VlessAddrs {
		syncer, err := newVlessSync(addr, a.config.VlessInboundTag, a.state)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, syncer)
		v2flyBackends[addr] = syncer.manager
	}
	if len(a.config.SSAddrs) > 0 && a.config.SSAPIToken == "" {
		return errors.New("MU_SS_API_TOKEN is required when MU_SS_ADDRS is set")
	}
	for _, addr := range a.config.SSAddrs {
		syncer, err := newSSSync(addr, a.config.SSAPIToken, a.state)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, syncer)
	}
	a.v2flyBackends = v2flyBackends
	return a.initMetrics(v2flyBackends)
}
func (a *App) initTracing() error {
	cfg := a.config.Tracing
	cfg.ServiceVersion = version
	cfg.NodeID = a.config.MuNodeID
	provider, err := tracing.InitTracer(a.ctx, cfg)
	if err != nil {
		return err
	}
	a.tracerProvider = provider
	a.tracer = tracing.Tracer()
	if cfg.Enabled {
		slog.Info("tracing enabled", "exporter", cfg.Exporter, "sample_rate", cfg.SampleRate)
	}
	return nil
}
func (a *App) initMetrics(backends map[string]*v2fly.Manager) error {
	metrics.BuildInfo.WithLabelValues(version, runtime.Version()).Set(1)
	if !a.config.MetricsEnabled {
		return nil
	}
	registry := prometheus.NewRegistry()
	if a.config.V2FlyMetricsEnabled {
		for addr, manager := range backends {
			registerer := prometheus.WrapRegistererWith(prometheus.Labels{"backend": addr}, registry)
			if err := registerer.Register(metrics.NewV2FlyCollector(a.ctx, manager.ReadNodeStats)); err != nil {
				return fmt.Errorf("register V2Fly metrics: %w", err)
			}
		}
	}
	mux := http.NewServeMux()
	handler := promhttp.HandlerFor(prometheus.Gatherers{prometheus.DefaultGatherer, registry}, promhttp.HandlerOpts{})
	mux.Handle("/metrics", promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handler))
	a.metricsServer = &http.Server{Addr: ":" + a.config.MetricsPort, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := a.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server failed", "addr", a.metricsServer.Addr, "error", err)
		}
	}()
	return nil
}
func (a *App) Run() {
	ctx, stop := signal.NotifyContext(a.ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	a.ctx = ctx
	var reportingDone chan struct{}
	if a.config.NodeMetricsReportingEnabled && len(a.v2flyBackends) > 0 {
		reportingDone = make(chan struct{})
		go func() {
			defer close(reportingDone)
			a.reportNodeMetrics(ctx)
		}()
	}
	a.jobs()
	if reportingDone != nil {
		<-reportingDone
	}
	if err := a.Close(); err != nil {
		slog.Error("shutdown failed", "error", err)
	}
}
func (a *App) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	var errs []error
	for _, syncer := range a.userSync {
		if closer, ok := syncer.(io.Closer); ok {
			errs = append(errs, closer.Close())
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if a.metricsServer != nil {
		errs = append(errs, a.metricsServer.Shutdown(ctx))
	}
	if a.tracerProvider != nil {
		errs = append(errs, tracing.Shutdown(ctx, a.tracerProvider))
	}
	if a.state != nil {
		errs = append(errs, a.state.Close())
	}
	return errors.Join(errs...)
}
func (a *App) jobs() {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-timer.C:
			if err := a.ping(); err != nil {
				slog.Error("ping failed", "error", err)
			}
			if err := a.syncTraffic(); err != nil {
				slog.Error("traffic sync failed", "error", err)
			}
			if err := a.syncUser(); err != nil {
				slog.Error("user sync failed", "error", err)
			}
			timer.Reset(60 * time.Second)
		}
	}
}
func (a *App) ping() (resultErr error) {
	ctx, span := a.tracer.Start(a.ctx, "App.ping")
	defer span.End()
	defer func() { finishSpan(ctx, resultErr, "ping failed") }()
	timer := prometheus.NewTimer(metrics.PingDuration)
	defer timer.ObserveDuration()
	_, err := a.muClient.Ping(ctx, &pb.PingRequest{NodeId: a.config.MuNodeID, Version: version})
	result := syncSuccess
	if err != nil {
		result = syncFailure
		metrics.ErrorsTotal.WithLabelValues("ping", "api_error").Inc()
	}
	metrics.PingCounter.WithLabelValues(result).Inc()
	if err == nil {
		metrics.MarkJobSuccess(metrics.JobPing)
	}
	return err
}
func (a *App) fetchUsers(ctx context.Context) ([]*pb.User, error) {
	response, err := a.muClient.GetUsers(ctx, &pb.GetUsersRequest{})
	if err != nil {
		return nil, err
	}
	if response == nil {
		return nil, errors.New("empty MU users response")
	}
	if _, _, err := desiredUsers(response.Users, protocolTrojan); err != nil {
		return nil, err
	}
	return response.Users, nil
}
func (a *App) syncUser() (resultErr error) {
	ctx, span := a.tracer.Start(a.ctx, "App.syncUser")
	defer span.End()
	defer func() {
		finishSpan(ctx, resultErr, "user sync failed")
		if resultErr == nil {
			metrics.MarkJobSuccess(metrics.JobSyncUser)
		}
	}()
	users, err := a.fetchUsers(ctx)
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues("sync_user", "api_error").Inc()
		return err
	}
	span.SetAttributes(attribute.Int("mu.users", len(users)), attribute.Int("sync.backends", len(a.userSync)))
	metrics.UsersCount.WithLabelValues(a.config.MuNodeID).Set(float64(len(users)))
	var errs []error
	for index, syncer := range a.userSync {
		syncType := syncer.Name()
		child, childSpan := a.tracer.Start(ctx, "App.syncUser."+syncType, trace.WithAttributes(
			attribute.String("sync.type", syncType), attribute.Int("sync.backend_index", index),
		))
		child, cancel := context.WithTimeout(child, backendTimeout)
		timer := prometheus.NewTimer(metrics.UserSyncDuration.WithLabelValues(syncType))
		err := syncer.Sync(child, users)
		cancel()
		timer.ObserveDuration()
		result := syncSuccess
		if err != nil {
			result = syncFailure
			errs = append(errs, fmt.Errorf("%s backend %d: %w", syncType, index, err))
			metrics.ErrorsTotal.WithLabelValues("sync_user", syncType+"_error").Inc()
			tracing.RecordError(child, err, "backend user sync failed")
		}
		childSpan.SetAttributes(attribute.String("sync.result", result))
		metrics.UserSyncCounter.WithLabelValues(result, syncType).Inc()
		childSpan.End()
	}
	return errors.Join(errs...)
}
func (a *App) syncTraffic() (resultErr error) {
	if a.state == nil {
		return errors.New("synchronization state is not initialized")
	}
	ctx, span := a.tracer.Start(a.ctx, "App.syncTraffic")
	defer span.End()
	timer := prometheus.NewTimer(metrics.TrafficSyncDuration)
	defer timer.ObserveDuration()
	defer func() {
		finishSpan(ctx, resultErr, "traffic sync failed")
		metrics.TrafficSyncCounter.WithLabelValues(metrics.Result(resultErr)).Inc()
		a.publishOutbox(span)
		if resultErr == nil {
			metrics.MarkJobSuccess(metrics.JobSyncTraffic)
		}
	}()
	var errs []error
	metrics.TrafficLogsUploaded.Set(0)
	// Retry the outbox even if fetching the next MU snapshot fails.
	flushErr := a.flushTraffic(ctx)
	if flushErr != nil {
		errs = append(errs, flushErr)
	}
	users, err := a.fetchUsers(ctx)
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues("sync_traffic", "api_error").Inc()
		return errors.Join(append(errs, err)...)
	}
	collected := make(map[string]int)
	for index, syncer := range a.userSync {
		syncType := syncer.Name()
		child, childSpan := a.tracer.Start(ctx, "App.syncTraffic."+syncType,
			trace.WithAttributes(attribute.String("sync.type", syncType), attribute.Int("sync.backend_index", index)),
		)
		child, cancel := context.WithTimeout(child, backendTimeout)
		logs, err := syncer.GetTraffic(child, users)
		cancel()
		childSpan.SetAttributes(attribute.Int("traffic.logs_collected", len(logs)))
		collected[syncType] += len(logs)
		metrics.TrafficLogsCollected.WithLabelValues(syncType).Set(float64(collected[syncType]))
		if err != nil {
			errs = append(errs, fmt.Errorf("%s backend %d: %w", syncType, index, err))
			metrics.ErrorsTotal.WithLabelValues("sync_traffic", syncType+"_error").Inc()
			tracing.RecordError(child, err, "backend traffic collection failed")
		}
		childSpan.End()
	}
	// Do not retry the same failed upload twice within one cycle.
	if flushErr == nil {
		if err := a.flushTraffic(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (a *App) flushTraffic(ctx context.Context) (resultErr error) {
	ctx, span := a.tracer.Start(ctx, "App.flushTraffic")
	defer span.End()
	defer func() { finishSpan(ctx, resultErr, "traffic upload failed") }()
	ctx, cancel := context.WithTimeout(ctx, backendTimeout)
	defer cancel()
	uploadedBatches, uploadedLogs := 0, 0
	defer func() {
		span.SetAttributes(attribute.Int("traffic.batches_uploaded", uploadedBatches), attribute.Int("traffic.logs_uploaded", uploadedLogs))
	}()
	for {
		snapshot := a.state.snapshot()
		if len(snapshot.Pending) == 0 {
			return nil
		}
		batch := snapshot.Pending[0]
		_, err := a.muClient.UploadTrafficLog(ctx, &pb.UploadTrafficLogRequest{UploadAt: batch.UploadAt, Logs: batch.Logs})
		if err != nil {
			metrics.ErrorsTotal.WithLabelValues("sync_traffic", "upload_error").Inc()
			return err
		}
		if err := a.state.acknowledge(); err != nil {
			return err
		}
		uploadedBatches++
		uploadedLogs += len(batch.Logs)
		metrics.TrafficLogsUploaded.Add(float64(len(batch.Logs)))
		for _, log := range batch.Logs {
			kind := protocolTrojan
			switch log.TrafficType {
			case int32(pb.TrafficType_TRAFFIC_TYPE_V2FLY):
				kind = protocolV2Fly
			case int32(pb.TrafficType_TRAFFIC_TYPE_SHADOWSOCKS):
				kind = protocolSS
			}
			metrics.TotalUploadBytes.WithLabelValues(kind).Add(float64(log.U))
			metrics.TotalDownloadBytes.WithLabelValues(kind).Add(float64(log.D))
		}
	}
}

// finishSpan marks the span in ctx as failed when err is non-nil.
func finishSpan(ctx context.Context, err error, description string) {
	tracing.RecordError(ctx, err, description)
}

// publishOutbox exports the size of the durable upload queue as metrics and span attributes.
func (a *App) publishOutbox(span trace.Span) {
	batches, logs := a.state.pendingStats()
	metrics.SetOutbox(batches, logs)
	span.SetAttributes(attribute.Int("traffic.outbox_batches", batches), attribute.Int("traffic.outbox_logs", logs))
}
