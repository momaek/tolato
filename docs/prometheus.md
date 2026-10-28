# Embedded node_exporter / Prometheus monitoring

Tolato-agent embeds the **upstream node_exporter v1.12.1 collector package** in the same binary and process. The feature is disabled by default. Enabling it uses the upstream default collector set for that OS, with the remaining supported collectors available through upstream flags. It does not install another service or listen on port 9100.

```text
upstream collectors in tolato-agent
        → existing authenticated WebSocket
        → tolato server's in-memory cache
        → GET /api/v1/metrics
        → Prometheus → Grafana
```

## Enable collection

Add the switch to an existing agent's command and restart it:

```sh
tolato-agent --server wss://tolato.example.com/ws/agent \
  --data-dir /var/lib/tolato --node-exporter.enabled
```

An already registered agent reuses its stored identity; `--token` is needed only for first registration. For a new installation:

```sh
curl -fsSL https://tolato.example.com/install.sh | bash -s -- \
  --server https://tolato.example.com --token YOUR_REGISTRATION_TOKEN \
  --node-exporter.enabled
```

For an existing systemd service, run `systemctl edit tolato-agent` and replace its command (retain your original server/data directory):

```ini
[Service]
ExecStart=
ExecStart=/usr/local/bin/tolato-agent --server wss://tolato.example.com/ws/agent --data-dir /var/lib/tolato --node-exporter.enabled
```

Then run `systemctl daemon-reload` and `systemctl restart tolato-agent`. Remove `--node-exporter.enabled` or use `--no-node-exporter.enabled` and restart to disable collection. A restart creates a new connection with an empty cache, so previously collected host metrics are no longer exported.

| Agent option | Default | Meaning |
| --- | --- | --- |
| `--node-exporter.enabled` | false | Initialize upstream collectors and enable metrics transport |
| `--node-exporter.interval` | 30s | Collection period, between 5s and 1h |
| `--node-exporter.timeout` | 10s | Collection deadline, positive and no greater than the interval |
| `--collector.disable-defaults` | false | Start with no host collectors enabled |
| `--collector.<name>` / `--no-collector.<name>` | upstream defaults | Enable/disable a host collector |
| `--web.disable-exporter-metrics` | false | Exclude Go/process self-metrics |

The agent retains upstream collector options, including filesystem/network include/exclude expressions, textfile directories, `--path.procfs`, `--path.sysfs`, and `--path.rootfs`. Options and collector availability vary by OS; `tolato-agent --help` lists the flags for that build. For example:

```sh
tolato-agent --server wss://tolato.example.com/ws/agent \
  --data-dir /var/lib/tolato --node-exporter.enabled \
  --collector.textfile.directory=/var/lib/node_exporter/textfile_collector \
  --collector.netdev.device-exclude='^(veth.*|docker.*)$'
```

Or select only a few collectors on a small machine:

```sh
tolato-agent --server wss://tolato.example.com/ws/agent \
  --data-dir /var/lib/tolato --node-exporter.enabled \
  --collector.disable-defaults --collector.cpu --collector.meminfo \
  --collector.loadavg --collector.filesystem --collector.netdev
```

Full integration makes upstream collector capabilities available; it does not enable all optional collectors. Some need kernel features, mounted host paths, external services, or additional privileges. For a containerized agent, the usual node_exporter host mounts/namespaces and matching `--path.*` options are still needed to measure the host rather than the container. Windows/node_exporter-windows is not included; release targets remain Linux and macOS.

`node_exporter_build_info` retains the upstream schema, with `version="1.12.1"`. Its revision/toolchain/platform describe the combined build. The default Go and process metrics describe **tolato-agent as a whole**, not a separate exporter process. HTTP-server flags and HTTP-request self-metrics are not applicable because there is no node-local exporter HTTP server.

## Configure Prometheus

Create a dedicated read-only API key owned by a user who can view the desired nodes. Store the key in a file readable by Prometheus:

```yaml
scrape_configs:
  - job_name: tolato
    scheme: https
    metrics_path: /api/v1/metrics
    scrape_interval: 30s
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/secrets/tolato-api-key
    static_configs:
      - targets: ['tolato.example.com']
```

The endpoint uses existing API-key authentication and resolves the owner's current node visibility on every scrape. Revoked keys and disabled owners are rejected. Removing a node grant removes that node's metrics and metadata from subsequent scrapes. Writable keys also work, but monitoring needs only read access. The endpoint is not exposed anonymously, and responses have `Cache-Control: no-store`.

Historical storage and retention belong to Prometheus. The server stores only the latest validated snapshot per active connection; it never waits for an agent during a scrape.

## Labels, queries, and dashboards

Upstream metric names, types, units, HELP text, and collector-specific labels are preserved. The server adds authenticated `node_id` to every forwarded metric. Alias, hostname, and IP live in `tolato_node_info{node_id,alias,hostname,ip}=1`; changing an alias does not recreate all host series.

This is one central scrape target. Its `up`, `job`, and default `instance` refer to the tolato server, not individual nodes. Dashboards written for one node_exporter scrape target per machine need to group/filter by `node_id` instead of `instance`.

Examples (the CPU counter is available where supported upstream):

```promql
# CPU usage percentage per node, using a window spanning several snapshots.
100 * (1 - avg by (node_id) (rate(node_cpu_seconds_total{mode="idle"}[5m])))

# Received bytes/second, separately for each interface and node.
rate(node_network_receive_bytes_total[5m])

# Available filesystem space as a percentage.
100 * node_filesystem_avail_bytes / node_filesystem_size_bytes

# Add human-readable metadata to a per-node CPU query.
(100 * (1 - avg by (node_id) (rate(node_cpu_seconds_total{mode="idle"}[5m]))))
  * on (node_id) group_left(hostname, alias) tolato_node_info

# Connected and enabled, but no valid recent snapshot.
(tolato_node_metrics_fresh == 0) and on (node_id) (tolato_node_metrics_enabled == 1)

# A particular upstream collector failed.
node_scrape_collector_success == 0
```

The agent samples on its own interval and Prometheus timestamps cached values at scrape time. Multiple scrapes may see the same snapshot; rate windows should span several collection intervals. No offline backlog or precise per-snapshot historical timeline is retained.

## Freshness and failure behavior

The server uses its receipt clock for expiry, so node clock skew does not govern the TTL. Configure an explicit TTL in the server YAML if needed:

```yaml
metrics:
  freshness_ttl: 0s # 0 = three times each agent's collection interval
```

| Metric | Meaning |
| --- | --- |
| `tolato_node_connected` | Agent WebSocket is currently connected |
| `tolato_node_metrics_enabled` | Connected agent reports node_exporter enabled |
| `tolato_node_metrics_fresh` | Latest validated snapshot is within TTL |
| `tolato_node_metrics_collection_success` | Latest snapshot passed collection/validation |
| `tolato_node_metrics_export_success` | Host snapshot is included in this scrape |
| `tolato_node_metrics_last_success_timestamp_seconds` | Agent collection time of the last valid snapshot |
| `tolato_node_metrics_last_received_timestamp_seconds` | Server receipt time of the last valid snapshot |

Before the first snapshot, freshness/export success and timestamps are zero. Old or disabled agents report no exporter data. Once a snapshot expires, its host metrics disappear while tolato status/metadata remain. Disconnect, replacement connection, or server restart discards the connection's cache; timestamps then start at zero. Deleting a node removes it from the scrape entirely.

An upstream collector failure is represented by `node_scrape_collector_success{collector=...}=0`; other successfully collected metrics can still be included, following upstream behavior. A registry/serialization/timeout/validation failure rejects the entire snapshot, clears its host metrics, and sets collection success to zero. Last-success timestamps remain until the connection is removed.

Conflicting metric types/HELP or flattened histogram/summary names across nodes suppress the entire conflicting node snapshot and set export success to zero. Nodes are considered in stable ID order. Custom textfiles must not define `tolato_*` metric names, `node_id` labels, or explicit sample timestamps; these are rejected to protect the central exporter's identity and freshness rules.

## Resource limits and compatibility

- One collection may run at a time, even after a timeout. Upstream collectors do not all support cancellation. A timed-out collector may remain running, but later ticks cannot create additional stuck collections. Restart the agent to recover a permanently stuck collector.
- Snapshots are limited to 4 MiB of uncompressed text and 50,000 samples. Oversize or invalid snapshots fail visibly; they are not silently truncated. The server also caps decoded gzip size to prevent decompression bombs.
- Transport uses gzip/base64 when this reduces payload size. Metrics have no unbounded send queue: if another WebSocket writer is busy, that snapshot is discarded. Metrics writes have a bounded deadline; as with other failed WebSocket writes, a failed write closes the socket and triggers reconnection.
- Agents negotiate metrics protocol version 1 through `register_ack`. With an older server, an opted-in agent stays connected without collecting/sending unsupported metrics. Old agents continue their existing heartbeat behavior on the new server.
- Heartbeats, command execution, file operations, and PTY remain separate protocol messages. Metrics are read-only host collection; existing remote operation permissions are unchanged.

## Building and verification

The agent now requires Go 1.25 or later. Linux release binaries remain static (`CGO_ENABLED=0`). Upstream macOS collectors require CGO and system frameworks, so macOS releases build with `CGO_ENABLED=1` on macOS runners for both amd64 and arm64. The upstream package is pinned in `agent/go.mod`; update the pin, the embedded version constant, and this document together, then rerun compatibility tests.

```sh
(cd agent && go test -race ./...)
(cd server && go test -race ./...)
```

The real PostgreSQL authorization test uses a dedicated disposable database and rolls its fixture transaction back:

```sh
cd server
TOLATO_TEST_POSTGRES_DSN='host=127.0.0.1 port=5432 user=postgres password=test dbname=tolato_metrics_test sslmode=disable' \
  go test -race ./internal/handler -run TestMetricsAuthorizationIntegration
```

Coverage includes comparison with the pinned upstream collector registry, optional textfile configuration, timeout containment, compressed payload limits, old-server negotiation, trusted identity, stale/disconnected caches, conflicting snapshots, replacement connections, and live API-key/node-permission revocation. The PostgreSQL test also verifies that a fleet of 205 nodes is not truncated to the interactive API's page size.

### Resource measurement (2026-10-28)

A local Linux/arm64 Alpine 3.20 container on Docker's LinuxKit VM (10 visible CPUs), capped at 512 MiB, was connected to a WebSocket test server. These are laboratory observations, not a VPS resource guarantee; available devices, filesystems, kernel features, and collector choices affect the results.

| Measurement | Observed |
| --- | --- |
| Stripped Linux/arm64 binary before embedding | 6.0 MiB |
| Stripped Linux/arm64 binary with embedding | 12.8 MiB |
| Container working set, feature disabled | approximately 5–9 MiB |
| Container working set, upstream defaults enabled | approximately 23 MiB |
| Samples per snapshot | approximately 1,405 |
| Uncompressed exposition text | approximately 112.5 KB |
| Compressed JSON payload, including base64 | approximately 23.6 KB |
| Mean process CPU, 5s sampling, 20s observation | approximately 0.55% of one core |

The faster 5s interval was used for the test; the default remains 30s. At a 30s interval, this payload size is about 0.8 KB/s per node before WebSocket/TLS overhead. Existing remote command execution succeeded with collection both enabled and disabled. The actual memory/CPU footprint on a small VPS should be measured with its own default and optional collectors before choosing an interval.
