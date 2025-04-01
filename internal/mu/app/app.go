package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/icebase/mu"
	pb "github.com/icebase/mu/proto/v1"
)

type App struct {
	muClient *mu.Client
	userSync []UserSync
}

func New(config *Config) *App {
	return &App{}
}

func (a *App) Run() {
	ctx := context.Background()
	timer := time.NewTimer(10 * time.Second)
	for {
		select {
		case <-timer.C:
			if err := a.sync(); err != nil {
				slog.Error("sync failed", "err", err)
			}
			timer.Reset(10 * time.Second)
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) sync() error {
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
