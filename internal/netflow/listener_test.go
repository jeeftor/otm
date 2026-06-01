package netflow

import (
	"net"
	"testing"
)

func TestListenerAcceptsAllowedNetFlowV9Packet(t *testing.T) {
	listener := NewListener("127.0.0.1:2055", []string{"10.0.0.1"}, nil)

	packet := make([]byte, 20)
	packet[1] = 9
	listener.observe(&net.UDPAddr{IP: net.ParseIP("10.0.0.1"), Port: 12345}, packet)

	status := listener.Status()
	if status.PacketCount != 1 {
		t.Fatalf("expected one packet, got %d", status.PacketCount)
	}
	if status.InvalidPackets != 0 {
		t.Fatalf("expected no invalid packets, got %d", status.InvalidPackets)
	}
	if status.VersionCounts[9] != 1 {
		t.Fatalf("expected one v9 packet, got %d", status.VersionCounts[9])
	}
}

func TestListenerRejectsUnexpectedExporter(t *testing.T) {
	listener := NewListener("127.0.0.1:2055", []string{"10.0.0.1"}, nil)

	packet := make([]byte, 20)
	packet[1] = 9
	listener.observe(&net.UDPAddr{IP: net.ParseIP("10.0.0.2"), Port: 12345}, packet)

	status := listener.Status()
	if status.PacketCount != 0 {
		t.Fatalf("expected no accepted packets, got %d", status.PacketCount)
	}
	if status.RejectedPackets != 1 {
		t.Fatalf("expected one rejected packet, got %d", status.RejectedPackets)
	}
}
