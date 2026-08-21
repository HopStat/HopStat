# HopStat

Network looking glass platform with BGP route lookup, ping, traceroute and AS path diagnostics. Single Go binary — embeds the React frontend and SQLite database with no external dependencies.

## Features

- **BGP route lookup** — query routes from connected routers or an embedded GoBGP peer; community badges show rule descriptions on hover; AS path map visualises each hop
- **Ping / Traceroute** — run from any node; RTT stats and hops stream live via SSE (including remote lg-node agents)
- **Hostname resolution** — query targets that are hostnames are resolved to IP on the HopStat server (Go `net.DefaultResolver`) before dispatch; private and link-local addresses are blocked
- **AS path map** — visual hop-by-hop ASN breakdown with GeoIP enrichment on every BGP result (MaxMind first, Team Cymru DNS fallback)
- **Network map** — on BGP lookups, a converging diagram of how every BGP-capable node reaches the target (local peers and remote agents alike); shared AS hops merge, the queried node's path is highlighted, and backup routes are drawn dashed
- **BGP Communities** — define community string rules in admin; matched badges on BGP results; public `/communities` catalogue linked from the footer
- **Quick queries** — preset command/target shortcuts on the public home page, managed in **Admin → Quick Queries**
- **Shareable results** — every query writes a clean path to the address bar (`/ping/8.8.8.8`, `/bgp/1.1.1.0/24`) and puts a non-default node in front of it (`/sofia/ping/8.8.8.8`); opening that link re-runs the query on that node. The result panel has **Copy link** and **Copy result** buttons for pasting into a ticket or chat
- **Multi-node** — direct router connections (SSH/Telnet) or remote agent deployment
- **Vendor support** — Cisco IOS/XR, Juniper JunOS, MikroTik RouterOS, Bird, Generic
- **Responsive public UI** — mobile-friendly query page with five locales (EN, TR, DE, FR, RU) and SEO metadata
- **Cloudflare-aware** — optional `behind_cloudflare` mode shows real visitor IP via `CF-Connecting-IP`
- **Admin panel** — manage nodes, site branding, BGP neighbors, Communities rules, quick queries, GeoIP lookup, footer links and audit logs
- **GeoIP** — MaxMind ASN + City databases with interval-based downloads (timestamps stored in SQLite)
- **Auto-update** — self-updater checks GitHub releases and hot-swaps the binary (disable in Docker — use image pull instead)
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

Build and run with Docker Compose (recommended):

```bash
cp .env.example .env   # set LG_ADMIN_PASSWORD
docker compose up -d --build
```

The container stores config and data under the `hopstat-data` volume (`/data`). On first start HopStat generates `/data/config.yaml` with random secrets unless you pin them via environment variables.

Admin credentials are written to the container log — check with `docker logs <container>`.

**Docker notes:**

- Set `LG_ADMIN_PASSWORD` for a known initial password (see `.env.example`).
- Set `LG_UPDATE_ENABLED=false` in containers — self-update replaces the binary inside the image; pull a new image instead.
- `NET_RAW` and `NET_ADMIN` capabilities are required for ping and traceroute.
- Optional GeoIP: uncomment `LG_GEOIP_LICENSE_KEY` and `LG_GEOIP_ACCOUNT_ID` in `docker-compose.yml`.

To pin secrets across image rebuilds:

```bash
-e LG_SECURITY_JWT_SECRET=$(openssl rand -hex 32) \
-e LG_SECURITY_CREDENTIAL_KEY=$(openssl rand -hex 32)
```

Or bootstrap with the installer:

```bash
bash install.sh --docker
```

See [`docker-compose.yml`](docker-compose.yml) for the full example.

### Build from source

Requires Go 1.25+ and Node.js 24+ (frontend build).

```bash
git clone https://github.com/HopStat/HopStat.git
cd HopStat
cd web/frontend && npm ci && npm run build && cd ../..
make build
./hopstat --mode=server
```

## Configuration

`config.yaml` is **auto-generated on first start** with every section from `config.example.yaml` — including flood control, GeoIP, BGP, query limits and TLS placeholders — plus random `jwt_secret` and `credential_key` values. No manual setup needed.

Site name, branding, query defaults and footer links are configured in **Admin → Settings**, not in YAML.

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
| `flood_control.enabled` | `LG_FLOOD_CONTROL_ENABLED` |
| `flood_control.http_rate_limit_per_min` | `LG_FLOOD_CONTROL_HTTP_RATE_LIMIT_PER_MIN` |
| `flood_control.query_rate_limit_per_min` | `LG_FLOOD_CONTROL_QUERY_RATE_LIMIT_PER_MIN` |
| `geoip.license_key` | `LG_GEOIP_LICENSE_KEY` |
| `geoip.account_id` | `LG_GEOIP_ACCOUNT_ID` |
| `geoip.update_interval` | `LG_GEOIP_UPDATE_INTERVAL` |
| `update.enabled` | `LG_UPDATE_ENABLED` |
| `query.max_concurrent` | `LG_QUERY_MAX_CONCURRENT` |

> **Note:** `LG_ADMIN_PASSWORD` and `LG_FORCE_ADMIN_PASSWORD` are special variables read directly at startup
> to set the admin password. They do not follow the viper key-path convention.
> On its own `LG_ADMIN_PASSWORD` only seeds an account that has never had a password set; adding
> `LG_FORCE_ADMIN_PASSWORD=1` replaces an existing one — see [Password recovery](#password-recovery).

### Config reference

See [`config.example.yaml`](config.example.yaml) for the complete reference. Key sections:

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "server"              # server | agent
  tls_cert: ""                # optional TLS certificate path
  tls_key: ""
  behind_cloudflare: false    # true when behind Cloudflare proxy
  trusted_proxies: []         # optional extra reverse-proxy CIDRs

database:
  path: "./lg.db"

security:
  jwt_secret: ""              # auto-generated; min 32 chars
  credential_key: ""          # auto-generated; 64 hex chars (AES-256-GCM for node credentials)

flood_control:
  enabled: true
  http_rate_limit_per_min: 100
  query_rate_limit_per_min: 100
  brute_force_max: 5
  brute_force_ban_min: 15

audit:
  retention_days: 90
  async_write: true

query:
  max_concurrent: 50
  default_timeout_sec: 30
  traceroute_timeout_sec: 60

geoip:
  asn_db_path: ""
  city_db_path: ""
  license_key: ""             # MaxMind account (free tier)
  account_id: ""
  db_dir: "./data/geoip"
  update_interval: "72h"      # downloads skipped until interval elapsed

update:
  enabled: true

bgp:
  listen_port: 11790
  router_id: ""
  local_as: 0
  listen_addresses: []
  add_path_receive: true      # learn multiple paths per prefix from ADD-PATH peers
```

**Agent mode** uses a separate template (`agent.port`, `agent.token`, etc.) — see [Deployment Modes](#deployment-modes).

## Deployment Modes

### Server mode (default)

Runs the HTTP API, React SPA and query engine. Connects directly to routers via SSH/Telnet or delegates to remote agents.

```bash
./hopstat --mode=server
```

### Agent mode

Lightweight REST server deployed on remote POPs. The central server discovers it as a node. Agents expose SSE streaming endpoints for live ping/traceroute output.

```bash
./hopstat --mode=agent
# Default port: 9090 (agent.port in config)
```

Auto-generated agent config includes a random `agent.token` used by the central server.

### Systemd service

```bash
sudo ./hopstat --install-service
# Installs to /usr/local/bin, generates /etc/hopstat/config.yaml,
# writes and starts /etc/systemd/system/hopstat.service
journalctl -u hopstat | grep -A 10 HOPSTAT   # view first-run credentials
```

## Admin Panel

Access at `/admin`. On a fresh install, a random admin password is generated and shown once in the installer output or service log. Change it in **Admin → Settings → Account** after logging in.

From the panel you can:

- Add router nodes (SSH/Telnet credentials are encrypted with AES-256-GCM)
- Configure site name, logo, header colour, active languages and query defaults in **Admin → Settings**
- Configure footer links (website, contact, terms, privacy, PeeringDB)
- Configure standalone nodes with an agent token for embedded API queries
- Configure BGP neighbors and default route AS for the embedded GoBGP peer (**Admin → BGP Neighbors**)
- Manage **Communities** rules — community string, severity, description and active toggle
- Manage **Quick Queries** — preset shortcuts shown on the public query page
- Run **GeoIP lookup** — inspect MaxMind/Cymru enrichment for any IP (**Admin → GeoIP**)
- View audit logs

### Password recovery

There is no self-service reset — no e-mail flow, no reset link. While you can still log in, change the
password in **Admin → Settings → Account**. If you are locked out, re-run `--bootstrap` with
`LG_FORCE_ADMIN_PASSWORD=1` to overwrite the password directly in the database.

**Systemd install:**

```bash
sudo systemctl stop hopstat
sudo env LG_ADMIN_PASSWORD='new-password' \
         LG_FORCE_ADMIN_PASSWORD=1 \
         /usr/local/bin/hopstat --bootstrap --config=/etc/hopstat/config.yaml
sudo systemctl start hopstat
```

`sudo` strips the environment, so `sudo env VAR=…` is required — plain `sudo LG_…=… hopstat` silently
drops both variables.

**Docker:**

```bash
docker compose stop hopstat
docker compose run --rm \
  -e LG_ADMIN_PASSWORD='new-password' \
  -e LG_FORCE_ADMIN_PASSWORD=1 \
  hopstat --bootstrap --config=/data/config.yaml
docker compose start hopstat
```

Pass both variables with `-e`. Putting them in `.env` is not enough — Compose only forwards variables
that are listed in the service's `environment:` block, and `LG_FORCE_ADMIN_PASSWORD` is not.

The output must contain `admin password set from LG_ADMIN_PASSWORD`. If it says
`admin password already configured — leaving unchanged`, the force flag never reached the process.

Notes:

- The new password is written to the database, so it survives restarts — no environment variable needs
  to stay set. Remove `LG_FORCE_ADMIN_PASSWORD` from any file you added it to, or every restart will
  overwrite the password you later set in the panel.
- Recovery targets the `admin@hopstat.local` account. If you changed the admin e-mail in
  **Settings → Account**, the reset fails with `no admin user found`; set it back first:

  ```bash
  sqlite3 /var/lib/hopstat/lg.db "UPDATE users SET email='admin@hopstat.local' WHERE id=1;"
  ```
- The same variables work on a normal server start (not just `--bootstrap`), which is how the Docker
  image is usually seeded.

### Communities

BGP community rules are configured in **Admin → Communities**. Each rule maps a community string (e.g. `65000:100`) to a description and severity (`info`, `success`, `warning`, `reject`). Active rules are:

- Matched automatically on BGP route lookups — colour-coded badges with hover descriptions
- Listed on the public **`/communities`** page (linked from the site footer)
- Exposed via **`GET /api/v1/communities`** (no authentication; active rules only)

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

HopStat prefers **MaxMind** for per-hop ASN and country enrichment (traceroute, AS path map). When databases are unavailable, **Team Cymru DNS** (`origin.asn.cymru.com` TXT lookups) is used as fallback.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for commit attribution (human authors only — no AI co-author trailers), repo hygiene, and release notes.

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
make test-cover    # internal packages — 100% statement coverage required
make lint
cd web/frontend && npm test
```

## License

MIT — see [LICENSE](LICENSE).
