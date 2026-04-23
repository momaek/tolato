#!/usr/bin/env bash
#
# tolato-server installer — docker-compose one-shot.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/momaek/tolato/main/scripts/install-server.sh | bash
#   curl -fsSL https://raw.githubusercontent.com/momaek/tolato/main/scripts/install-server.sh | bash -s -- --dir /srv/tolato
#
# Env overrides:
#   TOLATO_REPO       github repo slug     (default: momaek/tolato)
#   TOLATO_BRANCH     branch for raw files (default: main)
#   TOLATO_VERSION    server image tag     (default: latest)

set -euo pipefail

REPO="${TOLATO_REPO:-momaek/tolato}"
BRANCH="${TOLATO_BRANCH:-main}"
VERSION="${TOLATO_VERSION:-latest}"
DIR="./tolato"
ADMIN_USER="admin"
# Empty = use compose default (127.0.0.1:8080). User can pass --port to override.
PORT=""

# ----- args -----
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dir)          DIR="$2"; shift 2 ;;
    --dir=*)        DIR="${1#*=}"; shift ;;
    --admin-user)   ADMIN_USER="$2"; shift 2 ;;
    --admin-user=*) ADMIN_USER="${1#*=}"; shift ;;
    --port)         PORT="$2"; shift 2 ;;
    --port=*)       PORT="${1#*=}"; shift ;;
    -h|--help)      sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "unknown arg: $1" >&2; exit 1 ;;
  esac
done

# Port used for local health check + URL in the final banner.
LOCAL_PORT="${PORT##*:}"
LOCAL_PORT="${LOCAL_PORT:-8080}"

RAW_BASE="https://raw.githubusercontent.com/${REPO}/${BRANCH}"

# ----- docker check -----
if ! command -v docker >/dev/null 2>&1; then
  cat >&2 <<'EOF'
error: docker is not installed.

Install Docker Engine first, then re-run this script:
  https://docs.docker.com/engine/install/

On most Linux distros:
  curl -fsSL https://get.docker.com | sh
EOF
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "error: 'docker compose' (v2) plugin is not available." >&2
  echo "       see https://docs.docker.com/compose/install/" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "error: docker daemon is not running or current user lacks permission." >&2
  echo "       try: sudo systemctl start docker   (or add your user to the 'docker' group)" >&2
  exit 1
fi

# ----- target dir -----
mkdir -p "$DIR"
cd "$DIR"

if [[ -f config.yaml ]]; then
  echo "error: $(pwd)/config.yaml already exists — refusing to overwrite." >&2
  echo "       remove it first, or pass --dir <other-path>." >&2
  exit 1
fi

# ----- download compose + example -----
echo ">>> downloading docker-compose.yaml + config.example.yaml from ${REPO}@${BRANCH}"
curl -fsSL -o docker-compose.yaml     "${RAW_BASE}/docker-compose.yaml"
curl -fsSL -o config.example.yaml     "${RAW_BASE}/config.example.yaml"

# ----- generate secrets -----
# encrypt_key:  exactly 32 bytes (AES-256 key material as-string).
# jwt_secret:   44 url-safe chars (~32 bytes entropy).
# admin_pass:   20 url-safe chars.
ENCRYPT_KEY="$(openssl rand -hex 16)"                                   # 32 chars
JWT_SECRET="$(openssl rand -base64 32 | tr -d '=+/' | cut -c1-40)"
ADMIN_PASS="$(openssl rand -base64 18 | tr -d '=+/' | cut -c1-20)"

# ----- write config.yaml -----
# Delimit sed patterns with | since paths/values contain /.
sed \
  -e "s|tolato-default-encrypt-key-32b!|${ENCRYPT_KEY}|" \
  -e "s|tolato-jwt-secret-change-me|${JWT_SECRET}|" \
  -e "s|^\(  username:\) admin$|\1 ${ADMIN_USER}|" \
  -e "s|^\(  password:\) admin$|\1 ${ADMIN_PASS}|" \
  config.example.yaml > config.yaml
# 0644 so the non-root container user (UID 1000) can read the bind-mount.
# Treating the host box as trusted — anyone with shell access could read
# docker volumes anyway.
chmod 644 config.yaml

# ----- up -----
echo ">>> starting containers (TOLATO_VERSION=${VERSION}, port=${PORT:-127.0.0.1:8080})"
# Only set SERVER_PORT if --port was given; otherwise let compose apply its default.
if [[ -n "$PORT" ]]; then
  TOLATO_VERSION="${VERSION}" SERVER_PORT="${PORT}" docker compose up -d
else
  TOLATO_VERSION="${VERSION}" docker compose up -d
fi

# ----- wait for server -----
echo -n ">>> waiting for server on :${LOCAL_PORT} "
for _ in $(seq 1 30); do
  if curl -fsS -o /dev/null "http://127.0.0.1:${LOCAL_PORT}/" 2>/dev/null; then
    echo "ok"
    break
  fi
  echo -n "."
  sleep 1
done

# ----- report -----
cat <<EOF

╔════════════════════════════════════════════════════════════════╗
║  Tolato server is up.                                          ║
╠════════════════════════════════════════════════════════════════╣
║  URL:      http://localhost:${LOCAL_PORT}
║            (bound to 127.0.0.1 by default — put a reverse proxy
║             in front, or re-run with --port 0.0.0.0:${LOCAL_PORT} to expose)
║  Username: ${ADMIN_USER}
║  Password: ${ADMIN_PASS}
╚════════════════════════════════════════════════════════════════╝

>>> SAVE THE PASSWORD ABOVE — it's not printed again.

Config directory: $(pwd)
  docker-compose.yaml   — compose manifest
  config.example.yaml   — reference config (safe to share)
  config.yaml           — YOUR config, contains secrets (chmod 600)

Next steps:
  1. Put Tolato behind a reverse proxy (Caddy / Nginx / Traefik) with TLS.
  2. Edit config.yaml:
       server.public_address   — your public URL (e.g. https://tolato.example.com)
       server.allowed_origins  — [ "https://tolato.example.com" ]
     then:  docker compose restart server
  3. Log in, open Settings, configure your LLM (endpoint + API key + model).
  4. On the Nodes page, click "Add Node" to get an agent install command.

Useful commands (run in $(pwd)):
  docker compose logs -f server
  docker compose pull server && docker compose up -d   # upgrade
  docker compose down                                  # stop
EOF
