# ToLaTo MVP 后端实施模块拆解

## 1. 文档定位

本文档用于把现有后端文档收敛成可直接开发的实施拆解，不负责改写产品需求、交互语义或协议正文。

本文档的基线来源如下：

- [docs/prd.md](/Users/wentx/momaek/src/tolato/docs/prd.md)
  - 负责产品范围、页面边界与 MVP 验收口径
- [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md)
  - 负责后端最终架构、模块边界与运行模型
- [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md)
  - 负责 session、snapshot、多订阅与恢复语义
- [docs/api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md)
  - 负责 HTTP 与 `ws/ui` contract

本文档默认采用以下实现约束：

- `Console` 会话主通道只走 `ws/ui`
- `Nodes / History / Settings` 只走 HTTP 查询与配置接口
- `high` 风险在首版实现中仍走 approval，不直接阻断
- `Direct shell` 只保留模式占位，不提供真实执行能力
- 仓库当前仍以文档为主，以下模块按未来 Go 后端目录结构规划

本文档不展开：

- SQL DDL 与 migration 细节
- OpenAPI / JSON Schema 完整定义
- 前端 UI 组件与视觉实现
- 部署、CI/CD、IaC

按 Sprint 追踪当前已完成项与待完成项，见 [docs/backend_sprint_plan.md](/Users/wentx/momaek/src/tolato/docs/backend_sprint_plan.md)。

---

## 2. 模块总览

后端按实现边界拆成 10 个工程模块，而不是按页面需求拆稿：

1. 基础数据与仓储
2. `ws/ui` 传输层
3. `ws/agent` 传输层
4. Runtime
5. Policy 与 Tool 编排
6. Approval 与用户动作恢复
7. Execution 生命周期
8. 恢复与幂等
9. HTTP 查询与配置面
10. 横切基础设施

建议目标目录：

- `internal/server/transport/wsui`
- `internal/server/transport/wsagent`
- `internal/server/transport/ginhttp`
- `internal/server/app/runtime`
- `internal/server/app/session`
- `internal/server/app/execution`
- `internal/server/app/nodeview`
- `internal/server/app/history`
- `internal/server/app/settings`
- `internal/server/app/policy`
- `internal/server/domain`
- `internal/server/infra/store`
- `internal/server/infra/bus`
- `internal/server/infra/llm`
- `internal/server/infra/ws`

所有模块统一按以下模板理解和落地：

- 目标
- 职责
- 核心输入输出
- 依赖
- 关键实体 / 接口
- 非目标
- 完成标准

---

## 3. 详细模块拆解

### 3.1 模块 1：基础数据与仓储

目标：

- 建立后端事实层，保证 session、message、timeline、task、execution、audit、settings 的边界明确

职责：

- 定义核心实体与状态字段
- 提供 repository 最小读写能力
- 为 runtime、snapshot、history、settings 提供统一数据访问面

核心输入输出：

- 输入：session 状态变更、message 追加、tool 调用结果、task/execution 生命周期事件、settings 读写
- 输出：事实表记录、聚合视图原始数据、session 运行字段

依赖：

- `domain`
- `infra/store`

关键实体 / 接口：

- `sessions`
  - 关键字段：`status`、`revision`、`active_target_context`、`pending_action_type`、`pending_action_payload`、`current_task_id`、`current_execution_group_id`、`last_agent_state`
- `thread_messages`
- `timeline_rows`
- `tasks`
- `executions`
- `audits`
- `settings`
- `SessionRepository`
- `TimelineRepository`
- `TaskRepository`
- `ExecutionRepository`
- `AuditRepository`

非目标：

- 不在 repository 内实现业务状态机
- 不在数据库层直接做 snapshot 拼装语义

完成标准：

- 能支持 session snapshot、timeline 追加、task/execution 聚合、settings 持久化
- session 核心运行字段已稳定，可支撑暂停、恢复、重启扫描

### 3.2 模块 2：`ws/ui` 传输层

目标：

- 提供 `Console` 的唯一实时入口，承载 request/response 和增量事件

职责：

- 管理 UI 连接、active session 与 watched sessions
- 分发 `sessions.list`、`session.snapshot`、`session.message.submit`
- 分发原始 LLM 流式事件到 UI
- 接收 `session.target.confirm`、`session.approval.approve`、`session.approval.reject`、`session.operation.cancel`
- 将 runtime / session / execution 事件推送给正确连接

核心输入输出：

- 输入：浏览器 WebSocket 消息、app service 返回结果、事件 publisher 推送
- 输出：`sessions.list.response`、`session.snapshot.response`、`session.action.accepted`、`llm.sse.event` 与增量事件

依赖：

- `app/runtime`
- `app/session`
- `app/execution`
- `infra/ws`

关键实体 / 接口：

- `UIEventPublisher`
- `ClientRegistry`
- `subscriptions.update` 内部模型

非目标：

- 不直接改数据库
- 不直接调用模型
- 不承载 `Nodes / History / Settings` 页面取数

完成标准：

- 一个连接可同时维护 1 个 active session 与多个 watched sessions
- 切换 session 能通过 snapshot 恢复整页视图
- 后台 session 只收到 summary 级事件
- active session 可实时收到 OpenAI 原始 SSE 事件并渲染 `thinking` / `content` stream

### 3.3 模块 3：`ws/agent` 传输层

目标：

- 提供 Control Server 与 Node Agent 的实时通信链路

职责：

- agent 注册与鉴权
- heartbeat 与在线状态维护
- task dispatch 下发
- `execution.chunk`、`execution.finished` 回传

核心输入输出：

- 输入：agent 连接、heartbeat、执行输出分片、执行完成通知
- 输出：在线 agent 注册表、dispatch 消息、execution 生命周期事件

依赖：

- `app/execution`
- `infra/ws`
- `infra/bus`

关键实体 / 接口：

- `AgentRegistry`
- dispatch message
- execution callback message

非目标：

- agent 不负责规划、审批、总结
- `ws/agent` 不修改 session 语义

完成标准：

- server 能识别 agent 上下线
- dispatch 后能收到 chunk / finished 回传
- agent 侧只暴露 allowlist action

### 3.4 模块 4：Runtime

目标：

- 落地显式 Agent Loop，负责会话驱动、暂停、恢复和审计闭环

职责：

- 实现 `HandleUserMessage`
- 读取 session / message / tool 历史 / target context
- 调用 `LLMClient`
- 消费 provider 原始 SSE 流并透传到 `UIEventPublisher`
- 执行 Tool 或消费 Tool 结果
- 写入 `thread_messages`、`timeline_rows`、`tool_call`、`tool_result`、`audits`
- 在 `paused_wait_target_confirmation`、`paused_wait_approval`、`waiting_async_execution` 上暂停
- 在用户动作或 execution 完成后恢复 loop

核心输入输出：

- 输入：用户消息、恢复事件、execution 完成事件、provider continuation state
- 输出：assistant 文本、tool 调用、副作用写库、事件发布

依赖：

- repositories
- `ToolRegistry`
- `LLMClient`
- `UIEventPublisher`
- `LockManager`

关键实体 / 接口：

- `Runtime`
- `LLMClient`
- `ToolRegistry`
- `LockManager`

非目标：

- 不感知 Gin 或 WebSocket 协议对象
- 不直接操作连接对象

完成标准：

- 支持多轮 loop
- 支持等待用户动作再恢复
- 支持 execution 完成后继续总结并结束 session
- 支持在最终 row 落库前，把原始 `thinking` / `content` SSE 事件实时透传前端

### 3.5 模块 5：Policy 与 Tool 编排

目标：

- 定义模型可见的受控工具面，并把风险策略收口成可执行规则

职责：

- 提供 `list_nodes`
- 提供 `resolve_target_nodes`
- 提供 `request_target_confirmation`
- 提供 `propose_plan`
- 提供 `request_approval`
- 提供 `exec_on_nodes`
- 提供 `summarize_execution`
- 根据风险等级决定自动执行、approval 或阻断

核心输入输出：

- 输入：用户输入、节点清单、session target context、task / execution 聚合结果
- 输出：结构化 ToolResult、pending action、plan row、approval row、execution row、summary row

依赖：

- `app/policy`
- `app/execution`
- `app/nodeview` 或节点查询接口

关键实体 / 接口：

- `ToolRegistry`
- `ToolResult`
- 风险分级规则：`low`、`medium`、`high`、`forbidden`

非目标：

- 不暴露任意 shell tool
- 不让模型越过确认与审批边界

完成标准：

- 目标不明确时必须走 target confirmation
- `medium` / `high` 进入 approval
- `forbidden` 不生成 approval，不下发 Node Agent
- 普通 tool 调用生成 `tool_call_meta` / `tool_result_meta`

### 3.6 模块 6：Approval 与用户动作恢复

目标：

- 把 UI 按钮动作接成确定性的后端状态推进，而不是新的自然语言输入

职责：

- 实现 `session.target.confirm`
- 实现 `session.approval.approve`
- 实现 `session.approval.reject`
- 实现 `session.operation.cancel`
- 校验 session 当前状态、pending action、task 绑定与幂等键
- 追加 `tool_result_meta`
- 清理挂起状态并唤起 runtime 恢复

核心输入输出：

- 输入：按钮动作、`sessionId`、`taskId`、`idempotencyKey`
- 输出：状态推进、审计记录、resume 事件

依赖：

- `app/runtime`
- `app/session`
- repositories
- `LockManager`

关键实体 / 接口：

- `PendingActionType`
- idempotency 记录

非目标：

- 不生成新的 `user_message`
- 不允许绕开 session 状态校验直接推进 task

完成标准：

- 重复点击 approve / reject / cancel 幂等
- 错误状态下返回明确拒绝
- 正确生成用户动作对应的 `tool_result_meta`

### 3.7 模块 7：Execution 生命周期

目标：

- 将远端执行建模为独立于 session 的异步生命周期，并与 runtime 正确衔接

职责：

- 创建 task 与 executions
- fanout 下发到多个 Node Agent
- 聚合 chunk 与 finished
- 计算 task aggregate 与最终状态
- 在全部完成后触发 summary

核心输入输出：

- 输入：`exec_on_nodes` 下发请求、agent chunk、agent finished
- 输出：execution row、execution 明细、task aggregate、summary 触发条件

依赖：

- `ws/agent`
- repositories
- `UIEventPublisher`

关键实体 / 接口：

- `TaskStatus`
- `ExecutionStatus`
- execution aggregate

非目标：

- 不把 stdout / stderr 直接塞进 session 主体
- 不用 session 状态代替 task / execution 事实状态

完成标准：

- `session.status` 与 `task/execution.status` 解耦
- 多节点执行能聚合出 `success`、`failed`、`partial_failed`、`timeout`
- 所有 execution 终态后能继续 `summarize_execution`

### 3.8 模块 8：恢复与幂等

目标：

- 保证 session 状态机在并发、重复操作、断线与服务重启场景下仍然可恢复

职责：

- 提供 `session_id` 级互斥锁
- 防止同一 session 并发进入 loop
- 防止按钮重复点击重复推进
- 服务启动时扫描 `running`、`paused_wait_*`、`waiting_async_execution`
- 重建等待中的 session 与 execution 绑定关系

核心输入输出：

- 输入：锁请求、重启扫描、重复动作、未完成 task/execution
- 输出：busy 拒绝、幂等吞吐、恢复调度

依赖：

- `infra/bus`
- repositories
- `app/runtime`
- `app/execution`

关键实体 / 接口：

- `LockManager`
- session recovery scanner

非目标：

- 不在此模块定义新的业务流程
- 不主动篡改已完成 task 审计

完成标准：

- 同一 session 并发两条消息时，第二条返回 `session_busy`
- `paused_wait_*` 与 `waiting_async_execution` 在重启后可继续工作
- 重复动作不会多次推进 loop

### 3.9 模块 9：HTTP 查询与配置面

目标：

- 为 `Nodes / History / Settings` 提供独立于 `Console` 的查询与配置能力

职责：

- `app/nodeview`
  - 支撑 `/api/v1/nodes`、`/api/v1/nodes/{id}`
- `app/history`
  - 支撑 `/api/v1/history/tasks`、`/api/v1/history/tasks/{id}`
- `app/settings`
  - 支撑 `/api/v1/settings/model-config`
  - 支撑 `/api/v1/settings/password/change`
  - 支撑 `/api/v1/settings/sessions/revoke-others`
  - 支撑 `/api/v1/settings/preferences`
- `ginhttp` 只做参数绑定、鉴权、错误映射

核心输入输出：

- 输入：HTTP query、path params、settings payload
- 输出：节点投影、任务历史投影、设置读写结果

依赖：

- `app/nodeview`
- `app/history`
- `app/settings`
- repositories

关键实体 / 接口：

- `NodeViewService`
- `HistoryService`
- `SettingsService`

非目标：

- 不通过 `ws/ui` 取数
- 不触发 runtime loop、tool 调用、execution dispatch

完成标准：

- `/nodes`、`/history/tasks`、`/settings/*` 全部通过 HTTP 可用
- History 详情能展示 plan、approval、execution、tool meta、audit

### 3.10 模块 10：横切基础设施

目标：

- 为其余模块提供稳定的技术底座，同时保持 infra 不承载业务状态机

职责：

- `infra/store`
  - PostgreSQL repositories
- `infra/bus`
  - Redis locks、fanout、dispatch
- `infra/llm`
  - provider adapter 与 continuation state 支持
- `infra/ws`
  - hub、registry、codec、readpump、writepump
- 提供日志、指标、错误码、时钟、ID 生成、配置加载

核心输入输出：

- 输入：底层连接、配置、事件
- 输出：稳定的技术服务接口

依赖：

- 外部中间件与配置

关键实体 / 接口：

- `Clock`
- `IDGenerator`
- `Logger`
- metrics collector

非目标：

- infra 不定义业务语义
- infra 不承担 session 状态推进

完成标准：

- 各 app 模块只依赖抽象接口，不直接依赖具体中间件
- 错误、日志、指标、配置有统一落点

---

## 4. 模块接口与公共类型

本节只锁定实现必须统一的接口与类型，不重复完整 wire schema。

### 4.1 核心接口

- `SessionRepository`
  - 读取 session
  - 更新 session 状态、revision、pending action、target context、current task/execution 绑定
- `TimelineRepository`
  - 追加 row
  - 按 session 读取时间线
- `TaskRepository`
  - 创建 task
  - 更新 task 状态与 aggregate
  - 查询 task detail
- `ExecutionRepository`
  - 创建 execution
  - 写 chunk / finished
  - 聚合同 task execution 状态
- `AuditRepository`
  - 写审计记录
  - 读取 task 关联审计
- `UIEventPublisher`
  - 发布 timeline 事件
  - 发布 session summary 事件
  - 发布 execution 流事件
- `AgentRegistry`
  - 注册 agent
  - 维护在线 agent 与可下发目标
- `LockManager`
  - 获取 / 释放 `session_id` 锁
  - 处理幂等键
- `LLMClient`
  - 发送当前 loop 上下文
  - 返回 assistant 文本或 tool 调用
- `ToolRegistry`
  - 枚举可见 tools
  - 根据 tool name 调用实现

### 4.2 核心公共类型

- `SessionStatus`
  - `idle`
  - `running`
  - `paused_wait_target_confirmation`
  - `paused_wait_approval`
  - `waiting_async_execution`
  - `completed`
  - `failed`
- `PendingActionType`
  - `target_confirmation`
  - `approval`
- `ActiveTargetContext`
  - 以 [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md) 和 [docs/api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md) 为准
- `TimelineRowKind`
  - `user_message`
  - `assistant_text`
  - `target_confirmation`
  - `tool_call_meta`
  - `tool_result_meta`
  - `plan`
  - `approval`
  - `execution`
  - `summary`
- `TaskStatus`
  - `planned`
  - `waiting_approval`
  - `approved`
  - `queued`
  - `dispatched`
  - `running`
  - `success`
  - `failed`
  - `partial_failed`
  - `timeout`
  - `cancelled`
- `ApprovalStatus`
  - `not_required`
  - `pending`
  - `approved`
  - `rejected`
  - `cancelled`
- `ExecutionStatus`
  - `queued`
  - `dispatched`
  - `running`
  - `success`
  - `failed`
  - `timeout`
  - `cancelled`

命名约束：

- 模块命名、接口职责与目录边界必须对齐 [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md)
- 不在实施阶段随意新增平行 service 层或重复 repository

---

## 5. 交付顺序

将架构文档中的 12 步实现顺序展开成 4 个里程碑。

### 5.1 P0：基础事实层

前置依赖：

- 无

必须交付的模块：

- 模块 1：基础数据与仓储
- 模块 10：横切基础设施中的 store、clock、ID、logger、基础 config

对外可验证能力：

- 能创建与读取 session
- 能写入 message、timeline、task、execution、audit、settings
- 能支撑 session snapshot 所需最小事实读取

禁止提前做的内容：

- 不实现真实 runtime loop
- 不实现 execution fanout
- 不开始 `Nodes / History / Settings` 页面取数

### 5.2 P1：Console 实时链路

前置依赖：

- P0 完成

必须交付的模块：

- 模块 2：`ws/ui`
- 模块 4：Runtime 基础 loop
- 模块 5：Policy 与 Tool 编排中的 `list_nodes`、`resolve_target_nodes`、`request_target_confirmation`、`propose_plan`

对外可验证能力：

- UI 能建立 `ws/ui` 连接
- 能获取 session 列表与 session snapshot
- 能提交用户消息并生成 assistant / plan / target confirmation 相关 rows

禁止提前做的内容：

- 不接真实执行 fanout
- 不接 approval 后继续执行
- 不把 `Nodes / History / Settings` 混进 `ws/ui`

### 5.3 P2：执行与恢复

前置依赖：

- P1 完成

必须交付的模块：

- 模块 3：`ws/agent`
- 模块 6：Approval 与用户动作恢复
- 模块 7：Execution 生命周期
- 模块 8：恢复与幂等
- 模块 5：补齐 `request_approval`、`exec_on_nodes`、`summarize_execution`

对外可验证能力：

- `medium` / `high` 风险进入 approval
- approve 后执行，reject/cancel 后结束或阻断
- 多节点 execution 能 fanout、聚合、总结
- 断线重连和服务重启后可恢复 session

禁止提前做的内容：

- 不在此阶段扩成 Direct shell
- 不把 approval 逻辑拆成独立产品入口

### 5.4 P3：页面查询与设置

前置依赖：

- P2 完成

必须交付的模块：

- 模块 9：HTTP 查询与配置面
- 模块 10：补齐 bus、llm、ws 的生产化支撑

对外可验证能力：

- `/nodes`、`/nodes/{id}` 可浏览节点与详情
- `/history/tasks`、`/history/tasks/{id}` 可追溯任务闭环
- `/settings/*` 可读写模型配置、密码、偏好与其他会话管理

禁止提前做的内容：

- 不单独拆审计中心
- 不回退到 `ws/ui` 承担页面查询

---

## 6. 按模块映射的验收矩阵

### 6.1 Console 与 Runtime

- 单节点只读消息，无审批，loop 正常结束
- 目标待确认后，用户确认，plan 继续推进
- `medium` / `high` 风险进入 approval，approve 后执行，reject 后结束

### 6.2 Execution 与 Agent

- 多节点 fanout，部分失败，task 聚合为 `partial_failed`
- agent 回传 chunk 与 finished 后，execution 状态与 aggregate 正确推进
- 历史 task 审计始终绑定 `operation.target_snapshot`，不受后续 target 切换影响

### 6.3 并发、恢复与幂等

- 同一 session 并发提交两条消息，第二条被拒为 `session_busy`
- 用户重复点击 approve / reject，不重复推进状态机
- UI 断线重连后，`session.snapshot` 能恢复页面
- 控制服务重启后，`paused_wait_*` 与 `waiting_async_execution` 可恢复

### 6.4 HTTP 查询与配置面

- `/nodes`、`/history/tasks`、`/settings` 各自只依赖 HTTP 查询 / 配置面，不经 `ws/ui`
- 历史详情能同时展示 plan、approval、execution、tool meta、audit
- 节点详情能展示最近心跳与最近任务

---

## 7. 默认实现假设

- 仓库当前仍是文档仓，实施文档按未来 Go 后端目录结构编写，不要求与当前代码树对齐
- 本轮不补 SQL DDL、OpenAPI 全量定义或前端细节，只写到足够指导后端开发
- 默认按单工程、单团队实施，不拆多团队协作流程
- 以 [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md) 为后端结构真源
- 以 [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md) 和 [docs/api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md) 为交互真源
- 若后续调整 `high` 风险策略、页面边界或 contract，必须先回写上述基线文档，再更新本实施稿
