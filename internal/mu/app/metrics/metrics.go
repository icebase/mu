package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	labelStatus   = "status"
	labelSyncType = "sync_type"
)

var (
	// 操作计数器
	PingCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_ping_total",
			Help: "Total number of ping operations",
		},
		[]string{labelStatus}, // success, failure
	)

	UserSyncCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_user_sync_total",
			Help: "Total number of user sync operations",
		},
		[]string{labelStatus, labelSyncType}, // status: success/failure, sync_type: trojan/v2fly
	)

	TrafficSyncCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_traffic_sync_total",
			Help: "Total number of traffic sync operations",
		},
		[]string{labelStatus},
	)

	// 操作延迟直方图
	PingDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mu_ping_duration_seconds",
			Help:    "Duration of ping operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	UserSyncDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mu_user_sync_duration_seconds",
			Help:    "Duration of user sync operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{labelSyncType},
	)

	TrafficSyncDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "mu_traffic_sync_duration_seconds",
			Help:    "Duration of traffic sync operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)

	// 流量相关的指标
	TrafficLogsCollected = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mu_traffic_logs_collected",
			Help: "Number of traffic logs collected in last sync",
		},
		[]string{labelSyncType},
	)

	TrafficLogsUploaded = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mu_traffic_logs_uploaded",
			Help: "Number of traffic logs uploaded in last sync",
		},
	)

	// 用户相关的指标
	UsersCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mu_users_count",
			Help: "Number of users from API",
		},
		[]string{"node_id"},
	)

	// 错误计数器
	ErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_errors_total",
			Help: "Total number of errors",
		},
		[]string{"operation", "error_type"},
	)

	// 上传和下载流量总计
	TotalUploadBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_total_upload_bytes",
			Help: "Total upload bytes reported",
		},
		[]string{labelSyncType},
	)

	TotalDownloadBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_total_download_bytes",
			Help: "Total download bytes reported",
		},
		[]string{labelSyncType},
	)

	// BuildInfo exposes the running version, useful to correlate behavior changes with deployments.
	BuildInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mu_build_info",
			Help: "Build information; the value is always 1",
		},
		[]string{"version", "go_version"},
	)

	// MU API 请求指标（每个 RPC 一次 HTTP 调用）
	APIRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_api_requests_total",
			Help: "Total number of MU API requests by rpc, outcome and Twirp error code (empty on success; transport failures are reported by Twirp as internal)",
		},
		[]string{"rpc", labelStatus, "code"},
	)

	APIRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mu_api_request_duration_seconds",
			Help:    "Duration of MU API requests in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"rpc"},
	)

	// 任务最近一次成功时间，用于告警（time() - mu_job_last_success_timestamp_seconds）
	JobLastSuccess = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mu_job_last_success_timestamp_seconds",
			Help: "Unix time of the last fully successful run of a job (ping, sync_user, sync_traffic)",
		},
		[]string{"job"},
	)

	// 待上传流量队列（持久化 outbox）
	OutboxBatches = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mu_traffic_outbox_batches",
			Help: "Number of queued traffic batches waiting for upload",
		},
	)

	OutboxLogs = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "mu_traffic_outbox_logs",
			Help: "Number of queued traffic log records waiting for upload",
		},
	)

	// 后端（Trojan/V2Fly/VLESS）用户增删操作
	BackendUserOperations = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_backend_user_operations_total",
			Help: "Total number of user add/remove calls made to proxy backends",
		},
		[]string{labelSyncType, "operation", labelStatus},
	)
)

// Job names used by JobLastSuccess.
const (
	JobPing        = "ping"
	JobSyncUser    = "sync_user"
	JobSyncTraffic = "sync_traffic"
)

// Result returns the status label value for err.
func Result(err error) string {
	if err != nil {
		return "failure"
	}
	return "success"
}

// MarkJobSuccess records that job completed without errors just now.
func MarkJobSuccess(job string) {
	JobLastSuccess.WithLabelValues(job).SetToCurrentTime()
}

// ObserveAPIRequest records one MU API request.
func ObserveAPIRequest(rpc string, elapsed time.Duration, code string, err error) {
	APIRequests.WithLabelValues(rpc, Result(err), code).Inc()
	APIRequestDuration.WithLabelValues(rpc).Observe(elapsed.Seconds())
}

// SetOutbox publishes the size of the pending traffic queue.
func SetOutbox(batches, logs int) {
	OutboxBatches.Set(float64(batches))
	OutboxLogs.Set(float64(logs))
}

// ObserveBackendUserOperation records one add/remove call against a proxy backend.
func ObserveBackendUserOperation(syncType, operation string, err error) {
	BackendUserOperations.WithLabelValues(syncType, operation, Result(err)).Inc()
}
