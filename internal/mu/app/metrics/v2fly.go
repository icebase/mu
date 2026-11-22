package metrics

import (
	"context"
	"time"

	"github.com/icebase/mu/pkg/v2fly"
	"github.com/prometheus/client_golang/prometheus"
)

type nodeCollector struct {
	ctx  context.Context
	read func(context.Context) (v2fly.NodeStats, error)
}

func NewV2FlyCollector(ctx context.Context, read func(context.Context) (v2fly.NodeStats, error)) prometheus.Collector {
	return &nodeCollector{ctx: ctx, read: read}
}

var nodeMetrics = map[string]*prometheus.Desc{
	"up":                         prometheus.NewDesc("v2ray_up", "Whether the backend statistics request succeeded.", nil, nil),
	"scrape_duration_seconds":    prometheus.NewDesc("v2ray_scrape_duration_seconds", "Time spent reading this backend.", nil, nil),
	"uptime_seconds":             prometheus.NewDesc("v2ray_uptime_seconds", "Backend uptime in seconds.", nil, nil),
	"goroutines":                 prometheus.NewDesc("v2ray_goroutines", "Current backend goroutine count.", nil, nil),
	"memstats_alloc_bytes":       prometheus.NewDesc("v2ray_memstats_alloc_bytes", "Bytes currently allocated on the backend heap.", nil, nil),
	"memstats_alloc_bytes_total": prometheus.NewDesc("v2ray_memstats_alloc_bytes_total", "Total bytes allocated on the backend heap.", nil, nil),
	"memstats_sys_bytes":         prometheus.NewDesc("v2ray_memstats_sys_bytes", "Bytes obtained by the backend runtime from the OS.", nil, nil),
	"memstats_mallocs_total":     prometheus.NewDesc("v2ray_memstats_mallocs_total", "Total heap allocations.", nil, nil),
	"memstats_frees_total":       prometheus.NewDesc("v2ray_memstats_frees_total", "Total freed heap objects.", nil, nil),
	"memstats_num_gc":            prometheus.NewDesc("v2ray_memstats_num_gc", "Completed garbage collections.", nil, nil),
	"memstats_pause_total_ns":    prometheus.NewDesc("v2ray_memstats_pause_total_ns", "Cumulative garbage collection pause time in nanoseconds.", nil, nil),
	"uplink": prometheus.NewDesc("v2ray_traffic_uplink_bytes_total",
		"Cumulative uplink bytes by proxy tag.", []string{"dimension", "target"}, nil),
	"downlink": prometheus.NewDesc("v2ray_traffic_downlink_bytes_total",
		"Cumulative downlink bytes by proxy tag.", []string{"dimension", "target"}, nil),
}

func (c *nodeCollector) Describe(ch chan<- *prometheus.Desc) {
	for _, desc := range nodeMetrics {
		ch <- desc
	}
}

func (c *nodeCollector) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()
	snapshot, err := c.read(c.ctx)
	emit := func(name string, kind prometheus.ValueType, value float64, labels ...string) {
		ch <- prometheus.MustNewConstMetric(nodeMetrics[name], kind, value, labels...)
	}
	emit("scrape_duration_seconds", prometheus.GaugeValue, time.Since(start).Seconds())
	if err != nil {
		emit("up", prometheus.GaugeValue, 0)
		return
	}
	emit("up", prometheus.GaugeValue, 1)
	emit("uptime_seconds", prometheus.GaugeValue, float64(snapshot.Runtime.GetUptime()))
	emit("goroutines", prometheus.GaugeValue, float64(snapshot.Runtime.GetNumGoroutine()))
	emit("memstats_alloc_bytes", prometheus.GaugeValue, float64(snapshot.Runtime.GetAlloc()))
	emit("memstats_alloc_bytes_total", prometheus.CounterValue, float64(snapshot.Runtime.GetTotalAlloc()))
	emit("memstats_sys_bytes", prometheus.GaugeValue, float64(snapshot.Runtime.GetSys()))
	emit("memstats_mallocs_total", prometheus.CounterValue, float64(snapshot.Runtime.GetMallocs()))
	emit("memstats_frees_total", prometheus.CounterValue, float64(snapshot.Runtime.GetFrees()))
	emit("memstats_num_gc", prometheus.CounterValue, float64(snapshot.Runtime.GetNumGC()))
	emit("memstats_pause_total_ns", prometheus.CounterValue, float64(snapshot.Runtime.GetPauseTotalNs()))
	for _, traffic := range snapshot.Traffic {
		emit(traffic.Direction, prometheus.CounterValue, float64(traffic.Bytes), traffic.Dimension, traffic.Tag)
	}
}
