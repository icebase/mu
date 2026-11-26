// Package ssserver implements a Shadowsocks 2022 (SIP023) multi-user server
// that is managed remotely by mu through the SSManagerService gRPC API.
package ssserver

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	MethodAES128 = "2022-blake3-aes-128-gcm"
	MethodAES256 = "2022-blake3-aes-256-gcm"

	keySize128 = 16
	keySize256 = 32
)

// Config holds ss-server settings. ServerPSK and APIToken are secrets and must never be logged.
type Config struct {
	ListenAddr   string
	Method       string
	ServerPSK    string // base64 iPSK
	APIAddr      string
	APIToken     string
	UDPEnabled   bool
	UDPTimeout   time.Duration
	AllowPrivate bool // permit relaying to loopback, private and link-local destinations

	MetricsEnabled bool
	MetricsPort    string
}

// ConfigFromEnv reads SS_* variables:
//   - SS_LISTEN_ADDR, SS_METHOD, SS_SERVER_PSK / SS_SERVER_PSK_FILE
//   - SS_API_ADDR, SS_API_TOKEN
//   - SS_UDP_ENABLED, SS_UDP_TIMEOUT, SS_ALLOW_PRIVATE_NETWORK
//   - SS_METRICS_ENABLED, SS_METRICS_PORT
func ConfigFromEnv() (*Config, error) {
	v := viper.New()
	v.SetDefault("LISTEN_ADDR", ":8388")
	v.SetDefault("METHOD", MethodAES128)
	v.SetDefault("SERVER_PSK", "")
	v.SetDefault("SERVER_PSK_FILE", "")
	v.SetDefault("API_ADDR", "127.0.0.1:9091")
	v.SetDefault("API_TOKEN", "")
	v.SetDefault("UDP_ENABLED", true)
	v.SetDefault("UDP_TIMEOUT", "5m")
	v.SetDefault("ALLOW_PRIVATE_NETWORK", false)
	v.SetDefault("METRICS_ENABLED", true)
	v.SetDefault("METRICS_PORT", "9092")
	v.SetEnvPrefix("SS")
	v.AutomaticEnv()

	config := &Config{
		ListenAddr:     v.GetString("LISTEN_ADDR"),
		Method:         v.GetString("METHOD"),
		ServerPSK:      strings.TrimSpace(v.GetString("SERVER_PSK")),
		APIAddr:        v.GetString("API_ADDR"),
		APIToken:       v.GetString("API_TOKEN"),
		UDPEnabled:     v.GetBool("UDP_ENABLED"),
		AllowPrivate:   v.GetBool("ALLOW_PRIVATE_NETWORK"),
		MetricsEnabled: v.GetBool("METRICS_ENABLED"),
		MetricsPort:    v.GetString("METRICS_PORT"),
	}
	timeout, err := time.ParseDuration(v.GetString("UDP_TIMEOUT"))
	if err != nil {
		return nil, errors.New("SS_UDP_TIMEOUT is not a valid duration")
	}
	config.UDPTimeout = timeout
	if file := v.GetString("SERVER_PSK_FILE"); file != "" {
		if config.ServerPSK != "" {
			return nil, errors.New("set only one of SS_SERVER_PSK and SS_SERVER_PSK_FILE")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, errors.New("cannot read SS_SERVER_PSK_FILE")
		}
		config.ServerPSK = strings.TrimSpace(string(data))
	}
	return config, config.Validate()
}

// KeySize returns the raw key length required by the configured method.
func (c *Config) KeySize() int {
	if c.Method == MethodAES256 {
		return keySize256
	}
	return keySize128
}

// Validate never echoes secret values in its errors.
func (c *Config) Validate() error {
	if c.Method != MethodAES128 && c.Method != MethodAES256 {
		return fmt.Errorf("SS_METHOD must be %s or %s", MethodAES128, MethodAES256)
	}
	if c.ListenAddr == "" || c.APIAddr == "" {
		return errors.New("SS_LISTEN_ADDR and SS_API_ADDR are required")
	}
	if c.ServerPSK == "" {
		return errors.New("SS_SERVER_PSK or SS_SERVER_PSK_FILE is required")
	}
	if _, err := DecodeKey(c.ServerPSK, c.KeySize()); err != nil {
		return fmt.Errorf("server PSK: %w", err)
	}
	if c.APIToken == "" {
		return errors.New("SS_API_TOKEN is required")
	}
	if c.UDPEnabled && c.UDPTimeout < time.Second {
		return errors.New("SS_UDP_TIMEOUT must be at least 1s")
	}
	return nil
}

// DecodeKey decodes a base64 key and enforces the exact length SIP022 requires.
func DecodeKey(encoded string, size int) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("key is not valid base64")
	}
	if len(raw) != size {
		return nil, fmt.Errorf("key must decode to %d bytes", size)
	}
	return raw, nil
}
