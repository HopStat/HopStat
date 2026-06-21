# HopStat

Network looking glass platform with BGP route lookup, ping, traceroute and AS path diagnostics. Single Go binary — embeds the React frontend and SQLite database with no external dependencies.

## Features

- **BGP route lookup** — query routes from connected routers or an embedded GoBGP peer; community badges show rule descriptions on hover; AS path map visualises each hop
- **Ping / Tracert** — run from any node; RTT stats and hops stream live via SSE (including remote lg-node agents); hostnames are DNS-validated before dispatch
- **AS path map** — visual hop-by-hop ASN breakdown with GeoIP enrichment on every BGP result (MaxMind first, Cymru fallback)
- **BGP Communities** — define community string rules in admin; matched badges on BGP results; public `/communities` catalogue linked from the footer
- **Multi-node** — direct router connections (SSH/Telnet) or remote agent deployment
- **Vendor support** — Cisco IOS/XR, Juniper JunOS, MikroTik RouterOS, Bird, Generic
- **Responsive public UI** — mobile-friendly query page with five locales (EN, TR, DE, FR, RU) and SEO metadata
- **Cloudflare-aware** — optional `behind_cloudflare` mode shows real visitor IP via `CF-Connecting-IP`
- **Admin panel** — manage nodes, site branding, BGP neighbors, Communities rules, footer links and audit logs
- **GeoIP** — MaxMind ASN + City databases with interval-based downloads (timestamps stored in SQLite)
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

`config.yaml` is **auto-generated on first start** with every section from `config.example.yaml` — including flood control, GeoIP, BGP and TLS placeholders — plus random `jwt_secret` and `credential_key` values. No manual setup needed.

Site name, AS number and footer contact links are configured in **Admin → Settings**, not in YAML.

To customise, edit the generated file or override individual values with environment variables.

### Environment variables

All config keys can be overridden with `LG_` + the key path (dots → underscores, uppercased):

| Config key | Environment variable |
|---|---|
| `security.jwt_secret` | `LG_SECURITY_JWT_SECRET` |
| `security.credential_key` | `LG_SECURITY_CREDENTIAL_KEY` |
| `server.port` | `LG_SERVER_PORT` |
| `server.behind_cloudflare` | `LG_SERVER_BEHIND_CLOUDFLARE` |
| `database.path` | `LG_DATABASE_PATH` |
| `geoip.update_interval` | `LG_GEOIP_UPDATE_INTERVAL` |

> **Note:** `LG_ADMIN_PASSWORD` is a special variable read directly at startup to set the admin password. It does not follow the viper key-path convention.

### Minimal config reference

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  behind_cloudflare: false   # true when behind Cloudflare proxy
  trusted_proxies: []        # optional extra reverse-proxy CIDRs

security:
  # Auto-generated on first start — override only if you need stable values
  # (e.g. stateless Docker deployments without a persistent volume).
  jwt_secret: ""        # 64 hex chars: openssl rand -hex 32
  credential_key: ""    # 64 hex chars: openssl rand -hex 32

flood_control:
  enabled: true
  http_rate_limit_per_min: 10
  query_rate_limit_per_min: 10

geoip:
  license_key: ""       # MaxMind account (free tier)
  account_id: ""
  db_dir: "./data/geoip"
  update_interval: "72h"  # downloads skipped until interval elapsed

update:
  enabled: true

bgp:
  listen_port: 11790
  local_as: 0
```

See [`config.example.yaml`](config.example.yaml) for the complete reference.

## Deployment Modes

### Server mode (default)

Runs the HTTP API, React SPA and query engine. Connects directly to routers via SSH/Telnet or delegates to remote agents.

```bash
./hopstat --mode=server
```

### Agent mode

Lightweight REST server deployed on remote POPs. The central server discovers it as a node.
Agents v2+ expose SSE streaming endpoints for live ping/tracert output.

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

Access at `/admin`. On a fresh install, a random admin password is generated and shown once in the installer output or service log. Change it in **Admin → Settings** after logging in.

From the panel you can:
- Add router nodes (SSH/Telnet credentials are encrypted with AES-256-GCM)
- Configure site name, AS number and footer links (web, contact, terms, privacy, PeeringDB) in **Admin → Settings**
- Configure standalone nodes with an agent token for embedded API queries
- Configure BGP neighbors for the embedded GoBGP peer
- Manage **Communities** rules — community string, severity, description and active toggle
- View audit logs

### Communities

BGP community rules are configured in **Admin → Communities**. Each rule maps a community string (e.g. `65000:100`) to a description and severity (`info`, `success`, `warning`, `reject`). Active rules are:

- Matched automatically on BGP route lookups — colour-coded badges with hover descriptions
- Listed on the public **`/communities`** page (linked from the site footer)
- Exposed via **`GET /api/v1/communities`** (no authentication; active rules only)

The UI label **Communities** is used in all supported locales (EN, TR, DE, FR, RU).

## Cloudflare

When HopStat sits behind Cloudflare, enable real client IP detection:

```yaml
server:
  behind_cloudflare: true
```

Or set `LG_SERVER_BEHIND_CLOUDFLARE=true`. HopStat trusts Cloudflare proxy CIDRs and reads the visitor address from `CF-Connecting-IP`.

## GeoIP Enrichment

Obtain a [MaxMind](https://www.maxmind.com/) account and set in `config.yaml`:

```yaml
geoip:
  license_key: "..."
  account_id: "..."
  db_dir: "./data/geoip"
  update_interval: "72h"
```

Databases are downloaded automatically. Last-download timestamps are stored in SQLite so restarts within the interval do not trigger a new download.

HopStat prefers **MaxMind** for per-hop ASN and country enrichment (traceroute, AS path map). When databases are unavailable, **Team Cymru DNS** (`origin.asn.cymru.com`) is used as fallback.

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
