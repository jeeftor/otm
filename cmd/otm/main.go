package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jeeftor/otm/internal/app"
	"github.com/jeeftor/otm/internal/config"
	"github.com/jeeftor/otm/internal/netflow"
	"github.com/jeeftor/otm/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "otm: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h", "help":
			printRootHelp()
			return nil
		case "netflow":
			return runNetFlowCommand(context.Background(), os.Args[2:])
		case "version":
			fmt.Println("otm dev")
			return nil
		}
	}

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

func runNetFlowCommand(ctx context.Context, args []string) error {
	if len(args) == 0 || isHelpArg(args[0]) {
		printNetFlowHelp()
		return nil
	}

	switch args[0] {
	case "record":
		return runNetFlowRecord(ctx, args[1:])
	case "decode", "summarize":
		return runNetFlowDecode(args[1:])
	case "replay":
		return runNetFlowReplay(ctx, args[1:])
	default:
		return fmt.Errorf("unknown netflow command %q", args[0])
	}
}

func runNetFlowRecord(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("otm netflow record", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: otm netflow record [options]")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Capture raw NetFlow UDP packets into an .otmcap file.")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
	}
	listen := flags.String("listen", "0.0.0.0:2055", "UDP listen address")
	allowExporter := flags.String("allow-exporter", "", "comma-separated exporter IP allowlist")
	duration := flags.Duration("duration", 5*time.Minute, "capture duration")
	maxPackets := flags.Int("max-packets", 0, "stop after this many packets")
	out := flags.String("out", "netflow.otmcap", "capture output path")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}

	summary, err := netflow.Record(ctx, netflow.RecorderOptions{
		Listen:           *listen,
		OutputPath:       *out,
		AllowedExporters: splitCSVArg(*allowExporter),
		Duration:         *duration,
		MaxPackets:       *maxPackets,
	})
	if err != nil {
		return err
	}

	fmt.Printf("recorded %d packets to %s\n", summary.PacketCount, *out)
	printSummary(summary)
	return nil
}

func runNetFlowDecode(args []string) error {
	flags := flag.NewFlagSet("otm netflow decode", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: otm netflow decode --in <capture.otmcap> [--json]")
		fmt.Fprintln(flags.Output(), "       otm netflow decode <capture.otmcap> [--json]")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Decode and summarize an OTM NetFlow capture file.")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
	}
	input := flags.String("in", "", "capture input path")
	jsonOut := flags.Bool("json", false, "print JSON summary")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *input == "" && flags.NArg() > 0 {
		*input = flags.Arg(0)
	}
	if *input == "" {
		return fmt.Errorf("input capture path is required")
	}

	summary, err := netflow.DecodeFile(*input)
	if err != nil {
		return err
	}
	if *jsonOut {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}
	printSummary(summary)
	return nil
}

func runNetFlowReplay(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("otm netflow replay", flag.ContinueOnError)
	flags.SetOutput(os.Stdout)
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: otm netflow replay --in <capture.otmcap> --target <host:port> [--speed 10]")
		fmt.Fprintln(flags.Output(), "       otm netflow replay <capture.otmcap> --target <host:port>")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Replay captured NetFlow packets to a UDP target.")
		fmt.Fprintln(flags.Output(), "")
		fmt.Fprintln(flags.Output(), "Options:")
		flags.PrintDefaults()
	}
	input := flags.String("in", "", "capture input path")
	target := flags.String("target", "127.0.0.1:2055", "UDP replay target")
	speed := flags.Float64("speed", 1, "replay speed multiplier")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *input == "" && flags.NArg() > 0 {
		*input = flags.Arg(0)
	}
	if *input == "" {
		return fmt.Errorf("input capture path is required")
	}

	count, err := netflow.Replay(ctx, netflow.ReplayOptions{
		InputPath: *input,
		Target:    *target,
		Speed:     *speed,
	})
	if err != nil {
		return err
	}
	fmt.Printf("replayed %d packets to %s\n", count, *target)
	return nil
}

func printRootHelp() {
	fmt.Println(`OTM - OPNsense Traffic Monitor

Usage:
  otm [command]

Commands:
  serve                 Run the web app and NetFlow listener (default)
  validate-config       Validate local configuration
  validate-opnsense     Check OPNsense API and NetFlow setup
  netflow               NetFlow capture, decode, and replay utilities
  version               Print version
  help                  Show this help

Examples:
  otm
  otm serve
  otm validate-opnsense
  otm netflow record --listen 0.0.0.0:2055 --allow-exporter 192.168.0.1 --duration 5m --out captures/home.otmcap
  otm netflow decode captures/home.otmcap
  otm netflow replay captures/home.otmcap --target 127.0.0.1:2055 --speed 10

Configuration is read from environment variables and .env by default.`)
}

func printNetFlowHelp() {
	fmt.Println(`OTM NetFlow utilities

Usage:
  otm netflow <command> [options]

Commands:
  record       Capture raw UDP NetFlow packets to an .otmcap file
  decode       Decode and summarize an .otmcap file
  summarize    Alias for decode
  replay       Replay an .otmcap file to a UDP target

Examples:
  otm netflow record --listen 0.0.0.0:2055 --allow-exporter 192.168.0.1 --duration 5m --out captures/home.otmcap
  otm netflow decode captures/home.otmcap
  otm netflow decode --in captures/home.otmcap --json
  otm netflow replay captures/home.otmcap --target 127.0.0.1:2055 --speed 10

Use --help with a subcommand for command-specific flags.`)
}

func isHelpArg(arg string) bool {
	return arg == "--help" || arg == "-h" || arg == "help"
}

func printSummary(summary netflow.Summary) {
	fmt.Printf("packets: %d\n", summary.PacketCount)
	fmt.Printf("invalid packets: %d\n", summary.InvalidPackets)
	fmt.Printf("template flowsets: %d\n", summary.TemplateFlowsets)
	fmt.Printf("data flowsets: %d\n", summary.DataFlowsets)
	fmt.Printf("unknown flowsets: %d\n", summary.UnknownFlowsets)
	if !summary.FirstPacketAt.IsZero() {
		fmt.Printf("first packet: %s\n", summary.FirstPacketAt.Format(time.RFC3339Nano))
		fmt.Printf("last packet: %s\n", summary.LastPacketAt.Format(time.RFC3339Nano))
	}
	if len(summary.ExporterCounts) > 0 {
		fmt.Println("exporters:")
		for _, exporter := range summary.SortedExporterCounts() {
			fmt.Printf("  %s: %d\n", exporter, summary.ExporterCounts[exporter])
		}
	}
	if len(summary.VersionCounts) > 0 {
		fmt.Println("versions:")
		for version, count := range summary.VersionCounts {
			fmt.Printf("  %d: %d\n", version, count)
		}
	}
	if len(summary.SequenceGaps) > 0 {
		fmt.Println("sequence gaps:")
		for key, count := range summary.SequenceGaps {
			fmt.Printf("  %s: %d\n", key, count)
		}
	}
}

func splitCSVArg(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
