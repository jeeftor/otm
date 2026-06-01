package netflow

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"
)

// Status describes what the local NetFlow listener has observed.
type Status struct {
	Running          bool              `json:"running"`
	BindAddr         string            `json:"bind_addr"`
	AllowedExporters []string          `json:"allowed_exporters"`
	PacketCount      uint64            `json:"packet_count"`
	InvalidPackets   uint64            `json:"invalid_packets"`
	RejectedPackets  uint64            `json:"rejected_packets"`
	LastPacketAt     time.Time         `json:"last_packet_at,omitempty"`
	LastExporter     string            `json:"last_exporter,omitempty"`
	VersionCounts    map[uint16]uint64 `json:"version_counts"`
	LastError        string            `json:"last_error,omitempty"`
}

// Listener receives NetFlow UDP packets and records validation status.
type Listener struct {
	addr      string
	allowlist map[string]struct{}
	logger    *slog.Logger

	mu     sync.RWMutex
	status Status
}

// NewListener creates a NetFlow listener.
func NewListener(addr string, allowedExporters []string, logger *slog.Logger) *Listener {
	allowlist := make(map[string]struct{}, len(allowedExporters))
	for _, exporter := range allowedExporters {
		allowlist[exporter] = struct{}{}
	}
	return &Listener{
		addr:      addr,
		allowlist: allowlist,
		logger:    logger,
		status: Status{
			BindAddr:         addr,
			AllowedExporters: allowedExporters,
			VersionCounts:    make(map[uint16]uint64),
		},
	}
}

// Run starts the UDP listener until the context is canceled.
func (l *Listener) Run(ctx context.Context) error {
	addr, err := net.ResolveUDPAddr("udp", l.addr)
	if err != nil {
		l.setError(err)
		return err
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		l.setError(err)
		return err
	}
	defer conn.Close()

	l.mu.Lock()
	l.status.Running = true
	l.status.LastError = ""
	l.mu.Unlock()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	buf := make([]byte, 64*1024)
	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				l.mu.Lock()
				l.status.Running = false
				l.mu.Unlock()
				return nil
			}
			l.setError(err)
			continue
		}
		l.observe(remote, buf[:n])
	}
}

// Status returns a snapshot of listener status.
func (l *Listener) Status() Status {
	l.mu.RLock()
	defer l.mu.RUnlock()
	status := l.status
	status.VersionCounts = make(map[uint16]uint64, len(l.status.VersionCounts))
	for version, count := range l.status.VersionCounts {
		status.VersionCounts[version] = count
	}
	return status
}

func (l *Listener) observe(remote *net.UDPAddr, packet []byte) {
	exporter := remote.IP.String()
	now := time.Now().UTC()

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.allowlist) > 0 {
		if _, ok := l.allowlist[exporter]; !ok {
			l.status.RejectedPackets++
			return
		}
	}

	l.status.PacketCount++
	l.status.LastPacketAt = now
	l.status.LastExporter = exporter

	if len(packet) < 2 {
		l.status.InvalidPackets++
		return
	}

	version := binary.BigEndian.Uint16(packet[:2])
	l.status.VersionCounts[version]++
	if version != 9 {
		l.status.InvalidPackets++
		return
	}
	if len(packet) < 20 {
		l.status.InvalidPackets++
		return
	}
}

func (l *Listener) setError(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.status.LastError = err.Error()
	if l.logger != nil {
		l.logger.Error("netflow listener error", "error", err)
	}
}
