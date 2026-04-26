#!/bin/sh
# Ensure /app/data (the runtime data dir, e.g. geoip mmdb cache) is writable
# by the unprivileged tolato user. This handles the case where the host
# bind-mounted an empty directory that Docker created as root — same trick
# the official postgres image uses for /var/lib/postgresql/data.
set -e

chown -R tolato:tolato /app/data 2>/dev/null || true

exec su-exec tolato:tolato /usr/local/bin/tolato-server "$@"
