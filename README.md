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
# One-line installer (Linux, requires root)
curl -sSL https://raw.githubusercontent.com/HopStat/HopStat/main/install.sh | sudo bash
```

Or manually:

```bash
curl -Lo hopstat https://github.com/HopStat/HopStat/releases/latest/download/hopstat-linux-amd64
chmod +x hopstat
./hopstat --mode=server
# config.yaml is auto-generated with random secrets on first start.
# Admin credentials are printed to the console — change them after first login.
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
  --cap-add NET_RAW --cap-add NET_ADMIN \
  -p 8080:8080 \
  -v hopstat-data:/data \
  -e LG_ADMIN_PASSWORD=changeme \
  ghcr.io/hopstat/hopstat:latest
```

Config and secrets are **auto-generated** inside the `/data` volume on first start.  
Admin credentials are written to the container log — check with `docker logs <container>`.

To pin secrets across image rebuilds (stateless deployments):

```bash
-e LG_SECURITY_JWT_SECRET=$(openssl rand -hex 32) \
-e LG_SECURITY_CREDENTIAL_KEY=$(openssl rand -hex 32)
```

Or use Docker Compose:

```bash
docker compose up -d
```

See [`docker-compose.yml`](docker-compose.yml) for the full example.

### Build from source

Requires Go 1.23+ and Node.js 22+.

```bash
git clone https://github.com/HopStat/HopStat.git
cd HopStat
cd web/frontend && npm ci && npm run build && cd ../..
make build
./hopstat --mode=server
```

## Configuration

`config.yaml` is auto-generated with random secrets on first start — no manual setup needed.  
To customise, edit the generated file or override individual values with environment variables.

### Environment variables

All config keys can be overridden with `LG_` + the key path (dots → underscores, uppercased):

| Config key | Environment variable |
|---|---|
| `security.jwt_secret` | `LG_SECURITY_JWT_SECRET` |
| `security.credential_key` | `LG_SECURITY_CREDENTIAL_KEY` |
| `server.port` | `LG_SERVER_PORT` |
| `database.path` | `LG_DATABASE_PATH` |

> **Note:** `LG_ADMIN_PASSWORD` is a special variable read directly at startup to set the admin password. It does not follow the viper key-path convention.

### Minimal config reference

```yaml
server:
  port: 8080

security:
  # Auto-generated on first start — override only if you need stable values
  # (e.g. stateless Docker deployments without a persistent volume).
  jwt_secret: ""        # 64 hex chars: openssl rand -hex 32
  credential_key: ""    # 64 hex chars: openssl rand -hex 32

geoip:
  license_key: ""       # MaxMind account (free tier)
  account_id: ""
  db_dir: "./data/geoip"
  update_interval: "72h"

update:
  enabled: true
```

## Deployment Modes

### Server mode (default)

Runs the HTTP API, React SPA and query engine. Connects directly to routers via SSH/Telnet or delegates to remote agents.

```bash
./hopstat --mode=server
```

### Agent mode

Lightweight REST server deployed on remote POPs. The central server discovers it as a node.

```bash
./hopstat --mode=agent
# Default port: 9090
```

### Systemd service

```bash
sudo ./hopstat --install-service
# Installs to /usr/local/bin, generates /etc/hopstat/config.yaml,
# writes and starts /etc/systemd/system/hopstat.service
journalctl -u hopstat | grep -A 10 HOPSTAT   # view first-run credentials
```

## Admin Panel

Access at `/admin`. On first start, a random admin password is generated and printed to the console. Change it in **Admin → Users** after logging in.

From the panel you can:
- Add router nodes (SSH/Telnet credentials are encrypted with AES-256-GCM)
- Configure BGP neighbors for the embedded GoBGP peer
- Manage community string filtering rules
- View audit logs

## GeoIP Enrichment

Obtain a [MaxMind](https://www.maxmind.com/) account and set in `config.yaml`:

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
# Backend (restart manually after changes)
make run-server

# Frontend dev server with API proxy to backend
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

MIT — see [LICENSE](LICENSE).
