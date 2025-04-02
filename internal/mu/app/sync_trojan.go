package app

import (
	"context"
	"log/slog"

	"github.com/icebase/mu/pkg/trojan"
	pb "github.com/icebase/mu/proto/v1"
	"github.com/p4gefau1t/trojan-go/api/service"
)

type trojanSync struct {
	tjManager *trojan.Manager
	// 存储上一次的用户流量状态，用于计算增量
	userStatusMap map[string]*service.UserStatus
}

func newTrojanSync(addr string) (*trojanSync, error) {
	manager, err := trojan.NewManager(addr)
	if err != nil {
		return nil, err
	}
	return &trojanSync{
		tjManager:     manager,
		userStatusMap: make(map[string]*service.UserStatus),
	}, nil
}

func (t *trojanSync) Sync(ctx context.Context, users []*pb.User) error {
	logger := slog.Default().With("method", "trojan_sync")
	logger.Info("start sync trojan users")
	tjUsers, err := t.tjManager.ListUsers(ctx)
	if err != nil {
		logger.Error("list users failed", "error", err)
		return err
	}
	logger.Info("list users success", "count", len(tjUsers))

	// Create maps for easy lookup
	// Map of password -> user for mu users
	muUsersMap := make(map[string]*pb.User)
	// Map of password -> exists for trojan users
	tjUsersMap := make(map[string]bool)

	// Populate the maps
	for _, user := range users {
		// Use uuid as the password for trojan
		muUsersMap[user.V2RayUser.Uuid] = user
	}

	for _, tjUser := range tjUsers {
		tjUsersMap[tjUser.User.Password] = true
	}

	// Process trojan users: remove disabled users or users not in mu
	for _, tjUser := range tjUsers {
		password := tjUser.User.Password
		muUser, exists := muUsersMap[password]

		// If user doesn't exist in mu or is disabled, remove from trojan
		if !exists || muUser.Enable == 0 {
			logger.Info("removing user", "password", password,
				"exists_in_mu", exists,
				"enabled", exists && muUser.Enable == 1)

			err := t.tjManager.RemoveUser(ctx, password, "")
			if err != nil {
				logger.Error("failed to remove user", "password", password, "error", err)
				// Continue even if there's an error
			} else {
				logger.Info("user removed successfully", "password", password)
			}
		}
	}

	// Process mu users: add users that don't exist in trojan
	for _, muUser := range users {
		// Skip disabled users
		if muUser.Enable == 0 {
			continue
		}

		password := muUser.V2RayUser.Uuid
		// If user doesn't exist in trojan, add them
		if _, exists := tjUsersMap[password]; !exists {
			logger.Info("adding user", "password", password)

			err := t.tjManager.AddUser(ctx, password)
			if err != nil {
				logger.Error("failed to add user", "password", password, "error", err)
				// Continue even if there's an error
			} else {
				logger.Info("user added successfully", "password", password)
			}
		}
	}

	return nil
}

func (t *trojanSync) GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error) {
	logger := slog.Default().With("method", "trojan_get_traffic")
	logger.Info("starting traffic collection")

	// 创建收集流量数据的结果集
	var trafficLogs []*pb.UserTrafficLog

	// Process each trojan user
	processedCount := 0
	for _, user := range users {
		uuid := user.V2RayUser.Uuid

		// Get detailed user information including traffic
		userResp, err := t.tjManager.GetUser(ctx, uuid)
		if err != nil {
			logger.Error("failed to get user details", "uuid", uuid, "error", err)
			continue
		}

		if userResp == nil || userResp.Status == nil || userResp.Status.TrafficTotal == nil {
			logger.Warn("missing traffic data for user", "uuid", uuid)
			continue
		}

		// 首先尝试从状态中获取用户ID
		userID := int64(0) // 默认ID为0，在没有用户ID时使用

		// 计算流量增量：当前流量 - 之前存储的流量
		prevStatus, ok := t.userStatusMap[uuid]
		if ok && prevStatus != nil && prevStatus.TrafficTotal != nil {
			// 计算上传和下载流量的增量
			uploadDiff := int64(userResp.Status.TrafficTotal.UploadTraffic) - int64(prevStatus.TrafficTotal.UploadTraffic)
			downloadDiff := int64(userResp.Status.TrafficTotal.DownloadTraffic) - int64(prevStatus.TrafficTotal.DownloadTraffic)

			// 只记录正增长的流量
			if uploadDiff > 0 || downloadDiff > 0 {
				// 确保流量值非负
				if uploadDiff < 0 {
					uploadDiff = 0
				}
				if downloadDiff < 0 {
					downloadDiff = 0
				}

				// 创建新的流量日志
				trafficLog := &pb.UserTrafficLog{
					UserId: userID,
					Uuid:   uuid,
					U:      uploadDiff,
					D:      downloadDiff,
				}

				// 添加到结果集
				trafficLogs = append(trafficLogs, trafficLog)
				processedCount++

				logger.Info("processed traffic data",
					"uuid", uuid,
					"upload_diff", uploadDiff,
					"download_diff", downloadDiff)
			}
		}

		// 更新用户状态缓存
		t.userStatusMap[uuid] = userResp.Status
	}

	logger.Info("completed traffic collection", "processed_count", processedCount, "logs_count", len(trafficLogs))
	return trafficLogs, nil
}
