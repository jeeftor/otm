package config

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
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

// Load reads configuration through Viper using defaults, .env, and environment variables.
func Load() (Config, error) {
	envFile := strings.TrimSpace(os.Getenv("OTM_ENV_FILE"))
	if envFile == "" {
		envFile = ".env"
	}
	return LoadWithEnvFile(envFile)
}

// LoadWithEnvFile reads configuration from a specific dotenv file path.
func LoadWithEnvFile(envFile string) (Config, error) {
	v := newViper()
	if err := mergeEnvFile(v, envFile); err != nil {
		return Config{}, err
	}

	cfg := Config{
		WebAddr:                    v.GetString("web.addr"),
		WebAuthToken:               v.GetString("web.auth_token"),
		NetFlowAddr:                v.GetString("netflow.addr"),
		NetFlowAllowedExporters:    splitCSV(v.GetString("netflow.allowed_exporters")),
		CollectorAdvertiseAddr:     v.GetString("collector.advertise_addr"),
		DataDir:                    v.GetString("data.dir"),
		LogLevel:                   v.GetString("log.level"),
		OPNsenseURL:                strings.TrimRight(v.GetString("opnsense.url"), "/"),
		OPNsenseAPIKey:             v.GetString("opnsense.api_key"),
		OPNsenseAPISecret:          v.GetString("opnsense.api_secret"),
		OPNsenseAPIKeyFile:         v.GetString("opnsense.api_key_file"),
		OPNsenseAPISecretFile:      v.GetString("opnsense.api_secret_file"),
		OPNsenseInsecureSkipVerify: v.GetBool("opnsense.insecure_skip_verify"),
		OPNsenseTimeout:            v.GetDuration("opnsense.timeout"),
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

func newViper() *viper.Viper {
	v := viper.New()
	v.SetConfigType("env")
	v.SetEnvPrefix("OTM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	setDefaultAndBind(v, "web.addr", "127.0.0.1:8080", "OTM_WEB_ADDR")
	setDefaultAndBind(v, "web.auth_token", "", "OTM_WEB_AUTH_TOKEN")
	setDefaultAndBind(v, "netflow.addr", "0.0.0.0:2055", "OTM_NETFLOW_ADDR")
	setDefaultAndBind(v, "netflow.allowed_exporters", "", "OTM_NETFLOW_ALLOWED_EXPORTERS")
	setDefaultAndBind(v, "collector.advertise_addr", "", "OTM_COLLECTOR_ADVERTISE_ADDR")
	setDefaultAndBind(v, "data.dir", "./data", "OTM_DATA_DIR")
	setDefaultAndBind(v, "log.level", "info", "OTM_LOG_LEVEL")
	setDefaultAndBind(v, "opnsense.url", "", "OTM_OPNSENSE_URL")
	setDefaultAndBind(v, "opnsense.api_key", "", "OTM_OPNSENSE_API_KEY")
	setDefaultAndBind(v, "opnsense.api_secret", "", "OTM_OPNSENSE_API_SECRET")
	setDefaultAndBind(v, "opnsense.api_key_file", "", "OTM_OPNSENSE_API_KEY_FILE")
	setDefaultAndBind(v, "opnsense.api_secret_file", "", "OTM_OPNSENSE_API_SECRET_FILE")
	setDefaultAndBind(v, "opnsense.insecure_skip_verify", false, "OTM_OPNSENSE_INSECURE_SKIP_VERIFY")
	setDefaultAndBind(v, "opnsense.timeout", 5*time.Second, "OTM_OPNSENSE_TIMEOUT")

	return v
}

func setDefaultAndBind(v *viper.Viper, key string, value any, env string) {
	v.SetDefault(key, value)
	_ = v.BindEnv(key, env)
}

func mergeEnvFile(v *viper.Viper, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	values, err := gotenv.Read(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("parse env file %s: %w", path, err)
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set env %s from %s: %w", key, path, err)
		}
	}
	return nil
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
