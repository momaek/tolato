# 当前前后端交互 Schema（按现有代码）

说明
- 以下内容基于当前仓库代码，不按理想态补全
- 前端默认直连真实后端；只有显式设置 `VITE_USE_MOCK=true` 时才走 mock
- 当前代码里可以直接确认的错误 contract 主要来自 `ws/ui`：

```json
{
  "type": "error",
  "requestId": "optional",
  "error": {
    "code": "string",
    "message": "string"
  }
}
```

时间字段说明
- 后端 `time.Time` JSON 序列化后为 RFC3339 字符串

## 后端当前 HTTP 状态

- 当前仓库已提供 `transport/ginhttp` 中的 `Nodes / History / Settings` handler 与 route registration
- 当前已落地的 HTTP handler 仅包括：
  - `GET /api/v1/nodes`
  - `GET /api/v1/nodes/:id`
- `GET /api/v1/history/tasks`
- `GET /api/v1/history/tasks/:id`
- `GET /api/v1/settings/model-config`
- `PUT /api/v1/settings/model-config`
- `POST /api/v1/settings/model-config/test`
- `GET /api/v1/settings/account-security`
- `PUT /api/v1/settings/account-security`
- `POST /api/v1/settings/password/change`
- `POST /api/v1/settings/sessions/revoke-others`
- `GET /api/v1/settings/preferences`
- `PUT /api/v1/settings/preferences`
- 仓库已提供最小 `cmd/tolato-server` 入口，可启动 `healthz`、`Nodes`、`History` 和 `Settings` HTTP 路由
- 当前 HTTP 面仍主要用于开发联调；`Nodes` 数据源暂为静态开发数据，并非真实节点仓储
- `History` 当前使用开发种子数据拼装 detail，尚未覆盖完整 plan / approval / tool call 事实闭环
- `Settings` 当前使用开发种子数据与 `settings` repository 持久化，可完成本地读写，但密码变更与登出其他会话仍是开发态占位实现

### 当前已实现 HTTP Schema

### `GET /api/v1/nodes`

支持 query：

- `q`
- `status`
- `busy`
- `region`
- `tag`
- `limit`

响应：

```json
{
  "nodes": [
    {
      "id": "string",
      "hostname": "string",
      "region": "string",
      "os": "string",
      "version": "string",
      "tags": ["string"],
      "status": "string",
      "busy": true,
      "last_seen_at": "2026-03-22T12:00:00Z",
      "metrics": {
        "cpu": 0.32,
        "memory": 0.61,
        "disk": 0.48
      }
    }
  ]
}
```

错误响应：

```json
{
  "error": "string"
}
```

### `GET /api/v1/nodes/{id}`

响应：

```json
{
  "id": "string",
  "hostname": "string",
  "region": "string",
  "os": "string",
  "version": "string",
  "tags": ["string"],
  "status": "string",
  "busy": true,
  "last_seen_at": "2026-03-22T12:00:00Z",
  "metrics": {
    "cpu": 0.32,
    "memory": 0.61,
    "disk": 0.48
  },
  "ip_address": "string",
  "provider": "string",
  "kernel": "string",
  "uptime": "string",
  "agent_version": "string",
  "risk_signals": ["string"],
  "recent_tasks": [
    {
      "id": "string",
      "title": "string",
      "status": "string",
      "created_at": "2026-03-22T12:00:00Z"
    }
  ]
}
```

### `GET /api/v1/history/tasks`

响应：

```json
[
  {
    "id": "string",
    "title": "string",
    "summary": "string",
    "status": "planned | waiting_approval | approved | queued | dispatched | running | success | failed | partial_failed | timeout | cancelled",
    "approvalStatus": "not_required | pending | approved | rejected | cancelled",
    "risk": "low | medium | high | forbidden",
    "targetLabels": ["string"],
    "createdAt": "2026-03-22T12:00:00Z",
    "updatedAt": "2026-03-22T12:00:00Z"
  }
]
```

### `GET /api/v1/history/tasks/{id}`

响应：

```json
{
  "id": "string",
  "title": "string",
  "summary": "string",
  "status": "success",
  "approvalStatus": "approved",
  "risk": "medium",
  "targetLabels": ["string"],
  "createdAt": "2026-03-22T12:00:00Z",
  "updatedAt": "2026-03-22T12:00:00Z",
  "impact": "string",
  "steps": ["string"],
  "executions": [
    {
      "nodeId": "string",
      "label": "string",
      "status": "queued | running | success | failed | skipped",
      "stdoutTail": "string",
      "stderrTail": "string"
    }
  ],
  "auditEvents": [
    {
      "id": "string",
      "actor": "string",
      "eventType": "string",
      "description": "string",
      "createdAt": "2026-03-22T12:00:00Z"
    }
  ],
  "toolMeta": ["string"],
  "aiSummary": "string"
}
```

## 后端当前 `ws/ui` 协议结构

说明：

- 当前代码已实现 `transport/wsui`、`app/session`、`app/runtime`、`app/execution` 之间的协议和事件模型
- 当前 `cmd/tolato-server` 已接入 `GET /ws/ui` 与 `GET /ws/agent`
- 当前 Console 运行形态仍是开发态闭环：seed 一个 idle session，使用脚本化 LLM provider 和本地 fallback execution，不依赖真实 Agent 也能跑完低风险主链路

连接建立后，服务端主动发送：

```json
{
  "type": "connection.ready",
  "timestamp": "2026-03-22T12:00:00Z"
}
```

### 请求 envelope

```ts
type RequestEnvelope = {
  type: string
  requestId?: string
  payload?: object
}
```

当前已实现的请求类型：

```ts
type CurrentUiWsRequest =
  | { type: "sessions.list.request"; requestId?: string }
  | { type: "session.snapshot.request"; requestId?: string; payload: { sessionId: string } }
  | { type: "session.rows.request"; requestId?: string; payload: { sessionId: string; before?: string; limit?: number } }
  | { type: "session.message.submit"; requestId?: string; payload: { sessionId: string; text: string; clientMessageId: string } }
  | { type: "session.target.confirm"; requestId?: string; payload: { sessionId: string; nodeIds: string[]; scope: "single" | "multi" | "all_online"; idempotencyKey: string } }
  | { type: "session.approval.approve"; requestId?: string; payload: { sessionId: string; taskId: string; idempotencyKey: string } }
  | { type: "session.approval.reject"; requestId?: string; payload: { sessionId: string; taskId: string; reason?: string; idempotencyKey: string } }
  | { type: "session.operation.cancel"; requestId?: string; payload: { sessionId: string; taskId: string; idempotencyKey: string } }
  | { type: "subscriptions.update"; requestId?: string; payload: { activeSessionId: string; watchSessionIds: string[] } }
```

### 响应 envelope

```ts
type ResponseEnvelope =
  | { type: "sessions.list.response"; requestId?: string; payload: { items: SessionListItem[] } }
  | { type: "session.snapshot.response"; requestId?: string; payload: { snapshot: SessionSnapshot } }
  | { type: "session.rows.response"; requestId?: string; payload: { page: TimelinePage } }
  | { type: "session.action.accepted"; requestId?: string; payload: { sessionId: string; timestamp: string } }
  | { type: "error"; requestId?: string; error: { code: string; message: string } }
```

当前 `error.code` 会映射为：

- `session_busy`
- `invalid_argument`
- `not_found`
- `conflict`
- `duplicate_action`

### 当前已实现推送事件

```ts
type CurrentUiWsEvent =
  | { type: "session.state.updated"; eventScope: "timeline"; sessionId: string; status: SessionStatus; revision: number; timestamp: string }
  | { type: "timeline.row.appended"; eventScope: "timeline"; sessionId: string; row: TimelineRow; revision: number; timestamp: string }
  | { type: "thread.target.pending"; eventScope: "timeline"; sessionId: string; targetContext: ActiveTargetContext; revision: number; timestamp: string }
  | { type: "thread.target.confirmed"; eventScope: "timeline"; sessionId: string; targetContext: ActiveTargetContext; revision: number; timestamp: string }
  | { type: "execution.chunk"; eventScope: "timeline"; sessionId: string; taskId: string; executionId: string; nodeId: string; chunk: ExecutionChunk; timestamp: string }
  | { type: "execution.finished"; eventScope: "timeline"; sessionId: string; taskId: string; executionId: string; nodeId: string; status: string; timestamp: string }
  | { type: "session.summary.updated"; eventScope: "summary"; sessionId: string; summary: SessionSummary; timestamp: string }
  | { type: "session.finished"; eventScope: "summary"; sessionId: string; summary: SessionSummary; timestamp: string }
```

当前代码已落库并可通过 `timeline.row.appended` 推出的 row 类型包括：

- `user_message`
- `assistant_text`
- `target_confirmation`
- `tool_call_meta`
- `tool_result_meta`
- `plan`
- `approval`
- `execution`
- `summary`

### 当前未实现但目标态已定义的事件

- `llm.sse.event`
- `llm.response.completed`
- `thread.target.cleared`
- `session.requires_attention`
- `session.unread.updated`

## `Console` 目标态 `ws/ui` Contract

说明：

- 以下 contract 仍可作为目标态真源
- 但其中 `sessions.list.*`、`session.snapshot.*`、`session.rows.*`、`session.message.submit`、`session.target.confirm`、`session.approval.approve`、`session.approval.reject`、`session.operation.cancel`、`subscriptions.update` 以及部分 timeline/summary/execution 事件，当前代码已经落地
- 仍未实现的目标态能力，以本节和上一节“当前未实现”列表为准

按最新架构，`ws/ui` 只服务 `Console`，不承担 `Nodes / History / Settings` 页面查询。

### 请求

```ts
type UiWsRequest =
  | { type: "sessions.list.request"; requestId: string }
  | { type: "session.snapshot.request"; requestId: string; sessionId: string }
  | { type: "session.rows.request"; requestId: string; sessionId: string; before?: string; limit?: number }
  | { type: "session.message.submit"; requestId: string; sessionId: string; text: string; clientMessageId: string }
  | { type: "session.target.confirm"; requestId: string; sessionId: string; nodeIds: string[]; scope: "single" | "multi" | "all_online"; idempotencyKey: string }
  | { type: "session.approval.approve"; requestId: string; sessionId: string; taskId: string; idempotencyKey: string }
  | { type: "session.approval.reject"; requestId: string; sessionId: string; taskId: string; reason?: string; idempotencyKey: string }
  | { type: "session.operation.cancel"; requestId: string; sessionId: string; taskId: string; idempotencyKey: string }
  | { type: "subscriptions.update"; requestId: string; activeSessionId: string; watchSessionIds: string[] }
```

### 响应

```ts
type UiWsResponse =
  | { type: "connection.ready"; timestamp: string }
  | { type: "sessions.list.response"; requestId: string; items: SessionListItem[] }
  | { type: "session.snapshot.response"; requestId: string; snapshot: SessionSnapshot }
  | { type: "session.rows.response"; requestId: string; sessionId: string; rows: TimelineRow[]; nextBeforeCursor?: string }
  | { type: "session.action.accepted"; requestId: string; sessionId: string; timestamp: string }
  | { type: "error"; requestId?: string; code: string; message: string }
```

### 推送事件

```ts
type UiWsEvent =
  | { type: "session.state.updated"; sessionId: string; status: SessionStatus; revision: number; timestamp: string }
  | { type: "timeline.row.appended"; sessionId: string; row: TimelineRow; revision: number; timestamp: string }
  | { type: "llm.sse.event"; sessionId: string; responseId?: string; sequenceNumber?: number; upstreamEventType: string; rawEvent: object; timestamp: string }
  | { type: "llm.response.completed"; sessionId: string; rawResponse: object; timestamp: string }
  | { type: "thread.target.pending"; sessionId: string; targetContext: ActiveTargetContext; revision: number; timestamp: string }
  | { type: "thread.target.confirmed"; sessionId: string; targetContext: ActiveTargetContext; revision: number; timestamp: string }
  | { type: "thread.target.cleared"; sessionId: string; revision: number; timestamp: string }
  | { type: "execution.chunk"; sessionId: string; taskId: string; executionId: string; nodeId: string; chunk: ExecutionChunk; timestamp: string }
  | { type: "execution.finished"; sessionId: string; taskId: string; executionId: string; nodeId: string; status: string; timestamp: string }
  | { type: "session.summary.updated"; sessionId: string; summary: SessionSummary; timestamp: string }
  | { type: "session.requires_attention"; sessionId: string; reason: string; timestamp: string }
  | { type: "session.unread.updated"; sessionId: string; unread: number; timestamp: string }
  | { type: "session.finished"; sessionId: string; summary: SessionSummary; timestamp: string }
```

### 交互约束

- `connection.ready` 只表示连接可用，不表示任何 session 已恢复
- 前端切换 session 时，应先发 `session.snapshot.request`，必要时再通过 `session.rows.request` 拉更早 rows
- 前端必须显式发送 `subscriptions.update`，服务端不记忆旧连接上的瞬时订阅状态
- active session 接收完整 timeline 级事件，watch session 只接收 summary 级事件
- `timeline.row.appended`、`session.snapshot.response` 与 `session.rows.response` 都使用 `sessionId`，不再使用 `threadId`
- OpenAI 原始 SSE 事件通过 `llm.sse.event` 透传给前端，`rawEvent` 保持上游 JSON 结构
- OpenAI 非 stream 模型返回通过 `llm.response.completed` 透传给前端，`rawResponse` 保持上游 JSON 结构
- 前端直接消费原始 `response.output_text.delta` / `done` 与 `response.reasoning_text.delta` / `done` 来展示流式 `content` 和 `thinking`
- 对模型 `content` / `thinking`，后端只负责透传原始 JSON，不再额外拆专用字段；但 `tool_call`、`tool_result`、`approval`、`execution`、`summary` 仍由后端拼成结构化 contract
- 结构化 `TimelineRow` 仍作为稳定持久化 UI 事实层，不替代原始流事件

## 前端当前类型合同

来源：

- `web/src/shared/types/console.ts`
- `web/src/shared/types/node.ts`
- `web/src/shared/types/history.ts`
- `web/src/shared/types/settings.ts`
- `web/src/shared/ws/protocol.ts`

### `SessionInfo`

```ts
{
  id: string
  name: string
  role: string
}
```

### `NodeSummary`

```ts
{
  id: string
  hostname: string
  region: string
  os: string
  version: string
  tags: string[]
  status: "online" | "stale" | "offline"
  busy: boolean
  lastSeen: string
  metrics: {
    cpu: number
    memory: number
    disk: number
  }
}
```

### `TaskDetail`

```ts
{
  id: string
  mode: "ai_agent" | "direct_shell"
  inputText: string
  target: string[]
  createdAt: string
  status: "planned" | "waiting_approval" | "approved" | "queued" | "dispatched" | "running" | "success" | "failed" | "partial_failed" | "timeout" | "cancelled"
  approvalStatus: "not_required" | "pending" | "approved" | "rejected" | "cancelled"
  plan: {
    targetNodes: string[]
    summary: string
    estimatedImpact: string
    riskLevel: "low" | "medium" | "high" | "forbidden"
    requiresApproval: boolean
    steps: Array<{
      id: string
      action: string
      args: Record<string, string>
      risk: "low" | "medium" | "high" | "forbidden"
    }>
  }
  aggregate: {
    total: number
    success: number
    failed: number
    offlineSkipped: number
    running: number
  }
  summary: string
  executions: Array<{
    id: string
    taskId: string
    nodeId: string
    status: string
    startedAt?: string
    finishedAt?: string
    exitCode: number | null
    stdoutTail: string
    stderrTail: string
    streamSummary: string
  }>
}
```

### `UiWsEvent`

```ts
type UiWsEvent =
  | { type: "connection.ready"; timestamp: string }
  | { type: "connection.synced"; timestamp: string }
  | { type: "node.updated"; node: NodeSummary }
  | {
      type: "task.status"
      taskId: string
      status: "planned" | "waiting_approval" | "approved" | "queued" | "dispatched" | "running" | "success" | "failed" | "partial_failed" | "timeout" | "cancelled"
      timestamp: string
    }
```

## 当前前后端 Contract 不一致点

- 当前仓库已有 Gin router、HTTP handler 与 `cmd/tolato-server` 启动入口，因此 `/api/v1/nodes`、`/api/v1/history/tasks`、`/api/v1/settings/*` 可视为开发态已实现
- 前端 `Nodes / History / Settings` adapter 已能在非 mock 模式下直连对应 HTTP 接口，但 `History` 详情与 `Settings` 安全能力仍受后端当前简化事实模型限制
- 前端真实 `ws/ui` adapter 已处理 `requestId + payload` envelope；mock client 仍保留原前端动作模型
- 前端把审批操作建模为单一 `session.approval.action`，后端当前拆成 `session.approval.approve`、`session.approval.reject`、`session.operation.cancel`
- 前端 target confirmation 仍保留 `confirm | reselect | clear` 动作模型；真实后端当前已支持 `confirm`、`reselect`、`clear`
- 前端仍期望 `connection.synced`、`timeline.row.updated`、`session.requires_attention`、`session.unread.updated`、`llm.sse.event`、`llm.response.completed` 等事件；其中当前后端已实现 `llm.sse.event`、`llm.response.completed`、`session.requires_attention`、`session.unread.updated`，但 `timeline.row.updated` 仍未落地
- 前端 `session.summary.updated` 当前载荷是 `{ session: SessionListItem }`；后端当前载荷是 `{ summary: SessionSummary }`
- 前端 `execution.chunk` / `execution.finished` 当前期望的是带 `row + revision` 的 UI 视图；后端当前发送的是执行事实事件 `{ taskId, executionId, nodeId, chunk/status, timestamp }`
- 前端 `SessionSnapshot`、`TimelineRow`、`TargetContext`、`ExecutionRow`、`SummaryRow` 比后端当前 snapshot/view 更富 UI 语义，仍需要一层 adapter 映射
- 最新后端架构要求 `ws/ui` 只服务 `Console`，`Nodes / History / Settings` 必须走独立 HTTP 面；这一点与当前前端 mock 结构和后端实际状态仍未完全收口

## 联调基线建议

- 如果以后端为准，前端适配器需要先处理包装层、字段命名和枚举差异
- 如果以前端为准，需要补 `/api/v1/session`、`GET /api/v1/tasks`，并补齐节点指标和 UI WebSocket 事件

## 面向 PRD 新模型的增量 Contract（建议）

以下内容不是当前代码已实现的事实，而是为了支持 `docs/prd.md` 中“先进入会话，再解析 / 确认目标机器”的目标态所需增量。

其中 session 列表、session snapshot、切换 session 的恢复逻辑，以及“一个 WebSocket 连接同时订阅多个 session”的交互模型，以 [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md) 为专项定义。

根据最新 PRD 与后端架构，目标态后端需要明确分成两个面：

- `Console`
  - 继续通过 `ws/ui` 承载 session 请求 / 响应和增量事件
- `Nodes / History / Settings`
  - 通过 HTTP 提供查询与配置接口

因此：

- `ws/ui` 不承载 `Nodes / History / Settings` 页面查询
- `Direct shell` 仅作为模式占位，不提供真实执行接口

### 目标上下文

```ts
type TargetCandidate = {
  nodeId: string
  hostname: string
  region: string
  matchedBy: "hostname" | "region" | "tag" | "history" | "all_online"
  reason: string
}

type ActiveTargetContext = {
  status: "unset" | "pending_confirmation" | "confirmed"
  scope: "single" | "multi" | "all_online"
  nodeIds: string[]
  displayLabel: string
  source: "user_explicit" | "assistant_resolved" | "context_inherited"
  confidence: number
  candidates?: TargetCandidate[]
  sourceMessageId?: string
  confirmedAt?: string
}
```

前端展示规则建议：
- 顶部状态栏显示当前 `ActiveTargetContext`
- 当 `status = "pending_confirmation"` 时，显示待确认 badge，而不是直接进入执行
- 每条 plan / approval / execution row 都带一份只读目标标记，说明本次操作到底针对哪台机器或哪几台机器

### Row 展示模型建议

```ts
type TimelineRow =
  | { id: string; kind: "user_message"; createdAt: string; text: string }
  | { id: string; kind: "assistant_text"; createdAt: string; text: string }
  | { id: string; kind: "target_confirmation"; createdAt: string; targetContext: ActiveTargetContext }
  | { id: string; kind: "tool_call_meta"; createdAt: string; toolName: string; argsPreview?: string; source: "agent_loop" }
  | { id: string; kind: "tool_result_meta"; createdAt: string; toolName: string; status: "succeeded" | "failed"; text: string; source: "agent_loop" | "user_action" }
  | { id: string; kind: "plan"; createdAt: string; taskId: string }
  | { id: string; kind: "approval"; createdAt: string; taskId: string }
  | { id: string; kind: "execution"; createdAt: string; taskId: string }
  | { id: string; kind: "summary"; createdAt: string; taskId: string }
```

前端行为建议：
- 每次关键动作都追加新 row，不回写旧 row 的主要语义
- 按钮点击触发的确认 / 审批默认不生成 `user_message`
- 普通 tool 调用默认展示 `tool_call_meta` + `tool_result_meta`
- 按钮点击结果只展示 `tool_result_meta`，例如 `target_confirmation succeeded · 1 target confirmed`

### `SessionSnapshot` 目标态增量

```ts
{
  session: {
    id: string
    title: string
    status: "idle" | "running" | "paused_wait_target_confirmation" | "paused_wait_approval" | "waiting_async_execution" | "completed" | "failed"
    updatedAt: string
    revision: number
  }
  timeline: {
    rows: TimelineRow[]
    nextBeforeCursor?: string
    hasMoreBefore: boolean
  }
  llmStreamState?: {
    responseId?: string
    status: "streaming" | "completed"
    contentText?: string
    reasoningText?: string
    events?: Array<{
      sequenceNumber?: number
      upstreamEventType: string
      rawEvent: object
    }>
  }
}
```

约束建议：
- `llmStreamState` 只用于恢复当前仍在进行中的原始 reasoning / content stream
- 最终稳定展示仍以后续 `assistant_text` 和结构化 `TimelineRow` 为准

### `SessionInfo` 目标态增量

```ts
{
  id: string
  name: string
  role: string
  activeTargetContext: ActiveTargetContext
}
```

### `TaskDetail` 目标态增量

```ts
{
  id: string
  mode: "ai_agent" | "direct_shell"
  inputText: string
  target: string[]
  targetContext: ActiveTargetContext
  targetConfirmed: boolean
  createdAt: string
  status: "planned" | "waiting_approval" | "approved" | "queued" | "dispatched" | "running" | "success" | "failed" | "partial_failed" | "timeout" | "cancelled"
  ...
}
```

约束建议：
- `targetConfirmed = false` 时，不允许进入 `queued`、`dispatched` 或 `running`
- 低风险只读任务也必须先有确认后的目标，再允许自动执行
- 若本轮沿用了历史上下文，`targetContext.source` 必须标记为 `context_inherited`

### 页面查询面的新增 HTTP Contract（建议）

以下内容用于支撑最新 PRD 中的 `/nodes`、`/nodes/:id`、`/history`、`/settings`。

### `GET /api/v1/nodes`

用途：

- 支撑 `Nodes` 页面
- 返回节点列表页投影，而不是 runtime 原始实体

请求 query 建议：

- `q`
- `status`
- `busy`
- `region`
- `tag`
- `page`
- `page_size`

响应：

```json
{
  "items": [
    {
      "id": "string",
      "hostname": "string",
      "region": "string",
      "os": "string",
      "version": "string",
      "tags": ["string"],
      "status": "online | stale | offline",
      "busy": true,
      "last_seen": "2026-03-22T12:00:00Z",
      "metrics": {
        "cpu": 0.32,
        "memory": 0.61,
        "disk": 0.48
      }
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 100
}
```

### `GET /api/v1/nodes/{id}`

用途：

- 支撑 `Node Detail` 页面

响应：

```json
{
  "id": "string",
  "hostname": "string",
  "region": "string",
  "os": "string",
  "version": "string",
  "tags": ["string"],
  "status": "online | stale | offline",
  "busy": true,
  "last_seen": "2026-03-22T12:00:00Z",
  "metrics": {
    "cpu": 0.32,
    "memory": 0.61,
    "disk": 0.48
  },
  "recent_tasks": [
    {
      "task_id": "string",
      "input_text": "string",
      "status": "running | success | failed | timeout",
      "created_at": "2026-03-22T12:00:00Z"
    }
  ]
}
```

### `GET /api/v1/history/tasks`

用途：

- 支撑 `History` 页面任务列表

请求 query 建议：

- `status`
- `approval_status`
- `page`
- `page_size`

响应：

```json
{
  "items": [
    {
      "id": "string",
      "input_text": "string",
      "target_summary": "jp-tokyo-01",
      "status": "planned | waiting_approval | approved | queued | dispatched | running | success | failed | partial_failed | timeout | cancelled",
      "approval_status": "not_required | pending | approved | rejected | cancelled",
      "aggregate": {
        "total": 1,
        "success": 1,
        "failed": 0,
        "offline_skipped": 0,
        "running": 0
      },
      "created_at": "2026-03-22T12:00:00Z"
    }
  ],
  "page": 1,
  "page_size": 20,
  "total": 100
}
```

### `GET /api/v1/history/tasks/{id}`

用途：

- 支撑 `History` 页面详情区

响应：

```json
{
  "task": {
    "id": "string",
    "input_text": "string",
    "target_summary": "jp-tokyo-01",
    "status": "success",
    "approval_status": "approved",
    "plan": {},
    "aggregate": {
      "total": 1,
      "success": 1,
      "failed": 0,
      "offline_skipped": 0,
      "running": 0
    },
    "executions": [],
    "tool_meta_rows": [],
    "audits": []
  }
}
```

约束：

- `History` 不单独拆审计页
- 审计信息作为 task detail 的关联内容返回

### `GET /api/v1/settings/model-config`

```json
{
  "provider": "OpenAI",
  "model": "gpt-5.4",
  "endpoint": "https://api.openai.com/v1",
  "temperature": 0.2,
  "maxTokens": 2048,
  "timeoutSec": 60,
  "approvalMode": "balanced",
  "hasApiKey": true
}
```

### `PUT /api/v1/settings/model-config`

```json
{
  "provider": "OpenAI",
  "model": "gpt-5.4",
  "endpoint": "https://api.openai.com/v1",
  "apiKey": "string",
  "temperature": 0.2,
  "maxTokens": 2048,
  "timeoutSec": 60,
  "approvalMode": "balanced"
}
```

### `POST /api/v1/settings/model-config/test`

```json
{
  "ok": true,
  "message": "connection test succeeded"
}
```

### `POST /api/v1/settings/password/change`

请求：

```json
{
  "currentPassword": "string",
  "newPassword": "string"
}
```

响应：

```json
{
  "message": "password changed"
}
```

### `GET /api/v1/settings/account-security`

```json
{
  "username": "admin",
  "lastLoginAt": "2026-03-22T07:55:00Z",
  "mfaEnabled": true,
  "auditRetentionDays": 90
}
```

### `PUT /api/v1/settings/account-security`

```json
{
  "username": "admin",
  "lastLoginAt": "2026-03-22T07:55:00Z",
  "mfaEnabled": true,
  "auditRetentionDays": 90
}
```

### `POST /api/v1/settings/sessions/revoke-others`

响应：

```json
{
  "message": "other sessions revoked"
}
```

### `GET /api/v1/settings/preferences`

```json
{
  "preferredRegion": "Tokyo",
  "defaultMode": "ai_agent",
  "locale": "zh-CN",
  "compactTimeline": false,
  "streamMarkdown": true
}
```

### `PUT /api/v1/settings/preferences`

```json
{
  "preferredRegion": "Tokyo",
  "defaultMode": "ai_agent",
  "locale": "zh-CN",
  "compactTimeline": false,
  "streamMarkdown": true
}
```

### 风险策略 Contract 说明

后端目标态风险策略应明确为：

- `low`
  - 可自动执行
- `medium`
  - 必须 approval
- `high`
  - 当前后端实现基线仍允许 approval
- `forbidden`
  - 直接阻断

说明：

- 这与最新 PRD 中“`high` 默认阻断或保留为策略位”的文案存在差异
- 在产品语义统一前，API 合同以 [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md) 和 [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md) 为准

### 建议新增 HTTP 接口

### `POST /api/v1/targets/resolve`

请求：

```json
{
  "thread_id": "string",
  "message_id": "string",
  "input_text": "重启东京节点的 nginx"
}
```

响应：

```json
{
  "target_context": {
    "status": "pending_confirmation",
    "scope": "single",
    "node_ids": ["node_tokyo_01"],
    "display_label": "jp-tokyo-01",
    "source": "assistant_resolved",
    "confidence": 0.94,
    "candidates": [
      {
        "node_id": "node_tokyo_01",
        "hostname": "jp-tokyo-01",
        "region": "Tokyo",
        "matched_by": "region",
        "reason": "命中了“东京节点”"
      }
    ]
  }
}
```

### `POST /api/v1/threads/{id}/target-context/confirm`

请求：

```json
{
  "message_id": "string",
  "node_ids": ["node_tokyo_01"],
  "scope": "single"
}
```

响应：

```json
{
  "target_context": {
    "status": "confirmed",
    "scope": "single",
    "node_ids": ["node_tokyo_01"],
    "display_label": "jp-tokyo-01",
    "source": "assistant_resolved",
    "confidence": 0.94,
    "confirmed_at": "2026-03-21T12:00:00Z"
  }
}
```

### `DELETE /api/v1/threads/{id}/target-context`

用途：
- 用户主动清除当前会话已确认目标
- 避免后续消息错误继承旧目标

### 兼容说明：旧版 thread 视角增量事件草案

以下内容仅保留为早期 thread 视角草案，当前实现基线以上述 `Console` 目标态 `ws/ui` Contract 为准。

```ts
type UiWsEvent =
  | { type: "thread.target.pending"; threadId: string; targetContext: ActiveTargetContext; timestamp: string }
  | { type: "thread.target.confirmed"; threadId: string; targetContext: ActiveTargetContext; timestamp: string }
  | { type: "thread.target.cleared"; threadId: string; timestamp: string }
  | { type: "timeline.row.appended"; threadId: string; row: TimelineRow; timestamp: string }
```

用途：
- 驱动顶部目标 badge、消息流中的确认 row 和 plan row 目标标签同步刷新
- 明确区分“候选目标待确认”与“已确认、可执行”的两种状态
- 驱动 row-based 时间线持续追加，其中：
  - `tool_call_meta` 用于展示普通工具调用，如 `list_nodes`
  - `tool_result_meta` 用于展示工具结果，以及按钮型确认 / 审批的结果

示例：

```json
{
  "type": "timeline.row.appended",
  "threadId": "thread_123",
  "timestamp": "2026-03-21T12:00:00Z",
  "row": {
    "id": "row_001",
    "kind": "tool_call_meta",
    "createdAt": "2026-03-21T12:00:00Z",
    "toolName": "list_nodes",
    "argsPreview": "status=online,stale",
    "source": "agent_loop"
  }
}
```

```json
{
  "type": "timeline.row.appended",
  "threadId": "thread_123",
  "timestamp": "2026-03-21T12:00:01Z",
  "row": {
    "id": "row_002",
    "kind": "tool_result_meta",
    "createdAt": "2026-03-21T12:00:01Z",
    "toolName": "target_confirmation",
    "status": "succeeded",
    "text": "jp-tokyo-01 confirmed",
    "source": "user_action"
  }
}
```
