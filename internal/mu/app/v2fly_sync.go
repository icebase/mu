package app

import (
	"context"
	"log/slog"

	"github.com/icebase/mu/pkg/v2fly"
	pb "github.com/icebase/mu/proto/v1"
)

var (
	tag = "api"
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
	return nil
}

func (v *v2flySync) GetTraffic(ctx context.Context) ([]*pb.UserTrafficLog, error) {
	return nil, nil
}
