package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadReadsSecretFiles(t *testing.T) {
	unsetEnvForTest(t, "OTM_ENV_FILE")
	keyFile := writeTempSecret(t, "key-value\n")
	secretFile := writeTempSecret(t, "secret-value\n")

	t.Setenv("OTM_OPNSENSE_URL", "https://opnsense.local/")
	t.Setenv("OTM_OPNSENSE_API_KEY_FILE", keyFile)
	t.Setenv("OTM_OPNSENSE_API_SECRET_FILE", secretFile)
	t.Setenv("OTM_NETFLOW_ALLOWED_EXPORTERS", "10.0.0.1, 192.168.2.1")

	cfg, err := LoadWithEnvFile(filepath.Join(t.TempDir(), "missing.env"))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.OPNsenseURL != "https://opnsense.local" {
		t.Fatalf("unexpected URL: %q", cfg.OPNsenseURL)
	}
	if cfg.OPNsenseAPIKey != "key-value" {
		t.Fatalf("unexpected key: %q", cfg.OPNsenseAPIKey)
	}
	if cfg.OPNsenseAPISecret != "secret-value" {
		t.Fatalf("unexpected secret: %q", cfg.OPNsenseAPISecret)
	}
	if got := len(cfg.NetFlowAllowedExporters); got != 2 {
		t.Fatalf("expected 2 allowed exporters, got %d", got)
	}
}

func TestLoadReadsDotEnvWithoutOverridingEnvironment(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, []byte(`
OTM_WEB_ADDR=127.0.0.1:18080
OTM_NETFLOW_ALLOWED_EXPORTERS=10.0.0.1,10.0.0.2
OTM_OPNSENSE_TIMEOUT=9s
OTM_LOG_LEVEL=debug
`), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	unsetEnvForTest(t, "OTM_WEB_ADDR")
	unsetEnvForTest(t, "OTM_NETFLOW_ALLOWED_EXPORTERS")
	unsetEnvForTest(t, "OTM_OPNSENSE_TIMEOUT")
	t.Setenv("OTM_LOG_LEVEL", "warn")

	cfg, err := LoadWithEnvFile(envPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.WebAddr != "127.0.0.1:18080" {
		t.Fatalf("unexpected web address: %q", cfg.WebAddr)
	}
	if cfg.OPNsenseTimeout.String() != "9s" {
		t.Fatalf("unexpected timeout: %s", cfg.OPNsenseTimeout)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected environment to override .env, got %q", cfg.LogLevel)
	}
	if got := len(cfg.NetFlowAllowedExporters); got != 2 {
		t.Fatalf("expected two exporters from .env, got %d", got)
	}
}

func writeTempSecret(t *testing.T, value string) string {
	t.Helper()
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("write temp secret: %v", err)
	}
	return path
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	value, existed := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(key, value)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}
