#!/usr/bin/env bash
set -euo pipefail

# HopStat Network Looking Glass — Install Script
# Usage: curl -sSL https://raw.githubusercontent.com/HopStat/HopStat/main/install.sh | bash
# Or:    bash install.sh [--no-service] [--mode agent] [--version v2.0.0] [--docker] [--local] [--fresh]

REPO="HopStat/HopStat"
BINARY="hopstat"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/hopstat"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
DATA_DIR="/var/lib/hopstat"
SERVICE_FILE="/etc/systemd/system/hopstat.service"
MODE="server"
VERSION=""
NO_SERVICE=false
ADMIN_PASSWORD=""
ADMIN_EMAIL="admin@hopstat.local"
DOCKER=false
LOCAL=false
FRESH=false

while [[ $# -gt 0 ]]; do
  case $1 in
    --no-service) NO_SERVICE=true; shift ;;
    --mode)       MODE="$2"; shift 2 ;;
    --version)    VERSION="$2"; shift 2 ;;
    --docker)     DOCKER=true; shift ;;
    --local)      LOCAL=true; shift ;;
    --fresh)      FRESH=true; shift ;;
    --help|-h)
      echo "HopStat Network Looking Glass — Installer"
      echo ""
      echo "Usage: install.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --no-service      Skip systemd service installation"
      echo "  --mode MODE       Run mode: server (default) or agent"
      echo "  --version TAG     Install specific version (default: latest)"
      echo "  --docker          Bootstrap Docker volume with admin credentials (.env)"
      echo "  --local           Local dev install (macOS/Linux, no root, builds from source)"
      echo "  --fresh           Remove existing config/data and install from scratch"
      echo "  --help, -h        Show this help"
      echo ""
      echo "Examples:"
      echo "  curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash"
      echo "  bash install.sh --mode agent"
      echo "  bash install.sh --version v2.0.0"
      echo "  bash install.sh --local"
      echo "  bash install.sh --fresh --version v2.0.0"
      echo "  curl -sSL .../install.sh | sudo bash -s -- --fresh"
      exit 0
      ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $*"; }
ok()   { echo -e "${GREEN}[ OK ]${NC} $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $*"; }
fail() { echo -e "${RED}[FAIL]${NC} $*"; exit 1; }

echo -e "${CYAN}"
echo "  ╔════════════════════════════════════════════╗"
echo "  ║   HopStat — Network Looking Glass          ║"
echo "  ║   https://github.com/HopStat/HopStat       ║"
echo "  ╚════════════════════════════════════════════╝"
echo -e "${NC}"

generate_admin_password() {
  head -c 24 /dev/urandom | base64 | tr -d '=/+' | head -c 20
}

sed_inplace() {
  if [[ "$(uname -s)" == "Darwin" ]]; then
    sed -i '' "$@"
  else
    sed -i "$@"
  fi
}

read_config_server_port() {
  local config_file="$1"
  local default_port="${2:-8080}"

  if [[ -n "${LG_SERVER_PORT:-}" ]] && [[ "${LG_SERVER_PORT}" =~ ^[0-9]+$ ]]; then
    echo "${LG_SERVER_PORT}"
    return
  fi

  if [[ ! -f "$config_file" ]]; then
    echo "$default_port"
    return
  fi

  local port
  port=$(awk '
    /^server:/ { in_server=1; next }
    /^[a-zA-Z0-9_-]+:/ && !/^server:/ { if (in_server) exit; in_server=0 }
    in_server && $1 == "port:" { gsub(/"/, "", $2); print $2; exit }
  ' "$config_file" 2>/dev/null)

  if [[ -z "$port" ]] || ! [[ "$port" =~ ^[0-9]+$ ]]; then
    echo "$default_port"
  else
    echo "$port"
  fi
}

format_admin_ui_url() {
  local host="${1:-localhost}"
  local port="$2"

  if [[ "$port" == "443" ]]; then
    echo "https://${host}/admin"
  elif [[ "$port" == "80" ]]; then
    echo "http://${host}/admin"
  else
    echo "http://${host}:${port}/admin"
  fi
}

admin_ui_url_from_config() {
  local config_file="$1"
  local host="${2:-localhost}"
  local port
  port=$(read_config_server_port "$config_file")
  format_admin_ui_url "$host" "$port"
}

read_docker_compose_host_port() {
  local compose_file="$1"
  local container_port="$2"

  [[ -f "$compose_file" ]] || { echo "$container_port"; return; }

  local line spec host target
  while IFS= read -r line; do
    spec=$(echo "$line" | sed -E 's/^[[:space:]]*-[[:space:]]*"?(.*?)"?[[:space:]]*$/\1/')
    if [[ "$spec" =~ ^([0-9]+):([0-9]+)$ ]]; then
      host="${BASH_REMATCH[1]}"
      target="${BASH_REMATCH[2]}"
      if [[ "$target" == "$container_port" ]]; then
        echo "$host"
        return
      fi
    fi
  done < <(grep -E '^\s*-\s*"?[0-9]+:[0-9]+' "$compose_file" 2>/dev/null || true)

  echo "$container_port"
}

admin_ui_url_for_docker() {
  local compose_file="$1"
  local host="${2:-localhost}"
  local tmp_config container_port host_port

  tmp_config=$(mktemp)
  if docker compose run --rm --no-deps --entrypoint cat hopstat /data/config.yaml > "$tmp_config" 2>/dev/null; then
    container_port=$(read_config_server_port "$tmp_config")
  else
    container_port=8080
  fi
  rm -f "$tmp_config"

  host_port=$(read_docker_compose_host_port "$compose_file" "$container_port")
  format_admin_ui_url "$host" "$host_port"
}

stop_local_hopstat() {
  local pid_file="$1"
  if [[ -f "$pid_file" ]]; then
    local pid
    pid=$(cat "$pid_file")
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
      sleep 1
    fi
    rm -f "$pid_file"
  fi
}

fresh_cleanup() {
  local target_config="$1"
  local target_data="$2"
  local extra_cleanup="${3:-}"

  warn "Fresh install — removing existing HopStat data"
  if [[ -n "$extra_cleanup" ]]; then
    eval "$extra_cleanup"
  fi
  rm -f "${target_config}"
  rm -f "${target_data}/lg.db" "${target_data}/lg.db-shm" "${target_data}/lg.db-wal"
  rm -rf "${target_data}/geoip"
  ok "Cleaned ${target_config} and ${target_data}"
}

# ── Docker install (macOS/Linux with Docker) ─────────────────────────────────
if [[ "$DOCKER" == true ]]; then
  command -v docker &>/dev/null || fail "docker is required for --docker"
  docker compose version &>/dev/null || fail "docker compose is required for --docker"

  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  cd "$SCRIPT_DIR"

  ENV_FILE="${SCRIPT_DIR}/.env"
  if [[ "$FRESH" == true ]]; then
    docker compose down -v 2>/dev/null || true
    rm -f "$ENV_FILE"
    ok "Removed Docker volume and .env"
  fi

  if [[ -f "$ENV_FILE" ]] && grep -q '^LG_ADMIN_PASSWORD=' "$ENV_FILE" && [[ "$FRESH" != true ]]; then
    ADMIN_PASSWORD=$(grep '^LG_ADMIN_PASSWORD=' "$ENV_FILE" | cut -d= -f2-)
    info "Using existing LG_ADMIN_PASSWORD from .env"
  else
    ADMIN_PASSWORD=$(generate_admin_password)
    cat > "$ENV_FILE" << EOF
# HopStat Docker secrets (generated by install.sh)
LG_ADMIN_PASSWORD=${ADMIN_PASSWORD}
EOF
    chmod 600 "$ENV_FILE"
    ok "Wrote ${ENV_FILE}"
  fi

  info "Building Docker image..."
  docker compose build

  info "Bootstrapping data volume (config, database, admin user)..."
  LG_FORCE_ADMIN_PASSWORD=1 docker compose run --rm hopstat --bootstrap --config=/data/config.yaml

  info "Starting HopStat..."
  docker compose up -d

  sleep 3
  if docker compose ps --status running --format '{{.Name}}' | grep -q hopstat; then
    ok "HopStat is running"
  else
    warn "Container may have failed to start."
    warn "Check logs: docker compose logs hopstat"
  fi

  echo ""
  echo -e "${GREEN}╔════════════════════════════════════════════╗${NC}"
  echo -e "${GREEN}║   HopStat Docker install complete!         ║${NC}"
  echo -e "${GREEN}╚════════════════════════════════════════════╝${NC}"
  echo ""
  echo "  Admin UI:       $(admin_ui_url_for_docker "${SCRIPT_DIR}/docker-compose.yml")"
  echo "  Data volume:    hopstat-data"
  echo ""
  if [[ -n "$ADMIN_PASSWORD" ]]; then
    echo -e "  ${GREEN}Admin Email:    ${ADMIN_EMAIL}${NC}"
    echo -e "  ${GREEN}Admin Password: ${ADMIN_PASSWORD}${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠  Change this password in Admin → Settings after login.${NC}"
  fi
  echo ""
  echo "  Logs:     docker compose logs -f hopstat"
  echo "  Stop:     docker compose down"
  echo "  Reset:    bash install.sh --docker --fresh"
  echo ""
  exit 0
fi

# ── Local dev install (macOS / Linux, no root) ───────────────────────────────
if [[ "$LOCAL" == true ]]; then
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  cd "$SCRIPT_DIR"

  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)
  case $ARCH in
    x86_64|amd64)  ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    *) fail "Unsupported architecture: ${ARCH}" ;;
  esac
  [[ "$OS" == "darwin" || "$OS" == "linux" ]] || fail "--local supports macOS and Linux only"

  INSTALL_DIR="${HOME}/.local/bin"
  CONFIG_DIR="${HOME}/.config/hopstat"
  CONFIG_FILE="${CONFIG_DIR}/config.yaml"
  DATA_DIR="${HOME}/.local/share/hopstat"
  PID_FILE="${DATA_DIR}/hopstat.pid"
  LOG_FILE="${DATA_DIR}/hopstat.log"

  mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$DATA_DIR"
  ok "Local paths ready"

  command -v go &>/dev/null || fail "Go is required for --local (https://go.dev/dl/)"
  [[ -f "${SCRIPT_DIR}/go.mod" ]] || fail "Run --local from the HopStat source directory"

  if [[ ! -d "${SCRIPT_DIR}/web/dist" ]] || [[ ! -f "${SCRIPT_DIR}/web/dist/index.html" ]]; then
    if command -v npm &>/dev/null && [[ -f "${SCRIPT_DIR}/web/frontend/package.json" ]]; then
      info "Building frontend..."
      (cd "${SCRIPT_DIR}/web/frontend" && npm ci && npm run build)
      ok "Frontend built"
    else
      fail "web/dist missing — run 'cd web/frontend && npm ci && npm run build' first"
    fi
  fi

  stop_local_hopstat "$PID_FILE"
  if [[ -f "${SCRIPT_DIR}/docker-compose.yml" ]] && command -v docker &>/dev/null; then
    docker compose down 2>/dev/null || true
  fi

  if [[ "$FRESH" == true ]]; then
    fresh_cleanup "$CONFIG_FILE" "$DATA_DIR" \
      "rm -f \"${DATA_DIR}/hopstat.log\" \"${DATA_DIR}/hopstat-start.sh\" \"${DATA_DIR}/hopstat-stop.sh\""
  fi

  info "Building hopstat binary..."
  VERSION=$(git -C "$SCRIPT_DIR" describe --tags --always 2>/dev/null || echo "dev")
  CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=${VERSION}" \
    -o "${INSTALL_DIR}/${BINARY}" ./cmd/lg/
  ok "Binary built: ${INSTALL_DIR}/${BINARY}"

  if [[ "$MODE" == "server" && ! -f "$CONFIG_FILE" ]]; then
    ADMIN_PASSWORD=$(generate_admin_password)
    info "Bootstrapping config and admin user..."
    LG_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
    LG_DATABASE_PATH="${DATA_DIR}/lg.db" \
    LG_FORCE_ADMIN_PASSWORD=1 \
      "${INSTALL_DIR}/${BINARY}" --bootstrap --config="$CONFIG_FILE"
    ok "Config and admin user initialized"
  fi

  if [[ "$MODE" == "server" && -f "$CONFIG_FILE" ]]; then
    sed_inplace \
      -e "s|path: \"./lg.db\"|path: \"${DATA_DIR}/lg.db\"|" \
      -e "s|db_dir: \"./data/geoip\"|db_dir: \"${DATA_DIR}/geoip\"|" \
      "$CONFIG_FILE"
  fi

  cat > "${DATA_DIR}/hopstat-start.sh" << EOF
#!/usr/bin/env bash
cd "${DATA_DIR}"
exec "${INSTALL_DIR}/${BINARY}" --mode="${MODE}" --config="${CONFIG_FILE}"
EOF
  chmod +x "${DATA_DIR}/hopstat-start.sh"

  cat > "${DATA_DIR}/hopstat-stop.sh" << EOF
#!/usr/bin/env bash
PID_FILE="${PID_FILE}"
if [[ -f "\$PID_FILE" ]]; then
  kill "\$(cat "\$PID_FILE")" 2>/dev/null || true
  rm -f "\$PID_FILE"
fi
EOF
  chmod +x "${DATA_DIR}/hopstat-stop.sh"

  info "Starting HopStat..."
  (
    nohup "${DATA_DIR}/hopstat-start.sh" >> "${LOG_FILE}" 2>&1 </dev/null &
    echo $! > "${PID_FILE}"
  )
  sleep 2

  if kill -0 "$(cat "$PID_FILE")" 2>/dev/null; then
    ok "HopStat is running (pid $(cat "$PID_FILE"))"
  else
    warn "HopStat may have failed to start."
    warn "Check logs: tail -f ${LOG_FILE}"
  fi

  echo ""
  echo -e "${GREEN}╔════════════════════════════════════════════╗${NC}"
  echo -e "${GREEN}║   HopStat local install complete!          ║${NC}"
  echo -e "${GREEN}╚════════════════════════════════════════════╝${NC}"
  echo ""
  echo "  Binary:     ${INSTALL_DIR}/${BINARY}"
  echo "  Config:     ${CONFIG_FILE}"
  echo "  Data:       ${DATA_DIR}"
  echo "  Logs:       ${LOG_FILE}"
  echo "  Start:      ${DATA_DIR}/hopstat-start.sh"
  echo "  Stop:       ${DATA_DIR}/hopstat-stop.sh"
  if [[ ":$PATH:" != *":${INSTALL_DIR}:"* ]]; then
    echo ""
    echo "  Add to PATH:  export PATH=\"${INSTALL_DIR}:\$PATH\""
  fi
  if [[ "$MODE" == "server" ]]; then
    echo "  Admin UI:   $(admin_ui_url_from_config "${CONFIG_FILE}")"
    echo ""
    if [[ -n "$ADMIN_PASSWORD" ]]; then
      echo -e "  ${GREEN}Admin Email:    ${ADMIN_EMAIL}${NC}"
      echo -e "  ${GREEN}Admin Password: ${ADMIN_PASSWORD}${NC}"
      echo ""
      echo -e "  ${YELLOW}⚠  Change this password in Admin → Settings after login.${NC}"
    fi
  fi
  echo ""
  echo "  Stop:       ${DATA_DIR}/hopstat-stop.sh"
  echo "  Reinstall:  bash install.sh --local"
  echo "  Fresh:      bash install.sh --local --fresh"
  echo ""
  exit 0
fi

# ── Root check ────────────────────────────────────────────────────────────────
[[ $EUID -ne 0 ]] && fail "This script must be run as root (use sudo)"

# ── OS / arch ─────────────────────────────────────────────────────────────────
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case $ARCH in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "Unsupported architecture: ${ARCH}" ;;
esac

[[ "$OS" != "linux" ]] && fail "Only Linux is supported by this installer."

info "Detected: ${OS}/${ARCH} — mode: ${MODE}"

# ── Resolve latest version ────────────────────────────────────────────────────
if [[ -z "$VERSION" ]]; then
  info "Fetching latest release..."
  VERSION=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' \
    | sed -E 's/.*"([^"]+)".*/\1/' 2>/dev/null || echo "")
  [[ -z "$VERSION" ]] && fail "Could not determine latest version. Use --version to specify."
fi

info "Installing HopStat ${VERSION}..."

# ── Warn if already installed ─────────────────────────────────────────────────
if command -v hopstat &>/dev/null; then
  CURRENT=$("${INSTALL_DIR}/hopstat" --version 2>&1 | head -1 || echo "unknown")
  warn "HopStat already installed: ${CURRENT}"
  warn "Continuing with reinstall..."
fi

# ── Checksum tool ─────────────────────────────────────────────────────────────
SHA_TOOL=""
if command -v sha256sum &>/dev/null; then
  SHA_TOOL="sha256sum"
elif command -v shasum &>/dev/null; then
  SHA_TOOL="shasum -a 256"
fi

# ── Download binary ───────────────────────────────────────────────────────────
BINARY_NAME="${BINARY}-${OS}-${ARCH}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY_NAME}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

TMP_FILE=$(mktemp)
TMP_CHECKSUMS=$(mktemp)
cleanup() { rm -f "$TMP_FILE" "$TMP_CHECKSUMS"; }
trap cleanup EXIT

info "Downloading ${BINARY_NAME}..."
if ! curl -fsSL --progress-bar -o "$TMP_FILE" "$DOWNLOAD_URL"; then
  fail "Download failed: ${DOWNLOAD_URL}"
fi

# ── Integrity verification ────────────────────────────────────────────────────
if [[ -n "$SHA_TOOL" ]]; then
  if curl -fsSL -o "$TMP_CHECKSUMS" "$CHECKSUMS_URL" 2>/dev/null; then
    EXPECTED=$(grep -E "[[:space:]]\\*?${BINARY_NAME}\$" "$TMP_CHECKSUMS" | awk '{print $1}' | head -n1)
    if [[ -n "$EXPECTED" ]]; then
      ACTUAL=$($SHA_TOOL "$TMP_FILE" | awk '{print $1}')
      if [[ "$EXPECTED" != "$ACTUAL" ]]; then
        fail "Checksum mismatch for ${BINARY_NAME} — aborting."
      fi
      ok "Checksum verified"
    else
      warn "No checksum entry for ${BINARY_NAME} in checksums.txt — skipping verification."
    fi
  else
    warn "checksums.txt not available — skipping integrity check."
  fi
else
  warn "sha256sum/shasum not found — skipping integrity check."
fi

# ── Install binary ────────────────────────────────────────────────────────────
chmod +x "$TMP_FILE"
mv "$TMP_FILE" "${INSTALL_DIR}/${BINARY}"
ok "Binary installed: ${INSTALL_DIR}/${BINARY}"

INSTALLED_VERSION=$("${INSTALL_DIR}/${BINARY}" --version 2>&1 | head -1 || echo "unknown")
ok "${INSTALLED_VERSION}"

# ── Probe tools ───────────────────────────────────────────────────────────────
# A standalone or agent node shells out to these. Missing, they do not fail here —
# they fail later, once per query, as an exec error the operator has to go and read.
detect_pkg_manager() {
  local pm
  for pm in apt-get dnf yum apk pacman zypper; do
    if command -v "$pm" &>/dev/null; then echo "$pm"; return 0; fi
  done
  return 1
}

package_for() { # $1 = tool, $2 = package manager
  case "$1" in
    traceroute) echo "traceroute" ;;
    ping)       [[ "$2" == "apt-get" ]] && echo "iputils-ping" || echo "iputils" ;;
  esac
}

# Package managers are chatty even when told to be quiet, and their output does not belong
# in the middle of the installer's. It is kept and shown only if the install fails.
install_packages() { # $1 = package manager, rest = packages
  local pm=$1; shift
  local log status=0
  log=$(mktemp)
  case "$pm" in
    apt-get)
      { DEBIAN_FRONTEND=noninteractive apt-get update -qq \
          && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@"; } >"$log" 2>&1 || status=$?
      ;;
    dnf)    dnf install -y -q "$@"            >"$log" 2>&1 || status=$? ;;
    yum)    yum install -y -q "$@"            >"$log" 2>&1 || status=$? ;;
    apk)    apk add --no-cache -q "$@"        >"$log" 2>&1 || status=$? ;;
    pacman) pacman -Sy --noconfirm --needed "$@" >"$log" 2>&1 || status=$? ;;
    zypper) zypper --non-interactive install "$@" >"$log" 2>&1 || status=$? ;;
    *)      rm -f "$log"; return 1 ;;
  esac
  if [[ $status -ne 0 ]]; then
    warn "${pm} failed (exit ${status}):"
    tail -n 15 "$log" | sed 's/^/       /'
  fi
  rm -f "$log"
  return $status
}

MISSING_TOOLS=()
for TOOL in ping traceroute; do
  command -v "$TOOL" &>/dev/null || MISSING_TOOLS+=("$TOOL")
done

if [[ ${#MISSING_TOOLS[@]} -eq 0 ]]; then
  ok "Probe tools present: ping, traceroute"
else
  warn "Missing probe tools: ${MISSING_TOOLS[*]}"
  if PKG_MANAGER=$(detect_pkg_manager); then
    PACKAGES=()
    for TOOL in "${MISSING_TOOLS[@]}"; do
      PACKAGES+=("$(package_for "$TOOL" "$PKG_MANAGER")")
    done
    info "Installing with ${PKG_MANAGER}: ${PACKAGES[*]}"
    install_packages "$PKG_MANAGER" "${PACKAGES[@]}" \
      || warn "Package install returned an error — checking anyway"

    STILL_MISSING=()
    for TOOL in "${MISSING_TOOLS[@]}"; do
      command -v "$TOOL" &>/dev/null || STILL_MISSING+=("$TOOL")
    done
    if [[ ${#STILL_MISSING[@]} -eq 0 ]]; then
      ok "Probe tools installed: ${MISSING_TOOLS[*]}"
    else
      warn "Could not install: ${STILL_MISSING[*]}"
      warn "Queries using them will fail on this node until they are installed by hand."
    fi
  else
    warn "No supported package manager found (apt-get, dnf, yum, apk, pacman, zypper)."
    warn "Install ${MISSING_TOOLS[*]} by hand, or queries using them will fail on this node."
  fi
fi

# ── Create directories ────────────────────────────────────────────────────────
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
ok "Directories ready: ${CONFIG_DIR}, ${DATA_DIR}"

# Stop any running instance before bootstrap (avoids DB race on reinstall)
if command -v systemctl &>/dev/null; then
  systemctl stop hopstat 2>/dev/null || true
fi

if [[ "$FRESH" == true ]]; then
  fresh_cleanup "$CONFIG_FILE" "$DATA_DIR"
fi

# ── Bootstrap config & admin password (server mode only, first install) ───────
if [[ "$MODE" == "server" ]]; then
  if [[ ! -f "$CONFIG_FILE" ]]; then
    ADMIN_PASSWORD=$(generate_admin_password)
    info "Initializing config and admin user: ${CONFIG_FILE}"
    BOOT_LOG=$(mktemp)
    if ! LG_ADMIN_PASSWORD="$ADMIN_PASSWORD" \
      LG_DATABASE_PATH="${DATA_DIR}/lg.db" \
      LG_FORCE_ADMIN_PASSWORD=1 \
        "${INSTALL_DIR}/${BINARY}" --bootstrap --config="$CONFIG_FILE" 2>&1 | tee "$BOOT_LOG"; then
      rm -f "$BOOT_LOG"
      fail "Bootstrap failed — config/admin user was not initialized"
    fi
    if grep -q "admin password already configured" "$BOOT_LOG"; then
      warn "Admin password was NOT updated in ${DATA_DIR}/lg.db"
      warn "Your hopstat binary is outdated — update, then delete ${CONFIG_FILE} and re-run this installer."
      ADMIN_PASSWORD=""
    elif ! grep -qE "admin password set|generated random admin password" "$BOOT_LOG"; then
      warn "Could not confirm admin password was written to the database"
      ADMIN_PASSWORD=""
    fi
    rm -f "$BOOT_LOG"
    ok "Config and admin user initialized in ${DATA_DIR}"
  else
    info "Existing config found — skipping admin password setup"
  fi

  if [[ -f "$CONFIG_FILE" ]]; then
    if [[ "$(uname -s)" == "Darwin" ]]; then
      sed -i '' \
        -e "s|path: \"./lg.db\"|path: \"${DATA_DIR}/lg.db\"|" \
        -e "s|db_dir: \"./data/geoip\"|db_dir: \"${DATA_DIR}/geoip\"|" \
        "$CONFIG_FILE"
    else
      sed -i \
        -e "s|path: \"./lg.db\"|path: \"${DATA_DIR}/lg.db\"|" \
        -e "s|db_dir: \"./data/geoip\"|db_dir: \"${DATA_DIR}/geoip\"|" \
        "$CONFIG_FILE"
    fi
  fi
fi

# ── Systemd service ───────────────────────────────────────────────────────────
if [[ "$NO_SERVICE" == true ]]; then
  info "Skipping service installation (--no-service)"
  info "Run manually: ${INSTALL_DIR}/${BINARY} --mode=${MODE} --config=${CONFIG_FILE}"
else
  if ! command -v systemctl &>/dev/null; then
    warn "systemd not found — skipping service installation."
    info "Run manually: ${INSTALL_DIR}/${BINARY} --mode=${MODE} --config=${CONFIG_FILE}"
  else
    cat > "$SERVICE_FILE" << SERVICE
[Unit]
Description=HopStat Network Looking Glass
Documentation=https://github.com/HopStat/HopStat
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${DATA_DIR}
ExecStart=${INSTALL_DIR}/${BINARY} --mode=${MODE} --config=${CONFIG_FILE}
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=hopstat
AmbientCapabilities=CAP_NET_RAW CAP_NET_ADMIN

[Install]
WantedBy=multi-user.target
SERVICE
    ok "Service file written: ${SERVICE_FILE}"

    systemctl daemon-reload
    systemctl enable hopstat 2>/dev/null || true
    ok "Service enabled (hopstat)"

    if systemctl is-active hopstat &>/dev/null; then
      systemctl restart hopstat
      ok "Service restarted"
    else
      systemctl start hopstat
      ok "Service started"
    fi

    sleep 2
    if systemctl is-active hopstat &>/dev/null; then
      ok "HopStat is running"
    else
      warn "Service may have failed to start."
      warn "Check logs: journalctl -u hopstat -e"
    fi
  fi
fi

# ── Summary ───────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   HopStat installed successfully!          ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════╝${NC}"
echo ""
echo "  Binary:   ${INSTALL_DIR}/${BINARY}"
echo "  Config:   ${CONFIG_FILE}  (auto-generated on first start)"
echo "  Data:     ${DATA_DIR}"
echo ""
if [[ "$MODE" == "server" ]]; then
  echo "  Admin UI: $(admin_ui_url_from_config "${CONFIG_FILE}")"
  echo ""
  if [[ -n "$ADMIN_PASSWORD" ]]; then
    echo -e "  ${GREEN}Admin Email:    ${ADMIN_EMAIL}${NC}"
    echo -e "  ${GREEN}Admin Password: ${ADMIN_PASSWORD}${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠  Change this password in Admin → Settings after login.${NC}"
  fi
fi
echo ""
echo "  Logs:     journalctl -u hopstat -f"
echo "  Status:   systemctl status hopstat"
echo "  Update:   curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash"
echo "  Fresh:    curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash -s -- --fresh"
echo ""
