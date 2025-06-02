package app

import (
	"context"

	pb "github.com/icebase/mu/proto/v1"
)

type UserSync interface {
	Sync(ctx context.Context, users []*pb.User) error
	GetTraffic(ctx context.Context, users []*pb.User) ([]*pb.UserTrafficLog, error)
	Name() string
}
