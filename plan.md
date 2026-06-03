# OTM Implementation Plan

## Working Assumption

OTM is a self-hosted OPNsense Traffic Monitor focused on answering one question well:

> Which device is using my internet bandwidth, right now and over time?

The MVP will be a standalone Go web application, with the codebase shaped so it can later be packaged as an OPNsense plugin. OPNsense will be one adapter that supplies identity and router metadata. NetFlow v9 will be the first traffic source. SQLite will be the MVP database, used as a bounded local store for identity, recent raw flow detail, and hourly/daily rollups.

Go is a strong fit because it can produce a mostly static FreeBSD/amd64 binary for OPNsense while also producing Linux container builds for standalone self-hosting. The long-term packaging goal is one core service binary with two deployment modes:

```text
standalone mode: Docker/binary on a LAN host
plugin mode: FreeBSD binary managed by OPNsense configd/rc scripts
```

The repository remote is expected to be:

```text
git@github.com:jeeftor/otm.git
```

## Non-Goals For MVP

- Billing-grade traffic accounting.
- DPI, app/category classification, or content inspection.
- Automatic firewall policy enforcement.
- Multi-router clustering.
- Long-term raw flow warehousing.
- Full OPNsense plugin packaging in the first milestone.
- Mandatory Grafana, Prometheus, TimescaleDB, or ClickHouse.
- Fully solving all IPv6 identity edge cases before the IPv4-first workflow is useful.

## Core Product Shape

The MVP web GUI should be an operational console with these screens:

- **Setup / Onboarding**: OPNsense API status, NetFlow listener status, exact OPNsense NetFlow setup guidance, first valid packet confirmation, identity source status.
- **Overview**: collector health, flows per minute, devices seen, unknown devices, top talkers now, top talkers today, database size, last aggregation run.
- **Devices**: known devices, unknown devices, current IPs, MACs, hostnames, last seen, confidence, rename, merge, ignore.
- **Device Detail**: last 24 hours, hourly usage, daily usage, known identifiers, address assignment timeline, recent top external destinations.
- **Live Traffic**: rolling windows for last 1m, 5m, 15m, and 1h, with clear freshness indicators.
- **Reports**: hourly/daily summaries, per-device usage, top devices, top external destinations/services, CSV/JSON export.
- **Diagnostics**: sanitized setup checks, packet decode status, invalid/drop counts, DB write status, retention status, clock/timezone info, copyable diagnostics bundle.

The first useful milestone is:

```text
fake NetFlow + fake identity source -> SQLite -> web UI top talkers
```

The first real-router milestone is:

```text
OPNsense NetFlow v9 + OPNsense API identity -> SQLite -> today's usage by device
```

## Architecture

Build a collector-driven core with replaceable adapters.

Suggested layout:

```text
cmd/otm
cmd/otmctl
internal/config
internal/domain
internal/collectors
internal/collectors/netflowv9
internal/collectors/opnsense
internal/collectors/leasefile
internal/identity
internal/storage
internal/storage/sqlite
internal/app
internal/web
internal/migrations
packaging/docker
packaging/opnsense
```

Core domain types should be vendor-neutral:

- `Flow`
- `Device`
- `DeviceIdentifier`
- `IdentityObservation`
- `AddressAssignment`
- `Interface`
- `UsageBucket`
- `ExporterStatus`

Core interfaces should be narrow:

```go
type FlowCollector interface {
    Run(ctx context.Context, sink FlowSink) error
}

type FlowSink interface {
    RecordFlows(ctx context.Context, flows []domain.Flow) error
}

type IdentitySource interface {
    Snapshot(ctx context.Context) ([]domain.IdentityObservation, error)
}

type TrafficQueryStore interface {
    QueryTopTalkers(ctx context.Context, q TrafficQuery) ([]TalkerRow, error)
    QueryDeviceUsage(ctx context.Context, q DeviceUsageQuery) ([]UsageRow, error)
}
```

Avoid:

- Web handlers querying SQLite directly.
- NetFlow packet decoders writing directly to the database.
- OPNsense-specific domain models leaking outside the OPNsense adapter.
- A giant global `OPNsenseClient` interface that exposes unrelated router APIs.

## Plugin Readiness

The core implementation should remain independent of Docker, systemd, and OPNsense plugin mechanics. Packaging should wrap the same service binary.

OPNsense plugin implications:

- OPNsense plugins use MVC/API frontend pieces and `configd` actions to control privileged backend behavior.
- Service start/stop/restart should be exposed through `configd` actions and standard rc service scripts rather than arbitrary shell execution from PHP.
- Plugin configuration should be stored in OPNsense's configuration model and rendered into an OTM config file by templates.
- The Go binary should support explicit subcommands useful from `configd`, such as `run`, `validate-config`, `migrate`, `backup`, `restore`, and `version`.
- Runtime paths must be configurable so plugin mode can use FreeBSD/OPNsense paths and standalone mode can use container paths.

Suggested plugin-mode paths:

```text
/usr/local/bin/otm
/usr/local/etc/otm/config.yaml
/var/db/otm/otm.sqlite
/var/log/otm/otm.log
/usr/local/etc/rc.d/otm
/usr/local/opnsense/service/conf/actions.d/actions_otm.conf
```

Suggested plugin package skeleton:

```text
packaging/opnsense/
  src/opnsense/mvc/app/controllers/OPNsense/OTM/
  src/opnsense/mvc/app/models/OPNsense/OTM/
  src/opnsense/mvc/app/views/OPNsense/OTM/
  src/opnsense/service/conf/actions.d/actions_otm.conf
  src/opnsense/service/templates/OPNsense/OTM/
  src/etc/rc.d/otm
```

The plugin should initially be a thin management wrapper:

- Configure OTM.
- Start/stop/restart OTM.
- Show service status.
- Link to the OTM web UI.
- Display basic diagnostics from `otmctl` or an authenticated local endpoint.

The plugin should not duplicate the full OTM web GUI in PHP/Volt. Keep the primary UI in the Go app so standalone and plugin deployments share the same product.

## OPNsense Integration

The app should validate OPNsense setup rather than leaving users to guess.

Read-only MVP checks:

- API authentication works.
- OPNsense system/version info is reachable.
- ARP endpoint is reachable.
- NDP endpoint is reachable.
- DHCP/Kea lease endpoint is reachable when available.
- NetFlow config can be read.
- NetFlow status can be read.
- Insight/network metadata can be read if available.

Expected useful endpoints include:

```text
/api/diagnostics/netflow/getconfig
/api/diagnostics/netflow/is_enabled
/api/diagnostics/netflow/status
/api/diagnostics/networkinsight/get_interfaces
/api/diagnostics/interface/search_arp
/api/diagnostics/interface/search_ndp
```

The app may later offer opt-in NetFlow configuration, but MVP should not require write privileges. If write setup is added, use a separate explicit write credential or a one-time setup mode.

Recommended NetFlow settings:

- NetFlow version 9, because v5 does not support IPv6.
- Export only the interfaces needed for internet accounting.
- Avoid ingress/egress double-counting.
- Destination points to the app's UDP listener.
- The app only accepts NetFlow from the configured OPNsense exporter IP.

## Device Identity Model

Do not treat IP address as device identity. Store identity as time-aware evidence.

Sources:

- DHCPv4 leases.
- Kea/DNSMasq/ISC lease data where available.
- ARP table.
- NDP table.
- Hostnames.
- Manual labels.
- Future AP/switch/controller sources.

Reuse concepts from `jeeftor/dhcp-adguard-sync`:

- ISC/DNSMasq lease parsing.
- NDP polling for IPv6 addresses.
- File watch/debounce behavior for local lease files.
- OPNsense deployment lessons.
- Dry-run/logging patterns.

Do not reuse its one-way sync architecture as the core design. OTM is multi-source monitoring and attribution, not source-to-target synchronization.

Minimum identity schema:

```text
devices
  id
  display_name
  notes
  ignored
  created_at
  updated_at

device_identifiers
  id
  device_id
  kind              -- mac, hostname, duid, manual_label
  value
  first_seen_ts
  last_seen_ts
  confidence

identity_observations
  id
  source            -- dhcp, arp, ndp, manual, opnsense_api
  observed_ts
  ip
  mac
  hostname
  interface_name
  raw_confidence

address_assignments
  id
  device_id
  ip
  mac
  source
  valid_from_ts
  valid_to_ts
  last_seen_ts
  confidence
```

Rules:

- Manual labels override display names, not raw observations.
- Merges preserve history.
- Auto-merge only on strong evidence, such as same MAC on same interface/VLAN.
- Do not auto-merge based only on hostname or IP reuse.
- Show attribution provenance and confidence in the UI.

## Traffic Scope

The primary metric is internet bandwidth, not all LAN chatter.

Classify flows:

```text
local -> external = upload
external -> local = download
local -> local = local, ignored by default
router -> external = router traffic, separate bucket
link-local/multicast = ignored by default
unknown = unattributed bucket
```

IPv6 should be supported, but the MVP can be IPv4-first. Local IPv6 link-local chatter should not affect internet bandwidth reporting. Global IPv6 internet flows should be attributed when NDP/DHCP evidence is available.

## NetFlow Ingestion

Use NetFlow v9 as the first real traffic source.

Separate:

- UDP socket listener.
- Exporter allowlist.
- NetFlow v9 template cache.
- Packet decoder.
- Domain flow mapper.
- Flow sink / storage writer.

Security and reliability requirements:

- Treat UDP NetFlow packets as untrusted input.
- Drop packets from unexpected exporter IPs by default.
- Validate packet structure, field lengths, timestamps, template IDs, and record sizes.
- Track unknown templates, malformed packets, sequence gaps, duplicates, and decode failures.
- Do not log raw flows at info level.

Deduplication is best-effort. Use a bounded dedupe key based on:

- exporter ID/address
- source/engine ID when available
- packet sequence
- record ordinal
- fallback hash of normalized flow content

## Storage Plan

SQLite is the MVP store. It is viable if used as a rollup-first, retention-bounded embedded database.

Use SQLite for:

- App config metadata.
- Device identity timeline.
- Recent raw flow detail.
- Hourly rollups.
- Daily rollups.
- Exporter/template/checkpoint status.

Do not use SQLite as an indefinite raw NetFlow warehouse.

Recommended defaults:

- Raw flow retention: 7-30 days.
- Hourly rollup retention: 12-24 months.
- Daily rollup retention: longer, configurable.
- WAL mode enabled.
- `synchronous=NORMAL`.
- Batched writes through a single writer queue/goroutine.
- Minimal indexes.
- Retention deletes run in background chunks.

Minimum flow/rollup schema:

```text
flow_raw
  id
  exporter_id
  received_ts
  flow_start_ts
  flow_end_ts
  src_ip
  dst_ip
  src_port
  dst_port
  protocol
  ingress_interface
  egress_interface
  bytes
  packets
  direction
  lan_ip
  attributed_device_id
  attribution_confidence
  dedupe_key

usage_bucket_hour
  bucket_start_ts
  device_id
  interface_name
  direction
  bytes
  packets
  flow_count

usage_bucket_day
  day_start_ts
  device_id
  interface_name
  direction
  bytes
  packets
  flow_count

exporter_state
  exporter_id
  source_id
  last_sequence
  last_template_ts
  last_packet_ts
  sequence_gap_count
  invalid_packet_count
```

Store timestamps as integer UTC. The UI can render local time.

Aggregation rules:

- Normalize flow timestamps.
- Classify direction and LAN-side IP.
- Resolve device using address assignment intervals.
- Split long flows proportionally across hourly buckets.
- Keep recent raw flows for drilldown and re-attribution.
- Roll daily buckets from hourly buckets.
- Use an in-memory ring buffer for live rates rather than querying SQLite every second.

## Future Time-Series Database

SQLite starts the project, but a future time-series backend is likely useful.

Preferred next step: **TimescaleDB**.

Why:

- SQL compatibility.
- Good relational joins with device identity tables.
- Continuous aggregates.
- Grafana support.
- Easier migration from the SQLite logical model.

Potential later scale-out backend: **ClickHouse**.

Use ClickHouse if raw-flow analytics and long raw retention become central.

Do not use Prometheus as the traffic accounting database. Prometheus is useful for collector health metrics, packet drops, ingest rate, sequence gaps, queue depth, and DB write latency.

Storage abstraction should be through application repositories:

- `FlowWriter`
- `TrafficQueryStore`
- `IdentityStore`
- `MigrationRunner`

Do not build an elaborate generic database abstraction before real query patterns are known.

## Web GUI

Use a simple server-rendered Go web UI for MVP.

Recommended:

- Go templates.
- Minimal vanilla JS or HTMX for live refresh, filters, and merge actions.
- One chart library if needed.
- Compact admin-console layout.
- Tables and clear operational status over decorative dashboards.

Avoid:

- SPA framework by default.
- Drag-and-drop dashboards.
- Theme system.
- Global frontend state management.
- A marketing-style landing page.

The GUI should emphasize freshness and confidence:

- Last valid NetFlow packet time.
- Last OPNsense API poll time.
- Last identity update time.
- Last aggregation run.
- Attribution confidence.
- Unknown/unattributed usage.

## Security Plan

Treat the app as sensitive local network observability infrastructure.

Assets:

- OPNsense API key/secret.
- Web admin session.
- SQLite database.
- Device identity map.
- Flow and bandwidth history.
- Network topology metadata.

Safe defaults:

- Web auth required by default.
- No default admin password.
- First-run setup forces admin credential creation.
- Password hashes use Argon2id or bcrypt.
- Session cookies use `HttpOnly`, `SameSite`, and `Secure` when HTTPS is enabled.
- CSRF protection on state-changing routes.
- Login rate limiting.
- Web bind defaults to `127.0.0.1`; binding to `0.0.0.0` is explicit.
- NetFlow source allowlist defaults to the configured OPNsense IP.
- No external telemetry.
- No external OUI/vendor lookup unless explicitly enabled.
- Redacted logs and diagnostics.
- Conservative retention defaults.

Secrets:

- Prefer Docker secrets or mounted secret files.
- Environment variables are acceptable for MVP but documented as less ideal.
- Do not store OPNsense API secrets in SQLite unless an explicit encrypted-at-rest design is added.
- In plugin mode, avoid requiring an OPNsense API key for local data where possible. Prefer local read paths and OPNsense-managed configuration when running on the firewall.
- Never log secrets, auth headers, or full API request dumps.
- Support credential rotation without deleting the database.

Docker:

- Run as non-root.
- Mount only config, data, and optional secrets.
- Do not mount the Docker socket.
- Drop Linux capabilities unless required.
- Document UDP port exposure clearly.
- Avoid broad default web port publishing.

OPNsense plugin mode:

- Run the service under a dedicated unprivileged user if OPNsense packaging allows the required socket/file access.
- Keep privileged operations behind narrow `configd` actions.
- Bind the OTM web UI to localhost by default and expose it through an OPNsense menu link or reverse-proxy-style integration only when explicitly configured.
- Do not write high-volume raw flow data into `/conf/config.xml`.
- Store SQLite under `/var/db/otm`, not in the OPNsense configuration file or web root.
- Keep retention conservative by default to avoid filling firewall storage.
- Treat firewall CPU, disk, and flash endurance as constrained resources.

## Deployment Plan

Primary MVP install path:

```text
docker compose up
```

Compose should include:

- One app container.
- Persistent data volume or bind mount for SQLite.
- Explicit UDP NetFlow port mapping.
- Explicit web port mapping.
- Health check.
- Restart policy.
- `.env.example`.

Important environment/config values:

```text
OTM_WEB_ADDR
OTM_WEB_PORT
OTM_NETFLOW_ADDR
OTM_NETFLOW_PORT
OTM_NETFLOW_ALLOWED_EXPORTERS
OTM_SQLITE_PATH
OTM_TIMEZONE
OTM_RAW_RETENTION_DAYS
OTM_LOG_LEVEL
OTM_OPNSENSE_URL
OTM_OPNSENSE_API_KEY_FILE
OTM_OPNSENSE_API_SECRET_FILE
```

Support manual binary deployment and OPNsense plugin packaging later, but optimize first for Docker Compose on a LAN host or home server.

Plugin packaging should become a first-class release artifact after the standalone app proves ingestion, attribution, and retention. The intended release matrix is:

```text
Linux amd64/arm64 container image
Linux amd64/arm64 standalone binary
FreeBSD amd64 standalone binary
OPNsense plugin package wrapping the FreeBSD binary
```

The plugin path is attractive because it can access local lease files, ARP/NDP data, and NetFlow configuration without a separate API credential. The tradeoff is operational risk: running a database and traffic collector on the firewall can consume CPU, disk, and write cycles. The standalone deployment remains the safer default for heavier retention and Grafana/TimescaleDB integration.

## Observability And Operations

Use structured logs from day one.

Endpoints:

- `/healthz`: process liveness, no sensitive detail.
- `/readyz`: DB writable, collectors running, no sensitive detail.
- Authenticated diagnostics page: detailed sanitized state.

Track counters:

- NetFlow packets received.
- Flow records decoded.
- Unknown template count.
- Invalid packet count.
- Sequence gap count.
- Dropped packet count.
- OPNsense API poll failures.
- Identity observations created.
- DB write failures.
- Aggregation failures.
- Retention job status.
- Backup status.

Backups:

- Provide app-level SQLite backup using SQLite backup APIs or safe checkpointed copy.
- Include app version and config metadata.
- Restore only while ingestion is paused or during startup.
- Document manual Docker volume backup.

## Testing Strategy

Build fake sources early.

Provide Makefile targets for common local and integration workflows. The Makefile should be the stable entrypoint for development even when the underlying commands evolve.

Initial targets:

```text
make test
make lint
make fmt
make build
make build-freebsd
make docker-build
make docker-up
make docker-down
make fake-run
make fake-netflow
make migrate
make clean
```

Later integration targets:

```text
make test-integration
make test-opnsense-api
make test-netflow-listener
make netflow-record
make netflow-replay
make netflow-decode
make package-opnsense
```

Required tests:

- NetFlow v9 decoder fixture tests.
- Fake UDP NetFlow sender integration test.
- Fake OPNsense API server returning DHCP/ARP/NDP/interface snapshots.
- SQLite migration tests using temp databases.
- Identity interval tests with fake clock.
- Web handler tests against app service interfaces.

Golden cases:

- DHCP hostname changes.
- Same IP reused by different MACs.
- Static IP seen only in ARP.
- IPv6 NDP-only identity.
- Flow arrives before identity snapshot.
- NetFlow template missing or late.
- Duplicate NetFlow packet.
- OPNsense API unavailable while NetFlow continues.
- Long flow crossing an hour boundary.
- Local LAN flow ignored from internet totals.

## Development And Test Environments

The development machine and the OPNsense test box are expected to be different systems. Do not assume the app, fake data generator, and router all run on the same host.

This affects the design in practical ways:

- Bind addresses and advertised collector addresses must be explicit.
- The app should show the exact NetFlow destination IP/port OPNsense should use.
- The setup validator must distinguish local listener status from remote packet arrival.
- Test instructions should support a remote OPNsense router exporting NetFlow to the developer machine or to a separate Docker host.
- OPNsense API tests need configurable router URL, key, and secret.
- Integration tests that touch a real router must be opt-in and skipped by default.
- Docker Compose should work on the development box without OPNsense present by using fake collectors.
- Remote test fixtures should avoid mutating OPNsense unless a write-capable test mode is explicitly enabled.

Example environment variables for remote testing:

```text
OTM_TEST_OPNSENSE_URL
OTM_TEST_OPNSENSE_API_KEY_FILE
OTM_TEST_OPNSENSE_API_SECRET_FILE
OTM_TEST_COLLECTOR_ADVERTISE_ADDR
OTM_TEST_NETFLOW_EXPECTED_EXPORTER
```

The core local loop should stay fast:

```text
make test
make fake-run
make docker-up
```

Real-router validation should be a separate loop:

```text
make test-opnsense-api
make test-netflow-listener
```

## Test Utilities

Build small CLI utilities early so NetFlow and identity behavior can be tested without repeatedly depending on a live OPNsense router.

Recommended commands can live under `cmd/otmctl` or `cmd/` subcommands:

```text
otmctl netflow record
otmctl netflow replay
otmctl netflow decode
otmctl netflow summarize
otmctl opnsense snapshot
otmctl fixture redact
```

### NetFlow Recorder

Purpose: capture a short window of raw NetFlow UDP packets from OPNsense for decoder development and regression tests.

Example:

```text
otmctl netflow record \
  --listen 0.0.0.0:2055 \
  --allow-exporter 10.0.0.1 \
  --duration 5m \
  --out fixtures/netflow/home-5m.otmcap
```

Requirements:

- Store raw packet bytes, received timestamp, source address, and local listener address.
- Use a simple documented capture format, preferably length-prefixed records with a small JSON header or SQLite fixture DB.
- Record enough metadata to replay timing or replay as fast as possible.
- Refuse unbounded captures unless an explicit max duration or max size is provided.
- Default output should be treated as sensitive because it contains internal IPs, external IPs, ports, and timing.

### NetFlow Decoder

Purpose: inspect captures without running the full app.

Example:

```text
otmctl netflow decode fixtures/netflow/home-5m.otmcap
```

Output should include:

- Packet count.
- Exporter addresses.
- NetFlow versions seen.
- Template count.
- Decoded flow count.
- Unknown template count.
- Invalid packet count.
- Sequence gaps.
- Top internal IPs by bytes.
- Top external IPs by bytes.

### NetFlow Replay

Purpose: feed captured packets back into a local OTM instance for deterministic manual and integration testing.

Example:

```text
otmctl netflow replay \
  --in fixtures/netflow/home-5m.otmcap \
  --target 127.0.0.1:2055 \
  --speed 10x
```

Replay modes:

- Real-time replay using original packet spacing.
- Accelerated replay, such as `10x`.
- Immediate replay for fast tests.

### Redaction

Captures should not be committed unless redacted or intentionally synthetic.

Redaction utility goals:

- Replace internal IPs with deterministic test ranges.
- Replace external IPs with deterministic documentation ranges where possible.
- Replace MAC addresses and hostnames.
- Preserve enough structure for decoder and attribution tests.
- Keep packet/template structure valid after redaction.

If preserving binary NetFlow validity while redacting proves too complex, keep two fixture classes:

```text
private captures: local only, gitignored
public fixtures: synthetic/generated, safe to commit
```

### OPNsense Snapshot Utility

Purpose: capture router identity data for offline testing.

Example:

```text
otmctl opnsense snapshot \
  --url https://opnsense.local \
  --out fixtures/opnsense/snapshot.json
```

Snapshot should include sanitized forms of:

- Interfaces.
- ARP table.
- NDP table.
- DHCP/Kea leases when available.
- NetFlow config/status.

As with NetFlow captures, raw snapshots are sensitive and should be gitignored unless redacted.

## Milestones

### Milestone 0: Repository And Skeleton — COMPLETE

- [x] Initialize Go module.
- [x] Add Dockerfile and docker-compose.
- [x] Add FreeBSD build target in Makefile (`make build-freebsd`).
- [x] Config loading — cobra/viper with dotenv, secret files, env var override.
- [x] Health endpoints (`/healthz`, `/readyz`).
- [x] Structured logging (`log/slog`).
- [x] Minimal authenticated web shell — token-based auth, `/setup` status page.
- [x] `otm version`, `otm validate-config`, `otm validate-opnsense` commands.
- [x] OPNsense API client with endpoint validation report.
- [x] NetFlow UDP listener with allowlist, version detection, and live stats.
- [x] NetFlow capture/decode/replay CLI (`otm netflow record/decode/replay`, `.otmcap` format).
- [x] Makefile with all planned targets.
- [ ] SQLite migrations — **not done; blocks Milestone 1**.
- [ ] `internal/domain` core types — **not done; blocks Milestone 1**.

Verification:

```text
go test ./...
docker compose up
curl /healthz
```

### Milestone 1: Fake Data End-To-End — NEXT

Primary goal: prove the full ingest → store → query → render loop works before touching real hardware.

Tasks:

- [ ] Add `internal/domain` package with vendor-neutral types: `Flow`, `Device`, `DeviceIdentifier`, `IdentityObservation`, `AddressAssignment`, `UsageBucket`.
- [ ] Add `internal/storage/sqlite` with WAL-mode SQLite, schema migrations, and a `FlowWriter` / `TrafficQueryStore` / `IdentityStore` implementation.
- [ ] Add `internal/collectors/fake` — fake identity source (3–5 named devices with MACs) and fake flow generator (random bytes/packets toward external IPs).
- [ ] Wire fake collectors into `internal/app` behind the `FlowCollector` / `IdentitySource` interfaces; start them under `otm serve` when no real sources are configured.
- [ ] Write hourly rollup job that aggregates `flow_raw` into `usage_bucket_hour`.
- [ ] Add Overview web page: collector health, flows/min, top talkers now, DB size, last aggregation run.
- [ ] Add Devices web page: known devices, current IPs, last seen, bytes today.

Verification:

```text
make fake-run
# fake device appears in Devices page
# fake traffic appears in top talkers on Overview
# hourly bucket row is written to SQLite
```

### Milestone 2: NetFlow v9 Receiver — AFTER MILESTONE 1

The UDP listener exists and counts packets; it does not yet decode templates or flows.

- [ ] Add `internal/netflow/decoder` — NetFlow v9 template cache, flowset parser, domain `Flow` mapper.
- [ ] Implement exporter allowlist enforcement in the decoder path (already in listener).
- [ ] Track unknown templates, sequence gaps, and decode failures in listener stats.
- [ ] Persist decoded `Flow` records via `FlowWriter`.
- [ ] Show per-version and per-exporter decode diagnostics on `/setup`.
- [ ] Add fixture-based decoder tests (`internal/netflow/testdata/`).

Verification:

```text
fixture sender produces decoded records
invalid packets are counted and skipped
sequence gaps are tracked
```

### Milestone 3: OPNsense Identity Adapter — PARTIALLY STARTED

The OPNsense API client and validation report exist (`internal/opnsense`). Identity ingestion is not wired in.

- [x] OPNsense API client with TLS, key/secret auth.
- [x] Endpoint validation report (`/api/diagnostics/netflow/*`, ARP, NDP, DHCP).
- [ ] Implement `IdentitySource` adapter that polls ARP/NDP/DHCP lease endpoints on a configurable interval.
- [ ] Normalize API responses to `IdentityObservation` records and persist via `IdentityStore`.
- [ ] Resolve address-to-device assignments with time intervals.
- [ ] Surface identity polling status (last poll time, error, observation count) in the web UI.

Verification:

```text
OPNsense API validator reports endpoint status
known devices receive names/MAC/IPs
unknown devices remain reviewable
```

### Milestone 4: Attribution And Reports

- Classify local/external/router/local-link traffic.
- Attribute flows to devices at flow time.
- Split long flows across hourly buckets.
- Roll hourly into daily.
- Add Reports page and CSV export.

Verification:

```text
today usage by device is visible
last hour usage by device is visible
local LAN traffic is ignored by default
unknown traffic is visible as unattributed
```

### Milestone 5: Operations Hardening

- Add diagnostics bundle.
- Add backup/restore.
- Add retention jobs.
- Add redaction tests.
- Add setup documentation.

Verification:

```text
sanitized diagnostics contain no secrets
backup restores into a fresh instance
retention removes old raw flows without deleting rollups
```

### Milestone 6: OPNsense Plugin Proof Of Concept

- Package the FreeBSD binary under an OPNsense plugin skeleton.
- Add rc script for `otm`.
- Add `configd` actions for start, stop, restart, status, validate, and backup.
- Add minimal MVC model for plugin configuration.
- Render OTM config from OPNsense settings.
- Add OPNsense menu entry linking to OTM UI or showing a status page.
- Verify install, start, stop, upgrade, uninstall, and data preservation behavior in an OPNsense VM.

Verification:

```text
plugin installs on OPNsense VM
configd can start and stop OTM
OTM stores data under /var/db/otm
OPNsense UI shows service status
uninstall does not accidentally delete usage history unless explicitly requested
```

## Open Design Questions

- Should MVP use only OPNsense API identity, or also support local lease-file parsing from day one?
- Which NetFlow decoder library, if any, should be used versus writing a minimal v9 decoder?
- Should first-run auth be local password only, reverse-proxy trusted header mode, or both?
- What raw-flow retention default is acceptable for typical home networks: 7, 14, or 30 days?
- Should automatic OPNsense NetFlow configuration be included in MVP, or kept as post-MVP opt-in setup?
- Should Grafana support be limited to documented SQL queries initially, or should a dashboard JSON be generated later?
- In plugin mode, should OTM run its own web server, or should the plugin proxy/embed only selected status pages through OPNsense?
- How much raw flow retention is safe on typical OPNsense hardware without risking disk exhaustion?
- Should plugin mode default to local lease files/commands instead of OPNsense API calls?

## Current Recommendation

Start with the standalone Go app, Docker Compose, SQLite, fake data, and a simple web GUI, but keep all runtime paths, service control, and configuration sources plugin-friendly. Prove ingestion, attribution, rollups, and diagnostics before adding a time-series database, Grafana, or full OPNsense plugin packaging.

SQLite is the right MVP choice if raw retention is bounded and hourly/daily rollups are the durable product surface. TimescaleDB is the likely next database once retention, Grafana dashboards, or multi-month querying outgrow SQLite.

The eventual OPNsense plugin should be a packaging and management layer around the same Go service, not a separate implementation.
