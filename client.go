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
	apiDebug    bool
}

// ClientOption configures MU client diagnostics.
type ClientOption func(*Client)

// WithAPIDebug enables bounded remote error details in failure logs.
func WithAPIDebug(enabled bool) ClientOption {
	return func(c *Client) { c.apiDebug = enabled }
}

func NewClient(addr string, token string, nodeID string, options ...ClientOption) *Client {
	c := &Client{addr: addr, token: token, nodeID: nodeID}
	for _, option := range options {
		option(c)
	}
	c.client = pb.NewMUServiceJSONClient(addr, &diagnosticHTTPClient{
		client: &http.Client{
			Timeout:       10 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, twirp.WithClientPathPrefix(""))
	return c
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
	ctx, diagnostics := newRequestDiagnostics(ctx)
	response, err := c.client.GetUsers(c.newContext(ctx), req)
	return response, c.requestError(ctx, "GetUsers", "get MU users", diagnostics, err)
}

func (c *Client) UploadTrafficLog(ctx context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	req.NodeId = c.nodeID
	if req.UploadAt == 0 {
		req.UploadAt = time.Now().Unix()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx, diagnostics := newRequestDiagnostics(ctx)
	response, err := c.client.UploadTrafficLog(c.newContext(ctx), req)
	return response, c.requestError(ctx, "UploadTrafficLog", "upload MU traffic", diagnostics, err)
}

func (c *Client) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx, diagnostics := newRequestDiagnostics(ctx)
	response, err := c.client.Ping(c.newContext(ctx), req)
	return response, c.requestError(ctx, "Ping", "ping MU", diagnostics, err)
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
