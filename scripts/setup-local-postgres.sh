#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONTAINER_NAME="tolato-postgres-1"
PG_USER="tolato"
PG_PASSWORD="tolato"
PG_BOOTSTRAP_DB="postgres"
PG_APP_DB="tolato_control_plane"
if ! find "${ROOT_DIR}/db/migrations" -maxdepth 1 -name '*.sql' | grep -q .; then
  echo "migration files not found under ${ROOT_DIR}/db/migrations" >&2
  exit 1
fi

if docker ps --format '{{.Names}}' | grep -qx "${CONTAINER_NAME}"; then
  echo "Reusing running PostgreSQL container: ${CONTAINER_NAME}"
else
  echo "Starting local PostgreSQL via docker compose"
  docker compose -f "${ROOT_DIR}/compose.yaml" up -d postgres
fi

echo "Waiting for PostgreSQL to accept connections"
until docker exec -e PGPASSWORD="${PG_PASSWORD}" "${CONTAINER_NAME}" \
  pg_isready -U "${PG_USER}" -d "${PG_BOOTSTRAP_DB}" >/dev/null 2>&1; do
  sleep 1
done

DB_EXISTS="$(
  docker exec -e PGPASSWORD="${PG_PASSWORD}" "${CONTAINER_NAME}" \
    psql -U "${PG_USER}" -d "${PG_BOOTSTRAP_DB}" -tAc \
    "SELECT 1 FROM pg_database WHERE datname = '${PG_APP_DB}'"
)"

if [[ "${DB_EXISTS}" != "1" ]]; then
  echo "Creating database: ${PG_APP_DB}"
  docker exec -e PGPASSWORD="${PG_PASSWORD}" "${CONTAINER_NAME}" \
    psql -U "${PG_USER}" -d "${PG_BOOTSTRAP_DB}" -c "CREATE DATABASE ${PG_APP_DB}"
else
  echo "Database already exists: ${PG_APP_DB}"
fi

echo "Applying migrations to ${PG_APP_DB}"
while IFS= read -r migration; do
  echo "  -> $(basename "${migration}")"
  docker exec -i -e PGPASSWORD="${PG_PASSWORD}" "${CONTAINER_NAME}" \
    psql -v ON_ERROR_STOP=1 -U "${PG_USER}" -d "${PG_APP_DB}" < "${migration}"
done < <(find "${ROOT_DIR}/db/migrations" -maxdepth 1 -name '*.sql' | sort)

echo "PostgreSQL is ready for Tolato"
echo "DSN: postgres://${PG_USER}:${PG_PASSWORD}@127.0.0.1:5432/${PG_APP_DB}?sslmode=disable"
