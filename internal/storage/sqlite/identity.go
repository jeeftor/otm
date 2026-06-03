package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jeeftor/otm/internal/domain"
)

// WriteObservation persists one identity observation.
func (d *DB) WriteObservation(ctx context.Context, obs domain.IdentityObservation) error {
	_, err := d.db.ExecContext(
		ctx, `
		INSERT INTO identity_observations
			(source, observed_ts, ip, mac, hostname, interface_name, raw_confidence)
		VALUES (?,?,?,?,?,?,?)
	`,
		string(obs.Source),
		obs.ObservedAt.Unix(),
		obs.IP, obs.MAC, obs.Hostname, obs.InterfaceName,
		obs.RawConfidence,
	)
	if err != nil {
		return fmt.Errorf("write observation: %w", err)
	}
	return nil
}

// UpsertAddressAssignment inserts or updates a device↔IP address assignment.
func (d *DB) UpsertAddressAssignment(ctx context.Context, a domain.AddressAssignment) error {
	_, err := d.db.ExecContext(
		ctx, `
		INSERT INTO address_assignments
			(device_id, ip, mac, source, valid_from_ts, valid_to_ts, last_seen_ts, confidence)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(device_id, ip, mac, valid_from_ts)
		DO UPDATE SET
			last_seen_ts = excluded.last_seen_ts,
			valid_to_ts  = excluded.valid_to_ts,
			confidence   = excluded.confidence
	`,
		a.DeviceID, a.IP, a.MAC, string(a.Source),
		a.ValidFromAt.Unix(),
		a.ValidToAt.Unix(),
		a.LastSeenAt.Unix(),
		a.Confidence,
	)
	if err != nil {
		return fmt.Errorf("upsert address assignment: %w", err)
	}
	return nil
}

// GetDevices returns all non-ignored devices.
func (d *DB) GetDevices(ctx context.Context) ([]domain.Device, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, display_name, notes, ignored, created_at, updated_at
		FROM devices
		ORDER BY display_name ASC, id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("query devices: %w", err)
	}
	defer rows.Close()

	var result []domain.Device
	for rows.Next() {
		var dev domain.Device
		var createdTS, updatedTS int64
		var ignored int
		if err := rows.Scan(&dev.ID, &dev.DisplayName, &dev.Notes, &ignored, &createdTS, &updatedTS); err != nil {
			return nil, fmt.Errorf("scan device: %w", err)
		}
		dev.Ignored = ignored != 0
		dev.CreatedAt = time.Unix(createdTS, 0).UTC()
		dev.UpdatedAt = time.Unix(updatedTS, 0).UTC()
		result = append(result, dev)
	}
	return result, rows.Err()
}

// GetDeviceIdentifiers returns all identifiers for a device.
func (d *DB) GetDeviceIdentifiers(
	ctx context.Context,
	deviceID int64,
) ([]domain.DeviceIdentifier, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, device_id, kind, value, first_seen_ts, last_seen_ts, confidence
		FROM device_identifiers
		WHERE device_id = ?
		ORDER BY confidence DESC, last_seen_ts DESC
	`, deviceID)
	if err != nil {
		return nil, fmt.Errorf("query identifiers: %w", err)
	}
	defer rows.Close()

	var result []domain.DeviceIdentifier
	for rows.Next() {
		var di domain.DeviceIdentifier
		var firstTS, lastTS int64
		if err := rows.Scan(&di.ID, &di.DeviceID, &di.Kind, &di.Value, &firstTS, &lastTS, &di.Confidence); err != nil {
			return nil, fmt.Errorf("scan identifier: %w", err)
		}
		di.FirstSeenAt = time.Unix(firstTS, 0).UTC()
		di.LastSeenAt = time.Unix(lastTS, 0).UTC()
		result = append(result, di)
	}
	return result, rows.Err()
}

// GetActiveAssignments returns all address assignments with no explicit end time.
func (d *DB) GetActiveAssignments(ctx context.Context) ([]domain.AddressAssignment, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT id, device_id, ip, mac, source, valid_from_ts, valid_to_ts, last_seen_ts, confidence
		FROM address_assignments
		WHERE valid_to_ts = 0
		ORDER BY last_seen_ts DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query active assignments: %w", err)
	}
	defer rows.Close()

	var result []domain.AddressAssignment
	for rows.Next() {
		var a domain.AddressAssignment
		var fromTS, toTS, lastTS int64
		if err := rows.Scan(&a.ID, &a.DeviceID, &a.IP, &a.MAC, &a.Source,
			&fromTS, &toTS, &lastTS, &a.Confidence); err != nil {
			return nil, fmt.Errorf("scan assignment: %w", err)
		}
		a.ValidFromAt = time.Unix(fromTS, 0).UTC()
		if toTS != 0 {
			a.ValidToAt = time.Unix(toTS, 0).UTC()
		}
		a.LastSeenAt = time.Unix(lastTS, 0).UTC()
		result = append(result, a)
	}
	return result, rows.Err()
}

// UpsertDevice ensures a device row exists and returns its ID.
// If a device with a matching display_name already exists it is reused.
func (d *DB) UpsertDevice(ctx context.Context, dev domain.Device) (int64, error) {
	now := time.Now().Unix()
	res, err := d.db.ExecContext(ctx, `
		INSERT INTO devices (display_name, notes, ignored, created_at, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT DO NOTHING
	`, dev.DisplayName, dev.Notes, boolInt(dev.Ignored), now, now)
	if err != nil {
		return 0, fmt.Errorf("upsert device: %w", err)
	}
	id, _ := res.LastInsertId()
	if id != 0 {
		return id, nil
	}
	// Row already existed — look it up.
	var existing int64
	err = d.db.QueryRowContext(
		ctx,
		`SELECT id FROM devices WHERE display_name = ? LIMIT 1`, dev.DisplayName,
	).Scan(&existing)
	return existing, err
}

// UpsertDeviceIdentifier records a device identifier, updating last_seen on conflict.
func (d *DB) UpsertDeviceIdentifier(ctx context.Context, di domain.DeviceIdentifier) error {
	now := time.Now().Unix()
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO device_identifiers (device_id, kind, value, first_seen_ts, last_seen_ts, confidence)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(device_id, kind, value)
		DO UPDATE SET last_seen_ts = excluded.last_seen_ts, confidence = excluded.confidence
	`, di.DeviceID, string(di.Kind), di.Value, now, now, di.Confidence)
	if err != nil {
		return fmt.Errorf("upsert identifier: %w", err)
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
