#!/usr/bin/env bash
#
# tolato-agent installer
#
# Usage (typically via the command shown in the Web UI):
#   curl -fsSL http://<server>/install.sh | bash -s -- --token <token> --server <host:port>
#
# Env overrides:
#   TOLATO_AGENT_VERSION   release tag to install (default: latest)
#   TOLATO_REPO            github repo slug       (default: momaek/tolato)
#   TOLATO_INSTALL_DIR     binary install dir     (default: /usr/local/bin)
#   TOLATO_DATA_DIR        agent data dir         (default: /var/lib/tolato)
#   TOLATO_DOWNLOAD_BASE   binary download base   (default: derived from --server,
#                                                  e.g. https://<server>/releases)

set -euo pipefail

REPO="${TOLATO_REPO:-momaek/tolato}"
BIN_NAME="tolato-agent"
INSTALL_DIR="${TOLATO_INSTALL_DIR:-/usr/local/bin}"
DATA_DIR="${TOLATO_DATA_DIR:-/var/lib/tolato}"
SERVICE_NAME="tolato-agent"
VERSION="${TOLATO_AGENT_VERSION:-latest}"

TOKEN=""
SERVER=""

# ----- parse args -------------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    --token)    TOKEN="$2"; shift 2 ;;
    --token=*)  TOKEN="${1#*=}"; shift ;;
    --server)   SERVER="$2"; shift 2 ;;
    --server=*) SERVER="${1#*=}"; shift ;;
    -h|--help)
      sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

[[ -z "$TOKEN"  ]] && { echo "error: --token is required"  >&2; exit 1; }
[[ -z "$SERVER" ]] && { echo "error: --server is required" >&2; exit 1; }

# ----- normalize server → ws URL ---------------------------------------------
case "$SERVER" in
  ws://*|wss://*) WS_URL="$SERVER" ;;
  http://*)       WS_URL="ws://${SERVER#http://}/ws/agent" ;;
  https://*)      WS_URL="wss://${SERVER#https://}/ws/agent" ;;
  *)              WS_URL="ws://${SERVER}/ws/agent" ;;
esac

# ----- detect os / arch -------------------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *) echo "error: unsupported OS: $OS" >&2; exit 1 ;;
esac

ARCH_RAW="$(uname -m)"
case "$ARCH_RAW" in
  x86_64|amd64)   ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *) echo "error: unsupported arch: $ARCH_RAW" >&2; exit 1 ;;
esac

# ----- require root or passwordless sudo --------------------------------------
# We refuse to continue without privilege instead of prompting: this script is
# usually piped from `curl | bash`, where stdin isn't a tty so a sudo password
# prompt would hang.
if [[ ${EUID:-$(id -u)} -eq 0 ]]; then
  SUDO=""
else
  if ! command -v sudo >/dev/null 2>&1; then
    echo "error: not running as root and 'sudo' is not installed." >&2
    echo "       re-run as root: su -c '<command>'" >&2
    exit 1
  fi
  if ! sudo -n true 2>/dev/null; then
    echo "error: sudo requires a password (or is not permitted for this user)." >&2
    echo "       re-run the install command prefixed with sudo, e.g.:" >&2
    echo "         curl -fsSL <url> | sudo bash -s -- --token ... --server ..." >&2
    exit 1
  fi
  SUDO="sudo"
fi

# ----- download binary --------------------------------------------------------
ASSET="${BIN_NAME}-${OS}-${ARCH}"

# Pick a download base URL. Preference:
#   1. TOLATO_DOWNLOAD_BASE env (manual override)
#   2. derive from --server: use the tolato server itself as a mirror
#      (it proxies GET /releases/* to github.com so agents in regions where
#      github is blocked can still install).
# We always also fall back to direct GitHub on failure, so older servers
# without the /releases proxy still work.
GH_BASE="https://github.com/${REPO}/releases"
DOWNLOAD_BASE="${TOLATO_DOWNLOAD_BASE:-}"
if [[ -z "$DOWNLOAD_BASE" ]]; then
  case "$SERVER" in
    http://*|https://*) DOWNLOAD_BASE="${SERVER%/}/releases" ;;
    ws://*)             DOWNLOAD_BASE="http://${SERVER#ws://}";  DOWNLOAD_BASE="${DOWNLOAD_BASE%/ws/agent}/releases" ;;
    wss://*)            DOWNLOAD_BASE="https://${SERVER#wss://}"; DOWNLOAD_BASE="${DOWNLOAD_BASE%/ws/agent}/releases" ;;
    *)                  DOWNLOAD_BASE="http://${SERVER%/}/releases" ;;
  esac
fi

if [[ "$VERSION" == "latest" ]]; then
  URL="${DOWNLOAD_BASE}/latest/download/${ASSET}"
  GH_URL="${GH_BASE}/latest/download/${ASSET}"
else
  URL="${DOWNLOAD_BASE}/download/${VERSION}/${ASSET}"
  GH_URL="${GH_BASE}/download/${VERSION}/${ASSET}"
fi

TMP="$(mktemp -t tolato-agent.XXXXXX)"
trap 'rm -f "$TMP"' EXIT

download() {
  curl -fL --retry 3 --retry-delay 2 -o "$TMP" "$1"
}

echo ">>> downloading ${URL}"
if ! download "$URL"; then
  if [[ "$URL" != "$GH_URL" ]]; then
    echo ">>> mirror failed, falling back to ${GH_URL}"
    if ! download "$GH_URL"; then
      echo "error: download failed from both mirror and github. Is a release published for ${OS}/${ARCH}?" >&2
      exit 1
    fi
  else
    echo "error: download failed. Is a release published for ${OS}/${ARCH}?" >&2
    exit 1
  fi
fi

$SUDO install -m 0755 "$TMP" "${INSTALL_DIR}/${BIN_NAME}"
$SUDO mkdir -p "$DATA_DIR"
echo ">>> installed to ${INSTALL_DIR}/${BIN_NAME}"

# ----- systemd (linux) --------------------------------------------------------
if [[ "$OS" == "linux" ]] && command -v systemctl >/dev/null 2>&1; then
  UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"

  $SUDO tee "$UNIT_PATH" >/dev/null <<EOF
[Unit]
Description=Tolato Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
ExecStart=${INSTALL_DIR}/${BIN_NAME} --server ${WS_URL} --token ${TOKEN} --data-dir ${DATA_DIR}
Restart=always
RestartSec=5
# Note: --token is only used on first run; subsequent restarts reuse the stored identity.

[Install]
WantedBy=multi-user.target
EOF

  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now "${SERVICE_NAME}"
  echo ">>> service ${SERVICE_NAME} started"
  echo "    logs:   journalctl -u ${SERVICE_NAME} -f"
  echo "    status: systemctl status ${SERVICE_NAME}"
else
  cat <<EOF
>>> systemd not detected. Run manually:
    ${INSTALL_DIR}/${BIN_NAME} --server ${WS_URL} --token ${TOKEN} --data-dir ${DATA_DIR}
EOF
fi
