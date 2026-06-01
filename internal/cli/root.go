package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/jeeftor/otm/internal/app"
	"github.com/jeeftor/otm/internal/config"
	"github.com/jeeftor/otm/internal/server"
)

// Execute runs the OTM CLI.
func Execute(ctx context.Context, args []string) error {
	root := NewRootCommand(ctx)
	root.SetArgs(args)
	return root.Execute()
}

// NewRootCommand creates the root command.
func NewRootCommand(ctx context.Context) *cobra.Command {
	root := &cobra.Command{
		Use:   "otm",
		Short: "OPNsense Traffic Monitor",
		Long: `OTM is a self-hosted OPNsense Traffic Monitor.

It validates OPNsense API/NetFlow setup and provides NetFlow capture,
decode, and replay utilities for testing.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(ctx, cmd)
		},
	}

	root.PersistentFlags().String("env-file", os.Getenv("OTM_ENV_FILE"), "dotenv file to load")

	root.AddCommand(newServeCommand(ctx))
	root.AddCommand(newValidateConfigCommand())
	root.AddCommand(newValidateOPNsenseCommand())
	root.AddCommand(newVersionCommand())
	root.AddCommand(newNetFlowCommand(ctx))

	return root
}

func newServeCommand(ctx context.Context) *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the web app and NetFlow listener",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(ctx, cmd)
		},
	}
}

func newValidateConfigCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-config",
		Short: "Validate local configuration",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := loadConfigFromCommand(cmd)
			return err
		},
	}
}

func newValidateOPNsenseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "validate-opnsense",
		Short: "Check OPNsense API and NetFlow setup",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfigFromCommand(cmd)
			if err != nil {
				return err
			}
			logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
			application := app.New(cfg, logger)
			report := application.OPNsenseReport(cmd.Context())
			fmt.Printf("configured=%v checks=%d points_to_collector=%v\n", report.Configured, len(report.Checks), report.NetFlow.PointsToCollector)
			for _, check := range report.Checks {
				fmt.Printf("%s ok=%v status=%d error=%s\n", check.Name, check.OK, check.StatusCode, check.Error)
			}
			return nil
		},
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println("otm dev")
		},
	}
}

func runServer(ctx context.Context, cmd *cobra.Command) error {
	cfg, err := loadConfigFromCommand(cmd)
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	application := app.New(cfg, logger)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := application.NetFlow.Run(runCtx); err != nil {
			logger.Error("netflow listener stopped", "error", err)
		}
	}()

	return server.Run(runCtx, application)
}

func loadConfigFromCommand(cmd *cobra.Command) (config.Config, error) {
	envFile, err := cmd.Root().PersistentFlags().GetString("env-file")
	if err != nil {
		return config.Config{}, err
	}
	return config.LoadWithEnvFile(strings.TrimSpace(envFile))
}
