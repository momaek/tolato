# ToLaTo 前端 Sprint 开发计划

## 1. 使用说明

这份文档基于 [docs/frontend_development_modules.md](/Users/wentx/momaek/src/tolato/docs/frontend_development_modules.md) 拆成按 Sprint 推进的执行清单。

使用方式：

- 每个 Sprint 都有明确目标和交付范围
- 每个任务都使用 Markdown checkbox
- 完成后直接把 `- [ ]` 改成 `- [x]`
- 如果某个 Sprint 没做完，不要整体打勾，只勾掉已经完成的项

建议节奏：

- 默认按顺序执行 Sprint 0 -> Sprint 6
- `Console` 主链路优先，`Nodes / History / Settings` 后置
- 只有上一轮核心依赖稳定后，再进入下一轮

## 2. 总体里程碑

- [x] Sprint 0 完成：前端工程可运行，基础样式和 mock 可用
- [x] Sprint 1 完成：Console 骨架、路由、Store、基础 timeline 可运行
- [x] Sprint 2 完成：输入 -> 目标确认 -> 计划 主链路可运行
- [x] Sprint 3 完成：审批 -> 执行 -> 总结 主链路可运行
- [x] Sprint 4 完成：Nodes / Node Detail 可运行
- [x] Sprint 5 完成：History / Settings 可运行
- [ ] Sprint 6 完成：异常处理、测试、联调与收口完成

## 3. Sprint 0: 工程基线

### Sprint 目标

建立 `web/` 工程、UI 基础设施、类型和 mock 数据基线，让前端具备独立开发条件。

### 对应模块

- `M00`
- `M01`
- `M03`
- `M04`

### Checklist

- [x] 初始化 `web/`：Vite + Vue 3 + TypeScript
- [x] 接入 Vue Router
- [x] 接入 Pinia
- [x] 接入 Tailwind CSS
- [x] 接入 `shadcn-vue`
- [x] 接入 `markstream-vue`
- [x] 配置 ESLint / Prettier / 路径别名
- [x] 建立 `app / shared / entities / features / widgets / pages` 目录结构
- [x] 建立全局样式入口和 token 文件
- [x] 映射设计稿里的颜色、圆角、阴影、字体层级
- [x] 接入 logo 资源
- [x] 建立共享 UI 基础组件：`status badge / panel card / page header`
- [x] 定义前端领域类型：`session / timeline / task / node / settings`
- [x] 定义 HTTP adapter 层
- [x] 定义 `ws/ui` 协议类型
- [x] 建立 mock 数据：`sessions / timeline / nodes / history / settings`
- [x] 让页面在没有真实后端时能由 mock 驱动

### Sprint 完成定义

- [x] `web/` 可以本地启动
- [x] 页面能看到基础壳和样式
- [x] mock 数据可驱动后续页面开发

## 4. Sprint 1: Console 基础框架

### Sprint 目标

先把 Console 的页面骨架、路由、Store 和 timeline 基础渲染跑起来，为主交互打底。

### 对应模块

- `M02`
- `M05`
- `M06`
- `M07`
- `M08`

### Checklist

- [x] 建立全局路由：`/console /nodes /nodes/:id /history /history/:taskId /settings`
- [x] 建立全局导航壳：`Console / Nodes / History / Settings`
- [x] 搭建 `ConsolePage`
- [x] 搭建 Console 顶部区域
- [x] 搭建 Console 左侧 sidebar
- [x] 搭建 Console timeline 容器
- [x] 搭建 Console composer 容器
- [x] 建立 `useConnectionStore`
- [x] 建立 `useConsoleSessionListStore`
- [x] 建立 `useConsoleSessionViewStore`
- [x] 封装 `ws/ui client`
- [x] 支持 `sessions.list.request`
- [x] 支持 `session.snapshot.request`
- [x] 支持 `subscriptions.update`
- [x] 处理 `watch sessions` 的摘要更新：`session.summary.updated / session.requires_attention / session.unread.updated`
- [ ] 支持消费 `llm.sse.event`
- [x] 建立 session 列表切换逻辑
- [x] 切换 session 时先显示 skeleton，再用 snapshot 覆盖
- [x] 建立 timeline 基础 renderer
- [x] 顶部提供 `AI Agent / Direct shell` 模式切换，并让 `Direct shell` 进入说明态而非执行态
- [ ] 支持渲染：
  - [x] `user_message`
  - [x] `assistant_text`
  - [x] `tool_call_meta`
  - [x] `tool_result_meta`
- [x] 处理 timeline 滚动到底部
- [x] 处理 snapshot 的 `revision` 覆盖规则

### Sprint 完成定义

- [x] 可以切换 session
- [x] 可以看到基础 timeline
- [x] Console 不依赖真实执行流也能完成页面恢复

## 5. Sprint 2: Console 输入、目标确认、计划

### Sprint 目标

打通 `输入 -> 目标确认 -> 计划` 这条上半段主链路。

### 对应模块

- `M09`
- `M10`
- `M14`

### Checklist

- [x] 实现 composer 输入框
- [x] 实现发送按钮
- [x] 实现快捷 chips
- [x] 支持 `session.message.submit`
- [x] 会话繁忙时禁用输入
- [x] 当输入包含节点语义时，严格按 `tool_call_meta -> tool_result_meta -> target_confirmation` 顺序渲染
- [x] 渲染 `target_confirmation row`
- [x] 展示候选节点、匹配依据、scope、来源
- [x] 实现 `确认目标`
- [x] 实现 `重新选择`
- [x] 实现 `清除上下文`
- [x] 目标确认后只追加 `tool_result_meta`
- [x] 渲染 `plan row`
- [x] `target_confirmation row` 明确展示用户原始目标表达，并支持 `All online nodes` / 广播候选态
- [x] 顶部和左栏在多节点目标时展示广播提醒，提示“仅允许只读广播自动执行”
- [x] 若沿用上一轮已确认目标，明确展示 `沿用上一轮已确认目标`
- [x] `plan row` 展示 `input / target / target source / summary / risk / impact / steps / step args / requiresApproval`
- [ ] 在主时间线中实时渲染原始 `thinking` stream
- [ ] 在主时间线中实时渲染原始 `content` stream
- [x] 实现 `查看完整计划` 弹层
- [x] 低风险只读计划展示“将自动进入执行”的提示
- [x] 在顶部和左栏同步展示 target context
- [x] 支持沿用上一轮 target context 的展示

### Sprint 完成定义

- [x] 用户可以输入任务
- [x] 可以完成目标确认
- [x] 可以看到结构化计划而不是纯文本结果

## 6. Sprint 3: Console 审批、执行、总结

### Sprint 目标

打通 `审批 -> 执行 -> 总结` 这条下半段主链路，让 Console 形成完整闭环。

### 对应模块

- `M11`
- `M12`
- `M13`

### Checklist

- [x] 渲染 `approval row`
- [x] 展示审批原因、风险等级、影响范围、目标节点
- [x] 实现 `Approve`
- [x] 实现 `Reject`
- [x] 实现 `Cancel`
- [x] 审批动作写入 `tool_result_meta`
- [x] 渲染 `execution row`
- [x] 节点级展示 `queued / running / success / failed`
- [x] 支持 stdout tail
- [x] 支持 stderr tail
- [x] 异常节点默认展开
- [x] 支持 `execution.chunk` 事件处理
- [x] 支持 `execution.finished` 事件处理
- [x] 广播只读执行正确展示顶部 / 左栏广播提醒与节点级聚合
- [x] 广播写操作默认阻断或升级为更高审批提示，不按普通审批流直接放行
- [x] 渲染 `summary row`
- [x] 展示 `total / success / failed / skipped`
- [x] 在 summary 中渲染 AI 总结文本
- [x] 提供 summary 级操作按钮，例如复制结论

### Sprint 完成定义

- [x] Console 主链路闭环可跑通
- [ ] 能完整演示 `输入 -> 确认 -> 计划 -> 审批 -> 执行 -> 总结`

## 7. Sprint 4: Nodes 与 Node Detail

### Sprint 目标

完成节点查询面和单节点详情页，把 Console 左栏和独立节点页面的边界分开。

### 对应模块

- `M15`
- `M16`

### Checklist

- [x] 搭建 `NodesPage`
- [x] 实现节点统计卡
- [x] 实现节点搜索
- [x] 实现节点筛选：状态 / tag / region / busy
- [x] 实现节点表格
- [x] 实现 `View detail`
- [x] 实现 `Open in console`
- [x] 建立 `useNodesStore`
- [x] 建立 `useNodeDetailStore`
- [x] 搭建 `NodeDetailPage`
- [x] 展示节点概览信息
- [x] 展示节点指标卡
- [x] 展示最近任务
- [x] 展示风险提示 / attention 卡片
- [x] 支持从 Node Detail 跳回 Console

### Sprint 完成定义

- [x] 可以完成节点浏览和筛选
- [x] 可以查看独立节点详情页

## 8. Sprint 5: History 与 Settings

### Sprint 目标

补齐查询与配置面，让 MVP 四个一级页面都可用。

### 对应模块

- `M17`
- `M18`

### Checklist

- [x] 搭建 `HistoryPage`
- [x] 实现任务搜索
- [x] 实现任务筛选
- [x] 实现任务列表
- [x] 实现任务详情面板
- [x] 在历史详情里展示 `plan / approval / execution / tool meta / audit`
- [x] 支持从历史任务回到 Console
- [x] 搭建 `SettingsPage`
- [x] 实现模型配置面板
- [x] 实现账户安全面板
- [x] 实现偏好设置面板
- [x] 实现保存按钮
- [x] 实现表单脏状态检测

### Sprint 完成定义

- [x] `History` 可用于回看任务
- [x] `Settings` 可用于展示和保存配置

## 9. Sprint 6: 测试、异常处理、联调收口

### Sprint 目标

解决稳定性、错误处理、断线重连、测试和联调问题，让前端进入可持续迭代状态。

### 对应模块

- `M19`
- `M20`

### Checklist

- [x] 建立全局 toast 机制
- [ ] 处理 HTTP 请求错误态
- [ ] 处理 WebSocket 断线重连
- [ ] 处理 `session_busy`
- [ ] 处理 snapshot `revision` 过期
- [ ] 处理 snapshot 中未完成 `thinking` / `content` stream 的恢复
- [ ] 统一空态、加载态、错误态
- [ ] 为 adapter 写单元测试
- [ ] 为 store 写单元测试
- [ ] 为 timeline 关键组件写测试
- [ ] 为 Console 主链路写测试
- [ ] 覆盖 session 切换测试
- [ ] 覆盖 target confirm 测试
- [ ] 覆盖 approval 测试
- [ ] 覆盖 execution stream 测试
- [ ] 覆盖 OpenAI 原始 `thinking` / `content` SSE stream 测试
- [ ] 覆盖 summary 测试
- [ ] 完成 mock 到真实接口的切换检查
- [ ] 完成 `Console` 与 `Nodes / History / Settings` 的联调回归

### Sprint 完成定义

- [ ] 主流程有测试覆盖
- [ ] 常见异常路径有明确处理
- [ ] 可以开始进入下一阶段迭代

## 10. 当前状态面板

可以在每次迭代结束后，先更新这里，快速看总体进度。

### 当前 Sprint

- [ ] Sprint 0 进行中
- [x] Sprint 1 进行中
- [x] Sprint 2 进行中
- [x] Sprint 3 进行中
- [ ] Sprint 4 进行中
- [ ] Sprint 5 进行中
- [x] Sprint 6 进行中

### 已交付页面

- [x] Console
- [x] Nodes
- [x] Node Detail
- [x] History
- [x] Settings

### 已打通主链路

- [x] 输入任务
- [x] 目标确认
- [x] 计划预览
- [x] 审批
- [x] 执行流
- [x] 总结收口
