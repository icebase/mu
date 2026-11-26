package mu

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pb "github.com/icebase/mu/proto/v1"
)

func TestClientFailureDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", "request-123")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal","msg":"database unavailable fixture-token"}`))
	}))
	defer server.Close()
	client := NewClient(server.URL, "fixture-token", "node")
	_, err := client.Ping(context.Background(), &pb.PingRequest{})
	if err == nil {
		t.Fatal("expected RPC failure")
	}
	for _, want := range []string{`"http_status":500`, `"rpc":"Ping"`, `"request_id":"request-123"`, `"elapsed"`, `"code":"internal"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("missing %s in %s", want, logs.String())
		}
	}
	if strings.Contains(logs.String(), "database unavailable") || strings.Contains(logs.String(), "fixture-token") {
		t.Fatal("default logs leaked response text")
	}
}

func TestClientDebugDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		call   func(*Client) error
		want   string
	}{
		{"Ping", 500, `{"code":"internal","msg":"database unavailable fixture-token"}`, func(c *Client) error { _, err := c.Ping(context.Background(), &pb.PingRequest{}); return err }, "database unavailable [REDACTED]"},
		{"GetUsers", 502, `gateway failure fixture-token`, func(c *Client) error { _, err := c.GetUsers(context.Background(), &pb.GetUsersRequest{}); return err }, "gateway failure [REDACTED]"},
		{"UploadTrafficLog", 500, `{"code":"internal","msg":"upload failed fixture-token"}`, func(c *Client) error {
			_, err := c.UploadTrafficLog(context.Background(), &pb.UploadTrafficLogRequest{})
			return err
		}, "upload failed [REDACTED]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			old := slog.Default()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() { slog.SetDefault(old) })
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := NewClient(server.URL, "fixture-token", "node", WithAPIDebug(true))
			if err := tt.call(client); err == nil || strings.Contains(err.Error(), "fixture-token") {
				t.Fatal("expected safe error")
			}
			if !strings.Contains(logs.String(), tt.want) || !strings.Contains(logs.String(), `"rpc":"`+tt.name+`"`) {
				t.Fatalf("missing diagnostic detail: %s", logs.String())
			}
			if strings.Contains(logs.String(), "fixture-token") {
				t.Fatal("API token leaked")
			}
		})
	}
}

func TestDiagnosticTextBoundsAndRedaction(t *testing.T) {
	client := &Client{token: "fixture-token"}
	value := strings.Repeat("x", maxDiagnosticLength-3) + client.token + strings.Repeat("y", maxDiagnosticLength)
	got := client.diagnosticText(value)
	if strings.Contains(got, "fixture") || !strings.HasSuffix(got, "...[truncated]") || len(got) > maxDiagnosticLength+len("...[truncated]") {
		t.Fatal("diagnostic text was not redacted and bounded")
	}
}

func TestClientTimeoutDiagnostics(t *testing.T) {
	var logs bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	client := newTestClient(t, &clientTestService{slow: true})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := client.GetUsers(ctx, &pb.GetUsersRequest{}); err == nil {
		t.Fatal("expected timeout")
	}
	for _, want := range []string{`"context_error":"context deadline exceeded"`, `"transport_failure":true`, `"http_status":0`} {
		if !strings.Contains(logs.String(), want) {
			t.Fatalf("missing %s: %s", want, logs.String())
		}
	}
}

func TestClientDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirected" {
			redirected = true
			_, _ = w.Write([]byte(`{}`))
			return
		}
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	defer server.Close()
	client := NewClient(server.URL, "fixture-token", "node")
	if _, err := client.Ping(context.Background(), &pb.PingRequest{}); err == nil {
		t.Fatal("expected redirect failure")
	}
	if redirected {
		t.Fatal("RPC followed an unexpected redirect")
	}
}
