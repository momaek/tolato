#!/usr/bin/env bash
set -euo pipefail

# install-nodeagent.sh — Download, install, and configure tolato-nodeagent as a systemd service.
#
# Usage:
#   curl -fsSL https://your-server/install-nodeagent.sh | bash -s -- \
#     --server-url ws://10.0.0.1:8080/ws/agent \
#     --auth-token <token> \
#     --download-url https://releases.example.com/tolato-nodeagent-linux-amd64 \
#     --version v1.0.0
#
# Options:
#   --server-url     WebSocket endpoint of tolato-server (required)
#   --auth-token     Bearer token for agent authentication (required)
#   --download-url   URL to download the nodeagent binary (required)
#   --node-id        Unique node identifier (auto-generated from machine-id + hostname if omitted)
#   --version        Version label for the agent (default: unknown)
#   --region         Node region label (optional)
#   --tags           Comma-separated node tags (optional)
#   --install-dir    Installation directory (default: /usr/local/bin)

INSTALL_DIR="/usr/local/bin"
SERVICE_NAME="tolato-nodeagent"
BINARY_NAME="tolato-nodeagent"
SERVER_URL=""
NODE_ID=""
AUTH_TOKEN=""
DOWNLOAD_URL=""
VERSION="unknown"
REGION=""
TAGS=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --server-url)   SERVER_URL="$2";   shift 2 ;;
    --node-id)      NODE_ID="$2";      shift 2 ;;
    --auth-token)   AUTH_TOKEN="$2";    shift 2 ;;
    --download-url) DOWNLOAD_URL="$2"; shift 2 ;;
    --version)      VERSION="$2";      shift 2 ;;
    --region)       REGION="$2";       shift 2 ;;
    --tags)         TAGS="$2";         shift 2 ;;
    --install-dir)  INSTALL_DIR="$2";  shift 2 ;;
    *)
      echo "Unknown option: $1" >&2
      exit 1
      ;;
  esac
done

# Auto-generate node-id from machine-id + hostname if not specified
if [[ -z "$NODE_ID" ]]; then
  MACHINE_PREFIX=""
  if [[ -f /etc/machine-id ]]; then
    MACHINE_PREFIX=$(head -c 8 /etc/machine-id)
  elif [[ -f /var/lib/dbus/machine-id ]]; then
    MACHINE_PREFIX=$(head -c 8 /var/lib/dbus/machine-id)
  fi

  HOSTNAME_PART=$(hostname -s 2>/dev/null || echo "node")

  if [[ -n "$MACHINE_PREFIX" ]]; then
    NODE_ID="${MACHINE_PREFIX}-${HOSTNAME_PART}"
  else
    NODE_ID="${HOSTNAME_PART}"
  fi

  echo "==> Auto-generated node-id: ${NODE_ID}"
fi

if [[ -z "$SERVER_URL" || -z "$AUTH_TOKEN" || -z "$DOWNLOAD_URL" ]]; then
  echo "Error: --server-url, --auth-token, and --download-url are required." >&2
  exit 1
fi

BINARY_PATH="${INSTALL_DIR}/${BINARY_NAME}"

echo "==> Downloading ${BINARY_NAME} from ${DOWNLOAD_URL} ..."
TMP_FILE=$(mktemp)
trap 'rm -f "$TMP_FILE"' EXIT

if command -v curl &>/dev/null; then
  curl -fsSL -o "$TMP_FILE" "$DOWNLOAD_URL"
elif command -v wget &>/dev/null; then
  wget -qO "$TMP_FILE" "$DOWNLOAD_URL"
else
  echo "Error: curl or wget is required." >&2
  exit 1
fi

chmod 0755 "$TMP_FILE"

echo "==> Installing to ${BINARY_PATH} ..."
sudo mv "$TMP_FILE" "$BINARY_PATH"
trap - EXIT

echo "==> Creating systemd service ..."

EXTRA_ARGS=""
if [[ -n "$REGION" ]]; then
  EXTRA_ARGS="${EXTRA_ARGS} -region '${REGION}'"
fi
if [[ -n "$TAGS" ]]; then
  EXTRA_ARGS="${EXTRA_ARGS} -tags '${TAGS}'"
fi

sudo tee "/etc/systemd/system/${SERVICE_NAME}.service" > /dev/null <<EOF
[Unit]
Description=Tolato Node Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${BINARY_PATH} \\
  -server '${SERVER_URL}' \\
  -node-id '${NODE_ID}' \\
  -auth-token '${AUTH_TOKEN}' \\
  -agent-version '${VERSION}'${EXTRA_ARGS}
Restart=always
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

echo "==> Enabling and starting ${SERVICE_NAME} ..."
sudo systemctl daemon-reload
sudo systemctl enable "${SERVICE_NAME}"
sudo systemctl restart "${SERVICE_NAME}"

echo "==> Done! ${SERVICE_NAME} is running."
echo "    Node ID: ${NODE_ID}"
echo "    Binary:  ${BINARY_PATH}"
echo "    Version: ${VERSION}"
echo "    Status:  sudo systemctl status ${SERVICE_NAME}"
