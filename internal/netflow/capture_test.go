package netflow

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureRoundTripAndDecode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.otmcap")
	record := DecodedCaptureRecord{
		ReceivedAt: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		SourceAddr: "192.168.0.1:9995",
		LocalAddr:  "192.168.0.10:2055",
		Payload:    sampleNetFlowV9Packet(10),
	}

	var buf bytes.Buffer
	if err := WriteCaptureRecord(&buf, record); err != nil {
		t.Fatalf("WriteCaptureRecord returned error: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write capture file: %v", err)
	}

	records, err := ReadCaptureFile(path)
	if err != nil {
		t.Fatalf("ReadCaptureFile returned error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one record, got %d", len(records))
	}
	if !bytes.Equal(records[0].Payload, record.Payload) {
		t.Fatal("payload did not round trip")
	}

	summary, err := DecodeFile(path)
	if err != nil {
		t.Fatalf("DecodeFile returned error: %v", err)
	}
	if summary.PacketCount != 1 {
		t.Fatalf("expected one packet, got %d", summary.PacketCount)
	}
	if summary.VersionCounts[9] != 1 {
		t.Fatalf("expected one v9 packet, got %d", summary.VersionCounts[9])
	}
	if summary.TemplateFlowsets != 1 {
		t.Fatalf("expected one template flowset, got %d", summary.TemplateFlowsets)
	}
}

func TestReplaySendsCapturedPackets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.otmcap")
	var buf bytes.Buffer
	record := DecodedCaptureRecord{
		ReceivedAt: time.Now().UTC(),
		SourceAddr: "192.168.0.1:9995",
		LocalAddr:  "192.168.0.10:2055",
		Payload:    sampleNetFlowV9Packet(10),
	}
	if err := WriteCaptureRecord(&buf, record); err != nil {
		t.Fatalf("WriteCaptureRecord returned error: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write capture file: %v", err)
	}

	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer conn.Close()

	done := make(chan []byte, 1)
	go func() {
		packet := make([]byte, 1500)
		n, _, _ := conn.ReadFrom(packet)
		done <- packet[:n]
	}()

	count, err := Replay(context.Background(), ReplayOptions{
		InputPath: path,
		Target:    conn.LocalAddr().String(),
		Speed:     100,
	})
	if err != nil {
		t.Fatalf("Replay returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one replayed packet, got %d", count)
	}

	select {
	case packet := <-done:
		if !bytes.Equal(packet, record.Payload) {
			t.Fatal("replayed payload mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for replayed packet")
	}
}

func sampleNetFlowV9Packet(sequence uint32) []byte {
	packet := make([]byte, 24)
	packet[1] = 9 // version
	packet[3] = 1 // count
	packet[15] = byte(sequence)
	packet[19] = 7 // source id
	packet[22] = 0 // flowset length high byte
	packet[23] = 4 // flowset length low byte
	return packet
}
