package config

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains runtime settings for OTM.
type Config struct {
	WebAddr                    string
	WebAuthToken               string
	NetFlowAddr                string
	NetFlowAllowedExporters    []string
	CollectorAdvertiseAddr     string
	DataDir                    string
	LogLevel                   string
	OPNsenseURL                string
	OPNsenseAPIKey             string
	OPNsenseAPISecret          string
	OPNsenseAPIKeyFile         string
	OPNsenseAPISecretFile      string
	OPNsenseInsecureSkipVerify bool
	OPNsenseTimeout            time.Duration
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		WebAddr:                 envDefault("OTM_WEB_ADDR", "127.0.0.1:8080"),
		WebAuthToken:            strings.TrimSpace(os.Getenv("OTM_WEB_AUTH_TOKEN")),
		NetFlowAddr:             envDefault("OTM_NETFLOW_ADDR", "0.0.0.0:2055"),
		CollectorAdvertiseAddr:  strings.TrimSpace(os.Getenv("OTM_COLLECTOR_ADVERTISE_ADDR")),
		DataDir:                 envDefault("OTM_DATA_DIR", "./data"),
		LogLevel:                envDefault("OTM_LOG_LEVEL", "info"),
		OPNsenseURL:             strings.TrimRight(strings.TrimSpace(os.Getenv("OTM_OPNSENSE_URL")), "/"),
		OPNsenseAPIKey:          strings.TrimSpace(os.Getenv("OTM_OPNSENSE_API_KEY")),
		OPNsenseAPISecret:       strings.TrimSpace(os.Getenv("OTM_OPNSENSE_API_SECRET")),
		OPNsenseAPIKeyFile:      strings.TrimSpace(os.Getenv("OTM_OPNSENSE_API_KEY_FILE")),
		OPNsenseAPISecretFile:   strings.TrimSpace(os.Getenv("OTM_OPNSENSE_API_SECRET_FILE")),
		OPNsenseTimeout:         5 * time.Second,
		NetFlowAllowedExporters: splitCSV(os.Getenv("OTM_NETFLOW_ALLOWED_EXPORTERS")),
	}

	if raw := strings.TrimSpace(os.Getenv("OTM_OPNSENSE_TIMEOUT")); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse OTM_OPNSENSE_TIMEOUT: %w", err)
		}
		cfg.OPNsenseTimeout = timeout
	}

	if raw := strings.TrimSpace(os.Getenv("OTM_OPNSENSE_INSECURE_SKIP_VERIFY")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse OTM_OPNSENSE_INSECURE_SKIP_VERIFY: %w", err)
		}
		cfg.OPNsenseInsecureSkipVerify = value
	}

	if cfg.OPNsenseAPIKey == "" && cfg.OPNsenseAPIKeyFile != "" {
		value, err := readSecretFile(cfg.OPNsenseAPIKeyFile)
		if err != nil {
			return Config{}, fmt.Errorf("read OTM_OPNSENSE_API_KEY_FILE: %w", err)
		}
		cfg.OPNsenseAPIKey = value
	}

	if cfg.OPNsenseAPISecret == "" && cfg.OPNsenseAPISecretFile != "" {
		value, err := readSecretFile(cfg.OPNsenseAPISecretFile)
		if err != nil {
			return Config{}, fmt.Errorf("read OTM_OPNSENSE_API_SECRET_FILE: %w", err)
		}
		cfg.OPNsenseAPISecret = value
	}

	if cfg.CollectorAdvertiseAddr == "" {
		cfg.CollectorAdvertiseAddr = inferAdvertiseAddr(cfg.NetFlowAddr)
	}

	return cfg, nil
}

// OPNsenseConfigured reports whether enough API settings are present to call OPNsense.
func (c Config) OPNsenseConfigured() bool {
	return c.OPNsenseURL != "" && c.OPNsenseAPIKey != "" && c.OPNsenseAPISecret != ""
}

// Redacted returns a copy safe for display.
func (c Config) Redacted() Config {
	c.OPNsenseAPIKey = redact(c.OPNsenseAPIKey)
	c.OPNsenseAPISecret = redact(c.OPNsenseAPISecret)
	return c
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}

func readSecretFile(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func inferAdvertiseAddr(bind string) string {
	host, port, err := net.SplitHostPort(bind)
	if err != nil {
		return bind
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		return ":" + port
	}
	return net.JoinHostPort(host, port)
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 6 {
		return "******"
	}
	return value[:3] + "..." + value[len(value)-3:]
}
