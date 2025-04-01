package app

import (
	"context"

	pb "github.com/icebase/mu/proto/v1"
)

type v2flySync struct{}

func newV2flySync(addr string) *v2flySync {
	return &v2flySync{}
}

func (v *v2flySync) Sync(ctx context.Context, users []*pb.User) error {
	return nil
}
