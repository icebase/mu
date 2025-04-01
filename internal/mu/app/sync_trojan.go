package app

import (
	"context"
	"log/slog"

	"github.com/icebase/mu/pkg/trojan"
	pb "github.com/icebase/mu/proto/v1"
)

type trojanSync struct {
	tjManager *trojan.Manager
}

func newTrojanSync(addr string) (*trojanSync, error) {
	manager, err := trojan.NewManager(addr)
	if err != nil {
		return nil, err
	}
	return &trojanSync{
		tjManager: manager,
	}, nil
}

func (t *trojanSync) Sync(ctx context.Context, users []*pb.User) error {
	logger := slog.Default()
	tjUsers, err := t.tjManager.ListUsers(ctx)
	if err != nil {
		return err
	}
	logger.Info("[trojan] list users success", "count", len(tjUsers))

	return nil
}
