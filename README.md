# HopStat

Network looking glass platform with BGP route lookup, ping, traceroute, MTR and AS path diagnostics. Single Go binary — embeds the React frontend and SQLite database with no external dependencies.

## Features

- **BGP route lookup** — query routes from connected routers or an embedded GoBGP peer
- **Ping / Traceroute / MTR** — run from any node in your network
- **AS path analysis** — hop-by-hop ASN breakdown with GeoIP enrichment
- **Multi-node** — direct router connections (SSH/Telnet) or remote agent deployment
- **Vendor support** — Cisco IOS/XR, Juniper JunOS, MikroTik RouterOS, Bird, Generic
- **Admin panel** — manage nodes, users, BGP neighbors, community string filters and audit logs
- **Auto-update** — self-updater checks GitHub releases and hot-swaps the binary
- **Single binary** — React SPA, SQLite migrations and static assets are all embedded

## Quick Start

```bash
# Download the latest binary
curl -Lo hopstat https://github.com/HopStat/HopStat/releases/latest/download/hopstat-linux-amd64
chmod +x hopstat

# Generate secrets
export LG_JWT_SECRET=$(openssl rand -hex 32)
export LG_CREDENTIAL_KEY=$(openssl rand -hex 32)
export LG_ADMIN_PASSWORD=changeme

# Copy and edit config
cp config.example.yaml config.yaml
# Edit config.yaml: set security.jwt_secret, security.credential_key, server.org_name

./hopstat --mode=server --config=config.yaml
# Open http://localhost:8080
```

## Installation

### Prebuilt binaries

Download from [Releases](https://github.com/HopStat/HopStat/releases):

| Platform | Binary |
|----------|--------|
| Linux x86-64 | `hopstat-linux-amd64` |
| Linux ARM64 | `hopstat-linux-arm64` |

### Docker

```bash
docker run -d \
  -p 8080:8080 \
  -v ./data:/app/data \
  -e LG_JWT_SECRET=$(openssl rand -hex 32) \
  -e LG_CREDENTIAL_KEY=$(openssl rand -hex 32) \
  -e LG_ADMIN_PASSWORD=changeme \
  ghcr.io/hopstat/hopstat:latest
```

### Build from source

Requires Go 1.21+ and Node.js 20+.

```bash
git clone https://github.com/HopStat/HopStat.git
cd HopStat

# Build frontend
cd web/frontend && npm install && npm run build && cd ../..

# Build binary
make build
./hopstat --mode=server
```

## Configuration

Copy `config.example.yaml` and edit:

```yaml
server:
  port: 8080
  org_name: "My Network"
  as_number: "AS65000"

security:
  jwt_secret: ""        # 32-byte hex: openssl rand -hex 32
  credential_key: ""    # 32-byte hex: openssl rand -hex 32

update:
  enabled: true
  github_repo: "HopStat/HopStat"
```

All fields can be overridden with `LG_` prefixed environment variables.

## Deployment Modes

### Server mode (default)

Runs the HTTP API, React SPA and query engine. Connects directly to routers via SSH/Telnet or delegates to remote agents.

```bash
./hopstat --mode=server --config=config.yaml
```

### Agent mode

Lightweight REST server deployed on remote POPs. The central server discovers it as a node.

```bash
./hopstat --mode=agent --config=config.yaml
# Default port: 9090
```

## Admin Panel

Access at `/admin` — default credentials set via `LG_ADMIN_PASSWORD` at first boot or created through the admin UI.

From the panel you can:
- Add router nodes (SSH/Telnet credentials are encrypted with AES-256-GCM)
- Configure BGP neighbors for the embedded GoBGP peer
- Manage community string filtering rules
- View audit logs

## GeoIP Enrichment

Obtain a [MaxMind](https://www.maxmind.com/) account and set:

```yaml
geoip:
  license_key: "..."
  account_id: "..."
  db_dir: "./data/geoip"
  update_interval: "72h"
```

Databases are downloaded and refreshed automatically.

## Development

```bash
# Backend (hot-reload not included, restart manually)
make run-server

# Frontend dev server with proxy to backend
cd web/frontend && npm run dev
# Visit http://localhost:5173
```

Run tests:

```bash
make test
make test-race
make lint
```

## License

MIT
