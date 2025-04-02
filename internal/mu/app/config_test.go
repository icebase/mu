package app

import (
	"os"
	"reflect"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	// Save original environment variables to restore later
	originalEnvVars := map[string]string{
		"MU_API_BASE_URL": os.Getenv("MU_API_BASE_URL"),
		"MU_API_TOKEN":    os.Getenv("MU_API_TOKEN"),
		"MU_NODE_ID":      os.Getenv("MU_NODE_ID"),
		"MU_TROJAN_ADDRS": os.Getenv("MU_TROJAN_ADDRS"),
		"MU_V2FLY_ADDRS":  os.Getenv("MU_V2FLY_ADDRS"),
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
				"MU_API_BASE_URL": "",
				"MU_API_TOKEN":    "",
				"MU_NODE_ID":      "",
				"MU_TROJAN_ADDRS": "",
				"MU_V2FLY_ADDRS":  "",
			},
			expected: &Config{
				MuApiBaseURL: "",
				MuApiToken:   "",
				MuNodeID:     "",
				TrojanAddrs:  nil,
				V2flyAddrs:   nil,
			},
		},
		{
			name: "Full configuration",
			envVars: map[string]string{
				"MU_API_BASE_URL": "https://api.example.com",
				"MU_API_TOKEN":    "test-token",
				"MU_NODE_ID":      "node-1",
				"MU_TROJAN_ADDRS": "trojan1.example.com,trojan2.example.com",
				"MU_V2FLY_ADDRS":  "v2fly1.example.com,v2fly2.example.com",
			},
			expected: &Config{
				MuApiBaseURL: "https://api.example.com",
				MuApiToken:   "test-token",
				MuNodeID:     "node-1",
				TrojanAddrs:  []string{"trojan1.example.com", "trojan2.example.com"},
				V2flyAddrs:   []string{"v2fly1.example.com", "v2fly2.example.com"},
			},
		},
		{
			name: "Partial configuration",
			envVars: map[string]string{
				"MU_API_BASE_URL": "https://api.example.com",
				"MU_API_TOKEN":    "test-token",
				"MU_NODE_ID":      "node-2",
				"MU_TROJAN_ADDRS": "",
				"MU_V2FLY_ADDRS":  "v2fly.example.com",
			},
			expected: &Config{
				MuApiBaseURL: "https://api.example.com",
				MuApiToken:   "test-token",
				MuNodeID:     "node-2",
				TrojanAddrs:  nil,
				V2flyAddrs:   []string{"v2fly.example.com"},
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
