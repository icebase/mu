package mu

import (
	"context"
	"errors"
	"fmt"
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
	client := pb.NewMUServiceJSONClient(addr, &http.Client{Timeout: 10 * time.Second}, twirp.WithClientPathPrefix(""))
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
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := c.client.GetUsers(c.newContext(ctx), req)
	return response, apiError("get MU users", err)
}

func (c *Client) UploadTrafficLog(ctx context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	req.NodeId = c.nodeID
	if req.UploadAt == 0 {
		req.UploadAt = time.Now().Unix()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := c.client.UploadTrafficLog(c.newContext(ctx), req)
	return response, apiError("upload MU traffic", err)
}

func (c *Client) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := c.client.Ping(c.newContext(ctx), req)
	return response, apiError("ping MU", err)
}

// Remote messages can contain authentication material. Expose only the error category.
func apiError(operation string, err error) error {
	if err == nil {
		return nil
	}
	var remote twirp.Error
	if errors.As(err, &remote) {
		return fmt.Errorf("%s: %s", operation, remote.Code())
	}
	return fmt.Errorf("%s: transport failure", operation)
}
