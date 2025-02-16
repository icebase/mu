package proto

import (
	"context"
	"os"
	"testing"

	pb "github.com/icebase/mu/proto/v1"
)

func TestClient(t *testing.T) {
	c := NewClient(os.Getenv("MU_ADDR"), os.Getenv("MU_TOKEN"))
	resp, err := c.GetUsers(context.Background(), &pb.GetUsersRequest{})
	t.Log(resp, err)
}
