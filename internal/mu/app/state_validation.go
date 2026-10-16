package app

import (
	"errors"

	pb "github.com/icebase/mu/proto/v1"
)

func validateState(data stateData) error {
	invalid := errors.New("invalid synchronization state contents; refusing to reset billing or authorization history")
	for key, b := range data.Backends {
		if key == "" || b == nil || b.Users == nil || b.Applied == nil || b.Cursors == nil {
			return invalid
		}
		for id, user := range b.Users {
			if id == "" || user.UUID == "" {
				return invalid
			}
		}
		for id, user := range b.Applied {
			if _, exists := b.Users[id]; !exists || user.UUID == "" {
				return invalid
			}
		}
		for id, cursor := range b.Cursors {
			if id == "" || cursor.Up < 0 || cursor.Down < 0 {
				return invalid
			}
		}
	}
	count := 0
	for _, batch := range data.Pending {
		if batch.UploadAt <= 0 || len(batch.Logs) == 0 {
			return invalid
		}
		for _, log := range batch.Logs {
			count++
			if count > maxPendingLogs || log == nil || log.Uuid == "" || log.U < 0 || log.D < 0 {
				return invalid
			}
			if log.TrafficType != int32(pb.TrafficType_TRAFFIC_TYPE_TROJAN) && log.TrafficType != int32(pb.TrafficType_TRAFFIC_TYPE_V2FLY) {
				return invalid
			}
		}
	}
	return nil
}
