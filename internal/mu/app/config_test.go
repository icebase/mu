package app

import (
	"os"
	"reflect"
	"testing"

	"github.com/icebase/mu/internal/mu/app/tracing"
)

// defaultTracing returns the Tracing config values set as defaults by ConfigFromEnv.
func defaultTracing() tracing.Config {
	return tracing.Config{
		Enabled:     false,
		ServiceName: "mu-service",
		Endpoint:    "localhost:4317",
		Insecure:    true,
		SampleRate:  1.0,
	}
}

func TestConfigFromEnv(t *testing.T) {
	// Save original environment variables to restore later
	originalEnvVars := map[string]string{
		"MU_API_BASE_URL":      os.Getenv("MU_API_BASE_URL"),
		"MU_API_TOKEN":         os.Getenv("MU_API_TOKEN"),
		"MU_NODE_ID":           os.Getenv("MU_NODE_ID"),
		"MU_TROJAN_ADDRS":      os.Getenv("MU_TROJAN_ADDRS"),
		"MU_V2FLY_ADDRS":       os.Getenv("MU_V2FLY_ADDRS"),
		"MU_VLESS_ADDRS":       os.Getenv("MU_VLESS_ADDRS"),
		"MU_VLESS_INBOUND_TAG": os.Getenv("MU_VLESS_INBOUND_TAG"),
	}

	// Restore environment variables after test
	defer func() {
		for k, v := range originalEnvVars {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	tests := []struct {
		name     string
		envVars  map[string]string
		expected *Config
	}{
		{
			name: "Empty environment",
			envVars: map[string]string{
				"MU_API_BASE_URL":      "",
				"MU_API_TOKEN":         "",
				"MU_NODE_ID":           "",
				"MU_TROJAN_ADDRS":      "",
				"MU_V2FLY_ADDRS":       "",
				"MU_VLESS_ADDRS":       "",
				"MU_VLESS_INBOUND_TAG": "",
			},
			expected: &Config{
				MuAPIBaseURL:    "",
				MuAPIToken:      "",
				MuNodeID:        "",
				StateFile:       "data/mu-state.json",
				TrojanAddrs:     nil,
				V2flyAddrs:      nil,
				VlessInboundTag: "vless",
				MetricsEnabled:  true,
				MetricsPort:     "9090",
				Tracing:         defaultTracing(),
			},
		},
		{
			name: "Full configuration",
			envVars: map[string]string{
				"MU_API_BASE_URL":      "https://api.example.com",
				"MU_API_TOKEN":         "test-token",
				"MU_NODE_ID":           "node-1",
				"MU_TROJAN_ADDRS":      "trojan1.example.com,trojan2.example.com",
				"MU_V2FLY_ADDRS":       "v2fly1.example.com,v2fly2.example.com",
				"MU_VLESS_ADDRS":       "vless1.example.com, vless2.example.com,vless1.example.com",
				"MU_VLESS_INBOUND_TAG": "edge-vless",
			},
			expected: &Config{
				MuAPIBaseURL:    "https://api.example.com",
				MuAPIToken:      "test-token",
				MuNodeID:        "node-1",
				StateFile:       "data/mu-state.json",
				TrojanAddrs:     []string{"trojan1.example.com", "trojan2.example.com"},
				V2flyAddrs:      []string{"v2fly1.example.com", "v2fly2.example.com"},
				VlessAddrs:      []string{"vless1.example.com", "vless2.example.com"},
				VlessInboundTag: "edge-vless",
				MetricsEnabled:  true,
				MetricsPort:     "9090",
				Tracing:         defaultTracing(),
			},
		},
		{
			name: "Partial configuration",
			envVars: map[string]string{
				"MU_API_BASE_URL":      "https://api.example.com",
				"MU_API_TOKEN":         "test-token",
				"MU_NODE_ID":           "node-2",
				"MU_TROJAN_ADDRS":      "",
				"MU_V2FLY_ADDRS":       "v2fly.example.com",
				"MU_VLESS_ADDRS":       "",
				"MU_VLESS_INBOUND_TAG": "",
			},
			expected: &Config{
				MuAPIBaseURL:    "https://api.example.com",
				MuAPIToken:      "test-token",
				MuNodeID:        "node-2",
				StateFile:       "data/mu-state.json",
				TrojanAddrs:     nil,
				V2flyAddrs:      []string{"v2fly.example.com"},
				VlessInboundTag: "vless",
				MetricsEnabled:  true,
				MetricsPort:     "9090",
				Tracing:         defaultTracing(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for this test case
			for k, v := range tt.envVars {
				if v == "" {
					os.Unsetenv(k)
				} else {
					os.Setenv(k, v)
				}
			}

			// Call the function under test
			got := ConfigFromEnv()

			// Check if the result matches the expected configuration
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("ConfigFromEnv() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfigAPIDebug(t *testing.T) {
	t.Setenv("MU_API_DEBUG", "true")
	if !ConfigFromEnv().MuAPIDebug {
		t.Fatal("MU_API_DEBUG was not enabled")
	}
	t.Setenv("MU_API_DEBUG", "")
	if ConfigFromEnv().MuAPIDebug {
		t.Fatal("API diagnostics should default to disabled")
	}
}
