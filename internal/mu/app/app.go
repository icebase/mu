package app

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/icebase/mu"
	pb "github.com/icebase/mu/proto/v1"
)

type App struct {
	config *Config

	muClient *mu.Client
	userSync []UserSync

	ctx    context.Context
	cancel context.CancelFunc
}

func New(config *Config) *App {
	return &App{
		config: config,
	}
}

func (a *App) Init() error {
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.muClient = mu.NewClient(a.config.MuApiBaseURL, a.config.MuApiToken, a.config.MuNodeID)
	for _, addr := range a.config.TrojanAddrs {
		s, err := newTrojanSync(addr)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, s)
	}
	for _, addr := range a.config.V2flyAddrs {
		s, err := newV2flySync(addr)
		if err != nil {
			return err
		}
		a.userSync = append(a.userSync, s)
	}
	return nil
}

func (a *App) Run() {
	go a.jobs()
	sigs := make(chan os.Signal, 1)
	sig := <-sigs
	slog.Info("recv signale", "signal", sig)
	a.cancel()
	os.Exit(0)
}

func (a *App) jobs() {
	syncTimer := time.NewTimer(60 * time.Second)

	for {
		select {
		case <-syncTimer.C:
			if err := a.syncTraffic(); err != nil {
				slog.Error("sync traffic failed", "err", err)
			}
			if err := a.syncUser(); err != nil {
				slog.Error("sync user failed", "err", err)
			}
			syncTimer.Reset(60 * time.Second)
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *App) syncUser() error {
	ctx := a.ctx
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
	slog.Info("syncing traffic")

	ctx := a.ctx
	// 用于累积所有流量数据的切片
	var allTrafficLogs []*pb.UserTrafficLog

	resp, err := a.muClient.GetUsers(ctx, &pb.GetUsersRequest{})
	if err != nil {
		slog.Error("get users failed", "err", err)
		return err
	}

	// 从每个同步实现中获取流量数据
	for _, v := range a.userSync {
		logs, err := v.GetTraffic(ctx, resp.Users)
		if err != nil {
			slog.Error("get traffic failed", "err", err)
			// 继续处理其他同步实现，不中断整个过程
			continue
		}

		// 将获取到的流量数据添加到总集合中
		if len(logs) > 0 {
			slog.Info("collected traffic logs",
				"count", len(logs),
				"sync", v.Name())
			allTrafficLogs = append(allTrafficLogs, logs...)
		}
	}

	// 如果有流量数据，上传到服务器
	if len(allTrafficLogs) > 0 {
		slog.Info("uploading traffic logs", "count", len(allTrafficLogs))

		// 构建上传请求
		uploadReq := &pb.UploadTrafficLogRequest{
			Logs:     allTrafficLogs,
			UploadAt: time.Now().Unix(),
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
