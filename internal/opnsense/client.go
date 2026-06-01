package opnsense

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client calls the OPNsense API with key/secret Basic Auth.
type Client struct {
	baseURL string
	key     string
	secret  string
	http    *http.Client
}

// NewClient creates an OPNsense API client.
func NewClient(baseURL, key, secret string, insecureSkipVerify bool, timeout time.Duration) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // User opt-in for local/self-signed OPNsense installs.
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		key:     key,
		secret:  secret,
		http: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// CheckResult captures one OPNsense API probe.
type CheckResult struct {
	Name       string `json:"name"`
	Endpoint   string `json:"endpoint"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Report is the OPNsense validation report.
type Report struct {
	Configured  bool            `json:"configured"`
	Checks      []CheckResult   `json:"checks"`
	NetFlow     NetFlowFindings `json:"netflow"`
	GeneratedAt time.Time       `json:"generated_at"`
}

// NetFlowFindings summarizes whether OPNsense appears configured for this collector.
type NetFlowFindings struct {
	ConfigReadable    bool     `json:"config_readable"`
	EnabledHint       string   `json:"enabled_hint,omitempty"`
	VersionHint       string   `json:"version_hint,omitempty"`
	DestinationHint   string   `json:"destination_hint,omitempty"`
	ExpectedCollector string   `json:"expected_collector,omitempty"`
	PointsToCollector bool     `json:"points_to_collector"`
	Warnings          []string `json:"warnings,omitempty"`
}

// Validate checks the OPNsense API endpoints needed by the MVP.
func (c *Client) Validate(ctx context.Context, expectedCollector string) Report {
	report := Report{
		Configured:  c.baseURL != "" && c.key != "" && c.secret != "",
		GeneratedAt: time.Now().UTC(),
		NetFlow: NetFlowFindings{
			ExpectedCollector: expectedCollector,
		},
	}
	if !report.Configured {
		report.Checks = append(report.Checks, CheckResult{
			Name:     "api credentials",
			Endpoint: "environment",
			OK:       false,
			Error:    "set OTM_OPNSENSE_URL plus OTM_OPNSENSE_API_KEY/SECRET or *_FILE variants",
		})
		return report
	}

	endpoints := []struct {
		name     string
		endpoint string
		search   bool
	}{
		{name: "netflow config", endpoint: "/api/diagnostics/netflow/getconfig"},
		{name: "netflow enabled", endpoint: "/api/diagnostics/netflow/is_enabled"},
		{name: "netflow status", endpoint: "/api/diagnostics/netflow/status"},
		{name: "arp table", endpoint: "/api/diagnostics/interface/search_arp", search: true},
		{name: "ndp table", endpoint: "/api/diagnostics/interface/search_ndp", search: true},
	}

	for _, endpoint := range endpoints {
		body, check := c.callEndpoint(ctx, endpoint.name, endpoint.endpoint, endpoint.search)
		report.Checks = append(report.Checks, check)
		if endpoint.name == "netflow config" && check.OK {
			report.NetFlow.ConfigReadable = true
			report.NetFlow = analyzeNetFlowConfig(body, expectedCollector)
		}
	}

	return report
}

func (c *Client) callEndpoint(ctx context.Context, name, endpoint string, search bool) ([]byte, CheckResult) {
	methods := []string{http.MethodGet}
	if search {
		methods = []string{http.MethodPost, http.MethodGet}
	}

	var last CheckResult
	for _, method := range methods {
		body, result := c.do(ctx, method, endpoint, search)
		result.Name = name
		result.Endpoint = endpoint
		if result.OK {
			return body, result
		}
		last = result
	}
	return nil, last
}

func (c *Client) do(ctx context.Context, method, endpoint string, search bool) ([]byte, CheckResult) {
	var body io.Reader
	if method == http.MethodPost && search {
		body = bytes.NewBufferString(`{"rowCount":1,"current":1}`)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return nil, CheckResult{OK: false, Error: err.Error()}
	}
	req.SetBasicAuth(c.key, c.secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, CheckResult{OK: false, Error: err.Error()}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, CheckResult{OK: false, StatusCode: resp.StatusCode, Error: err.Error()}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return raw, CheckResult{OK: false, StatusCode: resp.StatusCode, Error: http.StatusText(resp.StatusCode)}
	}

	return raw, CheckResult{OK: true, StatusCode: resp.StatusCode}
}

func analyzeNetFlowConfig(raw []byte, expectedCollector string) NetFlowFindings {
	findings := NetFlowFindings{
		ConfigReadable:    true,
		ExpectedCollector: expectedCollector,
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		findings.Warnings = append(findings.Warnings, fmt.Sprintf("netflow config was not JSON: %v", err))
		return findings
	}

	values := flatten(payload)
	joined := strings.ToLower(strings.Join(values, " "))

	for _, value := range values {
		lower := strings.ToLower(value)
		if findings.EnabledHint == "" && (strings.Contains(lower, "enabled:true") || strings.Contains(lower, "enabled:1") || strings.Contains(lower, "enable:1")) {
			findings.EnabledHint = value
		}
		if findings.VersionHint == "" && strings.Contains(lower, "version") && (strings.Contains(lower, "9") || strings.Contains(lower, "v9")) {
			findings.VersionHint = value
		}
		if findings.DestinationHint == "" && (strings.Contains(lower, "destination") || strings.Contains(lower, "target") || strings.Contains(lower, "collector")) {
			findings.DestinationHint = value
		}
	}

	if expectedCollector != "" && expectedCollector != ":" {
		host, port := splitHostPortLoose(expectedCollector)
		joinedNoBrackets := strings.ReplaceAll(joined, "[", "")
		joinedNoBrackets = strings.ReplaceAll(joinedNoBrackets, "]", "")
		if host != "" && strings.Contains(joinedNoBrackets, strings.ToLower(host)) {
			findings.PointsToCollector = true
		}
		if port != "" && strings.Contains(joinedNoBrackets, ":"+port) {
			findings.PointsToCollector = true
		}
		if !findings.PointsToCollector {
			findings.Warnings = append(findings.Warnings, "could not find expected collector address/port in NetFlow config")
		}
	} else {
		findings.Warnings = append(findings.Warnings, "collector advertise address is not configured, so destination cannot be verified")
	}

	if findings.VersionHint == "" {
		findings.Warnings = append(findings.Warnings, "could not confirm NetFlow version 9 from config")
	}
	if findings.EnabledHint == "" {
		findings.Warnings = append(findings.Warnings, "could not confirm NetFlow is enabled from config")
	}

	return findings
}

func flatten(value any) []string {
	var out []string
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		switch typed := v.(type) {
		case map[string]any:
			for key, child := range typed {
				next := key
				if prefix != "" {
					next = prefix + "." + key
				}
				walk(next, child)
			}
		case []any:
			for _, child := range typed {
				walk(prefix, child)
			}
		default:
			out = append(out, fmt.Sprintf("%s:%v", prefix, typed))
		}
	}
	walk("", value)
	return out
}

func splitHostPortLoose(value string) (string, string) {
	host := value
	port := ""
	if strings.Contains(value, ":") {
		last := strings.LastIndex(value, ":")
		host = strings.Trim(value[:last], "[]")
		port = value[last+1:]
	}
	return host, port
}
