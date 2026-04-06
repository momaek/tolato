package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/agent"
	"github.com/momaek/tolato/server/internal/llm"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/security"
	"github.com/momaek/tolato/server/internal/store"
)

var chatUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// ChatWSHandler handles the frontend chat WebSocket connection at /ws/chat.
func ChatWSHandler(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Authenticate via query param token
		token := c.Query("token")
		if token == "" {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "Missing token query parameter",
			})
			return
		}

		// Validate JWT token
		claims, err := deps.ValidateToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "Invalid token",
			})
			return
		}
		_ = claims

		// Upgrade to WebSocket
		conn, err := chatUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[chat_ws] upgrade failed: %v", err)
			return
		}

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

func chatReadLoop(ctx context.Context, conn *websocket.Conn, deps *Deps, loops *loopRegistry, eventCh chan any) {
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[chat_ws] read error: %v", err)
			return
		}

		var msg model.WSMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("[chat_ws] unmarshal error: %v", err)
			continue
		}

		switch msg.Type {
		case model.WSTypeUserMessage:
			handleUserMessage(ctx, deps, loops, eventCh, msg.Payload)
		case model.WSTypeConfirmResponse:
			handleConfirmResponse(loops, msg.Payload)
		default:
			log.Printf("[chat_ws] unknown message type: %s", msg.Type)
		}
	}
}

func handleUserMessage(ctx context.Context, deps *Deps, loops *loopRegistry, eventCh chan any, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var evt model.WSUserMessageEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return
	}

	// Determine conversation ID from the message
	// The frontend sends conversation_id in the WSMessage envelope
	// For now, parse it from the payload or use the event
	var wsMsg struct {
		ConversationID string `json:"conversation_id"`
	}
	json.Unmarshal(data, &wsMsg)
	convID := wsMsg.ConversationID

	if convID == "" {
		log.Printf("[chat_ws] user_message missing conversation_id")
		return
	}

	// Load settings
	llmSettings := loadLLMSettings()
	chatSettings := loadChatSettings()

	// Build LLM client
	llmCfg := llm.ClientConfig{
		APIBaseURL:  llmSettings.APIBaseURL,
		APIKey:      llmSettings.APIKey,
		Model:       llmSettings.DefaultModel,
		Temperature: llmSettings.Temperature,
	}

	// Override model if specified in message
	if evt.Model != nil && *evt.Model != "" {
		llmCfg.Model = *evt.Model
	}

	llmClient := llm.NewClient(llmCfg, agent.ToolDefs())
	secChecker := security.NewChecker()
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
			cs := loadChatSettings()
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

func handleConfirmResponse(loops *loopRegistry, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var evt model.WSConfirmResponseEvent
	if err := json.Unmarshal(data, &evt); err != nil {
		return
	}

	// Find conversation ID from existing loops
	// The confirm response includes an ID (tool call ID)
	// We need to figure out which conversation this belongs to
	// For simplicity, broadcast to all loops (only one should be waiting)
	var wsMsg struct {
		ConversationID string `json:"conversation_id"`
	}
	json.Unmarshal(data, &wsMsg)

	if runner := loops.get(wsMsg.ConversationID); runner != nil {
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

func loadLLMSettings() model.LLMSettings {
	settings := model.LLMSettings{
		MaxRounds:   20,
		Temperature: 0.7,
	}
	if s, err := store.GetSetting("llm.api_base_url"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.APIBaseURL)
	}
	if s, err := store.GetSetting("llm.api_key"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.APIKey)
	}
	if s, err := store.GetSetting("llm.default_model"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.DefaultModel)
	}
	if s, err := store.GetSetting("llm.max_rounds"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.MaxRounds)
	}
	if s, err := store.GetSetting("llm.temperature"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.Temperature)
	}
	return settings
}

func loadChatSettings() model.ChatSettings {
	settings := model.ChatSettings{
		ContextRounds:       20,
		OutputTruncateLines: 100,
	}
	if s, err := store.GetSetting("chat.context_rounds"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.ContextRounds)
	}
	if s, err := store.GetSetting("chat.output_truncate_lines"); err == nil {
		json.Unmarshal([]byte(s.Value), &settings.OutputTruncateLines)
	}
	if s, err := store.GetSetting("chat.custom_system_prompt"); err == nil {
		var prompt string
		json.Unmarshal([]byte(s.Value), &prompt)
		if prompt != "" {
			settings.CustomSystemPrompt = &prompt
		}
	}
	return settings
}
