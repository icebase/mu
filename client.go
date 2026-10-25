package mu

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	pb "github.com/icebase/mu/proto/v1"
	"github.com/twitchtv/twirp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	observer    RequestObserver
}

const instrumentationName = "github.com/icebase/mu"

// RequestObserver is called after every MU API request. code is the Twirp error code when the
// server returned one; err is the sanitized error (nil on success).
type RequestObserver func(rpc string, elapsed time.Duration, code string, err error)

// WithRequestObserver registers a callback used for metrics.
func WithRequestObserver(observer RequestObserver) ClientOption {
	return func(c *Client) { c.observer = observer }
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
	var response *pb.GetUsersResponse
	err := c.call(ctx, "GetUsers", "get MU users", func(ctx context.Context) (err error) {
		response, err = c.client.GetUsers(ctx, req)
		return err
	})
	return response, err
}

func (c *Client) UploadTrafficLog(ctx context.Context, req *pb.UploadTrafficLogRequest) (*pb.UploadTrafficLogResponse, error) {
	req.NodeId = c.nodeID
	if req.UploadAt == 0 {
		req.UploadAt = time.Now().Unix()
	}
	var response *pb.UploadTrafficLogResponse
	err := c.call(ctx, "UploadTrafficLog", "upload MU traffic", func(ctx context.Context) (err error) {
		response, err = c.client.UploadTrafficLog(ctx, req)
		return err
	})
	return response, err
}

func (c *Client) Ping(ctx context.Context, req *pb.PingRequest) (*pb.PingResponse, error) {
	var response *pb.PingResponse
	err := c.call(ctx, "Ping", "ping MU", func(ctx context.Context) (err error) {
		response, err = c.client.Ping(ctx, req)
		return err
	})
	return response, err
}

// call runs one MU API request with a deadline, a client span, diagnostics and the optional observer.
// Only sanitized error details reach spans, logs and the observer; see apiError.
func (c *Client) call(ctx context.Context, rpc, operation string, invoke func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx, span := otel.Tracer(instrumentationName).Start(ctx, "MU."+rpc, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("rpc.system", "twirp"),
			attribute.String("rpc.method", rpc),
			attribute.String("mu.node_id", c.nodeID),
		))
	defer span.End()
	ctx, diagnostics := newRequestDiagnostics(ctx)
	rawErr := invoke(c.newContext(ctx))
	err := c.requestError(ctx, rpc, operation, diagnostics, rawErr)
	elapsed := time.Since(diagnostics.started)
	code := ""
	var remote twirp.Error
	if errors.As(rawErr, &remote) {
		code = string(remote.Code())
	}
	if diagnostics.status != 0 {
		span.SetAttributes(attribute.Int("http.response.status_code", diagnostics.status))
	}
	if diagnostics.requestID != "" {
		span.SetAttributes(attribute.String("mu.request_id", c.diagnosticText(diagnostics.requestID)))
	}
	if diagnostics.cfRay != "" {
		span.SetAttributes(attribute.String("mu.cf_ray", c.diagnosticText(diagnostics.cfRay)))
	}
	if err != nil {
		if code != "" {
			span.SetAttributes(attribute.String("rpc.twirp.error_code", code))
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	if c.observer != nil {
		c.observer(rpc, elapsed, code, err)
	}
	return err
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
