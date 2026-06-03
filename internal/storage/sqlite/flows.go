package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/jeeftor/otm/internal/domain"
)

// WriteFlows persists a batch of flows in a single transaction.
func (d *DB) WriteFlows(ctx context.Context, flows []domain.Flow) error {
	if len(flows) == 0 {
		return nil
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT OR IGNORE INTO flow_raw (
			exporter_id, received_ts, flow_start_ts, flow_end_ts,
			src_ip, dst_ip, src_port, dst_port, protocol,
			ingress_interface, egress_interface,
			bytes, packets, direction, lan_ip,
			attributed_device_id, attribution_confidence, dedupe_key
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, f := range flows {
		_, err := stmt.ExecContext(
			ctx,
			f.ExporterID,
			f.ReceivedAt.Unix(),
			f.FlowStartAt.Unix(),
			f.FlowEndAt.Unix(),
			f.SrcIP, f.DstIP,
			f.SrcPort, f.DstPort, f.Protocol,
			f.IngressInterface, f.EgressInterface,
			f.Bytes, f.Packets, string(f.Direction), f.LANIP,
			f.AttributedDeviceID, f.AttributionConfidence,
			f.DedupeKey,
		)
		if err != nil {
			return fmt.Errorf("insert flow: %w", err)
		}
	}

	return tx.Commit()
}

// RollupHour aggregates flow_raw rows that fall within the given clock hour into
// usage_bucket_hour, then does the same into usage_bucket_day.
// hour is truncated to the hour boundary internally.
func (d *DB) RollupHour(ctx context.Context, hour time.Time) error {
	h := hour.UTC().Truncate(time.Hour)
	next := h.Add(time.Hour)

	_, err := d.db.ExecContext(ctx, `
		INSERT INTO usage_bucket_hour (bucket_start_ts, device_id, interface_name, direction, bytes, packets, flow_count)
		SELECT
			? AS bucket_start_ts,
			COALESCE(attributed_device_id, 0) AS device_id,
			'' AS interface_name,
			direction,
			SUM(bytes)    AS bytes,
			SUM(packets)  AS packets,
			COUNT(*)      AS flow_count
		FROM flow_raw
		WHERE received_ts >= ? AND received_ts < ?
		  AND direction IN ('upload','download','router','unknown')
		GROUP BY device_id, interface_name, direction
		ON CONFLICT(bucket_start_ts, device_id, interface_name, direction)
		DO UPDATE SET
			bytes      = bytes      + excluded.bytes,
			packets    = packets    + excluded.packets,
			flow_count = flow_count + excluded.flow_count
	`, h.Unix(), h.Unix(), next.Unix())
	if err != nil {
		return fmt.Errorf("rollup hour: %w", err)
	}

	day := h.Truncate(24 * time.Hour)
	dayEnd := day.Add(24 * time.Hour)

	_, err = d.db.ExecContext(ctx, `
		INSERT INTO usage_bucket_day (day_start_ts, device_id, interface_name, direction, bytes, packets, flow_count)
		SELECT
			? AS day_start_ts,
			device_id,
			interface_name,
			direction,
			SUM(bytes),
			SUM(packets),
			SUM(flow_count)
		FROM usage_bucket_hour
		WHERE bucket_start_ts >= ? AND bucket_start_ts < ?
		GROUP BY device_id, interface_name, direction
		ON CONFLICT(day_start_ts, device_id, interface_name, direction)
		DO UPDATE SET
			bytes      = excluded.bytes,
			packets    = excluded.packets,
			flow_count = excluded.flow_count
	`, day.Unix(), day.Unix(), dayEnd.Unix())
	if err != nil {
		return fmt.Errorf("rollup day: %w", err)
	}

	return nil
}

// QueryTopTalkers returns devices ranked by total bytes in the query window.
func (d *DB) QueryTopTalkers(
	ctx context.Context,
	q domain.TrafficQuery,
) ([]domain.TalkerRow, error) {
	if q.Limit <= 0 {
		q.Limit = 20
	}
	rows, err := d.db.QueryContext(ctx, `
		SELECT
			COALESCE(f.attributed_device_id, 0) AS device_id,
			COALESCE(d.display_name, '')         AS display_name,
			f.lan_ip,
			SUM(f.bytes)   AS bytes,
			SUM(f.packets) AS packets,
			COUNT(*)       AS flow_count
		FROM flow_raw f
		LEFT JOIN devices d ON d.id = f.attributed_device_id
		WHERE f.received_ts >= ? AND f.received_ts < ?
		  AND f.direction IN ('upload','download')
		GROUP BY f.lan_ip
		ORDER BY bytes DESC
		LIMIT ?
	`, q.Since.Unix(), q.Until.Unix(), q.Limit)
	if err != nil {
		return nil, fmt.Errorf("query top talkers: %w", err)
	}
	defer rows.Close()

	var result []domain.TalkerRow
	for rows.Next() {
		var r domain.TalkerRow
		if err := rows.Scan(&r.DeviceID, &r.DisplayName, &r.LANIP, &r.Bytes, &r.Packets, &r.FlowCount); err != nil {
			return nil, fmt.Errorf("scan talker row: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// QueryDeviceUsage returns hourly usage rows for one device.
func (d *DB) QueryDeviceUsage(
	ctx context.Context,
	q domain.DeviceUsageQuery,
) ([]domain.UsageRow, error) {
	rows, err := d.db.QueryContext(ctx, `
		SELECT
			h.bucket_start_ts,
			h.device_id,
			COALESCE(d.display_name, '') AS display_name,
			h.direction,
			h.bytes,
			h.packets,
			h.flow_count
		FROM usage_bucket_hour h
		LEFT JOIN devices d ON d.id = h.device_id
		WHERE h.device_id = ? AND h.bucket_start_ts >= ? AND h.bucket_start_ts < ?
		ORDER BY h.bucket_start_ts ASC
	`, q.DeviceID, q.Since.Unix(), q.Until.Unix())
	if err != nil {
		return nil, fmt.Errorf("query device usage: %w", err)
	}
	defer rows.Close()

	var result []domain.UsageRow
	for rows.Next() {
		var r domain.UsageRow
		var ts int64
		if err := rows.Scan(&ts, &r.DeviceID, &r.DisplayName, &r.Direction, &r.Bytes, &r.Packets, &r.FlowCount); err != nil {
			return nil, fmt.Errorf("scan usage row: %w", err)
		}
		r.BucketStartAt = time.Unix(ts, 0).UTC()
		result = append(result, r)
	}
	return result, rows.Err()
}

// FlowStats returns aggregate counts useful for the Overview page.
type FlowStats struct {
	TotalFlows   int64
	TotalBytes   int64
	TotalPackets int64
	DBSizeBytes  int64
}

// Stats returns aggregate counts for the Overview page.
func (d *DB) Stats(ctx context.Context) (FlowStats, error) {
	var s FlowStats
	if err := d.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*), COALESCE(SUM(bytes),0), COALESCE(SUM(packets),0) FROM flow_raw`,
	).Scan(&s.TotalFlows, &s.TotalBytes, &s.TotalPackets); err != nil {
		return s, fmt.Errorf("query flow stats: %w", err)
	}
	if err := d.db.QueryRowContext(
		ctx,
		`SELECT page_count * page_size FROM pragma_page_count(), pragma_page_size()`,
	).Scan(&s.DBSizeBytes); err != nil {
		// Non-fatal; SQLite page_count pragma may not be available in all builds.
		s.DBSizeBytes = -1
	}
	return s, nil
}
