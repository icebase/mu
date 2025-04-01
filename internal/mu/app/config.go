package app

import (
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	MuApiBaseURL string
	MuApiToken   string

	TrojanAddrs []string
	V2flyAddrs  []string
}

// ConfigFromEnv initializes and returns a Config object with values from environment variables.
// Environment variable names:
// - MU_API_BASE_URL: API base URL
// - MU_API_TOKEN: API token
// - MU_TROJAN_ADDRS: Comma-separated list of Trojan addresses
// - MU_V2FLY_ADDRS: Comma-separated list of V2Fly addresses
func ConfigFromEnv() *Config {
	v := viper.New()
	
	// Set default values
	v.SetDefault("API_BASE_URL", "")
	v.SetDefault("API_TOKEN", "")
	v.SetDefault("TROJAN_ADDRS", "")
	v.SetDefault("V2FLY_ADDRS", "")
	
	// Set environment variables prefix
	v.SetEnvPrefix("MU")
	
	// Automatically bind environment variables to config keys
	v.AutomaticEnv()
	
	// Replace dots in env vars with underscores
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	
	// Create a config struct with values from environment variables
	config := &Config{
		MuApiBaseURL: v.GetString("API_BASE_URL"),
		MuApiToken:   v.GetString("API_TOKEN"),
	}
	
	// Handle comma-separated string lists
	if trojanAddrsStr := v.GetString("TROJAN_ADDRS"); trojanAddrsStr != "" {
		config.TrojanAddrs = strings.Split(trojanAddrsStr, ",")
	}
	
	if v2flyAddrsStr := v.GetString("V2FLY_ADDRS"); v2flyAddrsStr != "" {
		config.V2flyAddrs = strings.Split(v2flyAddrsStr, ",")
	}
	
	return config
}
