# ToLaTo Session 交互与同步模型

## 1. 文档定位

本文档单独定义 ToLaTo 中 `session` 的交互逻辑、后端同步模型和前端切换行为，重点回答以下问题：

- session 到底代表什么
- session 内的目标节点为何是可变的
- 切换 session 时前端如何恢复页面
- 是否只用 WebSocket 承载 session 数据
- 一个 WebSocket 是否可以同时订阅多个 session
- 当前活跃 session 和后台 session 的事件粒度如何区分

本文档是 [docs/prd.md](/Users/wentx/momaek/src/tolato/docs/prd.md)、[docs/backend_architecture.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture.md) 与 [docs/api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md) 的 session 专项补充。

---

## 2. 核心结论

ToLaTo 的 `session` 不是“绑定某台机器的固定容器”，而是：

`一个承载对话、Agent Loop、时间线、挂起动作和当前目标上下文的运行容器`

由此得出以下结论：

- 每个 session 有自己独立的 Agent Loop
- session 内的 `active_target_context` 是可变的
- 同一个 session 中，用户可以多次切换目标节点
- 某次 plan / approval / execution 必须绑定自己的不可变 `operation.target_snapshot`
- session 只保存运行态和引用，不内嵌完整聊天记录或 execution 明细
- 前端切换 session 时，不应该靠增量事件回放来恢复页面，而应获取该 session 的完整 `snapshot`
- session 列表、session snapshot、timeline 增量事件可以全部走一条 WebSocket 连接
- 一个 WebSocket 连接应支持同时订阅多个 session
- 当前活跃 session 接收完整 timeline 事件，后台 session 只接收 summary 级事件

---

## 3. 状态模型

### 3.1 `Session`

`Session` 保存：

- 当前 `status`
- 当前 `active_target_context`
- 当前挂起动作
- 当前 operation / task / execution group 的引用
- 用于恢复 loop 的 `last_agent_state`
- 会话级展示摘要

`Session` 不等于某台机器。

它也不应该直接内嵌：

- 完整聊天消息
- 完整 timeline rows
- execution stdout / stderr 明细

这些数据应通过 `session_id` / `thread_id` 关联到独立对象。

建议最小字段：

- `id`
- `status`
- `active_target_context`
- `pending_action_type`
- `pending_action_payload`
- `current_operation_id`
- `current_task_id`
- `current_execution_group_id`
- `last_agent_state`
- `updated_at`

### 3.1.1 持久化拆分

为了避免把 session 变成“巨大 JSON 容器”，建议按职责拆分：

- `Session`
  - 只保存运行态、挂起态和当前引用
- `ThreadMessage`
  - 保存真正的聊天消息，如 `user_message`、`assistant_text`
- `TimelineRow`
  - 保存前端时间线展示行，如 `plan`、`approval`、`execution`、`summary`、`tool_meta`
- `Task`
  - 保存一次逻辑执行意图
- `Execution`
  - 保存节点级执行事实和流式输出摘要

一句话：

- session 是运行容器
- message / timeline / task / execution 是 session 下面的事实数据

### 3.2 `ActiveTargetContext`

`active_target_context` 表示：

- 当前这条会话默认正在讨论哪台机器、哪几台机器，或全部在线节点

它是可变的。用户可以在同一 session 内：

- 从 `unset` 进入 `pending_confirmation`
- 从 `pending_confirmation` 进入 `confirmed`
- 从 `confirmed` 改成另一台节点
- 从单节点改成多节点
- 主动清空当前上下文

### 3.3 `OperationTargetSnapshot`

每一次 plan / approval / execution 都必须复制一份不可变的目标快照：

- `operation.target_snapshot`

目的：

- 审批记录明确知道用户是对哪台机器批准的
- 即使 session 后续换了目标，也不影响已生成的 plan 和 execution 审计

一句话：

- `session.active_target_context` 可变
- `operation.target_snapshot` 不可变

### 3.4 Async Execution 与 Session 的关系

`exec_on_nodes` 不应把远端执行过程同步塞进 Agent Loop 内存里等待到底，而应将其建模为 session 外部的异步执行生命周期。

当 loop 进入执行阶段时，推荐状态迁移为：

1. 创建 `task`
2. 创建一个或多个 `execution`
3. session 写入 `current_task_id`
4. session 写入 `current_execution_group_id`
5. session 清空 `pending_action`
6. session.status 进入 `waiting_async_execution`
7. timeline 追加 `execution row`

此时：

- session 只表达“当前正在等哪次执行”
- `task` / `execution` 负责保存执行事实
- Node Agent 的 `stdout` / `stderr` / `exit_code` 只写 execution 相关对象，不回写 session 主体

当所有 execution 完成后：

1. runtime 根据 `current_task_id` 读取聚合结果
2. session.status 从 `waiting_async_execution` 回到 `running`
3. runtime 使用 `last_agent_state` 恢复 Agent Loop
4. 继续生成 `summary` 或结束本轮操作

因此：

- `session.status` 是 Agent Loop 状态
- `task.status` / `execution.status` 是远端执行状态
- 二者允许同时存在，且语义不同

---

## 4. 每个 Session 一条独立 Loop

后端运行模型建议为：

- 每个 session 一条独立 `Agent Loop`
- 同一个 session 内只允许一个 active loop 串行运行
- 不同 session 之间可并发运行

典型状态：

- `idle`
- `running`
- `paused_wait_target_confirmation`
- `paused_wait_approval`
- `waiting_async_execution`
- `completed`
- `failed`

其中：

- `paused_wait_target_confirmation` / `paused_wait_approval` 表示在等用户动作
- `waiting_async_execution` 表示 loop 已经下发执行，当前在等 `task / execution` 生命周期推进
- 进入 `waiting_async_execution` 时，session 上通常不再保留 `pending_action`

用户在 A session 干活时，B session 的 loop 可以继续运行。  
这正是为什么前端需要能同时收到多个 session 的摘要事件。

---

## 5. 传输原则：业务数据统一走 WebSocket

为避免前端同时维护 HTTP 与 WebSocket 两套业务取数路径，建议：

- 业务数据统一走 WebSocket
- WebSocket 同时承担 request/response 与 event push
- HTTP 仅保留登录、鉴权、静态资源和极轻量 bootstrap 用途

因此，下列数据都应通过 WebSocket 获取：

- session 列表
- session snapshot
- 分页拉取更早 rows
- session 订阅关系更新
- timeline 增量事件
- session summary 增量事件

---

## 6. 多 Session 订阅模型

一个 WebSocket 连接不应只订阅一个 session。

更合理的模型是：

- 一个连接有一个 `active_session_id`
- 一个连接有若干 `watch_session_ids`

两者区别不在“是否订阅”，而在事件粒度：

### 6.1 Active Session

当前页面打开的 session。

服务端向该 session 推送完整事件：

- `timeline.row.appended`
- `timeline.row.updated`
- `thread.target.pending`
- `thread.target.confirmed`
- `thread.target.cleared`
- `execution.chunk`
- `execution.finished`
- `session.state.updated`

### 6.2 Background Watched Sessions

当前没打开，但用户仍希望感知状态变化的 session。

服务端只推轻量摘要事件：

- `session.summary.updated`
- `session.requires_attention`
- `session.finished`
- `session.unread.updated`

目的：

- 左侧 session 列表可以更新
- 用户在 A session 时，仍能知道 B session 已完成或需要关注
- 避免把后台 session 的完整 timeline 灌入当前页面

---

## 7. 切换 Session 的展示与恢复

### 7.1 切换的目标

切换 session 时，前端要恢复的不只是聊天记录，还包括整页会话视图：

- 顶部 header 状态
- 左侧 session 摘要
- 中间 timeline rows
- 当前 pending confirmation / pending approval
- 当前 execution 摘要
- composer 可用状态

因此切换 session 时，前端必须请求一份完整 `session snapshot`。

这里的 snapshot 是一个聚合视图：

- session 运行态来自 `Session`
- 聊天与结构化时间线来自 `ThreadMessage` / `TimelineRow`
- 执行摘要来自 `Task` / `Execution`

它不是一张单独的大表原样返回。

### 7.2 切换流程

建议流程如下：

1. 用户点击左侧某个 session
2. 前端先把该 session 高亮，并让主区进入 loading skeleton
3. 前端通过 WebSocket 发送 `session.snapshot.request`
4. 后端返回 `session.snapshot.response`
5. 前端用 snapshot 整包替换当前主区状态
6. 前端发送 `subscriptions.update`
7. 新 session 升级为 active，旧 session 退化为 background watch

### 7.3 为什么不能靠增量事件回放

因为 session 历史可能很长，而且切换时用户需要的是：

- 一次性恢复当前页面
- 而不是等待旧事件一点点重建

因此：

- 增量事件负责“切换之后继续更新”
- snapshot 负责“切换当下恢复页面”

---

## 8. Session Snapshot 结构

后端应返回一个“前端可直接渲染”的 snapshot，而不是让前端自己根据十几个字段重算页面。

建议结构：

```ts
type SessionSnapshot = {
  session: {
    id: string
    title: string
    status: "idle" | "running" | "paused_wait_target_confirmation" | "paused_wait_approval" | "waiting_async_execution" | "completed" | "failed"
    currentOperationId?: string
    currentTaskId?: string
    currentExecutionGroupId?: string
    updatedAt: string
    revision: number
  }
  headerState: {
    mode: "ai_agent" | "direct_shell"
    activeTargetLabel: string
    connectionLabel: string
  }
  sidebarSummary: {
    sessionLabel: string
    lastUpdated: string
    primaryText: string
    chips: string[]
  }
  activeTargetContext: ActiveTargetContext
  pendingAction?: {
    type: "target_confirmation" | "approval"
    taskId?: string
  }
  composerState: {
    disabled: boolean
    placeholder: string
  }
  timeline: {
    rows: TimelineRow[]
    nextBeforeCursor?: string
    hasMoreBefore: boolean
  }
  executionState?: {
    taskId: string
    status: string
    aggregate?: {
      total: number
      running: number
      success: number
      failed: number
    }
    summary?: string
  }
}
```

其中：

- `revision` 用于防止旧 snapshot 覆盖新状态
- `rows` 默认返回“最近一屏”，而不是全量历史
- `timeline.rows` 是渲染视图，不意味着这些 rows 物理内嵌在 session 对象中
- `executionState` 是执行摘要，用于恢复页面，不替代 `Task` / `Execution` 事实表

---

## 9. WebSocket Request / Response 协议

建议采用同一条连接上的 request/response + event 模型。

### 9.1 请求：获取 session 列表

```json
{
  "type": "sessions.list.request",
  "requestId": "req_001"
}
```

### 9.2 响应：session 列表

```json
{
  "type": "sessions.list.response",
  "requestId": "req_001",
  "items": [
    {
      "sessionId": "sess_A",
      "title": "jp-tokyo-01",
      "status": "running",
      "updatedAt": "2026-03-21T14:33:09Z",
      "activeTargetSummary": "jp-tokyo-01",
      "unread": 0
    }
  ]
}
```

### 9.3 请求：获取 session snapshot

```json
{
  "type": "session.snapshot.request",
  "requestId": "req_002",
  "sessionId": "sess_A"
}
```

### 9.4 响应：session snapshot

```json
{
  "type": "session.snapshot.response",
  "requestId": "req_002",
  "snapshot": {
    "session": {
      "id": "sess_A",
      "title": "jp-tokyo-01",
      "status": "paused_wait_approval",
      "updatedAt": "2026-03-21T14:32:41Z",
      "revision": 18
    },
    "headerState": {
      "mode": "ai_agent",
      "activeTargetLabel": "Confirmed target: jp-tokyo-01",
      "connectionLabel": "ws connected"
    },
    "sidebarSummary": {
      "sessionLabel": "Session · jp-tokyo-01",
      "lastUpdated": "2026-03-21T14:32:41Z",
      "primaryText": "Tokyo · Debian 11 · prod-web",
      "chips": ["confirmed", "write path"]
    },
    "timeline": {
      "rows": [],
      "hasMoreBefore": true,
      "nextBeforeCursor": "row_120"
    }
  }
}
```

### 9.5 请求：拉取更早 rows

```json
{
  "type": "session.rows.request",
  "requestId": "req_003",
  "sessionId": "sess_A",
  "before": "row_120",
  "limit": 50
}
```

### 9.6 请求：更新订阅关系

```json
{
  "type": "subscriptions.update",
  "activeSessionId": "sess_A",
  "watchSessionIds": ["sess_B", "sess_C"]
}
```

---

## 10. 增量事件模型

所有事件都必须带：

- `sessionId`
- `timestamp`
- `eventScope`

建议：

```ts
type EventScope = "timeline" | "summary"
```

### 10.1 活跃 session 的完整事件

```json
{
  "type": "timeline.row.appended",
  "eventScope": "timeline",
  "sessionId": "sess_A",
  "timestamp": "2026-03-21T14:32:42Z",
  "row": {
    "id": "row_121",
    "kind": "tool_result_meta",
    "createdAt": "2026-03-21T14:32:42Z",
    "toolName": "approval",
    "status": "succeeded",
    "text": "approval recorded",
    "source": "user_action"
  }
}
```

### 10.2 后台 session 的摘要事件

```json
{
  "type": "session.finished",
  "eventScope": "summary",
  "sessionId": "sess_B",
  "timestamp": "2026-03-21T14:33:00Z",
  "summary": {
    "title": "sg-prod-01",
    "status": "completed",
    "updatedAt": "2026-03-21T14:33:00Z",
    "unread": 1
  }
}
```

---

## 11. 前端状态管理建议

建议拆成两层 store：

### 11.1 Session List Store

保存左侧列表所需的轻量数据：

- `sessionId`
- `title`
- `status`
- `updatedAt`
- `activeTargetSummary`
- `unread`

### 11.2 Session View Store

按 `sessionId` 保存完整视图缓存：

- `snapshot`
- `rows`
- `nextBeforeCursor`
- `revision`

切换 session 时：

- 先展示列表里的轻量信息和 skeleton
- snapshot 返回后整包覆盖
- 后续再由增量事件继续刷新

---

## 12. 一致性与覆盖规则

### 12.1 Snapshot 覆盖规则

前端收到 snapshot 时，应比较：

- `snapshot.session.revision`

只有 revision 不落后于当前本地状态，才允许覆盖。

### 12.2 增量事件应用规则

活跃 session 的 timeline 事件直接追加。  
后台 session 的 summary 事件只更新列表，不更新主时间线。

### 12.3 切换中的竞态

典型竞态：

- 用户点击 session B
- B 的 snapshot 还没回来
- A 或 B 又收到新事件

建议：

- `requestId` 保证响应归属
- `revision` 保证旧 snapshot 不覆盖新状态
- 当前 `activeSessionId` 变化后，只允许对应响应进入主视图

---

## 13. 断线重连

WebSocket 断线重连后，前端应按以下顺序恢复：

1. 重新鉴权建立连接
2. 发送 `sessions.list.request`
3. 发送当前打开 session 的 `session.snapshot.request`
4. 发送 `subscriptions.update`

不要指望后端自动记住之前连接上的瞬时订阅状态。

---

## 14. 与 UI 的关系

这套模型直接决定了 UI 行为：

- 左侧 session 列表通过 summary 事件实时更新
- 当前 session 切换时通过 snapshot 恢复完整页面
- 当前 session 的聊天时间线由完整 timeline 事件驱动
- 后台 session 完成时仍然可以通过 toast、badge、红点提醒用户

因此：

- Session 列表不是静态导航
- Session 切换不是简单路由跳转
- Session 本身就是一个实时运行单元

---

## 15. 一句话总结

ToLaTo 的 session 交互模型应当是：

`每个 session 都有独立 Agent Loop；session 内目标上下文可变、操作目标快照不可变；前端通过一条支持多 session 订阅的 WebSocket 同时获取 session 列表、session snapshot 与增量事件，并在切换 session 时通过 snapshot 恢复整页视图。`
