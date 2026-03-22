# ToLaTo 控制台与补充页面 UI 设计稿

## 1. 文档目的

本稿用于补充 `docs/prd.md` 中 MVP 的前端信息架构与 UI 设计方案，明确：

- 当前架构图、草图与 PRD 的偏离点
- 主控制台与补充页面的页面结构和信息层级
- 关键卡片、弹层、抽屉和状态流转
- 一级页面的职责分工与导航关系
- 首版交互边界，避免产品语义滑向“裸 shell 直达执行”

本稿是线框级设计文档，不是高保真视觉稿，也不是前端代码说明。

当前 Pencil 逐屏状态稿说明另见 [`docs/ui_state_description.md`](./ui_state_description.md)，用于补充 Empty / Plan / Approval / Execution 四个状态页的落地描述。

---

## 2. 设计基准

本次 UI 设计以 `docs/prd.md` 为产品安全基线，并吸收现有架构图和草图中可保留的控制台形态。

### 2.1 保留的部分

- Web UI -> Control Server -> Agent Client 的整体控制台形态
- 控制台左侧上下文栏 + 中央主工作区 + 底部输入区 的工作方式
- 面向多节点广播和单节点操作的统一入口
- 以实时结果流和 AI 总结为主的运维控制体验
- 单用户工具的轻量信息架构，而非完整后台 admin

### 2.2 必须纠偏的部分

#### 纠偏 1：AI 不应直接表现为“生成命令立即执行”

PRD 要求主路径是：

`自然语言 -> 结构化计划 -> 风险识别 -> 必要审批 -> 执行 -> 结果聚合 -> 审计`

因此 UI 必须先展示时间线中的结构化 row，而不是把 AI 输出直接渲染成“系统开始跑命令”。

#### 纠偏 2：草图缺少审批与风险表达

现有草图更像一个带节点侧栏的执行终端，但 PRD 明确需要：

- 计划预览
- 风险等级
- 影响说明
- Approve / Reject / Cancel
- 聚合结果
- 审计时间线

这些必须进入主工作区。

#### 纠偏 3：`Direct shell` 首版不能成为真实执行入口

虽然保留 `Direct shell` 这个 tab 名称，但首版只作为占位入口：

- 不提供真正可执行的 shell 输入
- 不提供交互式终端
- 文案明确说明后续只会以“受限命令模式”开放

这样既保留扩展方向，也不违反 PRD 对安全模型的要求。

#### 纠偏 4：架构图中的 AI 输出文案需要修改

建议将架构图中 Claude API 的回流说明从“命令 + 建议”修改为：

- `计划 + 风险 + 建议`

同时在 Control Server 中补上：

- `Plan Engine`
- `Approval Gate`
- `Task Store / Audit Store`

否则 UI 与系统语义无法对齐。

---

## 3. 页面目标

本产品是单用户 AI 运维工具，不是多租户后台，因此前端信息架构应保持克制，但不能把所有需求都塞进一个控制台。

MVP 统一收敛为 4 个一级页面：

1. `Console`
2. `Nodes`
3. `History`
4. `Settings`

其中：

- `Console` 解决会话、目标确认、计划、审批、执行和总结闭环
- `Nodes` 解决全量节点资产浏览与筛选
- `Node Detail` 通过独立详情页承接完整单节点信息
- `History` 解决任务历史追溯，不单独拆审计页
- `Settings` 解决模型配置、账户安全和偏好设置

本稿仍以 `Console` 为主稿，但补充定义其余页面，避免前后文档对 MVP 页面边界理解不一致。

---

## 4. 页面结构

### 4.1 总体布局

全局导航只包含：

- `Console`
- `Nodes`
- `History`
- `Settings`

约束：

- `session` 列表固定存在于 `Console` 左侧，不进入全局导航
- `Node Detail` 通过 `/nodes/:id` 独立路由进入，不作为一级导航
- `Direct shell` 不是独立页面，而是 `Console` 顶部模式中的占位说明态

### 4.2 主控制台总体布局

采用桌面端优先的单页控制台布局：

```text
+--------------------------------------------------------------------------------------------------+
| Top Bar: 品牌 | 全局状态 | 当前目标上下文 | 模式切换(AI Agent / Direct shell) | 时钟/同步状态         |
+------------------------------+-------------------------------------------------------------------+
| Left Sidebar                 | Main Workspace                                                    |
| Session 列表                 | user / assistant rows                                             |
| 当前会话节点上下文           | target_confirmation row                                           |
| 候选节点 / 已确认目标        | tool_call_meta / tool_result_meta                                 |
| 节点摘要 / 风险提示          | plan / approval / execution / summary rows                        |
| 广播提醒                     | row-based timeline                                                |
+------------------------------+-------------------------------------------------------------------+
| Bottom Composer: 自然语言输入框 | 发送按钮 | 快捷操作 chips | 说明文案                               |
+--------------------------------------------------------------------------------------------------+
```

### 4.3 页面分区

#### 顶部状态栏

目标：建立“这是一个有边界、有状态、有目标节点的控制台”的认知。

包含以下元素：

- 产品名：`ToLaTo`
- 全局节点状态摘要：`4 online · 1 offline`
- 当前目标上下文：`未确认`、`待确认 jp-tokyo-01` 或 `已确认 2 nodes`
- 模式切换：
  - `AI Agent`
  - `Direct shell`
- 右上角状态：
  - 最近同步时间
  - 实时连接提示，如 `Control Server connected`

规则：

- `AI Agent` 默认激活
- `Direct shell` 可点击但进入说明态，不进入执行态
- 当目标上下文为多节点时，顶部应出现只读广播提醒
- 当目标上下文为 `pending_confirmation` 时，顶部显示待确认 badge，禁止直接进入执行

#### 左侧会话与上下文栏

目标：让用户先理解“我在哪个 session 里”，再看到当前会话关联的节点上下文，而不是把左侧做成全量资产页。

结构：

1. session 列表
2. 当前会话节点总览卡
3. 最近匹配节点 / 候选节点区
4. 已确认目标区
5. 节点摘要与风险提醒条

session 项显示字段：

- title
- 最近一条摘要
- 状态：idle / running / attention
- unread
- 更新时间

节点摘要项显示字段：

- `hostname`
- `region`
- `os`
- `tags`
- `status`
- `last_seen`
- `busy/idle`

状态表达建议：

- `online`：绿色圆点
- `busy`：蓝色或青色圆点
- `offline`：灰红色圆点
- 当前选中项：浅底高亮

交互规则：

- 点击 session 项切换当前会话
- 点击节点项把该节点作为“确认候选”带入主区
- 如需查看完整节点信息，跳转到独立 `Node Detail` 页面
- 点击 `All online nodes` 只生成候选目标，不直接进入广播执行
- 广播候选态下左栏顶部显示提示：
  - `仅允许只读任务自动执行`
  - `广播写操作需要更高审批或直接阻止`

#### 中央主工作区

目标：用时间线 row 的方式串起“输入 -> 目标确认 -> 计划 -> 审批 -> 执行 -> 结果”。

主工作区不是空白终端，也不是一张卡不断长高，而是按时间顺序追加新 row 的消息流。

典型 Row 顺序如下：

1. `user row`
2. `tool_call_meta row`
3. `tool_result_meta row`
4. `assistant target confirmation row`
5. `tool_result_meta row`
6. `assistant plan row`
7. `assistant approval row`
8. `tool_result_meta row`
9. `assistant execution row`
10. `assistant summary row`

规则：

- 每一次关键动作都应追加一个新的 row，而不是回头改写旧 row 的主要语义
- 当前 assistant 正在生成时，主时间线中允许出现一个流式中的 assistant 容器，实时展示原始 `thinking` 与 `content`
- 按钮触发的确认 / 审批不新增 `user row`
- 普通 tool 调用默认展示 `tool_call_meta row` 与 `tool_result_meta row`
- 按钮触发后的结果只展示为弱化的 `tool_result_meta row`
- 只有用户手动在输入框中输入“确认”“批准”等文本时，才算新的 `user row`

#### 底部输入区

目标：明确告诉用户系统会“先识别目标并请求确认，再决定是否生成计划或进入审批”，而不是“直接执行”。

组成：

- 主输入框
- 主按钮：`发送`
- 快捷操作 chips
- 辅助说明文案

占位文案建议：

`发送任务请求，AI 会先决定是否查询节点、确认目标、生成计划或进入审批`

快捷 chips 建议：

- `磁盘告警`
- `Docker 状态`
- `Nginx 自愈`
- `系统负载`
- `网络检查`

#### Console 数据来源与恢复规则

`Console` 的数据面固定分成两类：

- `ws/ui`：
  - `connection.ready`
  - `sessions.list.response`
  - `session.snapshot.response`
  - `session.rows.response`
  - `llm.sse.event`
  - `timeline.row.appended`
  - `thread.target.pending / confirmed / cleared`
  - `execution.chunk / execution.finished`
  - `session.summary.updated / session.requires_attention / session.unread.updated / session.finished`
- HTTP：
  - 不承担 `Console` 主链路取数
  - 仅保留登录、鉴权、静态资源和极轻量 bootstrap

固定规则：

- 页面初始化后，先建立 `ws/ui` 并等待 `connection.ready`
- 当前打开的 session 通过 `session.snapshot.response` 恢复整页，不靠增量事件回放
- 更早历史 rows 通过 `session.rows.request` 分页追加
- 当前轮 assistant 生成中的原始 `thinking` 与 `content` 通过 `llm.sse.event` 实时渲染
- 当前 active session 接收完整 timeline 级事件
- watch sessions 只更新左侧列表摘要、未读和 attention，不更新主时间线
- WebSocket 断线重连后，前端必须重新请求 `sessions.list`、当前 `session.snapshot` 和 `subscriptions.update`
- `TimelineRow` 用于展示稳定结果；`llm.sse.event` 用于展示本轮仍在生成中的原始 reasoning / content stream

---

## 5. 关键 Row 设计

### 5.1 系统消息 Row

用于承接系统级反馈，不与任务 row 混淆。

典型内容：

- `Control server ready. 4 agents connected.`
- `Current target context: unset`
- `Broadcast mode only auto-runs low-risk read plans`
- `Direct shell is not available in MVP`

视觉要求：

- 弱化于任务 row
- 使用浅底和单色文本
- 时间戳可选显示在右上角

在系统消息 row 之后，若本轮输入中包含节点语义且系统已匹配到候选目标，时间线必须先展示目标解析阶段的普通工具调用，再进入目标确认 row。

在目标确认 row 之前，若 Agent Loop 调用了普通工具，如 `list_nodes`、`get_node_details`、`resolve_target_nodes`，时间线中应先追加：

- `tool_call_meta row`
- `tool_result_meta row`

目标确认 row 必须展示：

- 用户原始输入中的目标表达，如 `东京节点`
- AI 匹配到的候选节点
- 匹配依据，如 `region = Tokyo`
- 当前是单节点、多节点还是 `All online nodes`
- 明确操作：
  - `确认目标`
  - `重新选择`
  - `清除上下文`

规则：

- 目标未确认时，不允许进入执行和审批
- 若本轮沿用了上一轮已确认目标，row 中必须提示 `沿用上一轮已确认目标`
- 目标确认后，顶部状态栏和后续任务 row 都要同步显示目标标签
- 用户点击 `确认目标` 后，不新增 `user row`
- 紧接着追加一条弱化的 `tool_result_meta row`，例如 `target_confirmation succeeded · 1 target confirmed`

### 5.2 计划预览 Row

这是目标确认完成后的第一个核心 row。

必须展示：

- 用户原始输入
- 目标节点
- 目标来源，如 `assistant_resolved` / `context_inherited`
- 计划摘要
- steps 列表
- 每个 step 的 action / args
- 风险等级
- 预估影响
- 是否需要审批

建议结构：

```text
[Plan Preview]
Input: 看看东京节点为什么 502
Target: jp-tokyo-01
Target Source: assistant_resolved
Summary: 检查 nginx、应用进程和错误日志
Risk: low
Impact: 只读诊断，不修改服务
Steps:
1. service_status(nginx)
2. tail_log(/var/log/nginx/error.log)
3. network_check(local upstream)
```

操作：

- `查看完整计划`
- 若是低风险只读任务，底部提示 `低风险计划将自动进入执行`

### 5.3 审批 Row

当 `requiresApproval = true` 时，计划 row 之后必须追加一条审批 row。

Row 内容：

- 审批原因
- 风险等级
- 影响说明
- 已确认目标
- 主要动作

按钮：

- `Approve`
- `Reject`
- `Cancel`

规则：

- 未审批前，任务不能进入 `queued` 或 `running`
- 用户点击 `Approve` / `Reject` 后，不新增 `user row`
- 审批结果以弱化的 `tool_result_meta row` 追加到时间线中，如：
  - `approval recorded · Approved by Alex at 14:32`

### 5.4 Tool Call Meta Row

`tool_call_meta row` 用于承接 Agent Loop 发起的普通工具调用，不与正常聊天消息混淆。

典型内容：

- `calling list_nodes(status=online,stale)`
- `calling resolve_target_nodes("东京节点")`

视觉要求：

- 比 assistant row 更弱
- 使用更小字号和更浅颜色
- 不占用大卡片容器
- 更接近一条时间线事件或审计脚注

### 5.5 Tool Result Meta Row

`tool_result_meta row` 用于承接普通工具结果，以及按钮触发后的用户动作结果。

典型内容：

- `list_nodes returned 4 online nodes`
- `target_confirmation succeeded · jp-tokyo-01 confirmed`
- `approval recorded · execution unlocked`
- `target_context cleared`

视觉要求：

- 比 assistant row 更弱
- 使用更小字号和更浅颜色
- 不占用大卡片容器
- 更接近一条时间线事件或审计脚注

### 5.6 执行日志 Row

目标是“看得清执行过程，但不退化成终端”。

结构建议：

- 顶部状态线：`queued -> dispatched -> running`
- 中部按节点分组显示输出
- 每组输出区分：
  - stdout
  - stderr
  - exit code

多节点时以分组折叠方式展示：

```text
sg-prod-01   running
us-east-02   success
jp-tokyo-01  failed
```

交互：

- 默认展开当前异常节点
- 正常节点默认折叠
- 支持点击“查看节点详情”

### 5.7 聚合结果 Row

用于替代“用户自己扫日志判断结果”。

必须展示：

- total
- success
- failed
- offline skipped
- final status

示例：

```text
Result Summary
Total: 4
Success: 3
Failed: 1
Offline skipped: 1
Final status: partial_failed
```

提供：

- `查看失败节点`
- `查看全部结果`

### 5.8 AI 总结 Row

这是任务闭环的收口卡。

内容：

- 问题归因
- 异常节点点名
- 关键建议
- 可复制的总结文案

例如：

```text
AI Summary
sg-prod-01 的根分区达到 87%，Nginx 正常但日志目录过大。
建议先清理 30 天前日志，再复查磁盘占用。
```

操作：

- `复制结论`
- `作为下一步建议保留`

首版不直接提供“一键执行建议操作”。

---

## 6. 关键弹层与抽屉

### 6.1 计划详情弹层

触发方式：点击 `查看完整计划`

展示：

- taskId
- mode
- inputText
- targetNodes
- summary
- 完整 steps
- riskLevel
- estimatedImpact
- requiresApproval

用途：

- 承接长计划和多步骤展示
- 避免主 row 内容体过长

### 6.2 审批确认弹层

触发方式：点击 `Approve`

内容：

- 风险等级说明
- 影响范围
- 目标节点数量
- 审批后的下一状态

广播写任务时：

- 若策略不允许，弹层直接显示拦截原因
- 不允许继续审批

### 6.3 节点详情跳转

触发方式：点击节点名、节点状态摘要或节点结果卡中的节点入口。

展示策略：

- `Console` 内不再以节点详情抽屉作为主方案
- 默认跳转到独立 `Node Detail` 页面
- 控制台只保留轻量节点摘要，不承载完整详情

这样做的目的：

- 保持控制台聚焦会话主流程
- 让节点信息具备独立路由和可分享链接
- 避免“节点管理”能力被塞进时间线页面

### 6.4 任务结果抽屉

触发方式：点击 `查看失败节点` 或 `查看全部结果`

展示：

- 按节点拆分的执行结果
- status
- exitCode
- stdout tail
- stderr tail
- startedAt / finishedAt

适用场景：

- 多节点广播执行后查看个体差异
- 快速定位失败节点

---

## 7. 模式设计

### 7.1 AI Agent 模式

这是 MVP 唯一可用的主执行模式。

流程：

1. 用户输入自然语言
2. Agent Loop 自主调用工具
3. AI 解析目标
4. 用户确认目标
5. 决定是否生成计划
6. 展示计划预览
7. 按风险决定是否审批
8. 执行
9. 聚合总结

模式说明文案建议：

`自然语言生成受控计划，写操作需审批，高风险不会直接执行`

### 7.2 Direct shell 模式

首版仅为占位说明页。

进入后只显示：

- 标题：`Direct shell`
- 说明：`该入口将在后续版本以“受限命令模式”开放`
- 边界说明：
  - 非裸 shell
  - 非交互式终端
  - 仍会经过策略校验和审计

不显示：

- 命令输入框
- 执行按钮
- 日志终端界面

这样可以保留产品路线，同时避免误导用户。

---

## 8. 状态流转设计

前端统一使用以下任务状态：

`planned -> waiting_approval -> approved -> queued -> dispatched -> running -> success | failed | timeout | cancelled`

### 8.1 状态可视表达

- `planned`：浅灰
- `waiting_approval`：琥珀色
- `approved`：蓝色
- `queued`：青色
- `dispatched`：蓝青色
- `running`：高亮蓝
- `success`：绿色
- `failed`：红色
- `timeout`：橙红色
- `cancelled`：灰色

### 8.2 状态对应 UI 行为

- `planned`：追加 plan row
- `waiting_approval`：追加 approval row 并锁定执行
- `approved`：追加 `tool_result_meta row`，记录审批已完成
- `queued/dispatched/running`：execution row 高亮，状态线推进
- `success/failed/timeout`：追加聚合结果 row 与 AI 总结 row
- `cancelled`：流程终止，保留已发生的审计和计划信息

---

## 9. 典型任务流示例

### 9.1 单节点只读诊断

用户输入：

`看看 sg-prod-01 为什么 502`

页面流程：

1. 系统解析 `sg-prod-01` 为候选目标
2. 追加 `tool_call_meta(list_nodes / resolve_target_nodes)`
3. 追加 `tool_result_meta`
4. 追加目标确认 row
5. 用户点击确认后追加 `tool_result_meta`
6. 追加 plan row
7. 风险为 `low`
8. 自动进入执行
9. 追加 execution row
10. 追加 summary row

### 9.2 单节点写操作

用户输入：

`重启东京节点的 nginx`

页面流程：

1. 解析 `东京节点` 为 `jp-tokyo-01`
2. 追加 `tool_call_meta(list_nodes / resolve_target_nodes)`
3. 追加 `tool_result_meta`
4. 追加目标确认 row
5. 用户点击确认后追加 `tool_result_meta`
6. 追加 plan row
7. 风险为 `medium`
8. 追加 approval row
9. 用户点击 Approve 后追加 `tool_result_meta`
10. 进入执行并追加 execution row
11. 返回 summary row

### 9.3 广播只读巡检

用户输入：

`检查所有在线节点的磁盘占用`

页面流程：

1. 系统解析出目标为 `All online nodes`
2. 追加 `tool_call_meta(list_nodes)`
3. 追加 `tool_result_meta`
4. 追加多节点目标确认 row
5. 页面顶部与左栏都显示广播提醒
6. 用户确认后追加 `tool_result_meta`
7. 追加广播 plan row
8. 风险为 `low`
9. 自动执行
10. 聚合结果 row 显示 success / failed / offline skipped

### 9.4 广播写操作

用户输入：

`重启所有节点的 nginx`

页面流程：

1. 系统解析出目标为 `All online nodes`
2. 追加 `tool_call_meta(list_nodes / resolve_target_nodes)`
3. 追加 `tool_result_meta`
4. 追加多节点目标确认 row
5. 用户确认后追加 `tool_result_meta`
6. 追加 plan row
7. 风险为 `high`
8. 追加 approval row 或阻断说明 row
9. 若策略不允许，直接阻断并保留拦截说明

---

## 10. 文案与信息表达建议

### 10.1 按钮文案

- 主按钮：`发送`
- 次按钮：`查看完整计划`
- 审批：`Approve`
- 拒绝：`Reject`
- 取消：`Cancel`
- 查看明细：`查看节点详情`
- 复制：`复制结论`

### 10.2 风险文案

- `Low risk · 只读检查，可自动执行`
- `Medium risk · 涉及服务操作，需要审批`
- `High risk · 涉及广播写操作或潜在破坏行为，需要更高审批`
- `Forbidden · 不允许执行`

### 10.3 占位说明文案

`Direct shell 将在后续版本以受限命令模式开放，不提供裸 shell 与交互式终端。`

---

## 11. 补充页面设计

### 11.1 Nodes 页面

页面目标：

- 浏览全部节点资产
- 快速完成搜索、筛选和状态判断
- 为控制台提供“从资产进入操作”的入口

推荐布局：

- 顶部页面头：标题、节点总量、次级操作
- 统计摘要区：online / busy / offline / attention
- 筛选工具条：搜索、region、tag、status、density
- 主列表区：表格或高密度列表

节点列表字段建议：

- hostname
- region
- os
- tags
- status
- last_seen
- CPU / 内存 / 磁盘摘要
- 当前任务状态

关键动作：

- 主动作：进入 `Node Detail`
- 次动作：`在控制台中打开`

### 11.2 Node Detail 页面

页面目标：

- 承载单节点完整信息
- 让用户快速判断是否需要继续排查或执行

推荐布局：

- 顶部：节点标题、状态 badge、回到控制台按钮
- 主列：基础信息、资源摘要、最近心跳、风险提醒
- 侧列：最近任务、最近异常、快捷操作说明

关键模块：

- 基础信息：hostname / region / os / tags / version
- 当前状态：online / offline / busy / idle
- 最近心跳：CPU / 内存 / 磁盘 / last_seen
- 最近任务：最近 3-10 条
- 返回控制台并带入目标节点

### 11.3 History 页面

页面目标：

- 追溯 task 级历史
- 查看 plan、approval、execution 与总结闭环
- 在任务详情里聚合关联审计信息

推荐布局：

- 顶部：标题、状态筛选、审批筛选、搜索
- 左侧或上方：任务列表
- 右侧或下方：当前选中 task 的详情面板

任务列表字段：

- 时间
- input_text 摘要
- target 范围
- status
- approval_status
- aggregate 摘要

详情区内容：

- plan
- approval 记录
- execution 聚合
- 节点级结果
- `tool_call_meta`
- `tool_result_meta`
- 关联审计字段

约束：

- 本页只做任务历史
- 不单独拆审计页

### 11.4 Settings 页面

页面目标：

- 管理模型接入
- 管理账户安全
- 管理个人偏好

推荐布局：

- 顶部：页面标题与辅助说明
- tab 导航：`模型配置 / 账户安全 / 偏好设置`
- 主内容区：表单和状态反馈

`模型配置` tab：

- provider
- model
- endpoint
- API key
- temperature
- max tokens
- timeout
- 连接测试

`账户安全` tab：

- 改密码
- 当前登录信息
- 登出其他会话

`偏好设置` tab：

- 语言
- 时间格式
- 默认视图
- 执行结果展开偏好

### 11.5 页面关系总结

- `Console` 是任务主入口
- `Nodes` 是资产总览入口
- `Node Detail` 是单节点完整上下文入口
- `History` 是任务追溯入口
- `Settings` 是模型与个人配置入口
- `session` 列表仅存在于 `Console`

---

## 12. 与 PRD 的对应关系

本设计稿与 PRD 对齐的核心项如下：

- 对齐 `11.1 产品信息架构`：`Console / Nodes / History / Settings` 四个一级页面
- 对齐 `11.2 Console 页面结构`：左侧 session 列表与节点上下文、中间主区、底部输入区、顶部模式与 Target
- 对齐 `11.3-11.6`：节点列表、节点详情、任务历史、设置页的职责边界
- 对齐 `11.7 关键交互`：目标确认 row、plan row、审批操作、tool_call_meta / tool_result_meta row、执行态展示、多节点摘要
- 对齐 `9.3 AI 计划生成`：展示结构化计划而非自由命令
- 对齐 `9.4 审批流`：审批前不得进入执行
- 对齐 `9.8 执行结果聚合`：统一展示 success / failed / offline skipped
- 对齐 `10.2 执行限制`：不把任意 shell 暴露为首版可执行入口

---

## 13. 评审结论

如果按本稿推进，整体设计将从“带节点列表的执行终端”修正为“以控制台为主、补充页面清晰分工”的 AI 运维工作台。

这份 UI 稿完成后，后续建议按以下顺序继续：

1. 先画低保真线框图
2. 再补高保真视觉风格
3. 最后再进入前端实现

如果需要，下一步可以继续补：

- 低保真 ASCII / Figma 式分屏线框
- 高保真视觉方向稿
- 前端路由与组件拆分建议
- 数据态与空态补图
