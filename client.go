package mu

import (
	"context"
	"net/http"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
)

const (
	TokenHeader = "X-Mu-Token"
)

var (
	_ pb.MUService = (*Client)(nil)
)

type Client struct {
	addr, token string
	nodeID      string
	client      pb.MUService
}

func NewClient(addr string, token string, nodeID string) *Client {
	client := pb.NewMUServiceJSONClient(addr, http.DefaultClient, twirp.WithClientPathPrefix(""))
	return &Client{client: client, addr: addr, token: token, nodeID: nodeID}
}

func (c *Client) newContext(ctx context.Context) context.Context {
	header := make(http.Header)
	header.Set(TokenHeader, c.token)
	ctx, err := twirp.WithHTTPRequestHeaders(ctx, header)
	if err != nil {
		panic(err)
	}
	return ctx
}

func (c *Client) GetUsers(ctx context.Context, req *pb.GetUsersRequest) (*pb.GetUsersResponse, error) {
	req.NodeId = c.nodeID
	return c.client.GetUsers(c.newContext(ctx), req)
}

func (c *Client) UploadTrafficLog(ctx context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	req.NodeId = c.nodeID
	req.UploadAt = time.Now().Unix()
	return c.client.UploadTrafficLog(c.newContext(ctx), req)
}

func (c *Client) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	return c.client.Ping(c.newContext(ctx), req)
}
