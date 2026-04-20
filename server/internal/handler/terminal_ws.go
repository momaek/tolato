package handler

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/node"
	"github.com/momaek/tolato/server/internal/store"
)

// TerminalWSHandler handles /ws/terminal connections from the browser.
// Lifecycle:
//   1. Upgrade
//   2. First message: {type:"auth", payload:{token}}
//   3. Next message:  {type:"open", payload:{node_id, cols, rows}}
//   4. Bidirectional:
//        browser -> input / resize / close / file_op
//        server  -> output / exit / error / file_result
func TerminalWSHandler(deps *Deps) gin.HandlerFunc {
	return func(c *gin.Context) {
		conn, err := chatUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Printf("[terminal_ws] upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		// --- Auth (mirror chat_ws) ---
		_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		_, raw, err := conn.ReadMessage()
		if err != nil {
			writeTermError(conn, "authentication timeout")
			return
		}

		var authMsg struct {
			Type    string `json:"type"`
			Payload struct {
				Token string `json:"token"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(raw, &authMsg); err != nil || authMsg.Type != model.WSTermTypeAuth || authMsg.Payload.Token == "" {
			writeTermError(conn, "invalid auth message")
			return
		}
		if _, err := deps.ValidateToken(authMsg.Payload.Token); err != nil {
			writeTermError(conn, "invalid or expired token")
			return
		}
		_ = conn.SetReadDeadline(time.Time{})
		_ = writeTerm(conn, model.WSTermTypeAuthOK, nil)

		// --- Open ---
		_, raw, err = conn.ReadMessage()
		if err != nil {
			return
		}
		var openEnv struct {
			Type    string                `json:"type"`
			Payload model.WSTermOpenPayload `json:"payload"`
		}
		if err := json.Unmarshal(raw, &openEnv); err != nil || openEnv.Type != model.WSTermTypeOpen {
			writeTermError(conn, "expected open message")
			return
		}
		nodeID := openEnv.Payload.NodeID
		if nodeID == "" {
			writeTermError(conn, "node_id required")
			return
		}

		ac, ok := deps.NodeManager.GetConn(nodeID)
		if !ok {
			writeTermError(conn, "node is offline")
			return
		}

		n, err := store.GetNodeByID(nodeID)
		if err != nil {
			writeTermError(conn, "node not found")
			return
		}

		cols := openEnv.Payload.Cols
		rows := openEnv.Payload.Rows
		if cols == 0 {
			cols = 80
		}
		if rows == 0 {
			rows = 24
		}

		// Open the PTY stream on the agent.
		ptyStream, err := ac.OpenStream(model.AgentTypePTYOpen, model.AgentPTYOpenPayload{
			Cols:  cols,
			Rows:  rows,
			Shell: openEnv.Payload.Shell,
			Cwd:   openEnv.Payload.Cwd,
		})
		if err != nil {
			writeTermError(conn, "failed to open PTY: "+err.Error())
			return
		}
		defer ptyStream.Close()

		_ = writeTerm(conn, model.WSTermTypeReady, model.WSTermReadyPayload{SessionID: ptyStream.ID})

		startedAt := time.Now()
		var exitCode int
		var exitSeen bool

		// Serialize browser writes under this sync.Mutex — many goroutines may
		// want to push frames to the browser (PTY output + file_op replies).
		var browserWriteMu sync.Mutex
		writeToBrowser := func(msgType string, payload any) error {
			browserWriteMu.Lock()
			defer browserWriteMu.Unlock()
			return conn.WriteJSON(model.WSMessage{Type: msgType, Payload: payload})
		}

		done := make(chan struct{})

		// --- Agent -> Browser (PTY stream) ---
		go func() {
			defer close(done)
			for frame := range ptyStream.Ch {
				switch frame.Type {
				case model.AgentTypePTYOutput:
					payloadBytes, _ := json.Marshal(frame.Payload)
					var out model.AgentPTYOutputPayload
					_ = json.Unmarshal(payloadBytes, &out)
					if err := writeToBrowser(model.WSTermTypeOutput, model.WSTermOutputPayload{Data: out.Data}); err != nil {
						return
					}
				case model.AgentTypePTYExit:
					payloadBytes, _ := json.Marshal(frame.Payload)
					var ex model.AgentPTYExitPayload
					_ = json.Unmarshal(payloadBytes, &ex)
					exitCode = ex.ExitCode
					exitSeen = true
					_ = writeToBrowser(model.WSTermTypeExit, model.WSTermExitPayload{ExitCode: ex.ExitCode, Error: ex.Error})
					return
				}
			}
		}()

		// --- Browser -> Agent (input / resize / close / file_op) ---
		readLoop := func() {
			for {
				_, raw, err := conn.ReadMessage()
				if err != nil {
					return
				}
				var msg model.WSMessage
				if err := json.Unmarshal(raw, &msg); err != nil {
					continue
				}
				switch msg.Type {
				case model.WSTermTypeInput:
					var p model.WSTermInputPayload
					payloadBytes, _ := json.Marshal(msg.Payload)
					_ = json.Unmarshal(payloadBytes, &p)
					_ = ptyStream.Send(model.AgentTypePTYInput, model.AgentPTYInputPayload{Data: p.Data})

				case model.WSTermTypeResize:
					var p model.WSTermResizePayload
					payloadBytes, _ := json.Marshal(msg.Payload)
					_ = json.Unmarshal(payloadBytes, &p)
					_ = ptyStream.Send(model.AgentTypePTYResize, model.AgentPTYResizePayload{Cols: p.Cols, Rows: p.Rows})

				case model.WSTermTypeClose:
					_ = ptyStream.Send(model.AgentTypePTYClose, model.AgentPTYClosePayload{})
					return

				case model.WSTermTypeFileOp:
					var p model.WSTermFileOpPayload
					payloadBytes, _ := json.Marshal(msg.Payload)
					_ = json.Unmarshal(payloadBytes, &p)
					go handleFileOp(ac, &p, writeToBrowser)
				}
			}
		}

		// Run the read loop until either side hangs up.
		readDone := make(chan struct{})
		go func() {
			defer close(readDone)
			readLoop()
		}()

		select {
		case <-done:
		case <-readDone:
		}

		// Tell agent to tear down PTY if the browser left first.
		if !exitSeen {
			_ = ptyStream.Send(model.AgentTypePTYClose, model.AgentPTYClosePayload{})
		}

		// Audit log (best-effort).
		dur := int64(time.Since(startedAt).Seconds())
		ec := exitCode
		nodeName := n.Name
		if n.Alias != nil && *n.Alias != "" {
			nodeName = *n.Alias
		}
		_ = store.CreateAuditLog(&model.AuditLog{
			NodeID:   nodeID,
			NodeName: nodeName,
			Command:  "[terminal session]",
			ExitCode: &ec,
			DurationMS: func() *int64 { v := dur * 1000; return &v }(),
			Source:   "terminal",
		})
	}
}

// handleFileOp issues a one-shot Request against the agent; the response is
// forwarded back to the browser as a file_result message tagged with the
// browser-supplied ReqID.
func handleFileOp(ac *node.AgentConn, p *model.WSTermFileOpPayload, writeToBrowser func(string, any) error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reply, err := ac.Request(ctx, model.AgentTypeFileOp, model.AgentFileOpPayload{
		Op:     p.Op,
		Path:   p.Path,
		Data:   p.Data,
		Mode:   p.Mode,
		Offset: p.Offset,
		Length: p.Length,
	}, 30*time.Second)
	if err != nil {
		_ = writeToBrowser(model.WSTermTypeFileResult, model.WSTermFileResultPayload{
			ReqID:  p.ReqID,
			Result: model.AgentFileResultPayload{OK: false, Error: err.Error()},
		})
		return
	}
	payloadBytes, _ := json.Marshal(reply.Payload)
	var res model.AgentFileResultPayload
	_ = json.Unmarshal(payloadBytes, &res)
	_ = writeToBrowser(model.WSTermTypeFileResult, model.WSTermFileResultPayload{
		ReqID:  p.ReqID,
		Result: res,
	})
}

func writeTerm(conn *websocket.Conn, msgType string, payload any) error {
	return conn.WriteJSON(model.WSMessage{Type: msgType, Payload: payload})
}

func writeTermError(conn *websocket.Conn, msg string) {
	_ = conn.WriteJSON(model.WSMessage{
		Type:    model.WSTermTypeTermError,
		Payload: model.WSTermErrorPayload{Message: msg},
	})
}
