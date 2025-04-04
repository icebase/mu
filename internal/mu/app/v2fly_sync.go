package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/icebase/mu/pkg/v2fly"
	pb "github.com/icebase/mu/proto/v1"
)

var (
	tag = "proxy"
)

type v2flySync struct {
	manager *v2fly.Manager
}

func newV2flySync(addr string) (*v2flySync, error) {
	manager, err := v2fly.NewManager(addr, tag)
	if err != nil {
		slog.Error("create v2fly manager failed", "addr", addr)
		return nil, err
	}
	return &v2flySync{
		manager: manager,
	}, nil
}

func (v *v2flySync) Sync(ctx context.Context, users []*pb.User) error {
	logger := slog.Default().With("method", "v2fly_sync")
	logger.Info("starting user sync", "users_count", len(users))

	// 获取 v2fly 服务器上的所有用户
	v2Users, err := v.manager.GetUserList(ctx, false)
	if err != nil {
		logger.Error("failed to get v2fly user list", "error", err)
		return err
	}

	// 创建映射方便查找
	apiUsersMap := make(map[string]*pb.User)
	v2UsersMap := make(map[string]v2fly.User)

	for _, user := range users {
		if user.V2RayUser != nil && user.V2RayUser.Uuid != "" {
			apiUsersMap[user.V2RayUser.Uuid] = user
		}
	}

	for _, user := range v2Users {
		v2UsersMap[user.User.GetUUID()] = user.User
	}

	// 检查要删除的用户
	for _, v2User := range v2Users {
		uuid := v2User.User.GetUUID()
		logger.Debug("checking v2fly user", "uuid", uuid)

		apiUser, ok := apiUsersMap[uuid]
		if !ok {
			// 用户不在 API 返回的列表中，应该被删除
			logger.Info("removing user not in API", "uuid", uuid)
			err := v.manager.RemoveUser(ctx, v2User.User)
			if err != nil {
				logger.Error("failed to remove user", "uuid", uuid, "error", err)
				// 继续处理其他用户，不中断
			}
			continue
		}

		// 如果用户被禁用，也应该被删除
		if apiUser.Enable == 0 {
			logger.Info("removing disabled user", "uuid", uuid)
			err := v.manager.RemoveUser(ctx, v2User.User)
			if err != nil {
				logger.Error("failed to remove disabled user", "uuid", uuid, "error", err)
				// 继续处理其他用户，不中断
			}
		}
	}

	// 添加或更新用户
	for _, apiUser := range users {
		// 跳过被禁用的用户
		if apiUser.Enable == 0 {
			continue
		}

		// 确保用户有 V2Ray 信息
		if apiUser.V2RayUser == nil || apiUser.V2RayUser.Uuid == "" {
			logger.Warn("user missing v2ray info", "user_id", apiUser.Id)
			continue
		}

		uuid := apiUser.V2RayUser.Uuid
		_, ok := v2UsersMap[uuid]
		if !ok {
			// 用户不在 v2fly 服务器上，需要添加
			logger.Info("adding new user", "uuid", uuid)

			// 创建用户对象
			v2User := &v2flyUser{
				uuid:    uuid,
				email:   fmt.Sprintf("%s@v2fly.local", uuid),
				level:   0,
				alterID: 0,
			}

			// 添加用户到 v2fly 服务器
			exist, err := v.manager.AddUser(ctx, v2User)
			if err != nil {
				logger.Error("failed to add user", "uuid", uuid, "error", err)
				continue
			}

			if exist {
				logger.Info("user already exists", "uuid", uuid)
			} else {
				logger.Info("user added successfully", "uuid", uuid)
			}
		}
	}

	logger.Info("user sync completed")
	return nil
}

// V2Ray 用户接口实现
type v2flyUser struct {
	uuid    string
	email   string
	level   uint32
	alterID uint32
}

func (u *v2flyUser) GetUUID() string {
	return u.uuid
}

func (u *v2flyUser) GetEmail() string {
	return u.email
}

func (u *v2flyUser) GetLevel() uint32 {
	return u.level
}

func (u *v2flyUser) GetAlterID() uint32 {
	return u.alterID
}

func (v *v2flySync) GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error) {
	logger := slog.Default().With("method", "v2fly_get_traffic")
	logger.Info("starting traffic collection")

	// 获取所有用户及其流量信息
	v2Users, err := v.manager.GetUserList(ctx, true) // 设置 reset 为 true，获取并重置流量统计
	if err != nil {
		logger.Error("failed to get v2fly user list", "error", err)
		return nil, err
	}

	// 创建 UUID 到 UserID 的映射
	uuidToUserID := make(map[string]int64)
	for _, user := range users {
		if user.V2RayUser != nil && user.V2RayUser.Uuid != "" {
			uuidToUserID[user.V2RayUser.Uuid] = user.Id
		}
	}

	// 创建流量日志结果集
	var trafficLogs []*pb.UserTrafficLog
	processedCount := 0

	// 处理每个用户的流量数据
	for _, v2User := range v2Users {
		uuid := v2User.User.GetUUID()

		// 检查是否有流量数据
		if v2User.TrafficInfo.Up == 0 && v2User.TrafficInfo.Down == 0 {
			continue // 跳过没有流量的用户
		}

		// 获取对应的 UserID
		userID, ok := uuidToUserID[uuid]
		if !ok {
			logger.Warn("user not found in mapping", "uuid", uuid)
			continue // 如果找不到对应的 UserID，跳过这条记录
		}

		// 创建流量日志
		trafficLog := &pb.UserTrafficLog{
			UserId: userID,
			Uuid:   uuid,
			U:      v2User.TrafficInfo.Up,
			D:      v2User.TrafficInfo.Down,
		}

		// 添加到结果集
		trafficLogs = append(trafficLogs, trafficLog)
		processedCount++

		logger.Info("processed traffic data",
			"uuid", uuid,
			"user_id", userID,
			"upload", v2User.TrafficInfo.Up,
			"download", v2User.TrafficInfo.Down)
	}

	logger.Info("completed traffic collection", "processed_count", processedCount, "logs_count", len(trafficLogs))
	return trafficLogs, nil
}
