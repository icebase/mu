package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// 操作计数器
	PingCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_ping_total",
			Help: "Total number of ping operations",
		},
		[]string{"status"}, // success, failure
	)

	UserSyncCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_user_sync_total",
			Help: "Total number of user sync operations",
		},
		[]string{"status", "sync_type"}, // status: success/failure, sync_type: trojan/v2fly
	)

	TrafficSyncCounter = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_traffic_sync_total",
			Help: "Total number of traffic sync operations",
		},
		[]string{"status"},
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
		[]string{"sync_type"},
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
		[]string{"sync_type"},
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
		[]string{"sync_type"},
	)

	TotalDownloadBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mu_total_download_bytes",
			Help: "Total download bytes reported",
		},
		[]string{"sync_type"},
	)
)
