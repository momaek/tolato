# ToLaTo 前端开发拆分建议

## 1. 结论先行

基于当前项目文档和设计稿，前端不应该按“单页面聊天框”来做，而应该拆成两条数据面：

- `Console`：实时会话面，主要通过 `ws/ui` 承载 session 列表、snapshot、timeline 增量事件、执行流
- `Nodes / Node Detail / History / Settings`：查询与配置面，走 HTTP

同时要明确 3 个基线：

1. `docs/ui_state_description.md` 是交互语义基线
2. `docs/tolato-ui-states.pen` 只能作为视觉参考，不能继续作为交互语义基线
3. `docs/session_interaction.md` 和 `docs/backend_architecture_manual_loop.md` 决定了前端的 WebSocket 模型、session 切换方式和 Store 切分方式

另外，如果使用 `shadcn-vue`，实际上还需要把 `Tailwind CSS` 一起纳入技术栈。建议首版前端基线为：

- Vue 3
- Vite
- TypeScript
- Vue Router
- Pinia
- Tailwind CSS
- shadcn-vue
- markstream-vue
- Zod
- `ofetch` 或 `axios` 二选一

## 2. 文档与设计稿对前端的直接约束

### 2.1 页面边界

来自 `docs/prd.md`、`docs/ui_console_design.md`、设计稿：

- 一级页面固定为 `Console / Nodes / History / Settings`
- `Node Detail` 是独立路由页，不是一级导航
- `Direct shell` 在 MVP 里只是占位模式，不是真终端

### 2.2 Console 的核心交互

来自 `docs/ui_state_description.md`、`docs/session_interaction.md`：

- 主区域是 `row-based timeline`，不是一张不断刷新的大卡片
- `plan / approval / execution / summary` 是结构化 row
- `tool_call_meta / tool_result_meta` 默认可见
- 按钮型确认和审批不新增 `user_message`
- 切换 session 时必须拉 `snapshot` 恢复页面，不能靠事件回放
- 一个 WebSocket 连接需要支持：
  - `active session`
  - `watch sessions`

### 2.3 当前联调风险

来自 `docs/api_contract.md`：

- 当前后端 contract 和前端目标态不一致
- 当前 `ws/ui` 还只是 `welcome` 占位
- 节点、任务、审计接口的包装层和字段名不一致

所以前端必须先做一层 `adapter`，不能直接把接口响应灌进页面组件。

## 3. 建议目录

建议不要把业务都堆在 `views/` 和单一 `stores/` 里，直接按“页面壳 + 领域实体 + 功能模块 + 组合区块”拆。

```text
web/
  src/
    app/
      main.ts
      App.vue
      router/
      providers/
      styles/
    shared/
      api/
      ws/
      config/
      lib/
      ui/
      types/
      mock/
    entities/
      session/
      timeline/
      task/
      node/
      settings/
    features/
      console-session-list/
      console-session-switch/
      console-target-confirm/
      console-composer/
      console-plan-preview/
      console-approval/
      console-execution-stream/
      console-summary/
      nodes-filter/
      history-filter/
      settings-model-config/
    widgets/
      app-shell/
      console-header/
      console-sidebar/
      console-timeline/
      console-composer/
      nodes-table/
      node-overview/
      history-task-list/
      history-task-detail/
      settings-panels/
    pages/
      console/
      nodes/
      node-detail/
      history/
      settings/
```

## 4. 建议路由

```text
/console
/console/:sessionId
/nodes
/nodes/:id
/history
/history/:taskId
/settings
```

约束：

- 默认进入 `/console`
- `Console` 内部通过 `sessionId` 决定 active session
- `History/:taskId` 建议用左右分栏或详情面板模式
- `Node Detail` 独立承接完整节点信息，不要继续塞回 Console 左栏

## 5. Pinia Store 切分

严格按 `docs/session_interaction.md` 的建议来，不要做一个“全局大 store”。

### 5.1 必要 Store

- `useAppStore`
  - 全局启动状态、当前用户、主题、全局错误提示
- `useConnectionStore`
  - `ws/ui` 连接状态、最近同步时间、重连状态
- `useConsoleSessionListStore`
  - 左侧 session 列表、未读、摘要、状态
- `useConsoleSessionViewStore`
  - 按 `sessionId` 缓存 snapshot、rows、revision、cursor、pending action
- `useNodesStore`
  - 节点列表、筛选项、分页、节点摘要指标
- `useNodeDetailStore`
  - 单节点详情、最近任务、风险提示
- `useHistoryStore`
  - 任务列表、筛选条件、任务详情
- `useSettingsStore`
  - 模型配置、安全设置、用户偏好

### 5.2 Console Store 关键规则

- `session list` 和 `session view` 必须分开
- snapshot 覆盖前必须比较 `revision`
- 后台 session 的摘要事件只更新列表，不更新主时间线
- 活跃 session 才消费完整 timeline / execution 事件

## 6. 组件分层原则

### 6.1 `shadcn-vue` 负责什么

- Button
- Input
- Tabs
- Dialog
- Sheet / Drawer
- Scroll Area
- Badge
- Dropdown Menu
- Tooltip
- Toast

### 6.2 业务组件自己写什么

以下不要直接用 markdown 或基础卡片糊出来，应该写成独立业务组件：

- `TimelineRowRenderer`
- `TargetConfirmationRow`
- `PlanPreviewRow`
- `ApprovalRow`
- `ExecutionRow`
- `SummaryRow`
- `SessionListItem`
- `NodeStatusCard`
- `TaskHistoryDetailPanel`

### 6.3 `markstream-vue` 的正确使用边界

建议只用于：

- `assistant_text`
- `summary` 里的 AI 解释文本
- 可能的设置说明 / 帮助文案

不建议用于：

- `plan`
- `approval`
- `execution`
- `tool_call_meta`
- `tool_result_meta`

原因：

- 这些 row 都是结构化业务对象，不是普通 markdown 消息
- 如果全部按 markdown 渲染，后面会很难做审批按钮、执行分组、状态更新和审计可视化

## 7. 小功能模块拆分清单

下面这份清单建议直接作为前端实施顺序。

### M00 工程初始化

- [ ] 初始化 `web/` 工程：Vite + Vue 3 + TypeScript
- [ ] 接入 Tailwind CSS 和 `shadcn-vue`
- [ ] 接入 Vue Router、Pinia、markstream-vue
- [ ] 配置 ESLint / Prettier / 基础别名
- [ ] 建立 `src/app`、`src/shared`、`src/entities`、`src/features`、`src/widgets`、`src/pages`

建议文件：

- `web/package.json`
- `web/vite.config.ts`
- `web/tsconfig.json`
- `web/src/main.ts`
- `web/src/app/App.vue`
- `web/src/app/router/index.ts`
- `web/src/app/styles/index.css`

### M01 设计系统和基础样式

- [ ] 把设计稿中的基础视觉元素映射为 Tailwind token
- [ ] 定义颜色、圆角、阴影、间距、字体层级
- [ ] 封装基础状态 badge、卡片容器、顶部状态条样式
- [ ] 接入 logo 资源

建议文件：

- `web/src/app/styles/tokens.css`
- `web/src/shared/ui/status-badge/`
- `web/src/shared/ui/panel-card/`
- `web/src/shared/ui/page-header/`
- `web/public/logo/`

### M02 路由壳和全局布局

- [ ] 搭建全局导航：`Console / Nodes / History / Settings`
- [ ] 搭建页面壳、面包屑、顶部连接状态区域
- [ ] 支持桌面端优先布局
- [ ] 预留移动端折叠策略，但首版先保证桌面体验

建议文件：

- `web/src/widgets/app-shell/AppShell.vue`
- `web/src/widgets/app-shell/AppSidebar.vue`
- `web/src/widgets/app-shell/AppTopbar.vue`
- `web/src/pages/console/ConsolePage.vue`
- `web/src/pages/nodes/NodesPage.vue`
- `web/src/pages/history/HistoryPage.vue`
- `web/src/pages/settings/SettingsPage.vue`

### M03 类型系统与 Contract Adapter

- [ ] 定义前端领域类型：`SessionListItem`、`SessionSnapshot`、`TimelineRow`、`TaskDetail`、`NodeSummary`
- [ ] 为 HTTP 响应做 adapter，吃掉包装层和 snake_case / camelCase 差异
- [ ] 为目标态 `ws/ui` 协议预留独立类型
- [ ] 保留 mock 和真实接口双实现

建议文件：

- `web/src/shared/types/console.ts`
- `web/src/shared/types/node.ts`
- `web/src/shared/types/history.ts`
- `web/src/shared/api/http-client.ts`
- `web/src/shared/api/adapters/`
- `web/src/shared/ws/protocol.ts`

### M04 Mock 数据和本地开发总线

- [ ] 基于文档构造 session snapshot mock
- [ ] 基于文档构造 timeline rows mock
- [ ] 模拟执行流、审批流、session 切换和断线重连
- [ ] 在真实 `ws/ui` 可用前，用 mock 驱动 Console 页面

建议文件：

- `web/src/shared/mock/sessions.ts`
- `web/src/shared/mock/timeline.ts`
- `web/src/shared/mock/nodes.ts`
- `web/src/shared/mock/history.ts`
- `web/src/shared/mock/settings.ts`

### M05 `ws/ui` Client

- [ ] 封装连接、鉴权、重连、心跳、request-response、多订阅
- [ ] 支持 `sessions.list.request`
- [ ] 支持 `session.snapshot.request`
- [ ] 支持 `session.message.submit`
- [ ] 支持 `session.target.confirm`
- [ ] 支持 `session.approval.approve / reject`
- [ ] 支持 `subscriptions.update`

建议文件：

- `web/src/shared/ws/ws-client.ts`
- `web/src/shared/ws/request-map.ts`
- `web/src/shared/ws/reconnect.ts`
- `web/src/shared/ws/event-bus.ts`

### M06 Console 页面骨架

- [ ] 搭建 Console 三块区域：顶部、左栏、主时间线、底部输入区
- [ ] 顶部显示连接状态、目标上下文、模式状态
- [ ] 左栏显示 session 列表、候选节点、已确认目标、节点摘要
- [ ] 主区域预留 timeline 容器

建议文件：

- `web/src/widgets/console-header/ConsoleHeader.vue`
- `web/src/widgets/console-sidebar/ConsoleSidebar.vue`
- `web/src/widgets/console-timeline/ConsoleTimeline.vue`
- `web/src/widgets/console-composer/ConsoleComposer.vue`

### M07 Session 列表与切换

- [ ] 获取 session 列表
- [ ] 切换 active session
- [ ] 切换时先展示 skeleton，再用 snapshot 整包覆盖
- [ ] 处理 watch sessions 的摘要更新

建议文件：

- `web/src/entities/session/model/session-list.store.ts`
- `web/src/entities/session/model/session-view.store.ts`
- `web/src/features/console-session-list/SessionList.vue`
- `web/src/features/console-session-switch/useSessionSwitch.ts`

### M08 Timeline Row 渲染框架

- [ ] 建立统一的 `TimelineRowRenderer`
- [ ] 按 `kind` 分发到不同 row 组件
- [ ] 处理时间线追加、局部更新、滚动到底部、拉取更早 rows
- [ ] 先把静态 row 渲染跑通

建议文件：

- `web/src/entities/timeline/model/timeline.ts`
- `web/src/features/timeline-row-renderer/TimelineRowRenderer.vue`
- `web/src/entities/timeline/ui/rows/UserMessageRow.vue`
- `web/src/entities/timeline/ui/rows/AssistantTextRow.vue`
- `web/src/entities/timeline/ui/rows/ToolCallMetaRow.vue`
- `web/src/entities/timeline/ui/rows/ToolResultMetaRow.vue`

### M09 目标确认模块

- [ ] 渲染 `target_confirmation row`
- [ ] 展示目标来源、候选节点、匹配依据、scope
- [ ] 处理 `确认目标 / 重新选择 / 清除上下文`
- [ ] 点击按钮后只追加 `tool_result_meta`

建议文件：

- `web/src/features/console-target-confirm/TargetConfirmationRow.vue`
- `web/src/features/console-target-confirm/useTargetConfirm.ts`

### M10 计划预览模块

- [ ] 渲染 `plan row`
- [ ] 展示 target、summary、risk、impact、steps
- [ ] 支持 `查看完整计划`
- [ ] 当低风险只读任务自动执行时，正确衔接后续状态

建议文件：

- `web/src/features/console-plan-preview/PlanPreviewRow.vue`
- `web/src/features/console-plan-preview/PlanDetailDialog.vue`

### M11 审批模块

- [ ] 渲染 `approval row`
- [ ] 展示审批原因、风险等级、影响范围、目标节点
- [ ] 支持 `Approve / Reject / Cancel`
- [ ] 审批动作落成 `tool_result_meta`

建议文件：

- `web/src/features/console-approval/ApprovalRow.vue`
- `web/src/features/console-approval/ApprovalDialog.vue`
- `web/src/features/console-approval/useApprovalAction.ts`

### M12 执行流模块

- [ ] 渲染 `execution row`
- [ ] 处理节点分组、`queued -> running -> success/failed`
- [ ] 支持 stdout/stderr tail、异常节点默认展开
- [ ] 处理 `execution.chunk` 和 `execution.finished`

建议文件：

- `web/src/features/console-execution-stream/ExecutionRow.vue`
- `web/src/features/console-execution-stream/ExecutionNodePanel.vue`
- `web/src/features/console-execution-stream/useExecutionStream.ts`

### M13 总结模块

- [ ] 渲染 `summary row`
- [ ] 展示 total / success / failed / skipped
- [ ] 展示 AI 总结和后续建议
- [ ] `summary` 中可局部使用 `markstream-vue`

建议文件：

- `web/src/features/console-summary/SummaryRow.vue`
- `web/src/features/console-summary/SummaryActions.vue`

### M14 Composer 模块

- [ ] 底部输入框、发送按钮、快捷 chips
- [ ] 发送 `session.message.submit`
- [ ] 会话繁忙时禁用输入
- [ ] 占位文案要体现“AI 会自行决定是否查节点 / 生成计划 / 进入审批”

建议文件：

- `web/src/features/console-composer/ComposerBox.vue`
- `web/src/features/console-composer/QuickActionChips.vue`
- `web/src/features/console-composer/useComposerSubmit.ts`

### M15 Nodes 列表页

- [ ] 顶部统计卡
- [ ] 节点搜索、筛选、状态标签
- [ ] 节点表格
- [ ] `View detail / Open in console`

建议文件：

- `web/src/pages/nodes/NodesPage.vue`
- `web/src/features/nodes-filter/NodesFilterBar.vue`
- `web/src/widgets/nodes-table/NodesTable.vue`
- `web/src/entities/node/model/nodes.store.ts`

### M16 Node Detail 页

- [ ] 节点概览
- [ ] 近期指标卡
- [ ] 最近任务
- [ ] 风险提示和跳转回 Console

建议文件：

- `web/src/pages/node-detail/NodeDetailPage.vue`
- `web/src/widgets/node-overview/NodeOverview.vue`
- `web/src/widgets/node-overview/NodeMetricsCards.vue`
- `web/src/entities/node/model/node-detail.store.ts`

### M17 History 页

- [ ] 任务列表
- [ ] 搜索和筛选
- [ ] 任务详情面板
- [ ] 展示 plan / approval / execution / tool meta / audit 摘要

建议文件：

- `web/src/pages/history/HistoryPage.vue`
- `web/src/widgets/history-task-list/HistoryTaskList.vue`
- `web/src/widgets/history-task-detail/HistoryTaskDetail.vue`
- `web/src/entities/task/model/history.store.ts`

### M18 Settings 页

- [ ] 模型配置
- [ ] 账户安全
- [ ] 偏好设置
- [ ] 保存按钮与脏数据检测

建议文件：

- `web/src/pages/settings/SettingsPage.vue`
- `web/src/widgets/settings-panels/ModelConfigPanel.vue`
- `web/src/widgets/settings-panels/AccountSecurityPanel.vue`
- `web/src/widgets/settings-panels/PreferencePanel.vue`
- `web/src/entities/settings/model/settings.store.ts`

### M19 状态反馈和异常处理

- [ ] 全局 toast
- [ ] 接口错误、WebSocket 断线、session busy、revision 过期处理
- [ ] 空态、加载态、错误态统一

建议文件：

- `web/src/shared/ui/app-toast/`
- `web/src/shared/lib/error-map.ts`
- `web/src/shared/lib/request-state.ts`

### M20 测试与验收

- [ ] 为 adapter、store、关键交互写单测
- [ ] 为 Console 主流程写组件测试
- [ ] 至少覆盖：
  - session 切换
  - target confirm
  - approval
  - execution stream
  - summary 收口

建议文件：

- `web/src/**/*.spec.ts`
- `web/e2e/console.spec.ts`

## 8. 推荐开发顺序

建议按下面顺序推进，不要先做 Nodes / History / Settings，再回头补 Console。

1. `M00-M04`
   - 先把工程、样式 token、adapter、mock 基础搭起来
2. `M05-M14`
   - 先把 `Console` 跑通，因为它是产品主路径，也是最复杂的路径
3. `M15-M18`
   - 再做 `Nodes / Node Detail / History / Settings`
4. `M19-M20`
   - 最后补错误处理、测试、联调收口

## 9. 首版最小可交付范围

如果要尽快进入“可看的前端 MVP”，建议先只做以下模块：

- `M00` 工程初始化
- `M01` 设计系统和基础样式
- `M03` 类型系统与 Contract Adapter
- `M04` Mock 数据和本地开发总线
- `M05` `ws/ui` Client
- `M06` Console 页面骨架
- `M07` Session 列表与切换
- `M08` Timeline Row 渲染框架
- `M09` 目标确认模块
- `M10` 计划预览模块
- `M11` 审批模块
- `M12` 执行流模块
- `M13` 总结模块
- `M14` Composer 模块

这一步完成后，即使后端 `ws/ui` 还没完全实现，也能靠 mock 跑通最关键的产品链路：

`输入 -> 目标确认 -> 计划 -> 审批 -> 执行 -> 总结`

## 10. 一句话建议

这套前端最重要的不是“先把页面画出来”，而是先把下面 4 个核心边界做对：

- `Console 走 WebSocket，其他页面走 HTTP`
- `session list store` 和 `session view store` 分离
- `row-based timeline` 用结构化组件渲染，不要全部 markdown 化
- `adapter` 先行，隔离当前 contract 不一致问题

## 11. Sprint 计划

为了方便按迭代跟踪完成情况，已补充一份 Sprint 版本的执行清单：

- [docs/frontend_sprint_plan.md](/Users/wentx/momaek/src/tolato/docs/frontend_sprint_plan.md)
