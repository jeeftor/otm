package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/jeeftor/otm/internal/app"
)

var setupTemplate = template.Must(template.New("setup").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>OTM Setup</title>
  <style>
    body { font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; margin: 0; background: #f7f8fa; color: #20242a; }
    main { max-width: 1120px; margin: 0 auto; padding: 32px 20px; }
    h1 { margin: 0 0 8px; font-size: 28px; }
    h2 { margin-top: 28px; font-size: 18px; }
    .muted { color: #64707d; }
    .grid { display: grid; gap: 16px; grid-template-columns: repeat(auto-fit, minmax(260px, 1fr)); }
    .panel { background: #fff; border: 1px solid #d9dee5; border-radius: 8px; padding: 16px; }
    .ok { color: #087a31; font-weight: 650; }
    .bad { color: #b42318; font-weight: 650; }
    .warn { color: #9a6700; font-weight: 650; }
    table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #d9dee5; border-radius: 8px; overflow: hidden; }
    th, td { padding: 10px 12px; border-bottom: 1px solid #e8ebef; text-align: left; font-size: 14px; vertical-align: top; }
    th { background: #eef1f5; }
    code { background: #eef1f5; padding: 2px 4px; border-radius: 4px; }
    pre { background: #111827; color: #f9fafb; padding: 12px; border-radius: 8px; overflow: auto; }
  </style>
</head>
<body>
<main>
  <h1>OTM Setup</h1>
  <p class="muted">OPNsense API and NetFlow validation. Generated {{ .GeneratedAt }}</p>

  <div class="grid">
    <section class="panel">
      <h2>NetFlow Listener</h2>
      <p>Status: {{ if .NetFlow.Running }}<span class="ok">running</span>{{ else }}<span class="bad">not running</span>{{ end }}</p>
      <p>Bind: <code>{{ .NetFlow.BindAddr }}</code></p>
      <p>Allowed exporters: <code>{{ .AllowedExporters }}</code></p>
      <p>Packets: <strong>{{ .NetFlow.PacketCount }}</strong></p>
      <p>Last exporter: <code>{{ .NetFlow.LastExporter }}</code></p>
      <p>Last packet: <code>{{ .NetFlow.LastPacketAt }}</code></p>
      {{ if .NetFlow.LastError }}<p class="bad">Error: {{ .NetFlow.LastError }}</p>{{ end }}
    </section>

    <section class="panel">
      <h2>Expected OPNsense Export</h2>
      <p>Collector address OPNsense should target:</p>
      <pre>{{ .Config.CollectorAdvertiseAddr }}</pre>
      <p class="muted">Set <code>OTM_COLLECTOR_ADVERTISE_ADDR</code> to the LAN address and UDP port that OPNsense can reach, for example <code>10.0.0.20:2055</code>.</p>
    </section>

    <section class="panel">
      <h2>OPNsense API</h2>
      <p>Configured: {{ if .OPNsense.Configured }}<span class="ok">yes</span>{{ else }}<span class="bad">no</span>{{ end }}</p>
      <p>URL: <code>{{ .Config.OPNsenseURL }}</code></p>
      <p>NetFlow points to collector: {{ if .OPNsense.NetFlow.PointsToCollector }}<span class="ok">yes</span>{{ else }}<span class="warn">not confirmed</span>{{ end }}</p>
      {{ range .OPNsense.NetFlow.Warnings }}<p class="warn">{{ . }}</p>{{ end }}
    </section>
  </div>

  <h2>OPNsense Endpoint Checks</h2>
  <table>
    <thead><tr><th>Check</th><th>Endpoint</th><th>Status</th><th>Error</th></tr></thead>
    <tbody>
      {{ range .OPNsense.Checks }}
      <tr>
        <td>{{ .Name }}</td>
        <td><code>{{ .Endpoint }}</code></td>
        <td>{{ if .OK }}<span class="ok">OK</span>{{ else }}<span class="bad">failed</span>{{ end }} {{ .StatusCode }}</td>
        <td>{{ .Error }}</td>
      </tr>
      {{ end }}
    </tbody>
  </table>

  <h2>Make OPNsense Send NetFlow Here</h2>
  <pre>NetFlow version: 9
Destination: {{ .Config.CollectorAdvertiseAddr }}
Exporter allowlist in OTM: {{ .AllowedExporters }}</pre>

  <p class="muted">JSON endpoints: <code>/api/status</code>, <code>/api/opnsense/validate</code>, <code>/api/netflow/status</code>.</p>
</main>
</body>
</html>`))

// Run starts the HTTP server.
func Run(ctx context.Context, a *app.App) error {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		status := a.NetFlow.Status()
		if !status.Running {
			http.Error(w, "netflow listener is not running", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready\n"))
	})
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/setup", http.StatusFound)
	})
	mux.HandleFunc("GET /setup", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(a, r) {
			deny(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.Config.OPNsenseTimeout+time.Second)
		defer cancel()
		data := statusResponse{
			GeneratedAt:      time.Now().UTC(),
			Config:           a.Config.Redacted(),
			AllowedExporters: strings.Join(a.Config.NetFlowAllowedExporters, ", "),
			NetFlow:          a.NetFlow.Status(),
			OPNsense:         a.OPNsenseReport(ctx),
		}
		if data.AllowedExporters == "" {
			data.AllowedExporters = "(any exporter accepted; set OTM_NETFLOW_ALLOWED_EXPORTERS)"
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := setupTemplate.Execute(w, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(a, r) {
			deny(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.Config.OPNsenseTimeout+time.Second)
		defer cancel()
		writeJSON(w, statusResponse{
			GeneratedAt:      time.Now().UTC(),
			Config:           a.Config.Redacted(),
			AllowedExporters: strings.Join(a.Config.NetFlowAllowedExporters, ", "),
			NetFlow:          a.NetFlow.Status(),
			OPNsense:         a.OPNsenseReport(ctx),
		})
	})
	mux.HandleFunc("GET /api/netflow/status", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(a, r) {
			deny(w)
			return
		}
		writeJSON(w, a.NetFlow.Status())
	})
	mux.HandleFunc("GET /api/opnsense/validate", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(a, r) {
			deny(w)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), a.Config.OPNsenseTimeout+time.Second)
		defer cancel()
		writeJSON(w, a.OPNsenseReport(ctx))
	})

	server := &http.Server{
		Addr:              a.Config.WebAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	a.Logger.Info("starting web server", "addr", a.Config.WebAddr)
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

type statusResponse struct {
	GeneratedAt      time.Time `json:"generated_at"`
	Config           any       `json:"config"`
	AllowedExporters string    `json:"allowed_exporters"`
	NetFlow          any       `json:"netflow"`
	OPNsense         any       `json:"opnsense"`
}

func authorized(a *app.App, r *http.Request) bool {
	if a.Config.WebAuthToken == "" {
		return true
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == a.Config.WebAuthToken {
		return true
	}
	if cookie, err := r.Cookie("otm_token"); err == nil && cookie.Value == a.Config.WebAuthToken {
		return true
	}
	if r.URL.Query().Get("token") == a.Config.WebAuthToken {
		return true
	}
	return false
}

func deny(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		http.Error(w, fmt.Sprintf("encode JSON: %v", err), http.StatusInternalServerError)
	}
}
