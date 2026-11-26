package app

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestV2FlyMetricsEndpoint(t *testing.T) {
	healthy, _, stats := reviewRealVMess(t)
	failing, _, _ := reviewRealVMess(t, true)
	for _, name := range []string{"inbound>>>proxy>>>traffic>>>uplink", "user>>>private@example.test>>>traffic>>>downlink", "outbound>>>invalid"} {
		counter, err := stats.RegisterCounter(name)
		if err != nil {
			t.Fatal(err)
		}
		counter.Set(123)
	}
	address := func(s *v2flySync) string { return strings.TrimSuffix(strings.TrimPrefix(s.key, "v2fly:"), ":"+tag) }
	t.Setenv("MU_V2FLY_METRICS_ENABLED", "true")
	cfg := ConfigFromEnv()
	cfg.MetricsPort = "0"
	cfg.MuNodeID = "metrics-test"
	cfg.StateFile = filepath.Join(t.TempDir(), "state.json")
	cfg.MetricsEnabled = true
	cfg.V2flyAddrs = []string{address(healthy)}
	cfg.VlessAddrs = []string{address(healthy), address(failing)}
	cfg.VlessInboundTag = "vless"
	a := New(cfg)
	if err := a.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	for range 2 {
		rec := httptest.NewRecorder()
		a.metricsServer.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
		body := rec.Body.String()
		for _, expected := range []string{
			"mu_build_info",
			"mu_traffic_sync_duration_seconds",
			"v2ray_up{backend=\"" + address(healthy) + "\"} 1",
			"v2ray_up{backend=\"" + address(failing) + "\"} 0",
			"v2ray_traffic_uplink_bytes_total{backend=\"" + address(healthy) + "\",dimension=\"inbound\",target=\"proxy\"} 123",
		} {
			if rec.Code != 200 || !strings.Contains(body, expected) {
				t.Fatalf("HTTP %d, missing %s", rec.Code, expected)
			}
		}
		if strings.Contains(body, "private@example.test") {
			t.Fatal("exported user identity")
		}
	}
}
