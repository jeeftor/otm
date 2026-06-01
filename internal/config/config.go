package config

import (
	"bufio"
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
	if err := loadDotEnv(envDefault("OTM_ENV_FILE", ".env")); err != nil {
		return Config{}, err
	}

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

func loadDotEnv(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open env file %s: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		key, value, ok, err := parseDotEnvLine(scanner.Text())
		if err != nil {
			return fmt.Errorf("parse env file %s line %d: %w", path, lineNumber, err)
		}
		if !ok {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from %s: %w", key, path, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan env file %s: %w", path, err)
	}
	return nil
}

func parseDotEnvLine(line string) (string, string, bool, error) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false, nil
	}
	line = strings.TrimPrefix(line, "export ")

	key, value, found := strings.Cut(line, "=")
	if !found {
		return "", "", false, fmt.Errorf("expected KEY=VALUE")
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false, fmt.Errorf("empty key")
	}
	for _, char := range key {
		if !((char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_') {
			return "", "", false, fmt.Errorf("invalid key %q", key)
		}
	}

	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		quote := value[0]
		if (quote == '\'' || quote == '"') && value[len(value)-1] == quote {
			unquoted, err := strconv.Unquote(value)
			if err != nil {
				if quote == '\'' {
					return key, value[1 : len(value)-1], true, nil
				}
				return "", "", false, err
			}
			return key, unquoted, true, nil
		}
	}
	if index := strings.Index(value, " #"); index >= 0 {
		value = strings.TrimSpace(value[:index])
	}
	return key, value, true, nil
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
