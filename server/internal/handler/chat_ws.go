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

		// Clear the read deadline for normal operation
		_ = conn.SetReadDeadline(time.Time{})

		// Send auth success
		_ = conn.WriteJSON(model.WSMessage{Type: "auth_ok"})

		// Register with session manager (kicks old connection)
		deps.SessionManager.Replace(conn)

		// Create shared event channel for all loop runners
		eventCh := make(chan any, 64)

		// Track active loop runners
		loops := &loopRegistry{
			runners: make(map[string]*agent.LoopRunner),
		}

		// Context for this connection
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer func() {
			deps.SessionManager.Remove(conn)
			conn.Close()
		}()

		// Writer goroutine: reads events and sends to frontend
		go chatWriteLoop(conn, eventCh)

		// Reader loop: reads messages from frontend
		chatReadLoop(ctx, conn, deps, loops, eventCh)
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

func chatReadLoop(ctx context.Context, conn *websocket.Conn, deps *Deps, loops *loopRegistry, eventCh chan any) {
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
			handleUserMessage(ctx, deps, loops, eventCh, msg.ConversationID, evt)
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

func handleUserMessage(ctx context.Context, deps *Deps, loops *loopRegistry, eventCh chan any, convID string, evt model.WSUserMessageEvent) {
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
	toolExec := agent.NewToolExecutor(deps.NodeManager, secChecker, chatSettings.OutputTruncateLines)
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

	// Run loop in background goroutine
	go func() {
		input := agent.UserMessageInput{
			ConversationID: convID,
			Content:        evt.Content,
			Model:          evt.Model,
			DefaultNodeID:  evt.DefaultNodeID,
		}
		runner.Run(ctx, input)
		loops.remove(convID)
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

func chatWriteLoop(conn *websocket.Conn, eventCh chan any) {
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

		if err := conn.WriteJSON(msg); err != nil {
			log.Printf("[chat_ws] write error: %v", err)
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

