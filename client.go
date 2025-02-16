package proto

import (
	"context"
	"net/http"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
)

var (
	_ pb.MUService = (*Client)(nil)
)

type Client struct {
	addr, token string
	client      pb.MUService
}

func NewClient(addr string, token string) *Client {
	client := pb.NewMUServiceJSONClient(addr, http.DefaultClient)
	return &Client{client: client, addr: addr, token: token}
}

func (c *Client) newContext(ctx context.Context) context.Context {
	header := make(http.Header)
	header.Set("Token", c.token)
	ctx, err := twirp.WithHTTPRequestHeaders(ctx, header)
	if err != nil {
		panic(err)
	}
	return ctx
}

func (c *Client) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	return c.client.GetUsers(c.newContext(ctx), req)
}

func (c *Client) UploadTrafficLog(ctx context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	return c.client.UploadTrafficLog(c.newContext(ctx), req)
}
