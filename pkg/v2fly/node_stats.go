package v2fly

import (
	"context"
	"errors"
	"strings"

	stats "github.com/v2fly/v2ray-core/v5/app/stats/command"
)

type NodeStats struct {
	Runtime *stats.SysStatsResponse
	Traffic []TagTraffic
}

type TagTraffic struct {
	Dimension, Tag, Direction string
	Bytes                     int64
}

// ReadNodeStats samples a backend without modifying its accounting state.
func (m *Manager) ReadNodeStats(ctx context.Context) (NodeStats, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	runtime, err := m.statsClient.GetSysStats(ctx, &stats.SysStatsRequest{})
	if err != nil {
		return NodeStats{}, rpcError("read backend runtime", err)
	}
	resp, err := m.statsClient.QueryStats(ctx, &stats.QueryStatsRequest{Patterns: []string{"inbound>>>", "outbound>>>"}})
	if err != nil {
		return NodeStats{}, rpcError("read backend traffic", err)
	}
	if runtime == nil || resp == nil {
		return NodeStats{}, errors.New("missing backend statistics")
	}
	result := NodeStats{Runtime: runtime}
	for _, stat := range resp.Stat {
		parts := strings.Split(stat.GetName(), ">>>")
		if len(parts) != 4 || parts[1] == "" || parts[2] != "traffic" {
			continue
		}
		if (parts[0] != "inbound" && parts[0] != "outbound") || (parts[3] != uplinkDirection && parts[3] != downlinkDirection) {
			continue
		}
		if stat.Value < 0 {
			return NodeStats{}, errors.New("negative backend traffic counter")
		}
		result.Traffic = append(result.Traffic, TagTraffic{Dimension: parts[0], Tag: parts[1], Direction: parts[3], Bytes: stat.Value})
	}
	return result, nil
}
