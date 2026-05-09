# Tolato 实现计划

> 本文档基于 design.md、loop-architecture.md、frontend-architecture.md、nodeprobe.md 四份设计文档与现有代码的对比分析，列出所有未完成功能及实现计划。

> **实现状态**：Phase 1、Phase 2、Phase X 已完成；**Phase 3（NodeProbe 链路监控）已于 2026-05-08 取消**，对应代码不会落地，下文小节仅作历史设计参考。
>
> **实现说明**：
> - LLM Client 使用 `openai-go/v2` SDK 替代了原计划的 `llm-sdk`
> - ContentBlock 已集成 markstream-vue 流式 Markdown 渲染
> - Toast 通知使用 vue-sonner，Light/Dark 主题通过 useTheme composable 切换
> - 审计日志支持行展开查看完整 stdout/stderr

## 现状总结

### 已完成

| 模块 | 状态 |
|------|------|
| JWT 登录认证 + 中间件 | ✅ |
| Conversation CRUD（后端 + 前端） | ✅ |
| Node 注册 + Agent WebSocket 连接 | ✅ |
| Node CRUD + 心跳指标缓存 | ✅ |
| Settings 4 组读写（后端 + 前端） | ✅ |
| Audit Log 查询（后端 + 前端） | ✅ |
| Message 存储（batch create, list, max seq） | ✅ |
| Agent 端完整实现（WS 客户端、命令执行器、系统采集器、身份管理） | ✅ |
| 前端 Login / Nodes / AuditLog / Settings 页面 | ✅ |
| shadcn-vue UI 组件库 | ✅ |
| Router + Auth Guard | ✅ |
| REST API Service + Pinia Stores (app, chat, nodes) | ✅ |
| TypeScript 类型定义（API + WS） | ✅ |
| GORM Models（含 NodeProbe 表） | ✅ |

---

## Phase 1：AI Chat Loop（核心功能）

> 这是产品的核心价值——用户通过自然语言对话管理服务器。

### 1.1 后端 — LLM Client

**新建文件：** `server/internal/llm/client.go`

- [x] 基于 `hoangvvo/llm-sdk` (sdk-go) 封装 LLM 客户端
- [x] 从 Settings 读取配置（api_base_url, api_key, model, temperature）
- [x] 支持流式 Chat Completion（streaming delta）
- [x] 自动组装 tool_calls（从多个 delta 中合并）
- [x] 定义 Tool Schema（list_nodes, get_node_info, execute_command）

### 1.2 后端 — Security Checker

**新建文件：** `server/internal/security/checker.go`

- [x] 从 Settings 加载敏感关键词列表和命令黑名单
- [x] `IsSensitive(command string) bool` — 关键词匹配检测
- [x] `IsBlacklisted(command string) bool` — 黑名单检测
- [x] 支持动态刷新配置（Settings 更新后生效）

### 1.3 后端 — Prompt Builder

**新建文件：** `server/internal/agent/prompt.go`

- [x] 构建基础 system prompt（角色定义 + 工具使用指南 + 输出格式）
- [x] 动态注入当前在线节点列表（从 NodeManager 获取）
- [x] 动态注入安全规则（从 Settings 读取）
- [x] 追加用户自定义 system prompt（从 ChatSettings 读取）

### 1.4 后端 — Event 类型定义

**新建文件：** `server/internal/agent/events.go`

- [x] `ReasoningEvent` — AI 推理过程 delta
- [x] `ContentEvent` — AI 内容输出 delta
- [x] `ToolCallEvent` — 工具调用信息
- [x] `ToolResultEvent` — 工具执行结果
- [x] `ConfirmRequestEvent` — 敏感操作确认请求
- [x] `DoneEvent` — 循环完成
- [x] `ErrorEvent` — 错误信息
- [x] `SessionReplacedEvent` — 连接被替换
- [x] Input 类型：`UserMessageInput`, `ConfirmInput`

### 1.5 后端 — Tool Executor

**新建文件：** `server/internal/agent/tools.go`

- [x] `list_nodes` — 调用 NodeManager 获取所有节点信息
- [x] `get_node_info` — 调用 NodeManager 获取单节点详情 + 实时指标
- [x] `execute_command` — 调用 NodeManager.ExecuteCommand 执行远程命令
- [x] 敏感操作检测（调用 SecurityChecker）
- [x] 写入 AuditLog（调用 store.CreateAuditLog）
- [x] 并行执行工具（sync.WaitGroup）
- [x] 输出截断处理（根据 output_truncate_lines 设置）

### 1.6 后端 — Loop Runner（核心引擎）

**新建文件：** `server/internal/agent/engine.go`

- [x] LoopRunner 结构体（持有 eventCh, inputCh, convID, store, llmClient 等依赖）
- [x] `Run()` 主循环：
  1. 构建 system prompt
  2. 加载历史消息（最近 N 轮，根据 context_rounds）
  3. 追加 user message
  4. for round < max_rounds:
     - 调用 LLM（流式）
     - 发送 ReasoningEvent / ContentEvent
     - 解析 tool_calls
     - 若无 tool_calls → break
     - 发送 ToolCallEvent
     - 检测敏感操作 → 发送 ConfirmRequestEvent → 等待 ConfirmInput
     - 并行执行工具
     - 发送 ToolResultEvent
     - 追加 tool result messages
  5. 发送 DoneEvent
  6. 持久化消息到数据库
- [x] Context 取消支持（前端断连时终止）
- [x] 错误处理（LLM 错误 → ErrorEvent → 终止）

### 1.7 后端 — Session Manager

**新建文件：** `server/internal/handler/session.go`

- [x] 维护单一活跃前端 WebSocket 连接
- [x] 新连接替换旧连接（向旧连接发送 `session_replaced`）
- [x] 连接关闭时取消所有活跃 Loop

### 1.8 后端 — Chat WebSocket Handler

**新建文件：** `server/internal/handler/chat_ws.go`

- [x] `ChatWSHandler` — WebSocket `/ws/chat` 端点
- [x] JWT 认证（从 query param 或 header 获取 token）
- [x] 通过 SessionManager 注册连接
- [x] 维护 `loops` map：conversationID → LoopRunner
- [x] Read goroutine：解析 WSMessage → 路由到对应 LoopRunner
  - `user_message` → 创建/获取 LoopRunner → 发送 UserMessageInput
  - `confirm_response` → 发送 ConfirmInput
- [x] Write goroutine：从 eventCh 读取 → 组装 WSMessage → 写入 WebSocket
- [x] 连接关闭清理（取消所有 Loop context）

**修改文件：** `server/internal/handler/router.go`

- [x] 注册 `GET /ws/chat` → ChatWSHandler

### 1.9 前端 — WebSocket Service

**新建文件：** `web/src/services/ws.ts`

- [x] WebSocket 单例管理
- [x] 自动重连（指数退避：1s → 30s）
- [x] 连接状态：connecting / connected / disconnected / replaced
- [x] 消息 handler 注册机制（按 type 分发）
- [x] `send(msg)` 方法
- [x] `session_replaced` 处理

### 1.10 前端 — Chat Store 完善

**修改文件：** `web/src/stores/chat.ts`

- [x] 增加 ConversationState 完整结构（messages, streaming, status, confirmRequest, error）
- [x] `loadConversation(id)` — 从 API 加载历史消息
- [x] `sendMessage(content)` — 通过 WS 发送 user_message
- [x] `confirmAction(id, approved)` — 通过 WS 发送 confirm_response
- [x] `stopLoop()` — 停止当前循环
- [x] WS 事件处理：
  - `reasoning` → 追加到 streaming.reasoning
  - `content` → 追加到 streaming.content
  - `tool_call` → 添加到 streaming.toolCalls
  - `tool_result` → 更新对应 toolCall 结果
  - `confirm_request` → 设置 confirmRequest，状态切 confirming
  - `done` → 完成 streaming message，状态切 idle
  - `error` → 设置 error，状态切 error

### 1.11 前端 — Chat 组件

**新建/修改文件：** `web/src/components/chat/`

- [x] `ChatTopBar.vue` — 可编辑标题、Model 选择器、默认 Node 选择器
- [x] `ChatMessages.vue` — 消息列表容器 + 自动滚动
- [x] `UserMessage.vue` — 用户消息气泡（右对齐）
- [x] `AssistantMessage.vue` — AI 消息容器
- [x] `ThinkingBlock.vue` — AI 推理过程（💭 可折叠，Collapsible）
- [x] `ContentBlock.vue` — markstream-vue 流式 Markdown + Shiki 代码高亮
- [x] `ToolCallCard.vue` — 工具执行卡片（执行中/成功/失败 三状态）
- [x] `ConfirmCard.vue` — 敏感操作确认卡片（⚠️ 样式，Approve/Reject 按钮）
- [x] `StreamingIndicator.vue` — 流式加载指示器
- [x] `ChatInput.vue` 完善 — 根据状态切换（idle=可输入, streaming=禁用+Stop 按钮, confirming=等待确认）

**修改文件：** `web/src/views/ChatView.vue`

- [x] 集成以上组件
- [x] 连接 Chat Store + WS Service
- [x] Shift+Enter 换行，Enter 发送
- [x] 快捷操作按钮功能实现

**新建文件：** `web/src/composables/useAutoScroll.ts`

- [x] 新消息/streaming delta 自动滚动到底部
- [x] 用户手动上滑 → 暂停自动滚动
- [x] "↓ 新消息" 浮动按钮回到底部

---

## Phase 2：External API + API Key 管理

> 支持外部集成（MCP Tools、第三方系统）。

### 2.1 后端 — API Key 管理

**新建文件：** `server/internal/handler/apikey.go`

- [x] `POST /api/api-keys` — 创建 API Key（生成随机 key，存储 hash，返回明文一次）
- [x] `GET /api/api-keys` — 列出所有 API Key（仅显示 prefix + 权限 + 状态）
- [x] `DELETE /api/api-keys/:id` — 撤销 API Key（设置 status=revoked）

**新建文件：** `server/internal/middleware/apikey.go`

- [x] API Key 鉴权中间件（`Authorization: Bearer <api_key>`）
- [x] 从 hash 查找匹配 key
- [x] 检查 key 状态（active/revoked）
- [x] 权限级别注入 context（readonly/standard/admin）
- [x] 更新 last_used_at

### 2.2 后端 — External API 端点

**新建文件：** `server/internal/handler/external.go`

- [x] `GET /api/v1/nodes` — 列出所有节点（API Key 鉴权）
- [x] `GET /api/v1/nodes/:id` — 节点详情
- [x] `POST /api/v1/nodes/:id/execute` — 执行命令
  - 请求体：`command`, `timeout`, `confirm`, `stream`
  - readonly 权限 → 403
  - 敏感命令 + standard 权限 + `confirm != true` → 返回 SensitiveOperationError
  - admin 权限 → 跳过确认
  - 写入 AuditLog（source=api, api_key_id）

**修改文件：** `server/internal/handler/router.go`

- [x] 注册 `/api/api-keys` 路由组（JWT 鉴权）
- [x] 注册 `/api/v1/` 路由组（API Key 鉴权中间件）

### 2.3 后端 — LLM 验证端点

**修改文件：** `server/internal/handler/setting.go`

- [x] `POST /api/settings/llm/verify` — 验证 API 配置并获取可用模型列表
  - 调用 LLM provider 的 `/v1/models` 端点
  - 返回模型列表或错误信息

### 2.4 后端 — 节点命令历史端点

**修改文件：** `server/internal/handler/node.go`

- [x] `GET /api/nodes/:id/commands` — 获取节点命令执行历史（从 AuditLog 按 node_id 过滤）

### 2.5 前端 — Node 详情页

**新建文件：** `web/src/views/NodeDetailView.vue`

- [x] 基本信息：hostname, OS, kernel, IP, Agent 版本
- [x] 实时指标：CPU, Memory, Disk, Load
- [x] 命令历史列表：Time, Command, Exit Code

**修改文件：** `web/src/router/index.ts`

- [x] 添加 `/nodes/:nodeId` 路由

### 2.6 前端 — API Key 管理（Settings 扩展）

**修改文件：** `web/src/views/SettingsView.vue`

- [x] 新增 "API Keys" Tab
- [x] 列出现有 API Keys（prefix + 权限 + 状态 + 最后使用时间）
- [x] 创建 API Key 对话框（选择权限级别）
- [x] 创建后一次性显示完整 key（提醒复制）
- [x] 撤销 API Key 按钮 + 确认对话框

---

## Phase 3：NodeProbe 链路监控（已取消 — 2026-05-08）

> ❌ **已决定不做**。下面的小节保留作为历史设计参考，所有 `[x]` 标记不代表实际实现状态——代码并未落地。如未来重启该方向请新建独立设计文档。

### 3.1 后端 — Probe 配置扩展

**修改文件：** `server/internal/config/config.go`

- [x] 添加 `ProbeConfig` 结构体：
  - `enabled`, `retention_days`
  - `telegram.bot_token`, `telegram.chat_id`
  - `alert_rules`（latency/packet_loss/tcp/bandwidth 阈值, offline_timeout, recovery_count）

**修改文件：** `server/config.yaml`

- [x] 添加 `probe:` 配置段

### 3.2 后端 — Probe Store

**新建文件：** `server/internal/probe/store.go`

- [x] ProbeLink CRUD（创建/列出/删除链路）
- [x] ProbeMetric 写入（批量插入指标）
- [x] ProbeMetric 查询（按 link_id + 时间范围）
- [x] ProbeAlert CRUD（创建/列出/更新 resolved_at）
- [x] 数据清理（删除过期 metrics 和已恢复 alerts）
- [x] 节点位置更新（canvas_x, canvas_y）

### 3.3 后端 — Alert Engine

**新建文件：** `server/internal/probe/alert.go`

- [x] 阈值检测逻辑（每次收到 metric report 时触发）
  - Latency avg > 200ms
  - Packet Loss > 5%
  - TCP Connect Time > 500ms
  - Bandwidth < 10Mbps
- [x] 告警创建（超阈值 + 无未恢复告警 → 新建 alert）
- [x] 恢复检测（连续 N 次正常 → 更新 resolved_at）
- [x] 离线检测（每 60s 扫描 last_seen 超时的节点）
- [x] 数据保留定时任务（每小时清理过期数据）

### 3.4 后端 — Telegram 通知

**新建文件：** `server/internal/probe/telegram.go`

- [x] Telegram Bot API 集成
- [x] 告警消息格式（🔴 链路异常 + 链路/类型/当前值/时间）
- [x] 恢复消息格式（🟢 链路恢复 + 持续时间）
- [x] 空配置时禁用通知

### 3.5 后端 — Probe API

**新建文件：** `server/internal/handler/probe_api.go`

- [x] `POST /api/v1/probe/report` — Agent 指标上报（Bearer Token 鉴权）
- [x] `GET /api/v1/probe/nodes` — 获取 probe 节点列表（含 role）
- [x] `GET /api/v1/probe/links` — 获取所有链路 + 最新指标 + 状态
- [x] `GET /api/v1/probe/links/:id/metrics` — 链路历史指标（?from=&to=）
- [x] `GET /api/v1/probe/alerts` — 告警列表（?link_id=&type=&status=）
- [x] `PUT /api/v1/probe/nodes/:id` — 更新节点位置（canvas_x, canvas_y）和 role
- [x] `POST /api/v1/probe/links` — 创建链路
- [x] `DELETE /api/v1/probe/links/:id` — 删除链路

### 3.6 后端 — Probe Config 下发

**修改文件：** `server/internal/handler/agent_ws.go`

- [x] Agent 注册完成后，推送 `probe_config` 消息
- [x] 拓扑变更时（链路创建/删除），重新推送配置到相关 Agent

### 3.7 Agent — Probe 模块

**新建文件：** `agent/internal/probe/`

- [x] `scheduler.go` — 探测调度器（30s 周期/5min 带宽测试）
- [x] `ping.go` — ICMP Ping 探测（latency min/avg/max + packet loss）
- [x] `tcp.go` — TCP 握手计时
- [x] `bandwidth.go` — HTTP 下载带宽测试
- [x] `reporter.go` — HTTP POST 上报指标到 Server
- [x] `fileserver.go` — 带宽测试文件服务（`serve-testfile` 子命令）

**修改文件：** `agent/internal/client/ws.go`

- [x] 处理 `probe_config` 消息 → 启动/更新 Probe 调度器

**修改文件：** `agent/cmd/agent/main.go`

- [x] 添加 `serve-testfile` 子命令（`--port`, `--size` 参数）

### 3.8 前端 — Monitor Store

**新建文件：** `web/src/stores/monitor.ts`

- [x] 节点列表（含位置、role、状态）
- [x] 链路列表（含最新指标、状态颜色）
- [x] 告警列表
- [x] `fetchNodes()`, `fetchLinks()`, `fetchAlerts()`
- [x] `updateNodePosition(id, x, y)` — 保存拖拽位置
- [x] `createLink(sourceId, targetId)`, `deleteLink(id)`

### 3.9 前端 — API Service 扩展

**修改文件：** `web/src/services/api.ts`

- [x] 添加 probe 相关 API 调用函数

### 3.10 前端 — Monitor 页面

**新建文件：** `web/src/views/MonitorView.vue`

- [x] 顶部统计卡片（总链路数、正常、告警、异常）
- [x] Canvas 拓扑画布区域
- [x] 底部最近 10 条告警列表

**新建文件：** `web/src/components/monitor/TopologyCanvas.vue`

- [x] Canvas 渲染（使用 HTML5 Canvas 或 SVG）
- [x] 节点拖拽 → 保存位置
- [x] 从节点边缘拖拽 → 创建链路
- [x] 鼠标滚轮 → 缩放
- [x] 拖拽空白区域 → 平移
- [x] 右键菜单（节点：编辑 role / 链路：查看详情、删除）

**新建文件：** `web/src/components/monitor/NodeCard.vue`

- [x] 状态灯（绿/黄/红）
- [x] 节点名称
- [x] Role 标签（entry / relay / landing）

**新建文件：** `web/src/components/monitor/LinkLine.vue`

- [x] Bezier 曲线 + 箭头
- [x] 颜色编码（绿=正常、橙=告警、红=异常、灰=无数据）
- [x] 中间标签（延迟 | 丢包率）
- [x] Hover tooltip（全部 4 项指标）

**新建文件：** `web/src/components/monitor/MetricChart.vue`

- [x] 基于 Chart.js 或类似库的图表组件
- [x] 支持 Line / Area / Bar 类型

### 3.11 前端 — Link Detail 页面

**新建文件：** `web/src/views/LinkDetailView.vue`

- [x] 返回按钮 + 链路标题
- [x] 时间范围选择器（1h, 6h, 24h, 7d）
- [x] 延迟趋势图（Line chart, min/avg/max）
- [x] 丢包率趋势图（Area chart）
- [x] TCP 握手时间图（Line chart）
- [x] 带宽趋势图（Bar chart）
- [x] 告警历史表格

### 3.12 前端 — Alerts 页面

**新建文件：** `web/src/views/AlertsView.vue`

- [x] 过滤器（链路、类型、状态）
- [x] 告警表格（时间、链路、类型、状态、持续时间）
- [x] 点击行 → 跳转链路详情

### 3.13 前端 — 路由 + 导航更新

**修改文件：** `web/src/router/index.ts`

- [x] 添加 `/monitor` → MonitorView
- [x] 添加 `/monitor/:linkId` → LinkDetailView
- [x] 添加 `/alerts` → AlertsView

**修改文件：** `web/src/components/AppSidebar.vue`

- [x] 添加 "Monitor" 导航项
- [x] 添加 "Alerts" 导航项

---

## Phase X：UI 增强（低优先级）

> 非核心功能，可在主要 Phase 完成后逐步迭代。

- [x] Light 主题支持 + `useTheme.ts` composable
- [x] 审计日志行展开查看完整 stdout/stderr
- [x] Settings Store 独立化（`stores/settings.ts`）
- [x] Toast 通知系统

---

## 依赖关系

```
Phase 1（Chat Loop）无外部阻塞依赖，可直接开始
  ├── 1.1 LLM Client        ─┐
  ├── 1.2 Security Checker   │
  ├── 1.3 Prompt Builder     ├─→ 1.6 Loop Runner ─→ 1.8 Chat WS Handler
  ├── 1.4 Event Types        │
  └── 1.5 Tool Executor     ─┘

  ├── 1.9 WS Service (前端)  ─┐
  └── 1.10 Chat Store        ├─→ 1.11 Chat 组件 + ChatView
                              │

Phase 2（External API）依赖 Phase 1 的 Tool Executor
  ├── 2.1 API Key 管理       ─→ 2.2 External API 端点
  ├── 2.3 LLM 验证           （独立）
  ├── 2.4 命令历史             （独立）
  └── 2.5-2.6 前端            （依赖后端端点）

Phase 3（NodeProbe）大部分独立于 Phase 1/2
  ├── 3.1 Config              ─┐
  ├── 3.2 Store               ├─→ 3.3 Alert Engine ─→ 3.4 Telegram
  └── 3.5 API                ─┘
  ├── 3.6 Config 下发          （依赖 Agent WS）
  ├── 3.7 Agent Probe         （依赖 3.6）
  └── 3.8-3.13 前端            （依赖 3.5 API）
```

---

## 验证计划

### Phase 1 验证
1. 启动 Server + Agent，确认 Agent 注册成功
2. 前端打开 Chat 页面，发送消息，验证：
   - AI reasoning 流式显示
   - AI content 流式 Markdown 渲染
   - tool_call 卡片正确显示（执行中 → 成功/失败）
   - 敏感命令弹出确认卡片
   - 确认/拒绝后流程正确继续/终止
3. 多 tab 打开验证 session_replaced 处理
4. 断连重连验证

### Phase 2 验证
1. 创建 API Key，验证返回明文 key
2. 用 curl 调用 `/api/v1/nodes`、`/api/v1/nodes/:id/execute` 验证权限控制
3. readonly key 执行命令 → 403
4. standard key 执行敏感命令不带 confirm → 返回 SensitiveOperationError
5. admin key 执行敏感命令 → 直接执行

### Phase 3 验证
1. Monitor 页面显示节点拓扑
2. 拖拽创建链路，验证 probe_config 推送到 Agent
3. Agent 开始探测，指标上报到 Server
4. 指标超阈值 → 告警创建 + Telegram 通知
5. 指标恢复 → 告警关闭 + Telegram 通知
6. Link Detail 页面图表正确显示历史数据
