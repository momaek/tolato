#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

"${ROOT_DIR}/scripts/setup-local-postgres.sh"
cd "${ROOT_DIR}"
exec go run ./cmd/tolato-server -config configs/server.local.yaml
