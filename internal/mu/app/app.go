package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/icebase/mu"
	"github.com/icebase/mu/internal/mu/app/metrics"
	"github.com/icebase/mu/internal/mu/app/tracing"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	version = "v0.0.1"
)

type App struct {
	config *Config

	muClient *mu.Client
	userSync []UserSync

	ctx    context.Context
	cancel context.CancelFunc

	// Observability
	tracer         trace.Tracer
	tracerProvider *sdktrace.TracerProvider
	metricsServer  *http.Server
}

func New(config *Config) *App {
	return &App{
		config: config,
	}
}

func (a *App) Init() error {
	a.ctx, a.cancel = context.WithCancel(context.Background())

	// 初始化 tracing
	if err := a.initTracing(); err != nil {
		return fmt.Errorf("failed to init tracing: %w", err)
	}

	// 初始化 metrics server
	if err := a.initMetrics(); err != nil {
		return fmt.Errorf("failed to init metrics: %w", err)
	}

	a.muClient = mu.NewClient(a.config.MuApiBaseURL, a.config.MuApiToken, a.config.MuNodeID)

	for _, addr := range a.config.TrojanAddrs {
		s, err := newTrojanSync(addr)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, s)
	}
	for _, addr := range a.config.V2flyAddrs {
		s, err := newV2flySync(addr)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, s)
	}
	return nil
}

// initTracing 初始化 OpenTelemetry tracing
func (a *App) initTracing() error {
	tp, err := tracing.InitTracer(a.ctx, a.config.Tracing)
	if err != nil {
		return err
	}
	a.tracerProvider = tp
	a.tracer = otel.Tracer("mu-app")
	return nil
}

// initMetrics 初始化 Prometheus metrics server
func (a *App) initMetrics() error {
	if !a.config.MetricsEnabled {
		return nil
	}

	// 注册 runtime metrics
	prometheus.MustRegister(prometheus.NewBuildInfoCollector())
	prometheus.MustRegister(prometheus.NewGoCollector())

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())

	a.metricsServer = &http.Server{
		Addr:    ":" + a.config.MetricsPort,
		Handler: mux,
	}

	go func() {
		slog.Info("starting metrics server", "port", a.config.MetricsPort)
		if err := a.metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics server error", "err", err)
		}
	}()

	return nil
}

func (a *App) Run() {
	go a.jobs()
	sigs := make(chan os.Signal, 1)
	sig := <-sigs
	slog.Info("recv signale", "signal", sig)

	// Cleanup
	a.cancel()

	// 关闭 tracing
	if a.tracerProvider != nil {
		if err := tracing.Shutdown(context.Background(), a.tracerProvider); err != nil {
			slog.Error("failed to shutdown tracer", "err", err)
		}
	}

	// 关闭 metrics server
	if a.metricsServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.metricsServer.Shutdown(ctx); err != nil {
			slog.Error("failed to shutdown metrics server", "err", err)
		}
	}

	os.Exit(0)
}

func (a *App) jobs() {
	syncTimer := time.NewTimer(60 * time.Second)

	for {
		select {
		case <-syncTimer.C:
			if err := a.ping(); err != nil {
				slog.Error("ping failed", "err", err)
			}
			if err := a.syncTraffic(); err != nil {
				slog.Error("sync traffic failed", "err", err)
			}
			if err := a.syncUser(); err != nil {
				slog.Error("sync user failed", "err", err)
			}
			syncTimer.Reset(60 * time.Second)
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) ping() error {
	// 创建 span
	ctx, span := a.tracer.Start(a.ctx, "App.ping",
		trace.WithAttributes(
			attribute.String("node_id", a.config.MuNodeID),
			attribute.String("version", version),
		),
	)
	defer span.End()

	// 记录 metrics
	timer := prometheus.NewTimer(metrics.PingDuration)
	defer timer.ObserveDuration()

	_, err := a.muClient.Ping(ctx, &pb.PingRequest{
		Version: version,
		NodeId:  a.config.MuNodeID,
	})

	if err != nil {
		metrics.PingCounter.WithLabelValues("failure").Inc()
		metrics.ErrorsTotal.WithLabelValues("ping", "api_error").Inc()
		tracing.RecordError(ctx, err, "ping failed")
		return err
	}

	metrics.PingCounter.WithLabelValues("success").Inc()
	return nil
}

func (a *App) syncUser() error {
	// 创建 span
	ctx, span := a.tracer.Start(a.ctx, "App.syncUser")
	defer span.End()

	resp, err := a.muClient.GetUsers(ctx, &pb.GetUsersRequest{})
	if err != nil {
		metrics.ErrorsTotal.WithLabelValues("sync_user", "api_error").Inc()
		tracing.RecordError(ctx, err, "failed to get users from API")
		return err
	}

	// 记录用户数量
	metrics.UsersCount.WithLabelValues(a.config.MuNodeID).Set(float64(len(resp.Users)))
	span.SetAttributes(attribute.Int("users.count", len(resp.Users)))

	// 同步到各个 sync 实现
	for _, v := range a.userSync {
		syncType := v.Name()

		// 创建子 span
		syncCtx, syncSpan := a.tracer.Start(ctx, "App.syncUser."+syncType,
			trace.WithAttributes(
				attribute.String("sync.type", syncType),
				attribute.Int("users.count", len(resp.Users)),
			),
		)

		// 记录 metrics
		timer := prometheus.NewTimer(metrics.UserSyncDuration.WithLabelValues(syncType))

		if err := v.Sync(syncCtx, resp.Users); err != nil {
			metrics.UserSyncCounter.WithLabelValues("failure", syncType).Inc()
			metrics.ErrorsTotal.WithLabelValues("sync_user", syncType+"_error").Inc()
			tracing.RecordError(syncCtx, err, fmt.Sprintf("failed to sync users to %s", syncType))
			timer.ObserveDuration()
			syncSpan.End()
			return err
		}

		timer.ObserveDuration()
		metrics.UserSyncCounter.WithLabelValues("success", syncType).Inc()
		syncSpan.End()
	}

	return nil
}

// syncTraffic 获取并上传用户流量数据
func (a *App) syncTraffic() error {
	slog.Info("syncing traffic")

	// 创建 span
	ctx, span := a.tracer.Start(a.ctx, "App.syncTraffic")
	defer span.End()

	// 记录 metrics
	timer := prometheus.NewTimer(metrics.TrafficSyncDuration)
	defer timer.ObserveDuration()

	// 用于累积所有流量数据的切片
	var allTrafficLogs []*pb.UserTrafficLog

	resp, err := a.muClient.GetUsers(ctx, &pb.GetUsersRequest{})
	if err != nil {
		slog.Error("get users failed", "err", err)
		metrics.ErrorsTotal.WithLabelValues("sync_traffic", "api_error").Inc()
		tracing.RecordError(ctx, err, "failed to get users from API")
		metrics.TrafficSyncCounter.WithLabelValues("failure").Inc()
		return err
	}

	span.SetAttributes(attribute.Int("users.count", len(resp.Users)))

	// 从每个同步实现中获取流量数据
	for _, v := range a.userSync {
		syncType := v.Name()

		// 创建子 span
		trafficCtx, trafficSpan := a.tracer.Start(ctx, "App.syncTraffic.getTraffic."+syncType,
			trace.WithAttributes(
				attribute.String("sync.type", syncType),
			),
		)

		logs, err := v.GetTraffic(trafficCtx, resp.Users)
		if err != nil {
			slog.Error("get traffic failed", "err", err, "sync", syncType)
			metrics.ErrorsTotal.WithLabelValues("sync_traffic", syncType+"_error").Inc()
			tracing.RecordError(trafficCtx, err, fmt.Sprintf("failed to get traffic from %s", syncType))
			trafficSpan.End()
			// 继续处理其他同步实现，不中断整个过程
			continue
		}

		// 将获取到的流量数据添加到总集合中
		if len(logs) > 0 {
			slog.Info("collected traffic logs",
				"count", len(logs),
				"sync", v.Name())

			// 记录收集的流量日志数量
			metrics.TrafficLogsCollected.WithLabelValues(syncType).Set(float64(len(logs)))

			// 统计流量
			var totalUpload, totalDownload int64
			for _, log := range logs {
				totalUpload += log.U
				totalDownload += log.D
			}

			// 更新流量计数器
			metrics.TotalUploadBytes.WithLabelValues(syncType).Add(float64(totalUpload))
			metrics.TotalDownloadBytes.WithLabelValues(syncType).Add(float64(totalDownload))

			trafficSpan.SetAttributes(
				attribute.Int("logs.count", len(logs)),
				attribute.Int64("traffic.upload", totalUpload),
				attribute.Int64("traffic.download", totalDownload),
			)

			allTrafficLogs = append(allTrafficLogs, logs...)
		}

		trafficSpan.End()
	}

	// 如果有流量数据，上传到服务器
	if len(allTrafficLogs) > 0 {
		slog.Info("uploading traffic logs", "count", len(allTrafficLogs))

		// 创建上传 span
		uploadCtx, uploadSpan := a.tracer.Start(ctx, "App.syncTraffic.upload",
			trace.WithAttributes(
				attribute.Int("logs.count", len(allTrafficLogs)),
			),
		)
		defer uploadSpan.End()

		// 构建上传请求
		uploadReq := &pb.UploadTrafficLogRequest{
			Logs:     allTrafficLogs,
			UploadAt: time.Now().Unix(),
		}

		// 上传流量日志
		_, err := a.muClient.UploadTrafficLog(uploadCtx, uploadReq)
		if err != nil {
			slog.Error("failed to upload traffic logs", "err", err)
			metrics.ErrorsTotal.WithLabelValues("sync_traffic", "upload_error").Inc()
			tracing.RecordError(uploadCtx, err, "failed to upload traffic logs")
			metrics.TrafficSyncCounter.WithLabelValues("failure").Inc()
			return err
		}

		slog.Info("traffic logs uploaded successfully", "count", len(allTrafficLogs))
		metrics.TrafficLogsUploaded.Set(float64(len(allTrafficLogs)))
		metrics.TrafficSyncCounter.WithLabelValues("success").Inc()
	} else {
		slog.Info("no traffic logs to upload")
		metrics.TrafficLogsUploaded.Set(0)
		metrics.TrafficSyncCounter.WithLabelValues("success").Inc()
	}

	return nil
}
