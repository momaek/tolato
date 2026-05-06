package handler

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/agent"
	"github.com/momaek/tolato/server/internal/llm"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/security"
	"github.com/momaek/tolato/server/internal/store"
)

// chatUpgrader is initialized by InitUpgraders with origin checking.
var chatUpgrader = websocket.Upgrader{}

// Heartbeat tuning. With no ping/pong, a dead client (browser refresh that
// dropped TCP, NAT idle timeout, kill -9) leaves the server's ReadMessage
// blocked forever and its goroutines parked. The browser auto-replies to WS
// Pings without JS involvement, so a one-sided pinger on the server is enough
// to detect dead peers.
const (
	chatPingPeriod  = 30 * time.Second
	chatReadTimeout = 60 * time.Second // must be > chatPingPeriod
)

// ChatWSHandler handles the frontend chat WebSocket connection at /ws/chat.
// Authentication is performed via the first message after connection upgrade:
//
//	{ "type": "auth", "payload": { "token": "<jwt>" } }
//
// The client must send this within 10 seconds, otherwise the connection is closed.
func ChatWSHandler(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Upgrade to WebSocket first (no auth required yet)
		conn, err := chatUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[chat_ws] upgrade failed: %v", err)
			return
		}

		// Wait for auth message with a deadline
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))

		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[chat_ws] auth read timeout or error: %v", err)
			_ = conn.WriteJSON(model.WSMessage{
				Type:    model.WSTypeError,
				Payload: model.WSErrorEvent{Message: "authentication timeout"},
			})
			conn.Close()
			return
		}

		var authMsg struct {
			Type    string `json:"type"`
			Payload struct {
				Token string `json:"token"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &authMsg); err != nil || authMsg.Type != "auth" || authMsg.Payload.Token == "" {
			log.Printf("[chat_ws] invalid auth message")
			_ = conn.WriteJSON(model.WSMessage{
				Type:    model.WSTypeError,
				Payload: model.WSErrorEvent{Message: "invalid auth message, expected {type: 'auth', payload: {token: '...'}}"},
			})
			conn.Close()
			return
		}

		// Validate JWT token
		if _, err := deps.ValidateToken(authMsg.Payload.Token); err != nil {
			log.Printf("[chat_ws] invalid token")
			_ = conn.WriteJSON(model.WSMessage{
				Type:    model.WSTypeError,
				Payload: model.WSErrorEvent{Message: "invalid or expired token"},
			})
			conn.Close()
			return
		}

		// Switch to heartbeat-driven read deadline. The pong handler bumps it
		// each time the browser auto-replies to our pings; if the client dies
		// silently, ReadMessage trips the deadline within chatReadTimeout and
		// the whole shutdown sequence runs.
		_ = conn.SetReadDeadline(time.Now().Add(chatReadTimeout))
		conn.SetPongHandler(func(string) error {
			return conn.SetReadDeadline(time.Now().Add(chatReadTimeout))
		})

		// Send auth success (still single-goroutine here — writer hasn't started yet)
		_ = conn.WriteJSON(model.WSMessage{Type: "auth_ok"})

		// Wrap conn in a ChatSession so all post-auth writes serialize through
		// one mutex. Multiple tabs are supported — each opens its own session.
		session := NewChatSession(conn)

		// Create shared event channel for all loop runners
		eventCh := make(chan any, 64)

		// Track active loop runners + a WaitGroup so we can join them at
		// shutdown before closing eventCh (otherwise a still-running runner
		// would panic on send-to-closed-channel).
		loops := &loopRegistry{
			runners: make(map[string]*agent.LoopRunner),
		}
		var runnersWG sync.WaitGroup

		// Context for this connection
		ctx, cancel := context.WithCancel(context.Background())

		// Writer goroutine: drains eventCh and writes to the session. We track
		// it with a WaitGroup so the deferred shutdown can wait for it to
		// finish flushing before the connection is fully torn down.
		var writerWG sync.WaitGroup
		writerWG.Add(1)
		go func() {
			defer writerWG.Done()
			chatWriteLoop(session, eventCh)
		}()

		// Pinger goroutine: sends WS Ping frames every chatPingPeriod so dead
		// peers are reaped. WriteControl is goroutine-safe per gorilla docs
		// (separate mutex from regular writes), so it doesn't need writeMu.
		var pingerWG sync.WaitGroup
		pingerWG.Add(1)
		go func() {
			defer pingerWG.Done()
			chatPingLoop(ctx, conn, chatPingPeriod)
		}()

		// Shutdown sequence (LIFO):
		//   1. cancel ctx → pinger exits, runners' ctx-aware sends + LLM stream + tool exec abort
		//   2. wait for runners to finish → no more sends on eventCh
		//   3. close(eventCh) → writer's `range` exits cleanly
		//   4. wait for writer + pinger to exit
		//   5. close the session
		// Doing it in this order means no goroutine writes to a closed conn
		// and no runner panics on sending to a closed channel.
		defer func() {
			_ = session.Close()
		}()
		defer pingerWG.Wait()
		defer writerWG.Wait()
		defer close(eventCh)
		defer runnersWG.Wait()
		defer cancel()

		// Reader loop: reads messages from frontend
		chatReadLoop(ctx, session, deps, loops, eventCh, &runnersWG)
	}
}

// chatPingLoop sends WebSocket Ping control frames at `period`. It returns
// when ctx is cancelled or WriteControl fails (peer gone). Pongs from the
// browser are handled by SetPongHandler set up in the caller.
func chatPingLoop(ctx context.Context, conn *websocket.Conn, period time.Duration) {
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			deadline := time.Now().Add(10 * time.Second)
			if err := conn.WriteControl(websocket.PingMessage, nil, deadline); err != nil {
				log.Printf("[chat_ws] ping failed, peer likely gone: %v", err)
				return
			}
		}
	}
}

type loopRegistry struct {
	mu      sync.Mutex
	runners map[string]*agent.LoopRunner
}

func (lr *loopRegistry) get(convID string) *agent.LoopRunner {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	return lr.runners[convID]
}

func (lr *loopRegistry) set(convID string, runner *agent.LoopRunner) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	lr.runners[convID] = runner
}

func (lr *loopRegistry) remove(convID string) {
	lr.mu.Lock()
	defer lr.mu.Unlock()
	delete(lr.runners, convID)
}

// chatInbound is the wire shape for a frame read from /ws/chat. Payload is
// kept as RawMessage so we can unmarshal it into the right typed struct once
// we know Type, without the marshal→unmarshal dance that `Payload any` forces.
type chatInbound struct {
	Type           string          `json:"type"`
	ID             string          `json:"id,omitempty"`
	ConversationID string          `json:"conversation_id,omitempty"`
	Payload        json.RawMessage `json:"payload,omitempty"`
}

func chatReadLoop(ctx context.Context, session *ChatSession, deps *Deps, loops *loopRegistry, eventCh chan any, runnersWG *sync.WaitGroup) {
	conn := session.Conn()
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[chat_ws] read error: %v", err)
			return
		}

		var msg chatInbound
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[chat_ws] unmarshal error: %v", err)
			continue
		}

		switch msg.Type {
		case model.WSTypeUserMessage:
			var evt model.WSUserMessageEvent
			if err := json.Unmarshal(msg.Payload, &evt); err != nil {
				log.Printf("[chat_ws] bad user_message payload: %v", err)
				continue
			}
			handleUserMessage(ctx, deps, loops, eventCh, runnersWG, msg.ConversationID, evt)
		case model.WSTypeConfirmResponse:
			var evt model.WSConfirmResponseEvent
			if err := json.Unmarshal(msg.Payload, &evt); err != nil {
				log.Printf("[chat_ws] bad confirm_response payload: %v", err)
				continue
			}
			handleConfirmResponse(loops, msg.ConversationID, evt)
		default:
			log.Printf("[chat_ws] unknown message type: %s", msg.Type)
		}
	}
}

func handleUserMessage(ctx context.Context, deps *Deps, loops *loopRegistry, eventCh chan any, runnersWG *sync.WaitGroup, convID string, evt model.WSUserMessageEvent) {
	if convID == "" {
		log.Printf("[chat_ws] user_message missing conversation_id")
		return
	}

	llmSettings := deps.Settings.LLM()
	chatSettings := deps.Settings.Chat()

	llmCfg := llm.ClientConfig{
		APIBaseURL:  normalizeLLMBaseURL(llmSettings.APIBaseURL),
		APIKey:      llmSettings.APIKey,
		Model:       llmSettings.DefaultModel,
		Temperature: llmSettings.Temperature,
	}
	if evt.Model != nil && *evt.Model != "" {
		llmCfg.Model = *evt.Model
	}

	llmClient := llm.NewClient(llmCfg, agent.ToolDefs())
	secChecker := security.NewChecker(deps.Settings)
	toolExec := agent.NewToolExecutor(deps.NodeManager, secChecker, deps.Settings, chatSettings.OutputTruncateLines)
	promptBuilder := agent.NewPromptBuilder()

	runner := agent.NewLoopRunner(agent.LoopRunnerConfig{
		ConversationID: convID,
		LLMClient:      llmClient,
		ToolExecutor:   toolExec,
		PromptBuilder:  promptBuilder,
		EventCh:        eventCh,
		MaxRounds:      llmSettings.MaxRounds,
		ContextRounds:  chatSettings.ContextRounds,
		GetNodeInfos: func() []agent.NodeInfo {
			return getNodeInfos(deps)
		},
		GetCustomPrompt: func() string {
			cs := deps.Settings.Chat()
			if cs.CustomSystemPrompt != nil {
				return *cs.CustomSystemPrompt
			}
			return ""
		},
	})

	loops.set(convID, runner)

	// Run loop in background goroutine. WaitGroup lets the connection's
	// shutdown sequence join all in-flight runners before closing eventCh.
	runnersWG.Add(1)
	go func() {
		defer runnersWG.Done()
		defer loops.remove(convID)
		input := agent.UserMessageInput{
			ConversationID: convID,
			Content:        evt.Content,
			Model:          evt.Model,
			DefaultNodeID:  evt.DefaultNodeID,
		}
		runner.Run(ctx, input)
	}()
}

func handleConfirmResponse(loops *loopRegistry, convID string, evt model.WSConfirmResponseEvent) {
	if convID == "" {
		return
	}
	if runner := loops.get(convID); runner != nil {
		runner.ReceiveConfirm(evt.Approved)
	}
}

func chatWriteLoop(session *ChatSession, eventCh chan any) {
	for evt := range eventCh {
		var msg model.WSMessage

		switch e := evt.(type) {
		case agent.ReasoningEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeReasoning,
				ConversationID: e.ConversationID,
				Payload:        model.WSReasoningEvent{Delta: e.Delta},
			}
		case agent.ContentEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeContent,
				ConversationID: e.ConversationID,
				Payload:        model.WSContentEvent{Delta: e.Delta},
			}
		case agent.ToolCallEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeToolCall,
				ConversationID: e.ConversationID,
				Payload: model.WSToolCallEvent{
					ID:   e.ID,
					Tool: e.Tool,
					Args: e.Args,
				},
			}
		case agent.ToolResultEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeToolResult,
				ConversationID: e.ConversationID,
				Payload: model.WSToolResultEvent{
					ID:     e.ID,
					Result: e.Result,
				},
			}
		case agent.ConfirmRequestEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeConfirmRequest,
				ConversationID: e.ConversationID,
				Payload: model.WSConfirmRequestEvent{
					ID:   e.ID,
					Tool: e.Tool,
					Args: e.Args,
				},
			}
		case agent.DoneEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeDone,
				ConversationID: e.ConversationID,
				Payload:        model.WSDoneEvent{},
			}
		case agent.ErrorEvent:
			msg = model.WSMessage{
				Type:           model.WSTypeError,
				ConversationID: e.ConversationID,
				Payload:        model.WSErrorEvent{Message: e.Message},
			}
		default:
			continue
		}

		if err := session.WriteJSON(msg); err != nil {
			log.Printf("[chat_ws] write error: %v", err)
			// Drain remaining events instead of returning, so runner sends
			// don't block. Read loop's ctx cancel will cause runners to exit
			// shortly; once they do, eventCh is closed and we return naturally.
			for range eventCh {
			}
			return
		}
	}
}

// Helper functions

func getNodeInfos(deps *Deps) []agent.NodeInfo {
	nodes, _, err := store.ListNodes(1, 200, "")
	if err != nil {
		return nil
	}
	infos := make([]agent.NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		info := agent.NodeInfo{
			ID:     n.ID,
			Name:   n.Name,
			IP:     n.IP,
			OS:     n.OS,
			Status: n.Status,
		}
		if n.Alias != nil {
			info.Alias = *n.Alias
		}
		infos = append(infos, info)
	}
	return infos
}

// normalizeLLMBaseURL trims trailing slashes and appends `/v1` when the URL
// doesn't already carry an OpenAI-style version segment. The OpenAI SDK and the
// /models endpoint both expect the URL to point at the version root.
func normalizeLLMBaseURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return s
	}
	if matched, _ := regexp.MatchString(`/v\d+(/|$)`, s); matched {
		return s
	}
	return s + "/v1"
}

