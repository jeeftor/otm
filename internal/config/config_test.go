package config

import (
	"os"
	"testing"
)

func TestLoadReadsSecretFiles(t *testing.T) {
	keyFile := writeTempSecret(t, "key-value\n")
	secretFile := writeTempSecret(t, "secret-value\n")

	t.Setenv("OTM_OPNSENSE_URL", "https://opnsense.local/")
	t.Setenv("OTM_OPNSENSE_API_KEY_FILE", keyFile)
	t.Setenv("OTM_OPNSENSE_API_SECRET_FILE", secretFile)
	t.Setenv("OTM_NETFLOW_ALLOWED_EXPORTERS", "10.0.0.1, 192.168.2.1")

	cfg, err := Load()
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

func writeTempSecret(t *testing.T, value string) string {
	t.Helper()
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatalf("write temp secret: %v", err)
	}
	return path
}
