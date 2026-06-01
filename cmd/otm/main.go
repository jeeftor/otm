package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jeeftor/otm/internal/app"
	"github.com/jeeftor/otm/internal/config"
	"github.com/jeeftor/otm/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "otm: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	application := app.New(cfg, logger)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "validate-config":
			_, err := config.Load()
			return err
		case "version":
			fmt.Println("otm dev")
			return nil
		case "validate-opnsense":
			report := application.OPNsenseReport(context.Background())
			fmt.Printf("configured=%v checks=%d points_to_collector=%v\n", report.Configured, len(report.Checks), report.NetFlow.PointsToCollector)
			for _, check := range report.Checks {
				fmt.Printf("%s ok=%v status=%d error=%s\n", check.Name, check.OK, check.StatusCode, check.Error)
			}
			return nil
		case "serve":
		default:
			return fmt.Errorf("unknown command %q", os.Args[1])
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := application.NetFlow.Run(ctx); err != nil {
			logger.Error("netflow listener stopped", "error", err)
		}
	}()

	return server.Run(ctx, application)
}
