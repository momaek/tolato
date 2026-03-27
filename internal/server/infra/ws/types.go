package ws

import (
	"errors"
	"time"
)

type ClientKind string

const (
	ClientKindUI    ClientKind = "ui"
	ClientKindAgent ClientKind = "agent"
)

type Client interface {
	ID() string
	Kind() ClientKind
	Send(msg []byte) bool
	Close(code int, reason string)
}

type Hub interface {
	Register(client Client)
	Unregister(clientID string)
}

type SessionRegistry interface {
	SetActive(clientID string, sessionID string)
	SetWatchSessions(clientID string, sessionIDs []string)
	ForgetClient(clientID string)
	PublishToSession(sessionID string, msg []byte)
	PublishToClient(clientID string, msg []byte)
	SummaryRecipients(sessionID string) []string
	IncrementUnread(sessionID string) []SessionUnreadState
	ClearUnread(clientID string, sessionID string) (int, bool)
	UnreadCount(clientID string, sessionID string) int
}

type SessionUnreadState struct {
	ClientID  string
	SessionID string
	Unread    int
}

type AgentPresenceSnapshot struct {
	NodeID        string
	ClientID      string
	Bound         bool
	LastHeartbeat *time.Time
	Hostname      string
	Region        string
	OS            string
	Version       string
	IPAddress     string
	Tags          []string
	Busy          bool
	Metrics       AgentNodeMetrics
}

type AgentRegistry interface {
	BindNode(nodeID string, clientID string, meta AgentNodeMetadata)
	UnbindNode(nodeID string, clientID string)
	ForgetClient(clientID string)
	Heartbeat(nodeID string, clientID string, state AgentNodeRuntime, at time.Time) error
	PublishDispatch(nodeID string, msg []byte) error
	Snapshots() []AgentPresenceSnapshot
}

type AgentNodeMetadata struct {
	Hostname  string
	Region    string
	OS        string
	Version   string
	IPAddress string
	Tags      []string
}

type AgentNodeMetrics struct {
	CPU    float64
	Memory float64
	Disk   float64
}

type AgentNodeRuntime struct {
	Busy    bool
	Metrics AgentNodeMetrics
}

var (
	ErrClientNotFound = errors.New("ws client not found")
	ErrNodeNotBound   = errors.New("ws node not bound")
)
