package mu

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/twitchtv/twirp"
)

const maxDiagnosticLength = 2048

type diagnosticsKey struct{}

type requestDiagnostics struct {
	started      time.Time
	status       int
	requestID    string
	cfRay        string
	transportErr error
}

func newRequestDiagnostics(ctx context.Context) (context.Context, *requestDiagnostics) {
	details := &requestDiagnostics{started: time.Now()}
	return context.WithValue(ctx, diagnosticsKey{}, details), details
}

// Wrap Do rather than RoundTrip so HTTP-client timeouts are recorded too.
type diagnosticHTTPClient struct{ client *http.Client }

func (c *diagnosticHTTPClient) Do(req *http.Request) (*http.Response, error) {
	response, err := c.client.Do(req)
	if details, ok := req.Context().Value(diagnosticsKey{}).(*requestDiagnostics); ok {
		details.transportErr = err
		if response != nil {
			details.status = response.StatusCode
			details.requestID = response.Header.Get("X-Request-ID")
			details.cfRay = response.Header.Get("CF-Ray")
		}
	}
	return response, err
}

func (c *Client) requestError(ctx context.Context, rpc, operation string, details *requestDiagnostics, err error) error {
	if err == nil {
		return nil
	}
	attrs := []any{
		"rpc", rpc, "node_id", c.diagnosticText(c.nodeID),
		"http_status", details.status, "elapsed", time.Since(details.started),
	}
	if details.requestID != "" {
		attrs = append(attrs, "request_id", c.diagnosticText(details.requestID))
	}
	if details.cfRay != "" {
		attrs = append(attrs, "cf_ray", c.diagnosticText(details.cfRay))
	}
	if ctx.Err() != nil {
		attrs = append(attrs, "context_error", ctx.Err().Error())
	}
	if details.transportErr != nil {
		attrs = append(attrs, "transport_failure", true)
		// URL errors can include credentials and query parameters; only expose the cause.
		if c.apiDebug {
			cause := details.transportErr
			for errors.Unwrap(cause) != nil {
				cause = errors.Unwrap(cause)
			}
			attrs = append(attrs, "transport_error", c.diagnosticText(cause.Error()))
		}
	}
	var remote twirp.Error
	if errors.As(err, &remote) {
		attrs = append(attrs, "code", remote.Code())
		if c.apiDebug {
			attrs = append(attrs, "remote_message", c.diagnosticText(remote.Msg()))
			if body := remote.Meta("body"); body != "" {
				attrs = append(attrs, "response_body", c.diagnosticText(body))
			}
		}
	}
	slog.ErrorContext(ctx, "MU API request failed", attrs...)
	return apiError(operation, err)
}

// Redact the configured API token before truncating, including when it straddles the limit.
func (c *Client) diagnosticText(value string) string {
	if c.token != "" {
		value = strings.ReplaceAll(value, c.token, "[REDACTED]")
	}
	if len(value) > maxDiagnosticLength {
		value = value[:maxDiagnosticLength] + "...[truncated]"
	}
	return value
}
