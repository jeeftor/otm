package netflow

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sort"
	"time"
)

const captureFormat = "otmcap-jsonl-v1"

// CaptureRecord is one raw UDP packet captured from a NetFlow exporter.
type CaptureRecord struct {
	Format     string    `json:"format"`
	ReceivedAt time.Time `json:"received_at"`
	SourceAddr string    `json:"source_addr"`
	LocalAddr  string    `json:"local_addr"`
	Payload    string    `json:"payload"`
}

// DecodedCaptureRecord is a capture record with decoded packet bytes.
type DecodedCaptureRecord struct {
	ReceivedAt time.Time
	SourceAddr string
	LocalAddr  string
	Payload    []byte
}

// RecorderOptions configures a NetFlow capture.
type RecorderOptions struct {
	Listen           string
	OutputPath       string
	AllowedExporters []string
	Duration         time.Duration
	MaxPackets       int
}

// ReplayOptions configures capture replay.
type ReplayOptions struct {
	InputPath string
	Target    string
	Speed     float64
}

// Summary contains header-level capture decode results.
type Summary struct {
	PacketCount       int                `json:"packet_count"`
	InvalidPackets    int                `json:"invalid_packets"`
	ExporterCounts    map[string]int     `json:"exporter_counts"`
	VersionCounts     map[uint16]int     `json:"version_counts"`
	TemplateFlowsets  int                `json:"template_flowsets"`
	DataFlowsets      int                `json:"data_flowsets"`
	UnknownFlowsets   int                `json:"unknown_flowsets"`
	SequenceGaps      map[string]int     `json:"sequence_gaps"`
	FirstPacketAt     time.Time          `json:"first_packet_at,omitempty"`
	LastPacketAt      time.Time          `json:"last_packet_at,omitempty"`
	PacketDiagnostics []PacketDiagnostic `json:"packet_diagnostics,omitempty"`
	lastSequences     map[string]uint32
}

// PacketDiagnostic captures useful decode detail for one packet.
type PacketDiagnostic struct {
	Index      int       `json:"index"`
	ReceivedAt time.Time `json:"received_at"`
	SourceAddr string    `json:"source_addr"`
	Version    uint16    `json:"version"`
	Count      uint16    `json:"count,omitempty"`
	Sequence   uint32    `json:"sequence,omitempty"`
	SourceID   uint32    `json:"source_id,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// Record captures UDP packets in the OTM capture format.
func Record(ctx context.Context, options RecorderOptions) (Summary, error) {
	if options.Listen == "" {
		options.Listen = "0.0.0.0:2055"
	}
	if options.OutputPath == "" {
		return Summary{}, fmt.Errorf("output path is required")
	}
	if options.Duration <= 0 && options.MaxPackets <= 0 {
		return Summary{}, fmt.Errorf("duration or max packets is required")
	}

	allowlist := make(map[string]struct{}, len(options.AllowedExporters))
	for _, exporter := range options.AllowedExporters {
		allowlist[exporter] = struct{}{}
	}

	addr, err := net.ResolveUDPAddr("udp", options.Listen)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve listen address: %w", err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return Summary{}, fmt.Errorf("listen udp: %w", err)
	}
	defer conn.Close()

	file, err := os.Create(options.OutputPath)
	if err != nil {
		return Summary{}, fmt.Errorf("create capture: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	recordCtx := ctx
	cancel := func() {}
	if options.Duration > 0 {
		recordCtx, cancel = context.WithTimeout(ctx, options.Duration)
	}
	defer cancel()

	go func() {
		<-recordCtx.Done()
		_ = conn.Close()
	}()

	summary := NewSummary()
	buf := make([]byte, 64*1024)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if recordCtx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return summary, nil
			}
			return summary, fmt.Errorf("read udp: %w", err)
		}

		sourceIP := remote.IP.String()
		if len(allowlist) > 0 {
			if _, ok := allowlist[sourceIP]; !ok {
				continue
			}
		}

		payload := make([]byte, n)
		copy(payload, buf[:n])
		record := DecodedCaptureRecord{
			ReceivedAt: time.Now().UTC(),
			SourceAddr: remote.String(),
			LocalAddr:  conn.LocalAddr().String(),
			Payload:    payload,
		}

		if err := WriteCaptureRecord(writer, record); err != nil {
			return summary, err
		}
		AddToSummary(&summary, record, summary.PacketCount+1)

		if options.MaxPackets > 0 && summary.PacketCount >= options.MaxPackets {
			return summary, nil
		}
	}
}

// Replay sends captured packets to a UDP target.
func Replay(ctx context.Context, options ReplayOptions) (int, error) {
	if options.InputPath == "" {
		return 0, fmt.Errorf("input path is required")
	}
	if options.Target == "" {
		return 0, fmt.Errorf("target is required")
	}
	if options.Speed <= 0 {
		options.Speed = 1
	}

	target, err := net.ResolveUDPAddr("udp", options.Target)
	if err != nil {
		return 0, fmt.Errorf("resolve target: %w", err)
	}
	conn, err := net.DialUDP("udp", nil, target)
	if err != nil {
		return 0, fmt.Errorf("dial udp: %w", err)
	}
	defer conn.Close()

	records, err := ReadCaptureFile(options.InputPath)
	if err != nil {
		return 0, err
	}

	var previous time.Time
	for index, record := range records {
		if index > 0 && !previous.IsZero() {
			delay := record.ReceivedAt.Sub(previous)
			if delay > 0 {
				scaled := time.Duration(float64(delay) / options.Speed)
				timer := time.NewTimer(scaled)
				select {
				case <-ctx.Done():
					timer.Stop()
					return index, ctx.Err()
				case <-timer.C:
				}
			}
		}

		if _, err := conn.Write(record.Payload); err != nil {
			return index, fmt.Errorf("write packet %d: %w", index+1, err)
		}
		previous = record.ReceivedAt
	}

	return len(records), nil
}

// DecodeFile summarizes an OTM capture file.
func DecodeFile(path string) (Summary, error) {
	records, err := ReadCaptureFile(path)
	if err != nil {
		return Summary{}, err
	}
	summary := NewSummary()
	for index, record := range records {
		AddToSummary(&summary, record, index+1)
	}
	return summary, nil
}

// NewSummary creates an initialized summary.
func NewSummary() Summary {
	return Summary{
		ExporterCounts: make(map[string]int),
		VersionCounts:  make(map[uint16]int),
		SequenceGaps:   make(map[string]int),
		lastSequences:  make(map[string]uint32),
	}
}

// WriteCaptureRecord writes a decoded packet as one JSONL capture row.
func WriteCaptureRecord(writer io.Writer, record DecodedCaptureRecord) error {
	encoded := CaptureRecord{
		Format:     captureFormat,
		ReceivedAt: record.ReceivedAt.UTC(),
		SourceAddr: record.SourceAddr,
		LocalAddr:  record.LocalAddr,
		Payload:    base64.StdEncoding.EncodeToString(record.Payload),
	}
	if err := json.NewEncoder(writer).Encode(encoded); err != nil {
		return fmt.Errorf("write capture record: %w", err)
	}
	return nil
}

// ReadCaptureFile reads an OTM capture file.
func ReadCaptureFile(path string) ([]DecodedCaptureRecord, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open capture: %w", err)
	}
	defer file.Close()

	var records []DecodedCaptureRecord
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var record CaptureRecord
		if err := json.Unmarshal(line, &record); err != nil {
			return nil, fmt.Errorf("parse capture line %d: %w", lineNumber, err)
		}
		if record.Format != captureFormat {
			return nil, fmt.Errorf(
				"parse capture line %d: unsupported format %q",
				lineNumber,
				record.Format,
			)
		}
		payload, err := base64.StdEncoding.DecodeString(record.Payload)
		if err != nil {
			return nil, fmt.Errorf("decode capture payload line %d: %w", lineNumber, err)
		}
		records = append(records, DecodedCaptureRecord{
			ReceivedAt: record.ReceivedAt,
			SourceAddr: record.SourceAddr,
			LocalAddr:  record.LocalAddr,
			Payload:    payload,
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan capture: %w", err)
	}
	return records, nil
}

// AddToSummary decodes packet header facts into a summary.
func AddToSummary(summary *Summary, record DecodedCaptureRecord, index int) {
	if summary.ExporterCounts == nil {
		summary.ExporterCounts = make(map[string]int)
	}
	if summary.VersionCounts == nil {
		summary.VersionCounts = make(map[uint16]int)
	}
	if summary.SequenceGaps == nil {
		summary.SequenceGaps = make(map[string]int)
	}
	if summary.lastSequences == nil {
		summary.lastSequences = make(map[string]uint32)
	}

	summary.PacketCount++
	summary.ExporterCounts[hostOnly(record.SourceAddr)]++
	if summary.FirstPacketAt.IsZero() || record.ReceivedAt.Before(summary.FirstPacketAt) {
		summary.FirstPacketAt = record.ReceivedAt
	}
	if record.ReceivedAt.After(summary.LastPacketAt) {
		summary.LastPacketAt = record.ReceivedAt
	}

	diagnostic := PacketDiagnostic{
		Index:      index,
		ReceivedAt: record.ReceivedAt,
		SourceAddr: record.SourceAddr,
	}
	defer func() {
		if len(summary.PacketDiagnostics) < 25 {
			summary.PacketDiagnostics = append(summary.PacketDiagnostics, diagnostic)
		}
	}()

	if len(record.Payload) < 2 {
		summary.InvalidPackets++
		diagnostic.Error = "packet too short for NetFlow version"
		return
	}

	version := binary.BigEndian.Uint16(record.Payload[:2])
	diagnostic.Version = version
	summary.VersionCounts[version]++
	if version != 9 {
		summary.InvalidPackets++
		diagnostic.Error = fmt.Sprintf("unsupported NetFlow version %d", version)
		return
	}
	if len(record.Payload) < 20 {
		summary.InvalidPackets++
		diagnostic.Error = "NetFlow v9 packet too short for header"
		return
	}

	count := binary.BigEndian.Uint16(record.Payload[2:4])
	sequence := binary.BigEndian.Uint32(record.Payload[12:16])
	sourceID := binary.BigEndian.Uint32(record.Payload[16:20])
	diagnostic.Count = count
	diagnostic.Sequence = sequence
	diagnostic.SourceID = sourceID

	sequenceKey := fmt.Sprintf("%s/%d", hostOnly(record.SourceAddr), sourceID)
	if previous, ok := summary.lastSequences[sequenceKey]; ok && sequence > previous+1 {
		summary.SequenceGaps[sequenceKey] += int(sequence - previous - 1)
	}
	summary.lastSequences[sequenceKey] = sequence

	offset := 20
	for offset+4 <= len(record.Payload) {
		flowsetID := binary.BigEndian.Uint16(record.Payload[offset : offset+2])
		length := int(binary.BigEndian.Uint16(record.Payload[offset+2 : offset+4]))
		if length < 4 || offset+length > len(record.Payload) {
			summary.InvalidPackets++
			diagnostic.Error = "invalid flowset length"
			return
		}
		switch {
		case flowsetID == 0:
			summary.TemplateFlowsets++
		case flowsetID >= 256:
			summary.DataFlowsets++
		default:
			summary.UnknownFlowsets++
		}
		offset += length
	}
	if offset != len(record.Payload) {
		summary.InvalidPackets++
		diagnostic.Error = "trailing partial flowset"
	}
}

// SortedExporterCounts returns exporter counts in stable order.
func (s Summary) SortedExporterCounts() []string {
	keys := make([]string, 0, len(s.ExporterCounts))
	for key := range s.ExporterCounts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func hostOnly(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err == nil {
		return host
	}
	return addr
}
