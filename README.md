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
