package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	pb "github.com/icebase/mu/proto/v1"
)

const nodeMetricsInterval = time.Minute

func (a *App) reportNodeMetrics(ctx context.Context) {
	ticker := time.NewTicker(nodeMetricsInterval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if err := a.reportNodeMetricsOnce(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("node metrics report failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *App) reportNodeMetricsOnce(ctx context.Context) error {
	batch := &pb.ReportNodeMetricsRequest{
		BatchId:  uuid.NewString(),
		Backends: make([]*pb.V2FlyBackendSnapshot, 0, len(a.v2flyBackends)),
	}
	for address, backend := range a.v2flyBackends {
		if err := ctx.Err(); err != nil {
			return err
		}
		sample := &pb.V2FlyBackendSnapshot{Backend: address, SampledAtUnixMillis: time.Now().UnixMilli()}
		stats, err := backend.ReadNodeStats(ctx)
		if err == nil {
			sample.Up = true
			sample.Runtime = &pb.V2FlyRuntimeStats{
				UptimeSeconds:           stats.Runtime.GetUptime(),
				Goroutines:              stats.Runtime.GetNumGoroutine(),
				HeapAllocBytes:          stats.Runtime.GetAlloc(),
				TotalAllocBytes:         stats.Runtime.GetTotalAlloc(),
				RuntimeSysBytes:         stats.Runtime.GetSys(),
				Mallocs:                 stats.Runtime.GetMallocs(),
				Frees:                   stats.Runtime.GetFrees(),
				GcCycles:                stats.Runtime.GetNumGC(),
				GcPauseTotalNanoseconds: stats.Runtime.GetPauseTotalNs(),
			}
			sample.Traffic = make([]*pb.V2FlyTrafficCounter, 0, len(stats.Traffic))
			for _, counter := range stats.Traffic {
				sample.Traffic = append(sample.Traffic, &pb.V2FlyTrafficCounter{
					Dimension: counter.Dimension,
					Tag:       counter.Tag,
					Direction: counter.Direction,
					Bytes:     uint64(counter.Bytes),
				})
			}
		}
		batch.Backends = append(batch.Backends, sample)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := a.muClient.ReportNodeMetrics(ctx, batch)
	return err
}
