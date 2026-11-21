package mu

import (
	"context"
	"errors"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
)

var _ pb.NodeMetricsService = (*Client)(nil)

// ReportNodeMetrics retries a transient failure once with the same batch ID.
func (c *Client) ReportNodeMetrics(ctx context.Context, req *pb.ReportNodeMetricsRequest) (*pb.ReportNodeMetricsResponse, error) {
	req.NodeId = c.nodeID
	var response *pb.ReportNodeMetricsResponse
	var rawErr error
	send := func() error {
		return c.call(ctx, "ReportNodeMetrics", "report MU node metrics", func(ctx context.Context) error {
			response, rawErr = c.nodeMetrics.ReportNodeMetrics(ctx, req)
			return rawErr
		})
	}
	err := send()
	if err == nil || req.BatchId == "" || !retryableNodeMetricsError(rawErr) {
		return response, err
	}
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		err = send()
		return response, err
	}
}

func retryableNodeMetricsError(err error) bool {
	var remote twirp.Error
	if !errors.As(err, &remote) {
		return true
	}
	switch remote.Code() {
	case twirp.Unavailable, twirp.DeadlineExceeded, twirp.Internal, twirp.ResourceExhausted:
		return true
	default:
		return false
	}
}
