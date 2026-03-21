# 当前前后端交互 Schema（按现有代码）

说明
- 以下内容基于当前仓库代码，不按理想态补全
- 前端默认 `VITE_USE_MOCK !== "false"`，即默认走 mock，不是默认直连真实后端
- 后端统一错误响应为：

```json
{
  "error": "string"
}
```

时间字段说明
- 后端 `time.Time` JSON 序列化后为 RFC3339 字符串

## 后端已实现 HTTP Schema

### `GET /api/v1/me`

```json
{
  "user": {
    "id": "string",
    "role": "string"
  }
}
```

### `GET /api/v1/nodes`

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
      "last_seen_at": "2026-03-19T12:00:00Z",
      "auth_secret_version": 1,
      "created_at": "2026-03-19T12:00:00Z",
      "updated_at": "2026-03-19T12:00:00Z"
    }
  ]
}
```

### `GET /api/v1/nodes/{id}`

```json
{
  "id": "string",
  "hostname": "string",
  "region": "string",
  "os": "string",
  "version": "string",
  "tags": ["string"],
  "status": "string",
  "last_seen_at": "2026-03-19T12:00:00Z",
  "auth_secret_version": 1,
  "created_at": "2026-03-19T12:00:00Z",
  "updated_at": "2026-03-19T12:00:00Z"
}
```

### `POST /api/v1/tasks/plan`

请求：

```json
{
  "mode": "ai_agent | manual_command",
  "target": ["string"],
  "input_text": "string"
}
```

响应：

```json
{
  "task_id": "string",
  "status": "approved | waiting_approval",
  "plan": {
    "target_nodes": ["string"],
    "summary": "string",
    "estimated_impact": "string",
    "risk_level": "string",
    "requires_approval": true,
    "steps": [
      {
        "action": "string",
        "args": {
          "key": "any"
        },
        "risk": "string",
        "timeout_sec": 30,
        "broadcast_allowed": true
      }
    ],
    "metadata": {
      "key": "string"
    }
  }
}
```

### `POST /api/v1/tasks/{id}/approve`

### `POST /api/v1/tasks/{id}/reject`

### `POST /api/v1/tasks/{id}/cancel`

响应结构一致：

```json
{
  "task_id": "string",
  "status": "string",
  "message": "string"
}
```

### `GET /api/v1/tasks/{id}`

```json
{
  "task": {
    "id": "string",
    "parent_task_id": "string",
    "mode": "string",
    "initiator_id": "string",
    "target": ["string"],
    "input_text": "string",
    "plan": {
      "target_nodes": ["string"],
      "summary": "string",
      "estimated_impact": "string",
      "risk_level": "string",
      "requires_approval": true,
      "steps": [
        {
          "action": "string",
          "args": {
            "key": "any"
          },
          "risk": "string",
          "timeout_sec": 30,
          "broadcast_allowed": true
        }
      ],
      "metadata": {
        "key": "string"
      }
    },
    "risk_level": "string",
    "approval_status": "string",
    "final_status": "string",
    "status_reason": "string",
    "created_at": "2026-03-19T12:00:00Z",
    "updated_at": "2026-03-19T12:00:00Z"
  }
}
```

### `GET /api/v1/tasks/{id}/executions`

```json
{
  "executions": [
    {
      "id": "string",
      "task_id": "string",
      "node_id": "string",
      "status": "string",
      "attempt": 1,
      "started_at": "2026-03-19T12:00:00Z",
      "finished_at": "2026-03-19T12:00:00Z",
      "exit_code": 0,
      "stdout_tail": "string",
      "stderr_tail": "string",
      "status_reason": "string"
    }
  ]
}
```

### `GET /api/v1/audits`（`task_id` 可选）

```json
{
  "events": [
    {
      "id": "string",
      "task_id": "string",
      "actor_id": "string",
      "event_type": "string",
      "payload": {
        "key": "any"
      },
      "created_at": "2026-03-19T12:00:00Z"
    }
  ]
}
```

## 后端当前 `ws/ui` Schema

- 路由：`GET /ws/ui`
- 当前实现只会在连接成功后主动发送一条占位消息

```json
{
  "type": "welcome",
  "message": "ws/ui placeholder connected"
}
```

## 前端当前 Zod 合同

来源：`web/src/shared/api/contracts.ts`

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

- 前端请求 `GET /api/v1/session`，后端实际实现的是 `GET /api/v1/me`
- 前端要求 `GET /api/v1/nodes` 返回裸数组 `NodeSummary[]`，后端实际返回 `{ "nodes": Node[] }`
- 前端要求 `GET /api/v1/tasks` 返回裸数组 `TaskDetail[]`，后端当前没有实现 `GET /api/v1/tasks`
- 前端要求 `GET /api/v1/tasks/{id}` 返回 `TaskDetail`，后端实际返回 `{ "task": Task }`
- 前端要求 `GET /api/v1/audits` 返回裸数组 `AuditEvent[]`，后端实际返回 `{ "events": AuditEvent[] }`
- 前端 `mode` 使用 `direct_shell`，后端计划请求 schema 当前是 `manual_command`
- 前端 `NodeSummary` 依赖 `busy`、`lastSeen`、`metrics`，后端 `Node` 当前没有这些字段
- 前端 `TaskPlanStep` 依赖 `id`，并假设 `args` 是 `Record<string, string>`；后端 `PlanStep` 当前没有 `id`，且 `args` 是 `map[string]any`
- 前端 `TaskExecution` 把 `startedAt` / `finishedAt` 视为可选、`exitCode` 视为可空；后端当前是固定字段且 `exit_code` 为 `int`
- 前端期望 `ws/ui` 推送 `connection.ready`、`connection.synced`、`node.updated`、`task.status`；后端当前只发送 `welcome` 占位消息
- PRD 新模型要求“先进入会话，再由 AI 解析并确认目标机器”，当前 contract 没有目标解析、目标确认和目标上下文事件

## 联调基线建议

- 如果以后端为准，前端适配器需要先处理包装层、字段命名和枚举差异
- 如果以前端为准，需要补 `/api/v1/session`、`GET /api/v1/tasks`，并补齐节点指标和 UI WebSocket 事件

## 面向 PRD 新模型的增量 Contract（建议）

以下内容不是当前代码已实现的事实，而是为了支持 `docs/prd.md` 中“先进入会话，再解析 / 确认目标机器”的目标态所需增量。

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

### WebSocket 增量事件建议

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
