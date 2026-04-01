package server

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/momaek/tolato/internal/nodeprobe/model"
)

// Queryer abstracts *sql.DB and *sql.Tx.
type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

// Store wraps the probe data access layer (backed by PostgreSQL).
type Store struct {
	q Queryer
}

// NewStore creates a Store for probe data.
func NewStore(q Queryer) *Store {
	return &Store{q: q}
}

// --- Nodes ---

func (s *Store) UpsertNode(ctx context.Context, n model.Node) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO probe_nodes (id, name, role, last_seen) VALUES ($1, $2, $3, $4)
		 ON CONFLICT(id) DO UPDATE SET name=EXCLUDED.name, role=EXCLUDED.role, last_seen=EXCLUDED.last_seen`,
		n.ID, n.Name, string(n.Role), n.LastSeen.UTC(),
	)
	return err
}

func (s *Store) ListNodes(ctx context.Context) ([]model.Node, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, name, role, last_seen FROM probe_nodes ORDER BY role, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		var role string
		if err := rows.Scan(&n.ID, &n.Name, &role, &n.LastSeen); err != nil {
			return nil, err
		}
		n.Role = model.NodeRole(role)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// --- Links ---

func (s *Store) UpsertLink(ctx context.Context, l model.Link) error {
	_, err := s.q.ExecContext(ctx,
		`INSERT INTO probe_links (id, source_id, target_id) VALUES ($1, $2, $3)
		 ON CONFLICT(id) DO UPDATE SET source_id=EXCLUDED.source_id, target_id=EXCLUDED.target_id`,
		l.ID, l.SourceID, l.TargetID,
	)
	return err
}

func (s *Store) ListLinks(ctx context.Context) ([]model.Link, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, source_id, target_id FROM probe_links`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		var l model.Link
		if err := rows.Scan(&l.ID, &l.SourceID, &l.TargetID); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}

func (s *Store) ListLinksWithStatus(ctx context.Context) ([]model.LinkStatus, error) {
	query := `
		SELECT l.id, l.source_id, l.target_id,
		       COALESCE(sn.name, l.source_id), COALESCE(tn.name, l.target_id),
		       m.latency_avg, m.packet_loss, m.tcp_connect_time, m.bandwidth_mbps, m.timestamp
		FROM probe_links l
		LEFT JOIN probe_nodes sn ON sn.id = l.source_id
		LEFT JOIN probe_nodes tn ON tn.id = l.target_id
		LEFT JOIN LATERAL (
		    SELECT latency_avg, packet_loss, tcp_connect_time, bandwidth_mbps, timestamp
		    FROM probe_metrics WHERE link_id = l.id ORDER BY timestamp DESC LIMIT 1
		) m ON true
		ORDER BY l.source_id, l.target_id
	`
	rows, err := s.q.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.LinkStatus
	for rows.Next() {
		var ls model.LinkStatus
		var latAvg, pktLoss, tcpTime, bwMbps sql.NullFloat64
		var ts sql.NullTime
		if err := rows.Scan(
			&ls.ID, &ls.SourceID, &ls.TargetID,
			&ls.SourceName, &ls.TargetName,
			&latAvg, &pktLoss, &tcpTime, &bwMbps, &ts,
		); err != nil {
			return nil, err
		}
		if latAvg.Valid {
			ls.LatencyAvg = &latAvg.Float64
		}
		if pktLoss.Valid {
			ls.PacketLoss = &pktLoss.Float64
		}
		if tcpTime.Valid {
			ls.TCPConnectTime = &tcpTime.Float64
		}
		if bwMbps.Valid {
			ls.BandwidthMbps = &bwMbps.Float64
		}
		if ts.Valid {
			t := ts.Time
			ls.LastUpdated = &t
		}
		ls.Status = computeLinkStatus(ls)
		out = append(out, ls)
	}
	return out, rows.Err()
}

func computeLinkStatus(ls model.LinkStatus) string {
	if ls.LatencyAvg == nil {
		return "unknown"
	}
	if (ls.LatencyAvg != nil && *ls.LatencyAvg > 200) ||
		(ls.PacketLoss != nil && *ls.PacketLoss > 5) ||
		(ls.TCPConnectTime != nil && *ls.TCPConnectTime > 500) {
		return "alert"
	}
	if (ls.LatencyAvg != nil && *ls.LatencyAvg > 100) ||
		(ls.PacketLoss != nil && *ls.PacketLoss > 1) ||
		(ls.TCPConnectTime != nil && *ls.TCPConnectTime > 200) {
		return "warn"
	}
	return "ok"
}

// --- Metrics ---

func (s *Store) InsertMetrics(ctx context.Context, rows []model.MetricRow) error {
	for _, r := range rows {
		var bw interface{}
		if r.BandwidthMbps != nil {
			bw = *r.BandwidthMbps
		}
		if _, err := s.q.ExecContext(ctx,
			`INSERT INTO probe_metrics (link_id, timestamp, latency_min, latency_avg, latency_max, packet_loss, tcp_connect_time, bandwidth_mbps)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			r.LinkID, r.Timestamp.UTC(),
			r.LatencyMin, r.LatencyAvg, r.LatencyMax,
			r.PacketLoss, r.TCPConnectTime, bw,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) QueryMetrics(ctx context.Context, linkID string, from, to time.Time) ([]model.MetricRow, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, link_id, timestamp, latency_min, latency_avg, latency_max,
		        packet_loss, tcp_connect_time, bandwidth_mbps
		 FROM probe_metrics WHERE link_id = $1 AND timestamp >= $2 AND timestamp <= $3
		 ORDER BY timestamp ASC`,
		linkID, from.UTC(), to.UTC(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMetricRows(rows)
}

func scanMetricRows(rows *sql.Rows) ([]model.MetricRow, error) {
	var out []model.MetricRow
	for rows.Next() {
		var r model.MetricRow
		var bw sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.LinkID, &r.Timestamp,
			&r.LatencyMin, &r.LatencyAvg, &r.LatencyMax,
			&r.PacketLoss, &r.TCPConnectTime, &bw); err != nil {
			return nil, err
		}
		if bw.Valid {
			r.BandwidthMbps = &bw.Float64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Alerts ---

func (s *Store) InsertAlert(ctx context.Context, a model.Alert) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx,
		`INSERT INTO probe_alerts (link_id, type, message, triggered_at) VALUES ($1, $2, $3, $4) RETURNING id`,
		a.LinkID, string(a.Type), a.Message, a.TriggeredAt.UTC(),
	).Scan(&id)
	return id, err
}

func (s *Store) ResolveAlert(ctx context.Context, id int64, resolvedAt time.Time) error {
	_, err := s.q.ExecContext(ctx,
		`UPDATE probe_alerts SET resolved_at = $1 WHERE id = $2`, resolvedAt.UTC(), id)
	return err
}

func (s *Store) ListAlerts(ctx context.Context, f model.AlertFilter) ([]model.Alert, error) {
	query := `SELECT id, link_id, type, message, triggered_at, resolved_at FROM probe_alerts WHERE true`
	args := []interface{}{}
	argIdx := 1

	if f.LinkID != nil {
		query += fmt.Sprintf(" AND link_id = $%d", argIdx)
		args = append(args, *f.LinkID)
		argIdx++
	}
	if f.Type != nil {
		query += fmt.Sprintf(" AND type = $%d", argIdx)
		args = append(args, string(*f.Type))
		argIdx++
	}
	if f.Status != nil {
		switch *f.Status {
		case "open":
			query += " AND resolved_at IS NULL"
		case "resolved":
			query += " AND resolved_at IS NOT NULL"
		}
	}
	query += " ORDER BY triggered_at DESC"
	if f.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	rows, err := s.q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []model.Alert
	for rows.Next() {
		var a model.Alert
		var typ string
		var resolved sql.NullTime
		if err := rows.Scan(&a.ID, &a.LinkID, &typ, &a.Message, &a.TriggeredAt, &resolved); err != nil {
			return nil, err
		}
		a.Type = model.AlertType(typ)
		if resolved.Valid {
			a.ResolvedAt = &resolved.Time
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}

func (s *Store) OpenAlerts(ctx context.Context, linkID string, alertType model.AlertType) ([]model.Alert, error) {
	status := "open"
	return s.ListAlerts(ctx, model.AlertFilter{
		LinkID: &linkID,
		Type:   &alertType,
		Status: &status,
	})
}

// --- Cleanup ---

func (s *Store) RunCleanup(ctx context.Context, retentionDays int, logger *log.Logger) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	cleanup := func() {
		before := time.Now().AddDate(0, 0, -retentionDays).UTC()
		res, _ := s.q.ExecContext(ctx, `DELETE FROM probe_metrics WHERE timestamp < $1`, before)
		n, _ := res.RowsAffected()
		res2, _ := s.q.ExecContext(ctx, `DELETE FROM probe_alerts WHERE triggered_at < $1`, before)
		n2, _ := res2.RowsAffected()
		if n > 0 || n2 > 0 {
			logger.Printf("probe cleanup: deleted %d metrics, %d alerts older than %d days", n, n2, retentionDays)
		}
	}

	cleanup()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cleanup()
		}
	}
}

// --- Node offline queries ---

func (s *Store) NodesOfflineSince(ctx context.Context, since time.Time) ([]model.Node, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, name, role, last_seen FROM probe_nodes WHERE last_seen < $1`, since.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []model.Node
	for rows.Next() {
		var n model.Node
		var role string
		if err := rows.Scan(&n.ID, &n.Name, &role, &n.LastSeen); err != nil {
			return nil, err
		}
		n.Role = model.NodeRole(role)
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

func (s *Store) LinksForTarget(ctx context.Context, targetID string) ([]model.Link, error) {
	rows, err := s.q.QueryContext(ctx,
		`SELECT id, source_id, target_id FROM probe_links WHERE target_id = $1`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []model.Link
	for rows.Next() {
		var l model.Link
		if err := rows.Scan(&l.ID, &l.SourceID, &l.TargetID); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
}
