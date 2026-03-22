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
	PublishSummary(sessionID string, msg []byte)
}

type AgentRegistry interface {
	BindNode(nodeID string, clientID string)
	UnbindNode(nodeID string, clientID string)
	Heartbeat(nodeID string, clientID string, at time.Time) error
	PublishDispatch(nodeID string, msg []byte) error
}

var (
	ErrClientNotFound = errors.New("ws client not found")
	ErrNodeNotBound   = errors.New("ws node not bound")
)
