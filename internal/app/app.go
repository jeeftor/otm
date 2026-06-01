package app

import (
	"context"
	"log/slog"

	"github.com/jeeftor/otm/internal/config"
	"github.com/jeeftor/otm/internal/netflow"
	"github.com/jeeftor/otm/internal/opnsense"
)

// App wires runtime services together.
type App struct {
	Config  config.Config
	NetFlow *netflow.Listener
	Logger  *slog.Logger
}

// New creates an App.
func New(cfg config.Config, logger *slog.Logger) *App {
	return &App{
		Config:  cfg,
		NetFlow: netflow.NewListener(cfg.NetFlowAddr, cfg.NetFlowAllowedExporters, logger),
		Logger:  logger,
	}
}

// OPNsenseReport runs the OPNsense setup validator.
func (a *App) OPNsenseReport(ctx context.Context) opnsense.Report {
	client := opnsense.NewClient(
		a.Config.OPNsenseURL,
		a.Config.OPNsenseAPIKey,
		a.Config.OPNsenseAPISecret,
		a.Config.OPNsenseInsecureSkipVerify,
		a.Config.OPNsenseTimeout,
	)
	return client.Validate(ctx, a.Config.CollectorAdvertiseAddr)
}
