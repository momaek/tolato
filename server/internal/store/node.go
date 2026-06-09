package store

import (
	"time"

	"github.com/google/uuid"
	"github.com/momaek/tolato/server/internal/geoip"
	"github.com/momaek/tolato/server/internal/model"
)

// --- Registration Tokens ---

// CreateRegistrationToken creates a reusable registration token with expiry.
// A non-positive expiry means the token never expires.
func CreateRegistrationToken(aliasPrefix *string, expiry time.Duration) (*model.RegistrationToken, error) {
	var expiresAt time.Time
	if expiry > 0 {
		expiresAt = time.Now().Add(expiry)
	} else {
		// Far-future sentinel so the existing "expires_at > now" check still passes.
		expiresAt = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	}
	token := &model.RegistrationToken{
		ID:          uuid.New().String(),
		AliasPrefix: aliasPrefix,
		ExpiresAt:   expiresAt,
	}
	if err := DB.Create(token).Error; err != nil {
		return nil, err
	}
	return token, nil
}

// GetRegistrationToken returns a token by ID if it exists and hasn't expired.
func GetRegistrationToken(tokenID string) (*model.RegistrationToken, error) {
	var token model.RegistrationToken
	if err := DB.Where("id = ? AND expires_at > ?", tokenID, time.Now()).First(&token).Error; err != nil {
		return nil, err
	}
	return &token, nil
}

// --- Nodes ---

// CreateNodeFromRegistration creates a new Node when an agent registers.
// geo carries optional country/city/ASN resolved from reg.IP; pass a zero
// geoip.Result if lookup was skipped or failed.
func CreateNodeFromRegistration(reg model.AgentRegisterPayload, alias *string, agentSecret string, geo geoip.Result) (*model.Node, error) {
	node := &model.Node{
		ID:            uuid.New().String(),
		Name:          reg.Hostname,
		Alias:         alias,
		IP:            reg.IP,
		CountryCode:   geo.CountryCode,
		City:          geo.City,
		ASN:           geo.ASN,
		OS:            reg.OS,
		Kernel:        reg.Kernel,
		AgentVersion:  reg.AgentVersion,
		CPUCores:      reg.CPUCores,
		MemoryTotalMB: reg.MemoryTotalMB,
		DiskTotalGB:   reg.DiskTotalGB,
		Status:        "online",
		AgentSecret:   agentSecret,
	}
	if err := DB.Create(node).Error; err != nil {
		return nil, err
	}
	return node, nil
}

// ListNodesMissingGeo returns nodes that have an IP but no GeoIP data yet.
// Used to backfill region/ASN once the geoip service has data available.
func ListNodesMissingGeo() ([]model.Node, error) {
	var nodes []model.Node
	err := DB.Where("ip <> '' AND country_code = ''").Find(&nodes).Error
	return nodes, err
}

// ListNodes returns paginated nodes with optional status filter.
func ListNodes(page, pageSize int, status string) ([]model.Node, int64, error) {
	var total int64
	query := DB.Model(&model.Node{})
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var nodes []model.Node
	offset := (page - 1) * pageSize
	q := DB.Order("created_at DESC").Offset(offset).Limit(pageSize)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	err := q.Find(&nodes).Error
	return nodes, total, err
}

// GetNodeByID returns a single node by ID.
func GetNodeByID(id string) (*model.Node, error) {
	var node model.Node
	if err := DB.First(&node, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// GetNodeBySecret finds a node by ID and validates its secret for reconnection.
func GetNodeBySecret(nodeID, secret string) (*model.Node, error) {
	var node model.Node
	if err := DB.Where("id = ? AND agent_secret = ?", nodeID, secret).First(&node).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

// UpdateNode updates node fields.
func UpdateNode(id string, updates map[string]any) error {
	return DB.Model(&model.Node{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteNode deletes a node by ID.
func DeleteNode(id string) error {
	return DB.Where("id = ?", id).Delete(&model.Node{}).Error
}

// UpdateHeartbeat updates the node's last heartbeat time and status.
func UpdateHeartbeat(id string) error {
	now := time.Now()
	return DB.Model(&model.Node{}).Where("id = ?", id).Updates(map[string]any{
		"last_heartbeat": &now,
		"status":         "online",
	}).Error
}

// SetNodeStatus updates a node's status.
func SetNodeStatus(id string, status string) error {
	return DB.Model(&model.Node{}).Where("id = ?", id).Update("status", status).Error
}

// MarkOffline flips a node online→offline using a conditional UPDATE and
// reports whether a real transition happened (RowsAffected==1). This makes the
// offline notification fire exactly once even when both the WS-disconnect
// handler and the background monitor race to mark the same node — and it's
// safe across multiple server instances (the DB row lock arbitrates).
func MarkOffline(id string) (changed bool, err error) {
	res := DB.Model(&model.Node{}).
		Where("id = ? AND status = ?", id, "online").
		Update("status", "offline")
	return res.RowsAffected == 1, res.Error
}

// MarkOnline flips a node offline→online (and refreshes last_heartbeat),
// reporting whether a real transition happened. Used to drive recovery
// notifications. Returns false when the node was already online (the common
// per-heartbeat case), so callers only notify on the actual recovery edge.
func MarkOnline(id string) (changed bool, err error) {
	now := time.Now()
	res := DB.Model(&model.Node{}).
		Where("id = ? AND status = ?", id, "offline").
		Updates(map[string]any{"status": "online", "last_heartbeat": &now})
	return res.RowsAffected == 1, res.Error
}

// ListStaleOnlineNodes returns nodes still marked online whose last heartbeat
// (or, if it never sent one, its creation time) is older than threshold. The
// background monitor uses this to catch agents that vanished without a clean
// WebSocket close (kill -9, network partition, host crash).
func ListStaleOnlineNodes(threshold time.Duration) ([]model.Node, error) {
	cutoff := time.Now().Add(-threshold)
	var nodes []model.Node
	err := DB.Where("status = ? AND COALESCE(last_heartbeat, created_at) < ?", "online", cutoff).
		Find(&nodes).Error
	return nodes, err
}
