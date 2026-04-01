-- NodeProbe: link monitoring tables

CREATE TABLE IF NOT EXISTS probe_nodes (
    id        TEXT PRIMARY KEY,
    name      TEXT NOT NULL,
    role      TEXT NOT NULL DEFAULT 'landing',
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS probe_links (
    id        TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES probe_nodes(id) ON DELETE CASCADE,
    target_id TEXT NOT NULL REFERENCES probe_nodes(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS probe_metrics (
    id               BIGSERIAL PRIMARY KEY,
    link_id          TEXT NOT NULL REFERENCES probe_links(id) ON DELETE CASCADE,
    timestamp        TIMESTAMPTZ NOT NULL,
    latency_min      DOUBLE PRECISION NOT NULL DEFAULT 0,
    latency_avg      DOUBLE PRECISION NOT NULL DEFAULT 0,
    latency_max      DOUBLE PRECISION NOT NULL DEFAULT 0,
    packet_loss      DOUBLE PRECISION NOT NULL DEFAULT 0,
    tcp_connect_time DOUBLE PRECISION NOT NULL DEFAULT 0,
    bandwidth_mbps   DOUBLE PRECISION
);

CREATE INDEX IF NOT EXISTS idx_probe_metrics_link_time
    ON probe_metrics (link_id, timestamp DESC);

CREATE TABLE IF NOT EXISTS probe_alerts (
    id           BIGSERIAL PRIMARY KEY,
    link_id      TEXT NOT NULL REFERENCES probe_links(id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    message      TEXT NOT NULL,
    triggered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_probe_alerts_link_time
    ON probe_alerts (link_id, triggered_at DESC);
