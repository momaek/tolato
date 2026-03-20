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

## 联调基线建议

- 如果以后端为准，前端适配器需要先处理包装层、字段命名和枚举差异
- 如果以前端为准，需要补 `/api/v1/session`、`GET /api/v1/tasks`，并补齐节点指标和 UI WebSocket 事件
