# ToLaTo MVP 后端 Sprint 开发计划

## 1. 文档定位

本文档基于 [docs/backend_implementation_plan.md](/Users/wentx/momaek/src/tolato/docs/backend_implementation_plan.md) 进一步拆成按 Sprint 组织的开发计划，用于追踪后端当前已完成项与待完成项。

状态约定：

- `[x]` 已完成
- `[ ]` 未完成
- 只有当该项对应的代码、接口、验证场景和必要文档都已落地后，才能勾选

当前仓库状态说明：

- 当前仓库已建立 Sprint 1 所需的后端工程基础骨架、domain 实体、PostgreSQL facts store 与初始 migration
- `infra/store` 已同时提供内存测试实现与 PostgreSQL 事实层实现，后续模块应默认接 PostgreSQL 真源
- Sprint 2 / 3 / 4 已有较多内部模块和测试落地，当前 `tolato-server` 也已接入开发态 `ws/ui` / `ws/agent` 主链路，但尚未形成完整前端联调闭环
- Sprint 5 已开始落 `Nodes / History / Settings` 查询与配置面的 service、`ginhttp` handler 与最小 `tolato-server` HTTP 启动入口，但仍未形成完整产品级联调闭环

前后端联动约束：

- 后端 Sprint 交付不能只看内部模块完成度，还要看是否满足当前前端页面与 Store 的消费方式
- `Console` 相关交互以 `ws/ui` 为准，`Nodes / History / Settings` 相关交互以 HTTP 为准
- 每个 Sprint 完成后，都要回看 [docs/frontend_sprint_plan.md](/Users/wentx/momaek/src/tolato/docs/frontend_sprint_plan.md) 中对应页面是否已具备真实后端接入条件

---

## 2. Sprint 总览

- [x] Sprint 0：文档基线与实施拆解收口
- [x] Sprint 1：基础事实层与仓储骨架
- [ ] Sprint 2：`ws/ui` 与 Console 基础实时链路
- [ ] Sprint 3：目标确认、审批与执行主链路
- [ ] Sprint 4：恢复、幂等与后台执行收口
- [ ] Sprint 5：`Nodes / History / Settings` 页面查询与设置能力

---

## 3. Sprint 0：文档基线与实施拆解收口

Sprint 目标：

- 收口后端架构真源、实施模块拆解与 Sprint 追踪文档，避免后续开发继续参照旧稿或口头约定

模块映射：

- 实施总纲
- 里程碑定义
- 验收矩阵基线

交付清单：

- [x] 旧 [docs/backend_architecture.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture.md) 已降级为归档入口，不再承载实现基线
- [x] [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md) 已作为后端最终架构真源
- [x] [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md) 已作为 session 专项真源
- [x] [docs/api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md) 已作为 HTTP 与 `ws/ui` contract 真源
- [x] [docs/backend_implementation_plan.md](/Users/wentx/momaek/src/tolato/docs/backend_implementation_plan.md) 已完成模块化实施拆解
- [x] [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md) 已增加实施拆解入口链接
- [x] [docs/backend_sprint_plan.md](/Users/wentx/momaek/src/tolato/docs/backend_sprint_plan.md) 已建立 Sprint 勾选追踪面

Sprint 完成标准：

- [x] 后端开发不再依赖旧架构正文
- [x] 实施顺序、模块边界、Sprint 跟踪入口都已明确

---

## 4. Sprint 1：基础事实层与仓储骨架

Sprint 目标：

- 先建立所有后端事实数据和最小基础设施，让后续 `ws/ui`、runtime、execution 都有稳定数据底座

模块映射：

- 模块 1：基础数据与仓储
- 模块 10：横切基础设施中的 `infra/store`、基础 `Clock`、`IDGenerator`、`Logger`、配置加载

交付清单：

- [x] 定义 `Session`、`PendingAction`、`ActiveTargetContext`、`Task`、`Execution`、`TimelineRow` 等核心 domain 实体
- [x] 明确 `sessions`、`thread_messages`、`timeline_rows`、`tool_calls`、`tool_results`、`tasks`、`executions`、`audits`、`settings` 的事实边界
- [x] 为 `sessions` 固定核心运行字段：`revision`、`pending_action`、`current_task_id`、`current_execution_group_id`、`last_agent_state`
- [x] 建立 `ThreadMessageRepository`
- [x] 建立 `SessionRepository`
- [x] 建立 `TimelineRepository`
- [x] 建立 `TaskRepository`
- [x] 建立 `ExecutionRepository`
- [x] 建立 `AuditRepository`
- [x] 建立 settings 读写仓储
- [x] 建立 `tool_call` / `tool_result` 事实写入与恢复读取抽象
- [x] 提供统一时间、ID、日志与基础配置抽象

Sprint 完成标准：

- [x] repository 已能支撑 session snapshot 所需最小读写
- [x] repository 已能支撑 timeline 追加、task/execution 状态更新、audit 写入
- [x] repository 已能支撑 runtime 持久化 `tool_call` / `tool_result` 并供恢复路径读取
- [x] 不在 repository 内编排业务状态机

---

## 5. Sprint 2：`ws/ui` 与 Console 基础实时链路

Sprint 目标：

- 打通 `Console` 的最小可用链路，让前端能连接、列 session、拉 snapshot、发消息，并得到基础 timeline 输出

模块映射：

- 模块 2：`ws/ui`
- 模块 4：Runtime 基础 loop
- 模块 5：Policy 与 Tool 编排中的 `list_nodes`、`resolve_target_nodes`、`request_target_confirmation`、`propose_plan`

交付清单：

- [x] 建立 `ws/ui` 连接管理、鉴权与消息分发骨架
- [x] 建立 `connection.ready`
- [x] 建立 `sessions.list.request` / `response`
- [x] 建立 `session.snapshot.request` / `response`
- [x] 建立 `session.rows.request` 历史分页链路
- [x] 建立 active session 与 watched sessions 订阅模型
- [x] 建立 `subscriptions.update`
- [x] 建立 `session.message.submit` 输入链路
- [x] 实现 `Runtime.HandleUserMessage` 的基础 loop
- [x] 接入 `LLMClient` 抽象与最小 provider adapter
- [x] 接入 `ToolRegistry` 抽象
- [x] 实现 `list_nodes`
- [x] 实现 `resolve_target_nodes`
- [x] 实现 `request_target_confirmation`
- [x] 实现 `propose_plan`
- [x] 开发态已透传模型流事件：stream 走 `llm.sse.event(rawEvent)`，完成态走 `llm.response.completed(rawResponse)`
- [x] 追加 `assistant_text`、`target_confirmation`、`plan`、`tool_call_meta`、`tool_result_meta` rows
- [x] 发布 `timeline.row.appended`、`thread.target.pending`、`thread.target.confirmed`、`session.state.updated`
- [x] 发布 `thread.target.cleared`
- [x] 发布 `llm.sse.event`
- [x] 发布 watch session 摘要事件：`session.summary.updated`
- [ ] 发布 watch session 摘要事件：`session.requires_attention`、`session.unread.updated`

前端交互交付：

- [ ] `useConnectionStore` 能通过真实后端收到 `connection.ready`
- [ ] `useConsoleSessionListStore` 能通过真实后端获取 `sessions.list.response`
- [ ] `useConsoleSessionViewStore` 能通过真实后端获取 `session.snapshot.response`
- [ ] 前端切换 active session 后，后端能正确处理 `subscriptions.update`
- [ ] 前端可通过真实后端消费原始 `thinking` / `content` stream
- [ ] timeline renderer 能消费真实的 `assistant_text`、`tool_call_meta`、`tool_result_meta`、`target_confirmation`、`plan`

Sprint 完成标准：

- [ ] UI 可建立 `ws/ui` 连接并获取 session 列表
- [ ] 切换 session 时可通过 snapshot 恢复完整主视图
- [ ] 用户发出只读消息后，后端可完成一轮或多轮 tool 调用并输出 timeline
- [ ] 目标不明确时，后端会进入 target confirmation 而不是直接执行
- [ ] 前端 session 列表可通过 watch 事件增量刷新，而不是依赖整页重载
- [ ] 前端可实时看到原始 `thinking` 与 `content` stream，而不是只看到最终 `assistant_text`

---

## 6. Sprint 3：目标确认、审批与执行主链路

Sprint 目标：

- 打通从目标确认到 approval，再到 fanout 执行和总结的主业务链路

模块映射：

- 模块 3：`ws/agent`
- 模块 5：补齐 `request_approval`、`exec_on_nodes`、`summarize_execution`
- 模块 6：Approval 与用户动作恢复
- 模块 7：Execution 生命周期

交付清单：

- [x] 建立 `ws/agent` 注册与 heartbeat 链路
- [x] 建立 agent 在线注册表与可下发目标映射
- [x] 定义 allowlist action dispatch 协议
- [x] 实现 `request_approval`
- [x] 实现 `session.target.confirm`
- [x] 实现 `session.approval.approve`
- [x] 实现 `session.approval.reject`
- [x] 实现 `session.operation.cancel`
- [x] 用户动作写入 `tool_result_meta`，且不生成新的 `user_message`
- [x] 按风险策略接入 `low` / `medium` / `high` / `forbidden`
- [x] 实现 `exec_on_nodes`
- [x] 创建 task 与 executions
- [x] 将 execution fanout 到多个 Node Agent
- [x] 接收 `execution.chunk`
- [x] 接收 `execution.finished`
- [x] 聚合 task status、approval status 与 execution aggregate
- [x] 在全部 execution 终态后调用 `summarize_execution`
- [x] 追加 `approval`、`execution`、`summary` rows

前端交互交付：

- [ ] `approval row` 能驱动前端展示审批原因、风险、影响与目标节点
- [ ] `Approve / Reject / Cancel` 按钮可通过真实后端完成闭环
- [ ] `execution.chunk` 能驱动前端 stdout / stderr tail
- [ ] `execution.finished` 能驱动前端节点级状态收口
- [ ] `summary row` 能驱动前端展示 `total / success / failed / skipped` 与 AI 总结文本

Sprint 完成标准：

- [ ] `medium` / `high` 风险进入 approval
- [ ] approve 后可继续执行，reject / cancel 后不会错误推进 loop
- [ ] 多节点执行能得到 `success`、`failed`、`partial_failed`、`timeout` 聚合结果
- [ ] execution 结束后 session 能继续总结并收口
- [ ] 前端可以完整演示 `输入 -> 确认 -> 计划 -> 审批 -> 执行 -> 总结`

---

## 7. Sprint 4：恢复、幂等与后台执行收口

Sprint 目标：

- 把并发、重复点击、断线重连、服务重启等真实运行问题补齐，确保状态机可恢复

模块映射：

- 模块 8：恢复与幂等
- 模块 10：横切基础设施中的 `infra/bus`、`infra/ws` 完整能力
- 模块 4 / 7：补齐 resume 与 async execution 恢复逻辑

交付清单：

- [x] 提供 `session_id` 级互斥锁
- [x] 防止同一 session 并发进入多个 loop
- [x] 建立按钮动作幂等键校验
- [x] 建立消息提交幂等与重复保护策略
- [x] 服务启动时扫描 `running`、`paused_wait_target_confirmation`、`paused_wait_approval`、`waiting_async_execution`
- [x] 对异常遗留 `running` session 标记 `failed` 并写恢复 audit
- [x] 对 `waiting_async_execution` 恢复 `current_task_id` / `current_execution_group_id` 绑定
- [x] 对未完成 execution 的状态进行补查或等待回传
- [x] 让 UI 断线重连后重新请求 snapshot 能恢复页面
- [x] 后台 session 只推送 `summary` 级事件，不灌完整 timeline
- [x] 返回明确的 `session_busy`、状态冲突或幂等重复错误

前端交互交付：

- [x] 前端断线重连后，重新发 `sessions.list.request` 和 `session.snapshot.request` 能恢复页面
- [x] 当前 active session 只收到 timeline 级事件，watch session 只收到 summary 级事件
- [x] 前端能消费 `session_busy`、状态冲突、幂等重复等错误码并正确禁用/恢复交互
- [ ] 前端断线重连后，仍能从 snapshot 恢复当前未完成的 `thinking` / `content` stream 缓冲

Sprint 完成标准：

- [x] 同一 session 并发提交两条消息，第二条会被拒绝
- [x] 用户重复点击 approve / reject / cancel，不会多次推进状态机
- [x] Control Server 重启后，`paused_wait_*` 与 `waiting_async_execution` 状态可恢复
- [x] Control Server 重启后，异常遗留 `running` session 会失败收口并写恢复审计
- [x] 当前 session 与后台 session 的事件粒度符合 [docs/session_interaction.md](/Users/wentx/momaek/src/tolato/docs/session_interaction.md)
- [ ] 前端重连与错误态处理可用真实后端联调验证

---

## 8. Sprint 5：`Nodes / History / Settings` 页面查询与设置能力

Sprint 目标：

- 补齐 `Console` 之外的独立 HTTP 查询与配置面，完成 MVP 四个一级页面的后端面

模块映射：

- 模块 9：HTTP 查询与配置面
- 模块 10：横切基础设施中的 HTTP handler、错误映射、配置读写支撑

交付清单：

- [x] 建立 `ginhttp` 路由与 handler 分层
- [x] 实现 `/api/v1/nodes`
- [x] 实现 `/api/v1/nodes/{id}`
- [x] 实现 `app/nodeview` 节点投影服务
- [x] 实现 `/api/v1/history/tasks`
- [x] 实现 `/api/v1/history/tasks/{id}`
- [x] 实现 `app/history` 历史聚合服务
- [ ] 让 History detail 返回 plan、approval、execution、`tool_call_meta`、`tool_result_meta`、audit
- [x] 实现 `/api/v1/settings/model-config`
- [x] 实现 `/api/v1/settings/model-config/test`
- [x] 实现 `/api/v1/settings/password/change`
- [x] 实现 `/api/v1/settings/sessions/revoke-others`
- [x] 实现 `/api/v1/settings/preferences`
- [x] 实现 `/api/v1/settings/account-security`
- [x] 实现 `app/settings` 配置与账户安全服务

前端交互交付：

- [ ] `NodesPage` 可通过真实 `/api/v1/nodes` 完成搜索、筛选、列表展示
- [ ] `NodeDetailPage` 可通过真实 `/api/v1/nodes/{id}` 完成详情展示
- [x] `HistoryPage` 可通过真实 `/api/v1/history/tasks` 与 `/api/v1/history/tasks/{id}` 完成列表和详情展示
- [x] `SettingsPage` 可通过真实 `/api/v1/settings/*` 完成读取、保存和测试连接
- [ ] 前端从 `Node Detail / History` 返回 `Console` 的跳转所需字段已由后端返回

Sprint 完成标准：

- [ ] `/nodes` 可提供搜索、筛选、`busy/idle` 与资源摘要
- [ ] `/nodes/{id}` 可提供最近心跳与最近任务
- [ ] `/history/tasks` 可按状态和审批状态筛选
- [ ] `/history/tasks/{id}` 可查看任务闭环与关联审计
- [x] `/settings/*` 可独立工作且不依赖 `ws/ui`
- [ ] `Nodes / History / Settings` 三个页面可与真实后端完成联调

---

## 9. 整体验收勾选板

- [ ] 单节点只读消息，无审批，loop 正常结束
- [ ] 目标待确认后，用户确认，plan 继续推进
- [ ] `medium` / `high` 风险进入 approval，approve 后执行，reject 后结束
- [ ] 多节点 fanout，部分失败，task 聚合为 `partial_failed`
- [ ] 同一 session 并发提交两条消息，第二条被拒为 `session_busy`
- [ ] 用户重复点击 approve / reject，不重复推进状态机
- [ ] UI 断线重连后，`session.snapshot` 能恢复页面
- [ ] 控制服务重启后，`paused_wait_*` 与 `waiting_async_execution` 可恢复
- [ ] `/nodes`、`/history/tasks`、`/settings` 各自只依赖 HTTP 查询 / 配置面，不经 `ws/ui`
- [ ] 历史详情能同时展示 plan、approval、execution、tool meta、audit
- [ ] 前端 Sprint 6 所需的真实后端交互面全部可联调

---

## 10. 使用规则

- 每完成一项开发任务并通过对应验证后，再手动把该项从 `[ ]` 改成 `[x]`
- 只有当某个 Sprint 下的交付清单和完成标准都满足时，才能把该 Sprint 在“总览”里勾选
- 如果后续实现顺序调整，应先回写 [docs/backend_implementation_plan.md](/Users/wentx/momaek/src/tolato/docs/backend_implementation_plan.md)，再更新本 Sprint 跟踪稿
