package domain

import (
	"context"
	"time"
)

// FlowSink accepts decoded flow records for storage.
type FlowSink interface {
	WriteFlows(ctx context.Context, flows []Flow) error
}

// FlowCollector runs a traffic source and emits flows to a sink.
type FlowCollector interface {
	Run(ctx context.Context, sink FlowSink) error
}

// IdentitySource returns a snapshot of current IP↔device bindings.
type IdentitySource interface {
	Snapshot(ctx context.Context) ([]IdentityObservation, error)
}

// TrafficQuery parameterises a top-talkers or usage query.
type TrafficQuery struct {
	Since time.Time
	Until time.Time
	Limit int
}

// DeviceUsageQuery parameterises a per-device usage query.
type DeviceUsageQuery struct {
	DeviceID int64
	Since    time.Time
	Until    time.Time
}

// TrafficQueryStore reads aggregated traffic data.
type TrafficQueryStore interface {
	QueryTopTalkers(ctx context.Context, q TrafficQuery) ([]TalkerRow, error)
	QueryDeviceUsage(ctx context.Context, q DeviceUsageQuery) ([]UsageRow, error)
}

// IdentityStore reads and writes device identity records.
type IdentityStore interface {
	WriteObservation(ctx context.Context, obs IdentityObservation) error
	UpsertAddressAssignment(ctx context.Context, a AddressAssignment) error
	GetDevices(ctx context.Context) ([]Device, error)
	GetDeviceIdentifiers(ctx context.Context, deviceID int64) ([]DeviceIdentifier, error)
	GetActiveAssignments(ctx context.Context) ([]AddressAssignment, error)
}

// FlowWriter persists raw flow records and runs rollups.
type FlowWriter interface {
	FlowSink
	RollupHour(ctx context.Context, hour time.Time) error
}
