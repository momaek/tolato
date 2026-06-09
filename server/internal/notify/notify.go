package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/settings"
	"github.com/momaek/tolato/server/internal/store"
)

const sendTimeout = 15 * time.Second

// Dispatcher turns node state transitions into notifications across all enabled
// channels. It reads its config live from the settings cache, so REST edits
// take effect without a restart. Token sources keep their cached tokens across
// dispatches and are only rebuilt when their config actually changes.
type Dispatcher struct {
	settings *settings.Cache
	client   *http.Client
	started  time.Time

	mu      sync.Mutex
	sources map[string]*sourceEntry
}

type sourceEntry struct {
	hash string // JSON of the source config; changes → rebuild (drops cached token)
	ts   *tokenSource
}

// New builds a Dispatcher bound to the settings cache. One per process.
func New(s *settings.Cache) *Dispatcher {
	return &Dispatcher{
		settings: s,
		client:   &http.Client{Timeout: sendTimeout},
		started:  time.Now(),
		sources:  make(map[string]*sourceEntry),
	}
}

// NotifyOffline fires an offline notification for a node (async, best-effort).
// No-op-ish when within the startup grace window — a restart briefly makes all
// nodes look stale and we don't want to storm.
func (d *Dispatcher) NotifyOffline(nodeID string) {
	cfg := d.settings.Notify()
	if d.inGrace(cfg) {
		log.Printf("notify: suppressing offline alert for %s (startup grace)", nodeID)
		return
	}
	d.emit(cfg, nodeID, EventOffline)
}

// OnDisconnect handles a clean WebSocket close. A dropped connection alone does
// not prove the node is gone — a brief network blip is indistinguishable from a
// crash, and alerting immediately turns every reconnect into an offline+recovery
// pair. So when the background monitor is active (OfflineThresholdSeconds > 0) we
// do nothing here and let the monitor flip the node offline only after its
// heartbeat has been silent past the threshold, which naturally absorbs flaps.
// When the monitor is disabled (<=0) there is no fallback detector, so we mark
// the node offline and alert immediately to preserve disconnect detection.
func (d *Dispatcher) OnDisconnect(nodeID string) {
	cfg := d.settings.Notify()
	if cfg.OfflineThresholdSeconds > 0 {
		return
	}
	if changed, _ := store.MarkOffline(nodeID); changed {
		d.NotifyOffline(nodeID) // honors the startup grace window
	}
}

// NotifyOnline fires a recovery notification for a node (async, best-effort).
// No-op when recovery notifications are disabled, or within the startup grace
// window — a restart can briefly mark every node offline (monitor) and then see
// them all reconnect, which without this guard would fire a recovery storm.
func (d *Dispatcher) NotifyOnline(nodeID string) {
	cfg := d.settings.Notify()
	if !cfg.RecoverNotify {
		return
	}
	if d.inGrace(cfg) {
		log.Printf("notify: suppressing recovery alert for %s (startup grace)", nodeID)
		return
	}
	d.emit(cfg, nodeID, EventOnline)
}

func (d *Dispatcher) inGrace(cfg model.NotifySettings) bool {
	if cfg.StartupGraceSeconds <= 0 {
		return false
	}
	return time.Since(d.started) < time.Duration(cfg.StartupGraceSeconds)*time.Second
}

func (d *Dispatcher) emit(cfg model.NotifySettings, nodeID string, typ EventType) {
	enabled := enabledChannels(cfg)
	if len(enabled) == 0 {
		return
	}
	go func() {
		n, err := store.GetNodeByID(nodeID)
		if err != nil {
			log.Printf("notify: load node %s failed: %v", nodeID, err)
			return
		}
		ev := eventFromNode(n, typ)
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()
		for _, ch := range enabled {
			if err := d.sendChannel(ctx, cfg, ch, ev); err != nil {
				log.Printf("notify: channel %q (%s) failed for node %s: %v", ch.Name, ch.Preset, ev.displayName(), err)
			} else {
				log.Printf("notify: sent %s for node %s via %q", ev.Type, ev.displayName(), ch.Name)
			}
		}
	}()
}

// Test sends a sample offline event through a single saved channel by name and
// returns the send error (nil on success). Used by the settings "test" button.
// It reads the saved config so real (unmasked) secrets are used — callers
// should save before testing.
func (d *Dispatcher) Test(channelName string) error {
	cfg := d.settings.Notify()
	var ch *model.NotifyChannel
	for i := range cfg.Channels {
		if cfg.Channels[i].Name == channelName {
			ch = &cfg.Channels[i]
			break
		}
	}
	if ch == nil {
		return fmt.Errorf("channel %q not found", channelName)
	}
	now := time.Now()
	ev := Event{
		NodeID:        "test-node",
		NodeName:      "test-node",
		NodeIP:        "127.0.0.1",
		Type:          EventOffline,
		At:            now,
		LastHeartbeat: &now,
	}
	ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
	defer cancel()
	return d.sendChannel(ctx, cfg, *ch, ev)
}

func enabledChannels(cfg model.NotifySettings) []model.NotifyChannel {
	out := make([]model.NotifyChannel, 0, len(cfg.Channels))
	for _, ch := range cfg.Channels {
		if ch.Enabled {
			out = append(out, ch)
		}
	}
	return out
}

// sendChannel renders and delivers one channel. For two-step channels it fetches
// a token first and, if the response signals the token is stale, refreshes it
// and retries once.
func (d *Dispatcher) sendChannel(ctx context.Context, cfg model.NotifySettings, ch model.NotifyChannel, ev Event) error {
	spec := resolveSpec(ch)
	if spec.URL == "" {
		return fmt.Errorf("no URL (preset %q + webhook override)", ch.Preset)
	}

	var ts *tokenSource
	if ch.TokenSource != "" {
		ts = d.resolveSource(cfg, ch.TokenSource)
		if ts == nil {
			return fmt.Errorf("token source %q not configured", ch.TokenSource)
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		token := ""
		if ts != nil {
			var err error
			if token, err = ts.Get(ctx); err != nil {
				return fmt.Errorf("token: %w", err)
			}
		}

		status, body, err := d.doSend(ctx, spec, ch.Params, ev, token)
		if err != nil {
			return err
		}

		// Token revoked before its advertised expiry → refresh + retry once.
		if ts != nil && attempt == 0 && ts.matchInvalidate(body) {
			ts.Invalidate()
			continue
		}

		if spec.SuccessCheck != nil {
			if matchJSON(body, spec.SuccessCheck) {
				return nil
			}
			return fmt.Errorf("success check failed (status %d): %s", status, trunc(body))
		}
		if status/100 == 2 {
			return nil
		}
		return fmt.Errorf("HTTP %d: %s", status, trunc(body))
	}
	return fmt.Errorf("failed after token refresh")
}

func (d *Dispatcher) doSend(ctx context.Context, spec model.NotifyWebhookSpec, params map[string]string, ev Event, token string) (int, []byte, error) {
	vars := renderVars(params, ev, token)

	method := strings.ToUpper(spec.Method)
	if method == "" {
		method = http.MethodPost
	}
	url := renderPlain(spec.URL, vars)
	body := renderBody(spec.BodyTemplate, vars)

	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	hasCT := false
	for k, v := range spec.Headers {
		if strings.EqualFold(k, "Content-Type") {
			hasCT = true
		}
		req.Header.Set(k, renderPlain(v, vars))
	}
	if !hasCT && body != "" {
		req.Header.Set("Content-Type", ctJSON)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("send: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

// resolveSource returns the live tokenSource for name, (re)building it only when
// the stored config changed since last use — preserving the cached token across
// dispatches. Returns nil if no source with that name exists.
func (d *Dispatcher) resolveSource(cfg model.NotifySettings, name string) *tokenSource {
	var found *model.NotifyTokenSource
	for i := range cfg.TokenSources {
		if cfg.TokenSources[i].Name == name {
			found = &cfg.TokenSources[i]
			break
		}
	}
	if found == nil {
		return nil
	}
	hashBytes, _ := json.Marshal(found)
	hash := string(hashBytes)

	d.mu.Lock()
	defer d.mu.Unlock()
	if e, ok := d.sources[name]; ok && e.hash == hash {
		return e.ts
	}
	ts := newTokenSource(*found, d.client)
	d.sources[name] = &sourceEntry{hash: hash, ts: ts}
	return ts
}
