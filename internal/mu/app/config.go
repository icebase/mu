package app

import (
	"strings"

	"github.com/icebase/mu/internal/mu/app/tracing"
	"github.com/spf13/viper"
)

type Config struct {
	MuAPIBaseURL string
	MuAPIToken   string
	MuAPIDebug   bool
	MuNodeID     string
	StateFile    string

	TrojanAddrs []string
	V2flyAddrs  []string

	// Metrics 配置
	MetricsEnabled bool
	MetricsPort    string

	// Tracing 配置
	Tracing tracing.Config
}

// ConfigFromEnv initializes and returns a Config object with values from environment variables.
// Environment variable names:
// - MU_API_BASE_URL: API base URL
// - MU_API_TOKEN: API token
// - MU_NODE_ID: Node identifier
// - MU_TROJAN_ADDRS: Comma-separated list of Trojan addresses
// - MU_V2FLY_ADDRS: Comma-separated list of V2Fly addresses
func ConfigFromEnv() *Config {
	v := viper.New()

	// Set default values
	v.SetDefault("API_BASE_URL", "")
	v.SetDefault("API_TOKEN", "")
	v.SetDefault("API_DEBUG", false)
	v.SetDefault("NODE_ID", "")
	v.SetDefault("TROJAN_ADDRS", "")
	v.SetDefault("V2FLY_ADDRS", "")
	v.SetDefault("STATE_FILE", "data/mu-state.json")

	// Metrics defaults
	v.SetDefault("METRICS_ENABLED", true)
	v.SetDefault("METRICS_PORT", "9090")

	// Tracing defaults
	v.SetDefault("TRACING_ENABLED", false)
	v.SetDefault("TRACING_SERVICE_NAME", "mu-service")
	v.SetDefault("TRACING_ENDPOINT", "localhost:4317")
	v.SetDefault("TRACING_INSECURE", true)
	v.SetDefault("TRACING_SAMPLE_RATE", 1.0)

	// Set environment variables prefix
	v.SetEnvPrefix("MU")

	// Automatically bind environment variables to config keys
	v.AutomaticEnv()

	// Replace dots in env vars with underscores
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Create a config struct with values from environment variables
	config := &Config{
		MuAPIBaseURL:   v.GetString("API_BASE_URL"),
		MuAPIToken:     v.GetString("API_TOKEN"),
		MuAPIDebug:     v.GetBool("API_DEBUG"),
		MuNodeID:       v.GetString("NODE_ID"),
		StateFile:      v.GetString("STATE_FILE"),
		MetricsEnabled: v.GetBool("METRICS_ENABLED"),
		MetricsPort:    v.GetString("METRICS_PORT"),
		Tracing: tracing.Config{
			Enabled:     v.GetBool("TRACING_ENABLED"),
			ServiceName: v.GetString("TRACING_SERVICE_NAME"),
			Endpoint:    v.GetString("TRACING_ENDPOINT"),
			Insecure:    v.GetBool("TRACING_INSECURE"),
			SampleRate:  v.GetFloat64("TRACING_SAMPLE_RATE"),
		},
	}

	// Handle comma-separated string lists
	if trojanAddrsStr := v.GetString("TROJAN_ADDRS"); trojanAddrsStr != "" {
		config.TrojanAddrs = parseAddresses(trojanAddrsStr)
	}

	if v2flyAddrsStr := v.GetString("V2FLY_ADDRS"); v2flyAddrsStr != "" {
		config.V2flyAddrs = parseAddresses(v2flyAddrsStr)
	}

	return config
}

func parseAddresses(value string) []string {
	var addresses []string
	seen := make(map[string]bool)
	for _, entry := range strings.Split(value, ",") {
		address := strings.TrimSpace(entry)
		if address != "" && !seen[address] {
			addresses = append(addresses, address)
			seen[address] = true
		}
	}
	return addresses
}
