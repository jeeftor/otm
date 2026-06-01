# OTM

OTM is an early self-hosted OPNsense Traffic Monitor. The first implementation focuses on setup validation:

- confirm the OPNsense API key works,
- check whether NetFlow status/config endpoints are readable,
- check whether OPNsense appears to be exporting NetFlow to this collector,
- listen for NetFlow v9 UDP packets and show packet status.

The long-term goal is per-device hourly/daily internet usage by combining NetFlow with DHCP, ARP, and NDP identity data.

## Quick Start

```bash
cp .env.example .env
make build
./bin/otm
```

Open:

```text
http://127.0.0.1:8080/setup
```

## Important Settings

OTM automatically reads `.env` from the current directory before reading configuration. Real environment variables take precedence over `.env` values. Use `OTM_ENV_FILE=/path/to/file` to load a different file.

`.env` is ignored by Git. Keep real API keys and secrets there or in secret files, not in committed files.

```text
OTM_WEB_ADDR=127.0.0.1:8080
OTM_NETFLOW_ADDR=0.0.0.0:2055
OTM_NETFLOW_ALLOWED_EXPORTERS=10.0.0.1
OTM_COLLECTOR_ADVERTISE_ADDR=10.0.0.20:2055
OTM_OPNSENSE_URL=https://10.0.0.1
OTM_OPNSENSE_API_KEY_FILE=./secrets/opnsense-key
OTM_OPNSENSE_API_SECRET_FILE=./secrets/opnsense-secret
```

Set `OTM_COLLECTOR_ADVERTISE_ADDR` to the IP and UDP port that OPNsense should export NetFlow to.

## Example: OPNsense At 192.168.0.1, OTM At 192.168.0.10

If OPNsense is your router at `192.168.0.1` and this app runs on a LAN host at `192.168.0.10`, use a `.env` like:

```text
OTM_WEB_ADDR=0.0.0.0:8080
OTM_WEB_AUTH_TOKEN=change-this-local-token

OTM_NETFLOW_ADDR=0.0.0.0:2055
OTM_NETFLOW_ALLOWED_EXPORTERS=192.168.0.1
OTM_COLLECTOR_ADVERTISE_ADDR=192.168.0.10:2055

OTM_OPNSENSE_URL=https://192.168.0.1
OTM_OPNSENSE_API_KEY_FILE=./secrets/opnsense-key
OTM_OPNSENSE_API_SECRET_FILE=./secrets/opnsense-secret
OTM_OPNSENSE_INSECURE_SKIP_VERIFY=true
```

Then configure OPNsense NetFlow export to send NetFlow v9 to:

```text
Host: 192.168.0.10
Port: 2055
Version: 9
```

Open the setup page from your LAN:

```text
http://192.168.0.10:8080/setup?token=change-this-local-token
```

For regular use, prefer the `Authorization: Bearer ...` header or a reverse proxy over putting the token in the URL.

## API Endpoints

```text
GET /healthz
GET /readyz
GET /setup
GET /api/status
GET /api/netflow/status
GET /api/opnsense/validate
```

If `OTM_WEB_AUTH_TOKEN` is set, pass it as:

```text
Authorization: Bearer <token>
```

## Development

```bash
make test
make lint
make build
make build-freebsd
```

## NetFlow Test Utilities

Record a short capture from OPNsense:

```bash
make netflow-record \
  NETFLOW_LISTEN=0.0.0.0:2055 \
  NETFLOW_ALLOW=192.168.0.1 \
  NETFLOW_DURATION=5m \
  NETFLOW_OUT=captures/home-5m.otmcap
```

Decode and summarize the capture:

```bash
make netflow-decode NETFLOW_IN=captures/home-5m.otmcap
```

Replay the capture into a local OTM instance:

```bash
make netflow-replay \
  NETFLOW_IN=captures/home-5m.otmcap \
  NETFLOW_TARGET=127.0.0.1:2055 \
  NETFLOW_SPEED=10
```

Capture files contain internal IPs, external IPs, ports, and timing metadata. They are ignored by Git via `*.otmcap`.
