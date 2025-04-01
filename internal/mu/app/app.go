package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/icebase/mu"
	pb "github.com/icebase/mu/proto/v1"
)

type App struct {
	config *Config

	muClient *mu.Client
	userSync []UserSync
}

func New(config *Config) *App {
	return &App{
		config: config,
	}
}

func (a *App) Init() error {
	a.muClient = mu.NewClient(a.config.MuApiBaseURL, a.config.MuApiToken)
	for _, addr := range a.config.TrojanAddrs {
		s, err := newTrojanSync(addr)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, s)
	}
	for _, addr := range a.config.V2flyAddrs {
		a.userSync = append(a.userSync, newV2flySync(addr))
	}
	return nil
}

func (a *App) Run() {
	ctx := context.Background()
	syncTimer := time.NewTimer(10 * time.Second)
	trafficTimer := time.NewTimer(30 * time.Second) // 流量统计间隔更长一些

	for {
		select {
		case <-syncTimer.C:
			if err := a.syncUser(); err != nil {
				slog.Error("sync user failed", "err", err)
			}
			syncTimer.Reset(10 * time.Second)
		
		case <-trafficTimer.C:
			if err := a.syncTraffic(); err != nil {
				slog.Error("sync traffic failed", "err", err)
			}
			trafficTimer.Reset(30 * time.Second)
		
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) syncUser() error {
	ctx := context.Background()
	resp, err := a.muClient.GetUsers(ctx, &pb.GetUsersRequest{})
	if err != nil {
		return err
	}
	for _, v := range a.userSync {
		if err := v.Sync(ctx, resp.Users); err != nil {
			return err
		}
	}
	return nil
}

// syncTraffic 获取并上传用户流量数据
func (a *App) syncTraffic() error {
	ctx := context.Background()
	
	// 用于累积所有流量数据的切片
	var allTrafficLogs []*pb.UserTrafficLog
	
	// 从每个同步实现中获取流量数据
	for _, v := range a.userSync {
		logs, err := v.GetTraffic(ctx)
		if err != nil {
			slog.Error("get traffic failed", "err", err)
			// 继续处理其他同步实现，不中断整个过程
			continue
		}
		
		// 将获取到的流量数据添加到总集合中
		if len(logs) > 0 {
			slog.Info("collected traffic logs", "count", len(logs))
			allTrafficLogs = append(allTrafficLogs, logs...)
		}
	}
	
	// 如果有流量数据，上传到服务器
	if len(allTrafficLogs) > 0 {
		slog.Info("uploading traffic logs", "count", len(allTrafficLogs))
		
		// 构建上传请求
		uploadReq := &pb.UploadTrafficLogRequest{
			Logs:      allTrafficLogs,
			UploadAt:  time.Now().Unix(),
		}
		
		// 上传流量日志
		_, err := a.muClient.UploadTrafficLog(ctx, uploadReq)
		if err != nil {
			slog.Error("failed to upload traffic logs", "err", err)
			return err
		}
		
		slog.Info("traffic logs uploaded successfully", "count", len(allTrafficLogs))
	} else {
		slog.Info("no traffic logs to upload")
	}
	
	return nil
}
