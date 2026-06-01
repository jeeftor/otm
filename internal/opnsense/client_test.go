package opnsense

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateReportsCollectorDestination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "key" || pass != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/api/diagnostics/netflow/getconfig":
			writeJSON(t, w, map[string]any{
				"netflow": map[string]any{
					"enabled":     true,
					"version":     "v9",
					"destination": "10.0.0.20:2055",
				},
			})
		default:
			writeJSON(t, w, map[string]any{"ok": true})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "key", "secret", false, time.Second)
	report := client.Validate(context.Background(), "10.0.0.20:2055")

	if !report.Configured {
		t.Fatal("expected report to be configured")
	}
	if !report.NetFlow.ConfigReadable {
		t.Fatal("expected netflow config readable")
	}
	if !report.NetFlow.PointsToCollector {
		t.Fatalf("expected netflow to point to collector, warnings: %v", report.NetFlow.Warnings)
	}
	if len(report.Checks) != 5 {
		t.Fatalf("expected 5 checks, got %d", len(report.Checks))
	}
	for _, check := range report.Checks {
		if !check.OK {
			t.Fatalf("expected check %s to be OK: %+v", check.Name, check)
		}
	}
}

func TestValidateMissingConfig(t *testing.T) {
	client := NewClient("", "", "", false, time.Second)
	report := client.Validate(context.Background(), "10.0.0.20:2055")

	if report.Configured {
		t.Fatal("expected report to be unconfigured")
	}
	if len(report.Checks) != 1 {
		t.Fatalf("expected one config check, got %d", len(report.Checks))
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode JSON: %v", err)
	}
}
