// Package domain contains vendor-neutral core types for OTM.
package domain

import "time"

// FlowDirection classifies a flow relative to the LAN boundary.
type FlowDirection string

const (
	DirectionUpload   FlowDirection = "upload"
	DirectionDownload FlowDirection = "download"
	DirectionLocal    FlowDirection = "local"
	DirectionRouter   FlowDirection = "router"
	DirectionUnknown  FlowDirection = "unknown"
)

// Flow is a single network flow record attributed to a LAN IP.
type Flow struct {
	ID                    int64
	ExporterID            string
	ReceivedAt            time.Time
	FlowStartAt           time.Time
	FlowEndAt             time.Time
	SrcIP                 string
	DstIP                 string
	SrcPort               uint16
	DstPort               uint16
	Protocol              uint8
	IngressInterface      uint32
	EgressInterface       uint32
	Bytes                 uint64
	Packets               uint64
	Direction             FlowDirection
	LANIP                 string
	AttributedDeviceID    int64
	AttributionConfidence float64
	DedupeKey             string
}

// Device is a logical network device known to OTM.
type Device struct {
	ID          int64
	DisplayName string
	Notes       string
	Ignored     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// IdentifierKind describes the type of a device identifier.
type IdentifierKind string

const (
	KindMAC         IdentifierKind = "mac"
	KindHostname    IdentifierKind = "hostname"
	KindDUID        IdentifierKind = "duid"
	KindManualLabel IdentifierKind = "manual_label"
)

// DeviceIdentifier is one stable identifier (MAC, hostname, etc.) linked to a device.
type DeviceIdentifier struct {
	ID          int64
	DeviceID    int64
	Kind        IdentifierKind
	Value       string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	Confidence  float64
}

// ObservationSource is the origin of an identity observation.
type ObservationSource string

const (
	SourceDHCP     ObservationSource = "dhcp"
	SourceARP      ObservationSource = "arp"
	SourceNDP      ObservationSource = "ndp"
	SourceManual   ObservationSource = "manual"
	SourceOPNsense ObservationSource = "opnsense_api"
	SourceFake     ObservationSource = "fake"
)

// IdentityObservation is a single sighting of an IP↔MAC↔hostname binding from any source.
type IdentityObservation struct {
	ID            int64
	Source        ObservationSource
	ObservedAt    time.Time
	IP            string
	MAC           string
	Hostname      string
	InterfaceName string
	RawConfidence float64
}

// AddressAssignment records the time interval during which a device owned an IP.
type AddressAssignment struct {
	ID          int64
	DeviceID    int64
	IP          string
	MAC         string
	Source      ObservationSource
	ValidFromAt time.Time
	ValidToAt   time.Time
	LastSeenAt  time.Time
	Confidence  float64
}

// UsageBucket is an aggregated bandwidth record for one device in one time window.
type UsageBucket struct {
	BucketStartAt time.Time
	DeviceID      int64
	InterfaceName string
	Direction     FlowDirection
	Bytes         uint64
	Packets       uint64
	FlowCount     uint64
}

// TalkerRow is one result row from a top-talkers query.
type TalkerRow struct {
	DeviceID    int64
	DisplayName string
	LANIP       string
	Bytes       uint64
	Packets     uint64
	FlowCount   uint64
}

// UsageRow is one result row from a per-device usage query.
type UsageRow struct {
	BucketStartAt time.Time
	DeviceID      int64
	DisplayName   string
	Direction     FlowDirection
	Bytes         uint64
	Packets       uint64
	FlowCount     uint64
}
