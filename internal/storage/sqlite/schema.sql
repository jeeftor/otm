PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS devices (
    id           INTEGER PRIMARY KEY,
    display_name TEXT    NOT NULL DEFAULT '',
    notes        TEXT    NOT NULL DEFAULT '',
    ignored      INTEGER NOT NULL DEFAULT 0,
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS device_identifiers (
    id            INTEGER PRIMARY KEY,
    device_id     INTEGER NOT NULL REFERENCES devices(id),
    kind          TEXT    NOT NULL,
    value         TEXT    NOT NULL,
    first_seen_ts INTEGER NOT NULL,
    last_seen_ts  INTEGER NOT NULL,
    confidence    REAL    NOT NULL DEFAULT 1.0,
    UNIQUE (device_id, kind, value)
);

CREATE INDEX IF NOT EXISTS idx_device_identifiers_value ON device_identifiers(kind, value);

CREATE TABLE IF NOT EXISTS identity_observations (
    id             INTEGER PRIMARY KEY,
    source         TEXT    NOT NULL,
    observed_ts    INTEGER NOT NULL,
    ip             TEXT    NOT NULL DEFAULT '',
    mac            TEXT    NOT NULL DEFAULT '',
    hostname       TEXT    NOT NULL DEFAULT '',
    interface_name TEXT    NOT NULL DEFAULT '',
    raw_confidence REAL    NOT NULL DEFAULT 1.0
);

CREATE INDEX IF NOT EXISTS idx_identity_observations_ip  ON identity_observations(ip);
CREATE INDEX IF NOT EXISTS idx_identity_observations_mac ON identity_observations(mac);
CREATE INDEX IF NOT EXISTS idx_identity_observations_ts  ON identity_observations(observed_ts);

CREATE TABLE IF NOT EXISTS address_assignments (
    id            INTEGER PRIMARY KEY,
    device_id     INTEGER NOT NULL REFERENCES devices(id),
    ip            TEXT    NOT NULL,
    mac           TEXT    NOT NULL DEFAULT '',
    source        TEXT    NOT NULL,
    valid_from_ts INTEGER NOT NULL,
    valid_to_ts   INTEGER NOT NULL DEFAULT 0,
    last_seen_ts  INTEGER NOT NULL,
    confidence    REAL    NOT NULL DEFAULT 1.0,
    UNIQUE (device_id, ip, mac, valid_from_ts)
);

CREATE INDEX IF NOT EXISTS idx_address_assignments_ip  ON address_assignments(ip);
CREATE INDEX IF NOT EXISTS idx_address_assignments_mac ON address_assignments(mac);

CREATE TABLE IF NOT EXISTS flow_raw (
    id                     INTEGER PRIMARY KEY,
    exporter_id            TEXT    NOT NULL DEFAULT '',
    received_ts            INTEGER NOT NULL,
    flow_start_ts          INTEGER NOT NULL,
    flow_end_ts            INTEGER NOT NULL,
    src_ip                 TEXT    NOT NULL DEFAULT '',
    dst_ip                 TEXT    NOT NULL DEFAULT '',
    src_port               INTEGER NOT NULL DEFAULT 0,
    dst_port               INTEGER NOT NULL DEFAULT 0,
    protocol               INTEGER NOT NULL DEFAULT 0,
    ingress_interface      INTEGER NOT NULL DEFAULT 0,
    egress_interface       INTEGER NOT NULL DEFAULT 0,
    bytes                  INTEGER NOT NULL DEFAULT 0,
    packets                INTEGER NOT NULL DEFAULT 0,
    direction              TEXT    NOT NULL DEFAULT 'unknown',
    lan_ip                 TEXT    NOT NULL DEFAULT '',
    attributed_device_id   INTEGER NOT NULL DEFAULT 0,
    attribution_confidence REAL    NOT NULL DEFAULT 0.0,
    dedupe_key             TEXT    NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_flow_raw_received_ts ON flow_raw(received_ts);
CREATE INDEX IF NOT EXISTS idx_flow_raw_lan_ip      ON flow_raw(lan_ip);
CREATE INDEX IF NOT EXISTS idx_flow_raw_device      ON flow_raw(attributed_device_id);

CREATE TABLE IF NOT EXISTS usage_bucket_hour (
    bucket_start_ts INTEGER NOT NULL,
    device_id       INTEGER NOT NULL,
    interface_name  TEXT    NOT NULL DEFAULT '',
    direction       TEXT    NOT NULL,
    bytes           INTEGER NOT NULL DEFAULT 0,
    packets         INTEGER NOT NULL DEFAULT 0,
    flow_count      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_start_ts, device_id, interface_name, direction)
);

CREATE TABLE IF NOT EXISTS usage_bucket_day (
    day_start_ts   INTEGER NOT NULL,
    device_id      INTEGER NOT NULL,
    interface_name TEXT    NOT NULL DEFAULT '',
    direction      TEXT    NOT NULL,
    bytes          INTEGER NOT NULL DEFAULT 0,
    packets        INTEGER NOT NULL DEFAULT 0,
    flow_count     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day_start_ts, device_id, interface_name, direction)
);

CREATE TABLE IF NOT EXISTS exporter_state (
    exporter_id          TEXT    NOT NULL,
    source_id            INTEGER NOT NULL DEFAULT 0,
    last_sequence        INTEGER NOT NULL DEFAULT 0,
    last_template_ts     INTEGER NOT NULL DEFAULT 0,
    last_packet_ts       INTEGER NOT NULL DEFAULT 0,
    sequence_gap_count   INTEGER NOT NULL DEFAULT 0,
    invalid_packet_count INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (exporter_id, source_id)
);
