# Loop 架构设计 — 方案 A：Goroutine-per-Conversation + Channel

## 1. 概述

本文档描述后端 AI Agent Loop 及其周边基础设施的实现架构。范围涵盖：

- **AI Agent Engine** — 核心 Loop（LLM 调用 → streaming → tool 调度 → 敏感确认 → 循环）
- **OpenAI Client** — 基于 `sashabaranov/go-openai` SDK 的流式调用封装
- **Node Agent Manager** — 管理 Node Agent WebSocket 连接、指令下发、结果收集
- **WebSocket Gateway** — 前端对话 WebSocket，事件推送与用户交互

### 1.1 设计原则

- **Goroutine-per-Conversation**：每个前端 WS 连接对应一个 goroutine 跑 Loop，Go 原生风格
- **Channel 通信**：Loop 与 WS handler 之间通过 channel 传递事件和用户输入
- **顺序 Loop 逻辑**：Loop 内部是纯顺序代码（调 LLM → 解析 → 执行 tool → 循环），不引入状态机
- **并行 Tool 执行**：同一轮多个 tool_calls 并行下发给 Node Agent，等全部返回后进入下一轮

---

## 2. 整体数据流

```
Frontend Browser
    │
    │ WebSocket (/ws/chat)
    ▼
┌─────────────────────────────────────────────────┐
│  WS Handler (per-connection goroutine)          │
│  ├─ 读取前端消息 → 写入 inputCh                   │
│  ├─ 从 eventCh 读取事件 → 序列化为 JSON 推送前端    │
│  └─ 管理连接生命周期                               │
└────────┬──────────────────────┬──────────────────┘
         │ inputCh              │ eventCh
         ▼                      ▲
┌─────────────────────────────────────────────────┐
│  Loop Runner (per-conversation goroutine)       │
│  ├─ 构建 messages（system prompt + history）      │
│  ├─ 调用 LLM（streaming）                         │
│  ├─ 解析 streaming chunks → 推送 event            │
│  ├─ 收到 tool_calls → 敏感检测 → 确认/执行          │
│  ├─ 并行下发 tool → 收集结果                       │
│  └─ 循环或结束                                    │
└────────┬──────────────────────┬──────────────────┘
         │                      │
         ▼                      ▼
┌──────────────────┐   ┌──────────────────────────┐
│  OpenAI Client   │   │  Node Agent Manager      │
│  (go-openai SDK) │   │  ├─ 连接池（agent conns） │
│  ├─ streaming    │   │  ├─ 指令下发              │
│  └─ tool_calls   │   │  └─ 结果收集              │
└──────────────────┘   └───────────┬──────────────┘
                                   │ WebSocket (/ws/agent)
                                   ▼
                           Node Agent × N (VPS)
```

---

## 3. 模块设计

### 3.1 WS Handler (`handler/chat_ws.go`)

职责：管理前端 WebSocket 连接，桥接前端与 Loop Runner。

```go
// ChatWSHandler 管理单个前端 WebSocket 连接
type ChatWSHandler struct {
    conn           *websocket.Conn
    conversationID string
    eventCh        chan Event    // Loop → 前端（带 buffer）
    inputCh        chan Input    // 前端 → Loop
    db             *gorm.DB
    loopRunner     *LoopRunner
}
```

**连接生命周期**：

1. 前端连接 `/ws/chat?conversation_id=xxx`
2. 升级 WebSocket，创建 `eventCh`（buffer=64）和 `inputCh`（buffer=1）
3. 启动两个 goroutine：
   - **读 goroutine**：从 WS 读取前端消息，解析后写入 `inputCh`
   - **写 goroutine**：从 `eventCh` 读取事件，序列化为 JSON 写入 WS
4. 等待前端发送 `user_message`
5. 收到消息后创建 `LoopRunner`，在新 goroutine 中启动 Loop
6. Loop 结束后回到等待状态（同一连接可以多次对话）
7. WS 断开时清理所有 goroutine 和 channel

**关键设计**：
- `eventCh` 带 buffer（64），防止 Loop 被前端写入速度阻塞
- `inputCh` buffer 为 1，因为 Loop 执行期间前端输入被禁用，只有 confirm_response 需要传入
- WS 断开时通过 `context.Cancel` 通知 Loop 停止

### 3.2 Loop Runner (`agent/engine.go`)

职责：核心 agentic loop，编排 LLM 调用和 Tool 执行。

```go
// LoopRunner 执行单次对话的 agent loop
type LoopRunner struct {
    ctx            context.Context
    conversationID string
    messages       []openai.ChatCompletionMessage  // 累积的 messages
    eventCh        chan<- Event                     // 向 WS handler 推送事件
    inputCh        <-chan Input                     // 从 WS handler 接收用户输入
    llmClient      *LLMClient                      // OpenAI 客户端封装
    toolExecutor   *ToolExecutor                    // Tool 执行器
    promptBuilder  *PromptBuilder                   // System Prompt 构建器
    config         LoopConfig                       // max_rounds, temperature 等
}

// LoopConfig 从 Settings 表加载
type LoopConfig struct {
    MaxRounds          int     // 默认 20
    Temperature        float64
    Model              string
    ContextRounds      int     // 历史保留轮数
    OutputTruncateLines int    // 命令输出截断行数
}
```

**Loop 主流程**（伪代码）：

```go
func (r *LoopRunner) Run(userMessage string) error {
    // 1. 构建 system prompt（动态注入在线节点列表）
    systemPrompt := r.promptBuilder.Build()

    // 2. 加载历史消息（最近 N 轮）
    r.messages = r.loadHistory(r.config.ContextRounds)

    // 3. 追加用户消息
    r.messages = append(r.messages, userMsg(userMessage))

    // 4. 进入 loop
    for round := 0; round < r.config.MaxRounds; round++ {
        // 4a. 调用 LLM（streaming）
        stream, err := r.llmClient.CreateStreamingChat(r.ctx, systemPrompt, r.messages, r.config)
        if err != nil {
            r.emit(ErrorEvent{Message: err.Error()})
            return err
        }

        // 4b. 消费 streaming response
        response, err := r.consumeStream(stream)
        // consumeStream 内部：
        //   - 每收到 reasoning delta → emit ReasoningEvent
        //   - 每收到 content delta  → emit ContentEvent
        //   - 累积完整 response（content + tool_calls）

        if err != nil {
            r.emit(ErrorEvent{Message: err.Error()})
            return err
        }

        // 4c. 追加 assistant message 到 messages
        r.messages = append(r.messages, response.ToAssistantMessage())

        // 4d. 检查是否有 tool_calls
        if len(response.ToolCalls) == 0 {
            // 没有 tool_calls → loop 结束
            break
        }

        // 4e. 处理 tool_calls（可能有多个，并行执行）
        toolResults, err := r.handleToolCalls(response.ToolCalls)
        if err != nil {
            return err
        }

        // 4f. 追加 tool result messages
        for _, result := range toolResults {
            r.messages = append(r.messages, result.ToToolMessage())
        }

        // continue → 下一轮 LLM 调用
    }

    // 5. 推送 done
    r.emit(DoneEvent{})

    // 6. 持久化消息到数据库
    r.saveMessages()

    return nil
}
```

### 3.3 Tool 执行器 (`agent/tools.go`)

职责：执行 tool calls，处理敏感操作确认，与 Node Agent Manager 交互。

```go
// ToolExecutor 负责 tool 调用的调度和执行
type ToolExecutor struct {
    nodeManager     *NodeManager       // Node Agent 连接管理
    securityChecker *SecurityChecker   // 敏感操作检测
    auditLogger     *AuditLogger       // 审计日志
    eventCh         chan<- Event        // 推送事件
    inputCh         <-chan Input        // 接收确认
}
```

**handleToolCalls 流程**：

```go
func (r *LoopRunner) handleToolCalls(toolCalls []ToolCall) ([]ToolResult, error) {
    results := make([]ToolResult, len(toolCalls))
    var wg sync.WaitGroup
    var mu sync.Mutex
    var firstErr error

    for i, tc := range toolCalls {
        // 推送 tool_call 事件给前端
        r.emit(ToolCallEvent{ID: tc.ID, Tool: tc.Name, Args: tc.Args})

        wg.Add(1)
        go func(idx int, call ToolCall) {
            defer wg.Done()

            result, err := r.executeSingleTool(call)
            mu.Lock()
            if err != nil && firstErr == nil {
                firstErr = err
            }
            results[idx] = result
            mu.Unlock()

            // 推送 tool_result 事件给前端
            r.emit(ToolResultEvent{ID: call.ID, Result: result})
        }(i, tc)
    }

    wg.Wait()
    return results, firstErr
}
```

**单个 Tool 执行（含敏感确认）**：

```go
func (r *LoopRunner) executeSingleTool(tc ToolCall) (ToolResult, error) {
    switch tc.Name {
    case "list_nodes":
        return r.toolExecutor.ListNodes()

    case "get_node_info":
        nodeID := tc.Args["node_id"].(string)
        return r.toolExecutor.GetNodeInfo(nodeID)

    case "execute_command":
        nodeID := tc.Args["node_id"].(string)
        command := tc.Args["command"].(string)
        timeout := getTimeoutOrDefault(tc.Args)

        // 敏感操作检测
        if r.toolExecutor.securityChecker.IsSensitive(command) {
            // 推送确认请求
            r.emit(ConfirmRequestEvent{
                ID:   tc.ID,
                Tool: tc.Name,
                Args: tc.Args,
            })

            // 阻塞等待用户确认
            response := r.waitForConfirm(tc.ID)
            if !response.Approved {
                return ToolResult{
                    Data: "用户拒绝了该操作",
                }, nil
            }
        }

        // 执行命令
        result, err := r.toolExecutor.nodeManager.ExecuteCommand(r.ctx, nodeID, command, timeout)
        if err != nil {
            return ToolResult{Error: err.Error()}, nil
        }

        // 截断输出
        result.Stdout = truncateOutput(result.Stdout, r.config.OutputTruncateLines)
        result.Stderr = truncateOutput(result.Stderr, r.config.OutputTruncateLines)

        // 审计日志
        r.toolExecutor.auditLogger.Log(nodeID, command, result)

        return result, nil
    }

    return ToolResult{Error: "unknown tool: " + tc.Name}, nil
}
```

**敏感操作等待确认**：

```go
func (r *LoopRunner) waitForConfirm(toolCallID string) ConfirmResponse {
    for {
        select {
        case input := <-r.inputCh:
            if confirm, ok := input.(ConfirmInput); ok && confirm.ID == toolCallID {
                return ConfirmResponse{Approved: confirm.Approved}
            }
        case <-r.ctx.Done():
            return ConfirmResponse{Approved: false}
        }
    }
}
```

### 3.4 Prompt 构建器 (`agent/prompt.go`)

职责：动态构建 System Prompt。

```go
type PromptBuilder struct {
    nodeManager *NodeManager
    db          *gorm.DB  // 读取自定义 prompt 设置
}

func (b *PromptBuilder) Build() string {
    // 1. 基础角色定义（硬编码）
    // 2. Tool 使用指引（硬编码）
    // 3. 当前在线节点列表（从 NodeManager 实时获取）
    // 4. 输出格式偏好（硬编码）
    // 5. 安全规范（硬编码）
    // 6. 用户自定义追加 prompt（从 Settings 读取）
}
```

动态注入的节点信息示例：
```
当前可用节点：
- node-abc: web-1, Ubuntu 22.04, 4C8G, IP: 1.2.3.4, 在线
- node-def: db-1, Debian 12, 2C4G, IP: 5.6.7.8, 在线
- node-ghi: cache-1, CentOS 9, 2C2G, IP: 9.10.11.12, 离线
```

### 3.5 OpenAI Client (`openai/client.go`)

职责：封装 `sashabaranov/go-openai` SDK，提供流式调用接口。

```go
type LLMClient struct {
    client *openai.Client
}

// NewLLMClient 从 Settings 加载 API Base URL 和 API Key 创建客户端
func NewLLMClient(apiBase, apiKey string) *LLMClient {
    config := openai.DefaultConfig(apiKey)
    config.BaseURL = apiBase
    return &LLMClient{client: openai.NewClientWithConfig(config)}
}
```

**流式调用**：

```go
// StreamResponse 封装流式响应的累积结果
type StreamResponse struct {
    Reasoning  string      // 完整 reasoning 文本
    Content    string      // 完整 content 文本
    ToolCalls  []ToolCall  // 解析后的 tool calls
}

func (c *LLMClient) CreateStreamingChat(
    ctx context.Context,
    systemPrompt string,
    messages []openai.ChatCompletionMessage,
    config LoopConfig,
) (*openai.ChatCompletionStream, error) {
    req := openai.ChatCompletionRequest{
        Model:       config.Model,
        Messages:    prependSystem(systemPrompt, messages),
        Stream:      true,
        Temperature: float32(config.Temperature),
        Tools:       getToolDefinitions(),  // list_nodes, get_node_info, execute_command
    }
    return c.client.CreateChatCompletionStream(ctx, req)
}
```

**consumeStream 处理**：

```go
func (r *LoopRunner) consumeStream(stream *openai.ChatCompletionStream) (*StreamResponse, error) {
    defer stream.Close()

    resp := &StreamResponse{}
    // tool_calls 需要跨 chunk 拼装
    toolCallBuffers := map[int]*ToolCallBuffer{}

    for {
        chunk, err := stream.Recv()
        if errors.Is(err, io.EOF) {
            break
        }
        if err != nil {
            return nil, err
        }

        delta := chunk.Choices[0].Delta

        // reasoning（推理模型返回）
        // 注意：go-openai 对 reasoning_content 的支持需确认，
        // 可能需要用 raw JSON 解析或等 SDK 更新
        if delta.ReasoningContent != "" {
            resp.Reasoning += delta.ReasoningContent
            r.emit(ReasoningEvent{Delta: delta.ReasoningContent})
        }

        // content
        if delta.Content != "" {
            resp.Content += delta.Content
            r.emit(ContentEvent{Delta: delta.Content})
        }

        // tool_calls（增量拼装）
        for _, tc := range delta.ToolCalls {
            idx := tc.Index
            if _, ok := toolCallBuffers[*idx]; !ok {
                toolCallBuffers[*idx] = &ToolCallBuffer{
                    ID:   tc.ID,
                    Name: tc.Function.Name,
                }
            }
            toolCallBuffers[*idx].ArgsJSON += tc.Function.Arguments
        }
    }

    // 解析完整的 tool calls
    for _, buf := range toolCallBuffers {
        tc, err := buf.Parse()
        if err != nil {
            return nil, fmt.Errorf("parse tool call %s: %w", buf.Name, err)
        }
        resp.ToolCalls = append(resp.ToolCalls, tc)
    }

    return resp, nil
}
```

### 3.6 Node Agent Manager (`node/manager.go`)

职责：管理所有 Node Agent 的 WebSocket 连接，指令下发与结果收集。

```go
type NodeManager struct {
    mu    sync.RWMutex
    conns map[string]*AgentConn  // nodeID → connection
    db    *gorm.DB
}

// AgentConn 表示一个 Node Agent 的 WebSocket 连接
type AgentConn struct {
    nodeID   string
    conn     *websocket.Conn
    mu       sync.Mutex                          // 保护 conn 写操作
    pending  map[string]chan AgentCommandResult   // commandID → result channel
    metrics  *NodeMetrics                        // 最新心跳数据（缓存）
}
```

**指令下发与结果收集**：

```go
func (m *NodeManager) ExecuteCommand(ctx context.Context, nodeID, command string, timeout int) (*CommandResult, error) {
    m.mu.RLock()
    conn, ok := m.conns[nodeID]
    m.mu.RUnlock()
    if !ok {
        return nil, fmt.Errorf("node %s not connected", nodeID)
    }

    // 生成 command ID
    cmdID := uuid.New().String()

    // 创建 result channel
    resultCh := make(chan AgentCommandResult, 1)
    conn.mu.Lock()
    conn.pending[cmdID] = resultCh
    conn.mu.Unlock()

    defer func() {
        conn.mu.Lock()
        delete(conn.pending, cmdID)
        conn.mu.Unlock()
    }()

    // 下发指令
    msg := AgentMessage{
        Type: "command",
        ID:   cmdID,
        Payload: AgentCommandPayload{
            Action:  "execute_command",
            Command: command,
            Timeout: timeout,
        },
    }

    conn.mu.Lock()
    err := conn.conn.WriteJSON(msg)
    conn.mu.Unlock()
    if err != nil {
        return nil, fmt.Errorf("send command to node %s: %w", nodeID, err)
    }

    // 等待结果（带超时）
    timeoutDuration := time.Duration(timeout+10) * time.Second  // 额外 10s 缓冲
    select {
    case result := <-resultCh:
        return &CommandResult{
            ExitCode:   result.ExitCode,
            Stdout:     result.Stdout,
            Stderr:     result.Stderr,
            DurationMS: result.DurationMS,
        }, nil
    case <-time.After(timeoutDuration):
        return nil, fmt.Errorf("command timeout on node %s after %ds", nodeID, timeout)
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}
```

**Agent WS 连接处理**（`handler/agent_ws.go`）：

每个 Node Agent 连接也是一个 goroutine，持续读取消息并分发：

```go
func (m *NodeManager) HandleAgentConnection(conn *websocket.Conn, nodeID string) {
    agentConn := &AgentConn{
        nodeID:  nodeID,
        conn:    conn,
        pending: make(map[string]chan AgentCommandResult),
    }

    m.mu.Lock()
    m.conns[nodeID] = agentConn
    m.mu.Unlock()

    defer func() {
        m.mu.Lock()
        delete(m.conns, nodeID)
        m.mu.Unlock()
        // 更新 node 状态为 offline
    }()

    // 持续读取 Agent 消息
    for {
        var msg AgentMessage
        if err := conn.ReadJSON(&msg); err != nil {
            break
        }

        switch msg.Type {
        case "register":
            // 更新 node 信息到数据库
        case "heartbeat":
            // 更新缓存的 metrics
            agentConn.metrics = parseMetrics(msg.Payload)
            // 更新 last_heartbeat
        case "command_result":
            // 将结果发送到对应的 pending channel
            if ch, ok := agentConn.pending[msg.ID]; ok {
                ch <- parseCommandResult(msg.Payload)
            }
        case "command_stream":
            // 流式输出（暂存或直接转发）
        }
    }
}
```

### 3.7 Security Checker (`security/checker.go`)

职责：检测命令是否匹配敏感操作规则。

```go
type SecurityChecker struct {
    db *gorm.DB
}

func (c *SecurityChecker) IsSensitive(command string) bool {
    // 1. 检查全局开关是否启用
    // 2. 从 Settings 加载敏感关键词列表（可缓存）
    // 3. 逐一匹配：strings.Contains(command, keyword)
    // 4. 返回是否匹配
}

func (c *SecurityChecker) IsBlacklisted(command string) bool {
    // 检查命令黑名单，匹配则直接拒绝
}
```

---

## 4. Event 类型定义

Loop 和 WS Handler 之间通过 channel 传递的事件类型：

```go
// Event 是 Loop → WS Handler 的事件接口
type Event interface {
    EventType() string
}

// Input 是 WS Handler → Loop 的输入接口
type Input interface {
    InputType() string
}

// --- 事件类型 ---
type ReasoningEvent struct{ Delta string }
type ContentEvent   struct{ Delta string }
type ToolCallEvent  struct{ ID, Tool string; Args map[string]any }
type ToolResultEvent struct{ ID string; Result any }
type ConfirmRequestEvent struct{ ID, Tool string; Args map[string]any }
type DoneEvent      struct{}
type ErrorEvent     struct{ Message string }

// --- 输入类型 ---
type UserMessageInput  struct{ Content string; Model, DefaultNodeID *string }
type ConfirmInput      struct{ ID string; Approved bool }
```

这些类型直接对应 `schema/go/ws.go` 中定义的 WebSocket 协议，WS Handler 负责双向转换。

---

## 5. 消息持久化策略

### 5.1 写入时机

消息在 **Loop 结束后批量写入**数据库，而不是每个 streaming chunk 都写。原因：
- streaming 期间写 DB 会增加延迟
- 如果 Loop 中途失败，不会留下不完整的消息记录
- 单用户场景不需要中间状态的持久化

### 5.2 写入内容

每次 Loop 结束后保存：
1. **用户消息**（role=user, content=用户输入）
2. **AI 消息**（role=assistant, content=最终文本, reasoning=思考过程, tool_calls=JSON）
3. **Tool Result 消息**（role=tool, tool_call_id=对应ID, content=结果JSON）
4. 如果是多轮 loop（多次 LLM 调用），每轮的 assistant + tool messages 都保存

### 5.3 历史加载

加载历史时按 `seq` 排序，取最近 `context_rounds` 轮。一轮 = 一组 user + assistant + tool messages。

---

## 6. 关键边界情况处理

### 6.1 LLM API 错误
- 网络错误 / 超时 → emit ErrorEvent，Loop 终止
- 4xx（rate limit 等）→ emit ErrorEvent，Loop 终止
- 不做自动重试（单用户可手动重发）

### 6.2 Node Agent 断连
- 执行命令时 node 不在线 → tool result 返回错误信息，LLM 会看到并处理
- 执行中 node 断连 → command timeout，返回超时错误

### 6.3 命令超时
- 默认 60s，可配置
- 超时后 node agent 端也应终止命令（agent 侧实现）
- 超时结果作为 tool result 返回给 LLM

### 6.4 Max Rounds 达到上限
- 默认 20 轮
- 达到上限后 emit DoneEvent，Loop 终止
- LLM 最后一轮的输出正常保存

### 6.5 并行 Tool Calls 中部分失败
- 部分 tool 失败时，成功的结果和失败的错误信息一起作为 tool results 返回给 LLM
- LLM 会看到错误并自行决定下一步

---

## 7. 目录结构

```
server/
├── cmd/server/main.go              # 入口：初始化各模块，启动 HTTP server
├── internal/
│   ├── config/
│   │   └── config.go               # config.yaml 加载
│   ├── handler/
│   │   ├── router.go               # Gin 路由注册（REST + WS）
│   │   ├── chat_ws.go              # 前端对话 WebSocket handler
│   │   └── agent_ws.go             # Node Agent WebSocket handler
│   ├── agent/
│   │   ├── engine.go               # LoopRunner：核心 loop 逻辑
│   │   ├── tools.go                # ToolExecutor：tool 调度与执行
│   │   ├── prompt.go               # PromptBuilder：system prompt 构建
│   │   └── events.go               # Event/Input 类型定义
│   ├── node/
│   │   └── manager.go              # NodeManager：agent 连接管理、指令下发
│   ├── llm/
│   │   └── client.go               # LLMClient：go-openai SDK 封装
│   ├── security/
│   │   └── checker.go              # SecurityChecker：敏感操作检测
│   ├── store/
│   │   ├── conversation.go         # 对话 CRUD
│   │   ├── message.go              # 消息读写
│   │   ├── node.go                 # 节点 CRUD
│   │   ├── audit.go                # 审计日志
│   │   └── setting.go              # 设置读写
│   └── model/
│       └── models.go               # GORM 模型（从 schema/go 复制或引用）
├── config.yaml
├── go.mod
└── go.sum
```

---

## 8. 依赖关系

```
main.go
  ├── config.Config
  ├── gorm.DB (SQLite/PostgreSQL)
  ├── handler.Router
  │     ├── handler.ChatWSHandler
  │     │     ├── agent.LoopRunner
  │     │     │     ├── llm.LLMClient
  │     │     │     ├── agent.ToolExecutor
  │     │     │     │     ├── node.NodeManager
  │     │     │     │     ├── security.SecurityChecker
  │     │     │     │     └── store.AuditLog
  │     │     │     └── agent.PromptBuilder
  │     │     │           └── node.NodeManager
  │     │     └── store.*
  │     └── handler.AgentWSHandler
  │           └── node.NodeManager
  └── node.NodeManager (singleton)
```

`NodeManager` 是全局单例，被 ChatWSHandler（通过 ToolExecutor）和 AgentWSHandler 共享。
