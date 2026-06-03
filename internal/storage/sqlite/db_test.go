package sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeeftor/otm/internal/domain"
	"github.com/jeeftor/otm/internal/storage/sqlite"
)

func openTemp(t *testing.T) *sqlite.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sqlite.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpen(t *testing.T) {
	db := openTemp(t)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestWriteFlowsAndQueryTopTalkers(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC()

	flows := []domain.Flow{
		{
			ExporterID: "test", ReceivedAt: now, FlowStartAt: now.Add(-5 * time.Second), FlowEndAt: now,
			SrcIP: "10.0.0.10", DstIP: "8.8.8.8", Bytes: 1000, Packets: 10,
			Direction: domain.DirectionUpload, LANIP: "10.0.0.10", DedupeKey: "a",
		},
		{
			ExporterID: "test", ReceivedAt: now, FlowStartAt: now.Add(-5 * time.Second), FlowEndAt: now,
			SrcIP: "8.8.8.8", DstIP: "10.0.0.10", Bytes: 5000, Packets: 20,
			Direction: domain.DirectionDownload, LANIP: "10.0.0.10", DedupeKey: "b",
		},
		{
			ExporterID: "test", ReceivedAt: now, FlowStartAt: now.Add(-5 * time.Second), FlowEndAt: now,
			SrcIP: "10.0.0.20", DstIP: "1.1.1.1", Bytes: 2000, Packets: 5,
			Direction: domain.DirectionUpload, LANIP: "10.0.0.20", DedupeKey: "c",
		},
	}
	if err := db.WriteFlows(ctx, flows); err != nil {
		t.Fatalf("WriteFlows: %v", err)
	}

	talkers, err := db.QueryTopTalkers(ctx, domain.TrafficQuery{
		Since: now.Add(-time.Minute), Until: now.Add(time.Minute), Limit: 10,
	})
	if err != nil {
		t.Fatalf("QueryTopTalkers: %v", err)
	}
	if len(talkers) != 2 {
		t.Fatalf("expected 2 talkers, got %d", len(talkers))
	}
	// .10 has 6000 total bytes; .20 has 2000 — .10 should be first.
	if talkers[0].LANIP != "10.0.0.10" {
		t.Errorf("expected 10.0.0.10 first, got %s", talkers[0].LANIP)
	}
}

func TestRollupHour(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Hour)

	flows := []domain.Flow{
		{
			ExporterID: "test", ReceivedAt: now.Add(time.Minute), FlowStartAt: now, FlowEndAt: now.Add(time.Minute),
			Bytes: 512, Packets: 4, Direction: domain.DirectionUpload, LANIP: "10.0.0.5", DedupeKey: "r1",
		},
	}
	if err := db.WriteFlows(ctx, flows); err != nil {
		t.Fatalf("WriteFlows: %v", err)
	}
	if err := db.RollupHour(ctx, now); err != nil {
		t.Fatalf("RollupHour: %v", err)
	}
	// Running again should be idempotent (UPSERT).
	if err := db.RollupHour(ctx, now); err != nil {
		t.Fatalf("RollupHour (second run): %v", err)
	}
}

func TestIdentity(t *testing.T) {
	db := openTemp(t)
	ctx := context.Background()

	devID, err := db.UpsertDevice(ctx, domain.Device{DisplayName: "laptop"})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if devID == 0 {
		t.Fatal("expected non-zero device ID")
	}

	// Idempotent second call should return same ID.
	devID2, err := db.UpsertDevice(ctx, domain.Device{DisplayName: "laptop"})
	if err != nil {
		t.Fatalf("UpsertDevice (2nd): %v", err)
	}
	if devID != devID2 {
		t.Errorf("expected same ID, got %d vs %d", devID, devID2)
	}

	if err := db.WriteObservation(ctx, domain.IdentityObservation{
		Source: domain.SourceFake, ObservedAt: time.Now(), IP: "10.0.0.5", MAC: "aa:bb:cc:dd:ee:ff",
		Hostname: "laptop", RawConfidence: 0.9,
	}); err != nil {
		t.Fatalf("WriteObservation: %v", err)
	}

	devices, err := db.GetDevices(ctx)
	if err != nil {
		t.Fatalf("GetDevices: %v", err)
	}
	if len(devices) != 1 || devices[0].DisplayName != "laptop" {
		t.Errorf("unexpected devices: %+v", devices)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
