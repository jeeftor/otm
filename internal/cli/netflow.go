package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/jeeftor/otm/internal/netflow"
)

func newNetFlowCommand(ctx context.Context) *cobra.Command {
	command := &cobra.Command{
		Use:   "netflow",
		Short: "NetFlow capture, decode, and replay utilities",
	}
	command.AddCommand(newNetFlowRecordCommand(ctx))
	command.AddCommand(newNetFlowDecodeCommand())
	command.AddCommand(newNetFlowReplayCommand(ctx))
	return command
}

func newNetFlowRecordCommand(ctx context.Context) *cobra.Command {
	var listen string
	var allowExporter string
	var duration time.Duration
	var maxPackets int
	var output string

	command := &cobra.Command{
		Use:   "record",
		Short: "Capture raw UDP NetFlow packets into an .otmcap file",
		RunE: func(_ *cobra.Command, _ []string) error {
			summary, err := netflow.Record(ctx, netflow.RecorderOptions{
				Listen:           listen,
				OutputPath:       output,
				AllowedExporters: splitCSVArg(allowExporter),
				Duration:         duration,
				MaxPackets:       maxPackets,
			})
			if err != nil {
				return err
			}
			fmt.Printf("recorded %d packets to %s\n", summary.PacketCount, output)
			printSummary(summary)
			return nil
		},
	}

	command.Flags().StringVar(&listen, "listen", "0.0.0.0:2055", "UDP listen address")
	command.Flags().
		StringVar(&allowExporter, "allow-exporter", "", "comma-separated exporter IP allowlist")
	command.Flags().DurationVar(&duration, "duration", 5*time.Minute, "capture duration")
	command.Flags().IntVar(&maxPackets, "max-packets", 0, "stop after this many packets")
	command.Flags().StringVar(&output, "out", "netflow.otmcap", "capture output path")
	return command
}

func newNetFlowDecodeCommand() *cobra.Command {
	var input string
	var jsonOutput bool

	command := &cobra.Command{
		Use:     "decode [capture.otmcap]",
		Aliases: []string{"summarize"},
		Short:   "Decode and summarize an .otmcap file",
		Args: func(_ *cobra.Command, args []string) error {
			if input == "" && len(args) == 0 {
				return fmt.Errorf("input capture path is required")
			}
			if len(args) > 1 {
				return fmt.Errorf("accepts at most one capture path")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if input == "" && len(args) > 0 {
				input = args[0]
			}
			summary, err := netflow.DecodeFile(input)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(os.Stdout)
				encoder.SetIndent("", "  ")
				return encoder.Encode(summary)
			}
			printSummary(summary)
			return nil
		},
	}

	command.Flags().StringVar(&input, "in", "", "capture input path")
	command.Flags().BoolVar(&jsonOutput, "json", false, "print JSON summary")
	return command
}

func newNetFlowReplayCommand(ctx context.Context) *cobra.Command {
	var input string
	var target string
	var speed float64

	command := &cobra.Command{
		Use:   "replay [capture.otmcap]",
		Short: "Replay captured NetFlow packets to a UDP target",
		Args: func(_ *cobra.Command, args []string) error {
			if input == "" && len(args) == 0 {
				return fmt.Errorf("input capture path is required")
			}
			if len(args) > 1 {
				return fmt.Errorf("accepts at most one capture path")
			}
			return nil
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if input == "" && len(args) > 0 {
				input = args[0]
			}
			count, err := netflow.Replay(ctx, netflow.ReplayOptions{
				InputPath: input,
				Target:    target,
				Speed:     speed,
			})
			if err != nil {
				return err
			}
			fmt.Printf("replayed %d packets to %s\n", count, target)
			return nil
		},
	}

	command.Flags().StringVar(&input, "in", "", "capture input path")
	command.Flags().StringVar(&target, "target", "127.0.0.1:2055", "UDP replay target")
	command.Flags().Float64Var(&speed, "speed", 1, "replay speed multiplier")
	return command
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
