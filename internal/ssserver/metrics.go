package ssserver

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics are aggregate on purpose: per-user labels would make cardinality grow with the user base.
var (
	usersGauge    = promauto.NewGauge(prometheus.GaugeOpts{Name: "ss_server_users", Help: "Number of configured users"})
	sessionsGauge = promauto.NewGauge(prometheus.GaugeOpts{Name: "ss_server_active_sessions", Help: "Active TCP connections and UDP associations"})
	authFailures  = promauto.NewCounter(prometheus.CounterOpts{
		Name: "ss_server_rejected_total", Help: "Connections or packets rejected before a user was identified",
	})
	uploadBytes   = promauto.NewCounter(prometheus.CounterOpts{Name: "ss_server_upload_bytes_total", Help: "Payload bytes relayed from clients to destinations"})
	downloadBytes = promauto.NewCounter(prometheus.CounterOpts{Name: "ss_server_download_bytes_total", Help: "Payload bytes relayed from destinations to clients"})
)
