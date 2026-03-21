# ToLaTo UI 状态稿说明

## 1. 文档目的

本文档基于 [`docs/tolato-ui-states.pen`](./tolato-ui-states.pen) 的当前画布内容，对 ToLaTo 主控制台的 4 个核心 UI 状态进行逐屏文字说明。

它与 [`docs/ui_console_design.md`](./ui_console_design.md) 的关系如下：

- `ui_console_design.md` 负责说明产品边界、设计原则和状态机思路。
- 本文档负责把当前 Pencil 画布已经落下来的页面结构、卡片顺序、关键文案和状态差异整理成可读文本。

本文档主要面向产品评审和前端实现对齐，不作为视觉规范，也不替代前端代码说明。

---

## 2. 画布来源与阅读方式

当前 Pencil 文件包含 4 个主状态页：

- Empty State
- Plan State
- Approval State
- Execution State

阅读本稿时，建议把它理解为同一个控制台在不同任务阶段的页面快照，而不是 4 个彼此独立的页面。

---

## 3. 共用界面骨架

四个状态页都使用统一的三段式结构：

- 顶部状态栏：品牌、全局状态、当前模式和目标范围
- 左侧节点栏：节点概览、广播范围或目标节点、节点状态列表
- 右侧主工作区：系统提示、任务相关卡片、底部输入区

整体是桌面端优先的单页控制台，主工作区并非终端，而是围绕“计划、审批、执行、汇总”展开的卡片流。

### 3.1 顶部状态栏

顶部状态栏在四个状态页中都保持一致的结构，只是右侧状态提示随任务阶段变化。

固定元素：

- 品牌名为 `ToLaTo`
- 左上副标题说明当前工作台语义
- 右侧使用状态 badge + 一段短描述表达当前阶段

状态差异：

- Empty State：`workspace idle`，强调控制面健康且当前空闲
- Plan State：`plan ready`，强调已生成低风险结构化计划
- Approval State：`approval required`，强调写操作被阻断，等待批准
- Execution State：`running`，强调正在流式执行并聚合结果

### 3.2 左侧节点栏

左栏承担“目标范围确认”和“节点健康感知”两件事。

在广播态中：

- 标题为 `Nodes`
- 概览显示 `4 online · 1 offline`
- 顶部有 `All nodes` 卡片，标注 `Broadcast · 4 active`
- 下方是节点栈，展示节点名、地域、系统或简化地域、状态标签

在单节点审批态中：

- 标题切换为 `Target node`
- 顶部概览变为 `1 target · 4 online · 1 offline`
- 目标节点单独收敛成一张卡片，突出 `jp-tokyo-01`
- 卡片内额外显示 `target` 和 `risk-gated` 标签，表示当前任务已进入审批门

节点状态表达保持一致：

- 在线节点使用浅色卡片和中性标签
- 运行中或被选中的节点使用更深一层底色强调
- 离线节点使用弱化文字和红色状态标签

### 3.3 主工作区

主工作区始终包含两个层级：

- 上部内容区：承载系统提示、计划、审批、执行、汇总等任务卡片
- 下部输入区：承载任务输入、主按钮和状态 chips

内容区会随着任务推进逐步展开，而不是直接显示一个可交互 shell。

### 3.4 底部输入区

四个状态页都保留底部输入区，说明用户始终可以基于当前上下文继续发起任务，但输入区的按钮和 chips 会随阶段变化。

固定结构：

- 左侧输入框
- 右侧主按钮
- 下方一行状态 chips 或快捷任务 chips

语义差异：

- Empty State：鼓励发起首个任务，主按钮为 `生成计划`
- Plan State：允许在计划基础上重新生成，主按钮为 `重新生成`
- Approval State：输入仍保留，但不鼓励并行执行，底部强调“审批等待中”
- Execution State：执行中可准备下一步，主按钮切换为 `新任务`

### 3.5 视觉基调与组件风格

当前画布不是高保真营销风格，而是偏控制台与操作面板的产品化视觉。

可提炼的共性如下：

- 背景为暖灰白色，整体对比温和
- 大面积使用白色卡片承载信息，辅以浅灰描边和轻阴影
- 主按钮为深色实心按钮，次按钮和标签使用浅灰底
- 风险提示使用琥珀色背景与描边
- 离线风险使用淡红标签提示
- 圆角整体较大，卡片和输入区统一使用柔和圆角
- 字体使用 `Inter`，标题字重大，正文和辅助信息层级清晰

---

## 4. Empty State

### 4.1 页面目的

空态用于说明：控制台已经连接就绪，但尚未进入任何具体任务。它要先建立“这是一个计划驱动的 AI 运维控制台”的认知，而不是命令行。

### 4.2 顶栏内容

顶栏左侧显示品牌和副标题：

- `ToLaTo`
- `Idle workspace for structured ops, approvals, and execution.`

右侧显示：

- `workspace idle`
- `Control server healthy`
- `4 online · 1 offline · sync < 2s`

这组信息传达的是系统健康、连接正常、尚无待处理任务。

### 4.3 左栏节点区内容

左栏展示标准广播态节点总览：

- 顶部为 `Workspace / Nodes / 4 online · 1 offline`
- `All nodes` 卡片标记当前默认目标范围为广播
- 下方依次列出 5 台节点

节点示例信息包括：

- `sg-prod-01`，Busy，最近 2 秒内心跳
- `us-east-02`，Online
- `jp-tokyo-01`，Online
- `de-fra-03`，Online
- `hk-01`，Offline，最近 13 分钟前在线

这里已经把“在线、繁忙、离线”三类状态同时铺开，便于后续任务态复用。

### 4.4 主区卡片结构与先后顺序

主区从上到下分为两张卡：

1. `SYSTEM` 提示卡  
内容说明控制面已经就绪、4 个 agent 已通过 WebSocket 连接、广播默认仍受只读约束。

2. 空态引导卡  
标题为 `Start with a natural-language task`，正文强调系统会先生成结构化计划，再进入审批、执行与结果汇总。

引导卡下方用 3 张并排小卡说明核心工作流：

- `1. Generate plan`
- `2. Review / approve`
- `3. Execute / summarize`

这部分是空态的重点，它把控制台核心路径显式讲清楚。

### 4.5 输入区文案与按钮状态

输入框占位文案为：

`描述你的任务，AI 会先生成执行计划，再逐步显示后续状态`

主按钮：

- `生成计划`

快捷 chips：

- `磁盘告警`
- `Docker 状态`
- `日志排查`
- `Nginx 自愈`
- `网络检查`

这些 chips 既是快捷入口，也是任务类型的产品提示。

### 4.6 相对其他状态的变化点

空态没有任务卡片，没有审批信息，也没有执行结果。它只负责：

- 建立产品心智
- 说明 AI Agent 是默认主模式
- 明确 Direct shell 只是次级入口，不是当前工作主线

---

## 5. Plan State

### 5.1 页面目的

计划态用于承接“用户已输入任务，系统已产出结构化计划，但尚未进入执行”的阶段。当前画布选择的是一个低风险、只读、广播型任务，因此页面重点是预览计划而非审批。

### 5.2 顶栏内容

左侧副标题改为：

- `A low-risk plan is ready for review before execution.`

右侧状态区显示：

- `plan ready`
- `AI generated structured steps`
- `Read-only scope · approval not required`

这明确告诉用户：当前任务已经有结构化结果，而且由于风险较低，不必先走审批。

### 5.3 左栏节点区内容

左栏仍是广播态：

- `Workspace / Nodes / 4 online · 1 offline`
- `All nodes` 卡片显示 `Broadcast · 4 active`

节点列表和空态相似，但 `jp-tokyo-01` 被更深底色高亮，说明当前计划预览中存在视觉焦点节点。整体语义仍然是“面向所有在线节点的广播任务”。

### 5.4 主区卡片结构与先后顺序

主区从上到下为三段：

1. Header 卡  
左侧显示 `Target` + `All nodes (broadcast)`，右侧保留模式切换：
`AI Agent` 激活，`Direct shell` 为未激活态。

2. `PLAN` 提示卡  
说明 AI 已生成低风险只读计划，用户可以先审核结构，再继续后续执行。

3. `Plan preview` 主卡  
这是计划态核心卡片，包含：

- 标题：`Plan preview`
- 风险标签：`low risk`
- 执行性质标签：`read-only`
- 输入原文：`检查所有在线节点的磁盘占用和 Docker 状态`
- 三个执行步骤：
  - `01 Check disk usage and root partition pressure`
  - `02 Inspect Docker daemon and container health`
  - `03 Merge node status into one cluster-level summary`
- 底部说明：`Approval not required`
- 次要操作：`View full plan`

卡片下方还有一句提示：

`Next state after review: execution stream`

这句提示把当前页面与执行态直接串起来。

### 5.5 输入区文案与按钮状态

输入框中保留当前任务原文，主按钮改成：

- `重新生成`

下方状态 chips 为：

- `计划已生成`
- `审批未触发`
- `可继续执行`

这说明计划态底部输入区承担的是“沿当前任务继续编辑”的功能，而不是空态里的首次发起。

### 5.6 相对其他状态的变化点

与空态相比，计划态新增了完整的计划预览卡，并且通过风险和只读标签清楚表达：

- 当前任务是广播任务
- 当前任务是只读任务
- 当前任务可以跳过审批进入执行

它仍然没有出现执行日志、结果聚合或 AI 总结，因此用户注意力集中在“计划对不对”。

---

## 6. Approval State

### 6.1 页面目的

审批态用于承接“计划已生成，但因涉及写操作而被风险门拦住”的阶段。当前画布选择的是单节点服务重启，因此主目标是让用户看清影响范围、审计要求和批准动作。

### 6.2 顶栏内容

左侧副标题改为：

- `A write action is blocked until an explicit approval is recorded.`

右侧状态区显示：

- `approval required`
- `Medium-risk change pending`
- `Target locked to jp-tokyo-01`

这里的重点从“计划已就绪”切换为“计划被阻断，等待明确授权”。

### 6.3 左栏节点区内容

审批态左栏不再显示广播列表，而是切换成单节点聚焦模式：

- 标题为 `Target node`
- 概览显示 `1 target · 4 online · 1 offline`
- 目标节点卡片展示 `jp-tokyo-01`
- 补充信息为 `Tokyo · Debian 11`
- 状态标签包含：
  - `online`
  - `risk-gated`

这表明页面已经从“节点总览”收拢到“本次审批到底作用在哪一台机器”。

### 6.4 主区卡片结构与先后顺序

主区从上到下为四段：

1. Header 卡  
左侧为 `Target / jp-tokyo-01`，右侧仍为 `AI Agent` 激活，`Direct shell` 未激活。

2. `APPROVAL` 提示卡  
直接点明：计划中包含 `restart_service(nginx)`，在显式批准前不得执行。

3. `Pending plan` 卡  
用于简述待审批计划，内容包括：

- 标题：`Pending plan`
- 风险标签：`medium risk`
- 输入原文：`重启东京节点的 nginx`
- 执行步骤：
  - `01 restart_service(nginx)`
  - `02 record approver identity and timestamp before dispatch`

4. 审批主卡  
这是页面的核心操作区，包含：

- 标签：`approval required`
- 风险说明：`Risk: medium`
- 标题：`Approve restart_service(nginx)?`
- 说明文案：该操作会短暂中断 `jp-tokyo-01` 上的 nginx，只有在审计记录写入后才会放行
- 两条影响说明：
  - `Impact / brief nginx interruption`
  - `Audit / approver + timestamp recorded`
- 三个操作按钮：
  - `Approve`
  - `Reject`
  - `Cancel`

整个审批页的结构非常明确，主区已经不再讨论“计划怎么生成”，而是在问“要不要让这次变更发生”。

### 6.5 输入区文案与按钮状态

底部输入框仍保留任务原文：

- `重启东京节点的 nginx`

审批态底部没有主动作按钮，输入区更多起到上下文保留作用。下方 chips 为：

- `审批等待中`
- `执行未开始`

这在语义上比隐藏输入区更稳妥，因为用户仍能回看原请求，但不会误以为流程已经进入执行。

### 6.6 相对其他状态的变化点

与计划态相比，审批态的核心变化有三点：

- 目标范围从广播收拢到单节点
- 主区从“计划是否合理”切到“风险是否可接受”
- 页面第一次出现明确的审计语义和批准动作

这页也是当前设计里最接近“安全门”的一页。

---

## 7. Execution State

### 7.1 页面目的

执行态用于承接任务已开始分发和回传结果的阶段。当前画布选择的是广播型只读任务，因此页面重点不是单机日志细节，而是跨节点流式状态、聚合结果和 AI 总结。

### 7.2 顶栏内容

左侧副标题改为：

- `Streaming node output and collapsing it into one operator summary.`

右侧状态区显示：

- `running`
- `Live task in progress`
- `4 online nodes · partial results received`

页面顶部先把“流式执行中”和“部分结果已到达”这两个事实说清楚。

### 7.3 左栏节点区内容

左栏回到广播态：

- `Workspace / Nodes / 4 online · 1 offline`
- `All nodes` 卡片显示 `Broadcast · 4 active`

节点列表同步为执行结果态：

- `sg-prod-01`：`success`
- `us-east-02`：`running`
- `jp-tokyo-01`：`success`
- `de-fra-03`：`success`
- `hk-01`：`offline`

这样用户不需要进入主区，也能先从左栏扫一眼整体进度。

### 7.4 主区卡片结构与先后顺序

主区从上到下为四段：

1. Header 卡  
左侧为 `Target / All nodes (broadcast)`，右侧不是模式切换，而是 `AI Agent` + `running` 组合，说明当前正在执行。

2. `STREAM` 提示卡  
说明任务已分发到 4 个在线节点，系统正在流式汇总：
`stdout`、`stderr`、`exit codes` 和 aggregate state。

3. `Execution stream` 主卡  
顶部显示状态线：
`queued → dispatched → running`

下方逐节点列出当前阶段结果：

- `sg-prod-01 / success · df -h complete · docker healthy`
- `us-east-02 / running · collecting Docker container state`
- `jp-tokyo-01 / success · root usage 61% · nginx healthy`
- `de-fra-03 / success · no anomalies detected`

4. 汇总区  
最下方拆成两张卡：

- `Aggregate`
  - `Total 4`
  - `Success 3`
  - `Running 1`
  - `Offline skipped 1`
- `AI summary`
  - `Cluster is stable overall. sg-prod-01 shows mild disk growth. No write action is recommended yet; inspect the running node after Docker status returns.`

这说明执行态不是单纯把日志堆出来，而是已经开始把结果收束成可读结论。

### 7.5 输入区文案与按钮状态

底部输入框继续保留当前任务原文：

- `检查所有在线节点的磁盘占用和 Docker 状态`

主按钮切换为：

- `新任务`

下方 chips 为：

- `执行中`
- `日志回传中`
- `结果聚合中`

这让页面既能保持执行上下文，也能为后续任务衔接留出入口。

### 7.6 相对其他状态的变化点

执行态是第一个同时出现“过程”和“结果”的页面：

- 过程体现在执行流与节点状态推进
- 结果体现在 Aggregate 与 AI summary

它把“看日志”和“看结论”放在同一屏内完成，没有退化成传统终端页。

---

## 8. 状态流转说明

结合当前画布，主控制台的典型状态推进可整理为：

`Empty -> Plan -> Execution`

适用于：

- 低风险
- 只读
- 不需要人工审批的任务

例如当前画布中的广播巡检任务。

另一条路径为：

`Empty -> Plan -> Approval -> Execution`

适用于：

- 单节点或多节点写操作
- 中高风险动作
- 需要留下明确审计记录的任务

例如当前画布中的 `restart_service(nginx)`。

Execution State 之后，页面在同一屏内通过两类卡片完成收口：

- `Aggregate`：回答总体结果如何
- `AI summary`：回答运维人员下一步应该怎么理解结果

因此当前设计不是“执行页结束后再跳结果页”，而是在执行页内部逐步长出结果层。

---

## 9. 实现对齐要点

如果前端基于本稿落地，建议优先保持以下约束：

- `AI Agent` 是默认主模式，`Direct shell` 不能表现成 MVP 的真实执行主路径
- 广播态和单节点态必须在左栏和主区头部同时表达清楚
- 计划态必须先展示结构化步骤，而不是直接开始执行
- 审批态必须突出风险、影响和审计记录要求
- 执行态必须同时展示执行流、聚合结果和 AI 总结
- 底部输入区始终存在，但按钮文案和状态 chips 需要跟随任务阶段变化

这些要点是当前 Pencil 稿最核心的产品语义，不宜在实现时省略或合并。
