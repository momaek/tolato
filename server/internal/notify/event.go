// Package notify delivers node offline/online notifications through a single
// webhook engine. Channels (Telegram/Discord/Slack/WeCom/Feishu/custom) differ
// only by preset defaults; two-step platforms reference a TokenSource that
// exchanges credentials for a cached bearer token.
package notify

import (
	"fmt"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// EventType is the kind of state transition that triggered a notification.
type EventType string

const (
	EventOffline EventType = "offline"
	EventOnline  EventType = "online"
)

// Event carries the node state change handed to each channel's template.
type Event struct {
	NodeID        string
	NodeName      string
	NodeAlias     string
	NodeIP        string
	Type          EventType
	At            time.Time
	LastHeartbeat *time.Time
}

// eventFromNode builds an Event from a stored node and a transition type.
func eventFromNode(n *model.Node, typ EventType) Event {
	alias := ""
	if n.Alias != nil {
		alias = *n.Alias
	}
	return Event{
		NodeID:        n.ID,
		NodeName:      n.Name,
		NodeAlias:     alias,
		NodeIP:        n.IP,
		Type:          typ,
		At:            time.Now(),
		LastHeartbeat: n.LastHeartbeat,
	}
}

// displayName prefers the user alias, falling back to the reported hostname.
func (e Event) displayName() string {
	if e.NodeAlias != "" {
		return e.NodeAlias
	}
	return e.NodeName
}

// Message is the default human-readable text exposed to templates as
// {{message}}. Channels with richer formats can ignore it and compose their own
// body from the individual {{node_*}} placeholders.
func (e Event) Message() string {
	switch e.Type {
	case EventOnline:
		return fmt.Sprintf("✅ 节点恢复在线: %s (%s)", e.displayName(), e.NodeIP)
	default:
		last := "未知"
		if e.LastHeartbeat != nil {
			last = e.LastHeartbeat.Format("2006-01-02 15:04:05")
		}
		return fmt.Sprintf("🔴 节点离线告警: %s (%s)，最后心跳: %s", e.displayName(), e.NodeIP, last)
	}
}
