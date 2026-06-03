// Package fake provides deterministic synthetic data sources for development and testing.
package fake

import (
	"context"
	"time"

	"github.com/jeeftor/otm/internal/domain"
)

// knownDevices is a fixed set of fake LAN devices.
var knownDevices = []struct {
	Name string
	MAC  string
	IP   string
}{
	{"laptop", "aa:bb:cc:11:22:33", "10.0.0.10"},
	{"desktop", "aa:bb:cc:44:55:66", "10.0.0.11"},
	{"phone", "aa:bb:cc:77:88:99", "10.0.0.12"},
	{"nas", "aa:bb:cc:aa:bb:cc", "10.0.0.20"},
	{"smart-tv", "aa:bb:cc:dd:ee:ff", "10.0.0.30"},
}

// IdentitySource is a synthetic IdentitySource that returns a fixed set of devices.
type IdentitySource struct{}

// NewIdentitySource returns an IdentitySource backed by fake device data.
func NewIdentitySource() *IdentitySource {
	return &IdentitySource{}
}

// Snapshot returns one observation per fake device.
func (s *IdentitySource) Snapshot(_ context.Context) ([]domain.IdentityObservation, error) {
	now := time.Now().UTC()
	obs := make([]domain.IdentityObservation, 0, len(knownDevices))
	for _, d := range knownDevices {
		obs = append(obs, domain.IdentityObservation{
			Source:        domain.SourceFake,
			ObservedAt:    now,
			IP:            d.IP,
			MAC:           d.MAC,
			Hostname:      d.Name,
			InterfaceName: "em0",
			RawConfidence: 1.0,
		})
	}
	return obs, nil
}

// DeviceSeeds returns the fixed device list so other fake components can reference them.
func DeviceSeeds() []struct {
	Name string
	MAC  string
	IP   string
} {
	return knownDevices
}
