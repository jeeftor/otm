package fake

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	mathrand "math/rand"
	"time"

	"github.com/jeeftor/otm/internal/domain"
)

// externalIPs is a small set of documentation-range IPs used as fake internet destinations.
var externalIPs = []string{
	"203.0.113.1",
	"203.0.113.2",
	"198.51.100.1",
	"198.51.100.10",
	"192.0.2.1",
	"192.0.2.50",
}

// FlowCollector emits synthetic upload/download flows at a steady interval.
type FlowCollector struct {
	interval time.Duration
	rng      *mathrand.Rand
}

// NewFlowCollector returns a FlowCollector that emits flows every interval.
func NewFlowCollector(interval time.Duration) *FlowCollector {
	seed, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	return &FlowCollector{
		interval: interval,
		//nolint:gosec // Fake data does not need cryptographic randomness.
		rng: mathrand.New(mathrand.NewSource(seed.Int64())),
	}
}

// Run emits fake flows to sink on each tick until ctx is cancelled.
func (c *FlowCollector) Run(ctx context.Context, sink domain.FlowSink) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			flows := c.generate(now)
			if err := sink.WriteFlows(ctx, flows); err != nil {
				return fmt.Errorf("fake flow sink: %w", err)
			}
		}
	}
}

func (c *FlowCollector) generate(now time.Time) []domain.Flow {
	devices := DeviceSeeds()
	flows := make([]domain.Flow, 0, len(devices)*2)
	for _, dev := range devices {
		ext := externalIPs[c.rng.Intn(len(externalIPs))]
		uploadBytes := uint64(c.rng.Intn(50_000) + 1_000)
		downloadBytes := uint64(c.rng.Intn(500_000) + 10_000)

		start := now.Add(-time.Duration(c.rng.Intn(30)+1) * time.Second)
		flows = append(
			flows,
			domain.Flow{
				ExporterID:            "fake",
				ReceivedAt:            now,
				FlowStartAt:           start,
				FlowEndAt:             now,
				SrcIP:                 dev.IP,
				DstIP:                 ext,
				SrcPort:               uint16(c.rng.Intn(60000) + 1024),
				DstPort:               443,
				Protocol:              6,
				Bytes:                 uploadBytes,
				Packets:               uint64(uploadBytes/1000 + 1),
				Direction:             domain.DirectionUpload,
				LANIP:                 dev.IP,
				AttributionConfidence: 1.0,
				DedupeKey:             dedupeKey(dev.IP, ext, now, "up"),
			},
			domain.Flow{
				ExporterID:            "fake",
				ReceivedAt:            now,
				FlowStartAt:           start,
				FlowEndAt:             now,
				SrcIP:                 ext,
				DstIP:                 dev.IP,
				SrcPort:               443,
				DstPort:               uint16(c.rng.Intn(60000) + 1024),
				Protocol:              6,
				Bytes:                 downloadBytes,
				Packets:               uint64(downloadBytes/10_000 + 1),
				Direction:             domain.DirectionDownload,
				LANIP:                 dev.IP,
				AttributionConfidence: 1.0,
				DedupeKey:             dedupeKey(dev.IP, ext, now, "dn"),
			},
		)
	}
	return flows
}

func dedupeKey(lanIP, extIP string, t time.Time, dir string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s|%s|%s|%d|%s", lanIP, extIP, dir, t.UnixNano(), hex.EncodeToString(b))
}
