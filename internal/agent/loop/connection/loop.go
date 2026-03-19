package connection

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/momaek/tolato/internal/agent/infra/persistence"
	"github.com/momaek/tolato/internal/agent/transport/enroll"
	"github.com/momaek/tolato/internal/agent/transport/wsclient"
	"github.com/momaek/tolato/internal/shared/config"
	"github.com/momaek/tolato/internal/shared/protocol"
	"github.com/momaek/tolato/internal/shared/types"
	"go.uber.org/zap"
)

type Loop struct {
	Logger       *zap.Logger
	Config       config.AgentConfig
	Store        persistence.IdentityStore
	EnrollClient enroll.Client
	WSClient     wsclient.Client
	Incoming     chan<- protocol.Envelope
}

func (l Loop) Run(ctx context.Context) error {
	identity, err := l.ensureIdentity(ctx)
	if err != nil {
		return err
	}

	retryTicker := time.NewTicker(l.Config.ReconnectInterval())
	defer retryTicker.Stop()

	for {
		wsURL, err := l.wsURL(identity)
		if err != nil {
			return err
		}

		if err := l.WSClient.Connect(ctx, wsURL); err != nil {
			l.Logger.Warn("agent ws connect failed", zap.Error(err))
			if strings.Contains(err.Error(), "bad handshake") {
				identity, err = l.reEnroll(ctx)
				if err != nil {
					l.Logger.Warn("agent re-enroll failed", zap.Error(err))
				}
			}
		} else {
			l.Logger.Info("agent connected to ws/agent", zap.String("node_id", identity.NodeID))
			if err := l.sendHello(ctx, identity); err != nil {
				l.Logger.Warn("agent hello failed", zap.Error(err))
			}
			if err := l.runSession(ctx, identity); err != nil {
				l.Logger.Warn("agent session ended", zap.Error(err))
			}
			_ = l.WSClient.Close()
		}

		select {
		case <-ctx.Done():
			return nil
		case <-retryTicker.C:
		}
	}
}

func (l Loop) wsURL(identity types.AgentIdentity) (string, error) {
	parsed, err := url.Parse(l.Config.Agent.ServerBaseURL)
	if err != nil {
		return "", err
	}

	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	default:
		parsed.Scheme = "ws"
	}
	parsed.Path = "/ws/agent"
	query := parsed.Query()
	query.Set("node_id", identity.NodeID)
	query.Set("secret", identity.Secret)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func (l Loop) ensureIdentity(ctx context.Context) (types.AgentIdentity, error) {
	identity, err := l.Store.Load(ctx)
	if err == nil {
		return identity, nil
	}

	resp, err := l.EnrollClient.Enroll(ctx, types.EnrollRequest{
		Hostname: l.Config.Agent.Hostname,
		Region:   l.Config.Agent.Region,
		OS:       l.Config.Agent.OS,
		Version:  l.Config.Agent.Version,
		Tags:     []string{"bootstrap"},
	})
	if err != nil {
		return types.AgentIdentity{}, err
	}

	identity = types.AgentIdentity{
		NodeID:   resp.NodeID,
		Secret:   resp.Secret,
		Hostname: l.Config.Agent.Hostname,
		Region:   l.Config.Agent.Region,
		OS:       l.Config.Agent.OS,
		Version:  l.Config.Agent.Version,
	}
	if err := l.Store.Save(ctx, identity); err != nil {
		return types.AgentIdentity{}, err
	}
	return identity, nil
}

func (l Loop) reEnroll(ctx context.Context) (types.AgentIdentity, error) {
	resp, err := l.EnrollClient.Enroll(ctx, types.EnrollRequest{
		Hostname: l.Config.Agent.Hostname,
		Region:   l.Config.Agent.Region,
		OS:       l.Config.Agent.OS,
		Version:  l.Config.Agent.Version,
		Tags:     []string{"bootstrap"},
	})
	if err != nil {
		return types.AgentIdentity{}, err
	}

	identity := types.AgentIdentity{
		NodeID:   resp.NodeID,
		Secret:   resp.Secret,
		Hostname: l.Config.Agent.Hostname,
		Region:   l.Config.Agent.Region,
		OS:       l.Config.Agent.OS,
		Version:  l.Config.Agent.Version,
	}
	if err := l.Store.Save(ctx, identity); err != nil {
		return types.AgentIdentity{}, err
	}
	return identity, nil
}

func (l Loop) sendHello(ctx context.Context, identity types.AgentIdentity) error {
	env, err := protocol.NewEnvelope(protocol.TypeHello, "", identity.NodeID, 1, protocol.HelloPayload{
		SessionID:    uuid.NewString(),
		AgentVersion: identity.Version,
		Capabilities: []string{"heartbeat", "dispatch", "execution", "supervisor"},
	})
	if err != nil {
		return err
	}
	return l.WSClient.Send(ctx, env)
}

func (l Loop) runSession(ctx context.Context, identity types.AgentIdentity) error {
	heartbeatTicker := time.NewTicker(l.Config.HeartbeatInterval())
	defer heartbeatTicker.Stop()

	var seq int64 = 2

	for {
		select {
		case <-ctx.Done():
			return nil
		case env := <-l.WSClient.Incoming():
			select {
			case l.Incoming <- env:
			default:
				l.Logger.Warn("dropping incoming dispatch envelope", zap.String("type", env.Type))
			}
		case err := <-l.WSClient.Errors():
			return err
		case <-heartbeatTicker.C:
			payload := protocol.HeartbeatPayload{
				Hostname: identity.Hostname,
				Load:     "placeholder",
				Memory:   "placeholder",
				Disk:     "placeholder",
				Busy:     false,
			}
			env, err := protocol.NewEnvelope(protocol.TypeHeartbeat, "", identity.NodeID, seq, payload)
			if err != nil {
				return fmt.Errorf("build heartbeat envelope: %w", err)
			}
			seq++
			if err := l.WSClient.Send(ctx, env); err != nil {
				return err
			}
		}
	}
}
