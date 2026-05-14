#!/usr/bin/env bash
set -euo pipefail

# HopStat Network Looking Glass — Install Script
# Usage: curl -sSL https://raw.githubusercontent.com/HopStat/HopStat/main/install.sh | bash
# Or:    bash install.sh [--no-service] [--mode agent] [--version v1.0.0]

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

while [[ $# -gt 0 ]]; do
  case $1 in
    --no-service) NO_SERVICE=true; shift ;;
    --mode)       MODE="$2"; shift 2 ;;
    --version)    VERSION="$2"; shift 2 ;;
    --help|-h)
      echo "HopStat Network Looking Glass — Installer"
      echo ""
      echo "Usage: install.sh [OPTIONS]"
      echo ""
      echo "Options:"
      echo "  --no-service      Skip systemd service installation"
      echo "  --mode MODE       Run mode: server (default) or agent"
      echo "  --version TAG     Install specific version (default: latest)"
      echo "  --help, -h        Show this help"
      echo ""
      echo "Examples:"
      echo "  curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | bash"
      echo "  bash install.sh --mode agent"
      echo "  bash install.sh --version v1.2.0"
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
echo "  ║   HopStat — Network Looking Glass         ║"
echo "  ║   https://github.com/HopStat/HopStat      ║"
echo "  ╚════════════════════════════════════════════╝"
echo -e "${NC}"

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

# ── Create directories ────────────────────────────────────────────────────────
mkdir -p "$CONFIG_DIR" "$DATA_DIR"
ok "Directories ready: ${CONFIG_DIR}, ${DATA_DIR}"

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
echo -e "${GREEN}║   HopStat installed successfully!         ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════╝${NC}"
echo ""
echo "  Binary:   ${INSTALL_DIR}/${BINARY}"
echo "  Config:   ${CONFIG_FILE}  (auto-generated on first start)"
echo "  Data:     ${DATA_DIR}"
echo ""
if [[ "$MODE" == "server" ]]; then
  echo "  Admin UI: http://localhost:8080/admin"
  echo ""
  echo "  First-run admin credentials appear in the service log:"
  echo -e "  ${CYAN}journalctl -u hopstat | grep -A 10 HOPSTAT${NC}"
fi
echo ""
echo "  Logs:     journalctl -u hopstat -f"
echo "  Status:   systemctl status hopstat"
echo "  Update:   curl -sSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash"
echo ""
