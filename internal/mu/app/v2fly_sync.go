package app

import (
	"context"
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

func (v *v2flySync) Name() string {
	return "v2fly"
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
		email := apiUser.V2RayUser.Email
		_, ok := v2UsersMap[uuid]
		if !ok {
			// 用户不在 v2fly 服务器上，需要添加
			logger.Info("adding new user", "uuid", uuid)

			// 创建用户对象
			v2User := &v2flyUser{
				uuid:    uuid,
				email:   email,
				level:   apiUser.V2RayUser.Level,
				alterID: apiUser.V2RayUser.AlterId,
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
	logger.Info("starting traffic collection", "users_count", len(users))

	// 创建流量日志结果集
	var trafficLogs []*pb.UserTrafficLog
	processedCount := 0

	var u, d int64

	// 处理每个用户的流量数据
	for _, user := range users {

		trafficInfo, err := v.manager.GetTrafficAndReset(ctx,
			v2fly.NewUser(user.V2RayUser.Email, user.V2RayUser.Uuid, user.V2RayUser.AlterId, user.V2RayUser.Level))
		if err != nil {
			logger.Error("failed to get traffic",
				"uuid", user.V2RayUser.Uuid,
				"error", err)
			continue
		}

		if trafficInfo.Up == 0 && trafficInfo.Down == 0 {
			logger.Info("skipping user with no traffic",
				"email", user.V2RayUser.Email,
				"uuid", user.V2RayUser.Uuid)
			continue
		}

		// 创建流量日志
		trafficLog := &pb.UserTrafficLog{
			UserId: user.Id,
			Uuid:   user.V2RayUser.Uuid,
			U:      trafficInfo.Up,
			D:      trafficInfo.Down,
		}

		u += trafficInfo.Up
		d += trafficInfo.Down

		// 添加到结果集
		trafficLogs = append(trafficLogs, trafficLog)
		processedCount++

		logger.Info("processed traffic data",
			"uuid", user.V2RayUser.Uuid,
			"user_id", user.Id,
			"upload", trafficInfo.Up,
			"download", trafficInfo.Down)
	}

	logger.Info("completed traffic collection",
		"processed_count", processedCount, "logs_count", len(trafficLogs), "upload", u, "download", d)
	return trafficLogs, nil
}
