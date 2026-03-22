PRD：AI 驱动的多节点 VPS 聊天式运维控制台（MVP）

1. 产品名称

ToLaTo

2. 产品定位

一个面向多台 VPS / 服务器节点的聊天式 AI 运维控制台。

用户通过 Web UI 进入一个持续会话，无需预先选择目标节点范围，直接用自然语言和 Agent 对话。Agent 可以：

- 连续理解上下文
- 给出解释、建议和下一步动作
- 以结构化 row 形式展示计划、审批和执行结果，并显式展示 tool call / tool result 元信息
- 在用户批准后调用节点执行工具
- 聚合多节点执行结果并输出总结

系统主模型不是“单次任务生成计划后执行”，而是“持续聊天的受控 Agent”。

在这个模型里：

- Control Server 是主 Agent
- NodeAgent 是 `exec_on_nodes` 的远端执行器，不是独立 Agent
- `plan` 是聊天流中的结构化 row，不是产品阶段
- `approval` 是工具结果，不是单独业务入口

⸻

3. 背景与问题

当前多台 VPS 的运维通常存在这些问题：

- 节点分散，登录和操作成本高
- 批量巡检、批量执行效率低
- 运维知识依赖人工记忆
- 缺少自然语言入口，非标准命令不易组织
- 缺少统一审批、审计、结果聚合能力
- 传统工具难以承载“先问、再分析、再执行、再追问”的连续对话流程

传统 SSH 工具能解决“连上去执行”，但不能很好解决：

- 多节点统一编排
- AI 辅助分析与建议
- 连续上下文对话
- 可审批的工具调用
- 执行过程可视化
- 审计留痕

本产品目标是提供一个 AI + 多节点控制 + 审批执行 的统一聊天工作台。

⸻

4. 产品目标

4.1 核心目标

让用户可以通过持续对话，对多台 VPS 节点进行：

- 状态查询
- 日志查看
- 服务操作
- 批量任务执行
- AI 辅助分析与建议
- 审批后执行高风险动作

4.2 MVP 目标

第一阶段只做“可控、可审计、可上线”的最小版本：

- 支持节点在线管理
- 支持 WebSocket 长连接
- 支持聊天式 Agent 会话
- 支持结构化 plan / approval / execution / summary row
- 支持人工 approve 后执行
- 支持单节点 / 多节点 / 全部节点执行
- 支持会话内目标节点语义解析与用户确认
- 支持实时日志回传
- 支持执行结果聚合
- 支持基础审计记录

4.3 非目标

MVP 暂不做：

- 完整交互式终端
- 任意裸 shell 直通执行
- 自动执行高危操作
- 文件上传分发
- 自动回滚编排
- 复杂工作流引擎
- 独立 admin 后台
- 多租户与成员管理
- 独立 session 列表页
- 独立审计中心
- 多租户企业权限体系
- 开放式第三方工具生态

⸻

5. 目标用户

5.1 主要用户

- 个人开发者
- 小团队运维人员
- 独立站 / SaaS 维护者
- 管理多地域 VPS 的技术负责人

5.2 用户特征

- 熟悉 Linux 基本操作
- 有多台服务器管理需求
- 希望降低重复运维操作成本
- 接受 AI 参与分析，但希望保留人工审批权
- 希望在同一个会话里连续追问、调整目标机器和继续执行

⸻

6. 典型使用场景

场景 1：查看某台机器状态

用户进入会话后直接输入：

帮我看看这台机器 CPU、内存、磁盘和 Docker 状态

系统：

1. assistant 结合会话上下文和节点元信息解析目标机器
2. 消息流先追加 `tool_call_meta(list_nodes)` 与 `tool_result_meta`
3. assistant 通过 `target_confirmation row` 请求用户确认是否为 `sg-prod-01`
4. 用户点击确认后，消息流追加一条 `tool_result_meta`
5. 生成只读 `plan row`
6. 自动执行到目标节点
7. 在消息流中持续显示执行结果
8. 输出 `summary row`

场景 2：诊断网站 502

用户输入：

看看 `sg-prod-01` 为什么 502

系统：

1. assistant 解析出目标为 `sg-prod-01`
2. 消息流追加 `tool_call_meta(resolve_target_nodes)` 与 `tool_result_meta`
3. 通过 `target_confirmation row` 向用户确认该节点
4. 用户点击确认后，消息流追加一条 `tool_result_meta`
5. 生成只读 `plan row`
6. 执行只读检查
7. 汇总原因
8. 给出建议，如“建议重启 myapp”
9. 若用户继续要求重启，则生成 `approval row`
10. 用户点击 approve 后，再追加一条 `tool_result_meta` 并执行

场景 3：批量巡检

用户输入：

检查所有在线节点的系统负载和磁盘占用

系统：

1. assistant 解析出目标是“所有在线节点”
2. 消息流追加 `tool_call_meta(list_nodes)` 与 `tool_result_meta`
3. 通过 `target_confirmation row` 提示将对 `All online nodes` 执行
4. 用户点击确认后，消息流追加一条 `tool_result_meta`，标记本次为多节点操作
5. 生成广播型 `plan row`
6. 对所有在线节点执行
7. 聚合异常节点
8. 输出批量执行总结

场景 4：受控服务重启

用户输入：

重启东京节点的 nginx

系统：

1. assistant 解析 `东京节点` 为 `jp-tokyo-01`
2. 消息流追加 `tool_call_meta(resolve_target_nodes)` 与 `tool_result_meta`
3. 通过 `target_confirmation row` 请求用户确认
4. 用户点击确认后，消息流追加一条 `tool_result_meta`
5. assistant 解释风险和影响
6. 生成 `plan row`
7. 生成 `approval row`
8. 用户点击 approve 后，消息流追加一条 `tool_result_meta`
9. Agent 调用节点执行工具
10. 返回执行结果和总结

场景 5：会话中沿用已确认目标

用户上一轮已经确认目标为东京和新加坡节点，然后输入：

先重启 nginx 试试看

系统：

1. assistant 明确说明将沿用上一轮已确认的东京和新加坡节点
2. 若用户未否认，则把本轮操作绑定到这两个节点
3. 消息流追加一条 `tool_result_meta`，显示 `2 nodes confirmed`

⸻

7. 核心原则

7.1 安全优先

系统不以“任意 shell 执行”为设计中心，而以“受控工具执行”为中心。
OWASP 对 OS Command Injection 的建议是优先避免将外部输入直接交给 shell，采用参数化、输入校验和 allowlist。

7.2 审批优先

写操作、高风险操作必须经过 approve，不允许默认自动执行。
审批对象是 Agent 发起的工具调用，而不是自由文本命令。

7.3 最小权限

NodeAgent 使用最小权限账号运行；涉及提权时，通过精确约束的 sudoers 策略执行有限命令。

7.4 连接实时

节点与控制端使用 WebSocket 长连接，适合双向实时消息与执行结果流式回传。

7.5 聊天优先

产品主交互是持续会话，而不是一次性提交任务。
Agent 可以连续回复、追问、建议和等待审批，再继续执行。

⸻

8. 产品范围（MVP）

8.1 包含

节点管理

- 节点注册
- 节点上线 / 离线状态展示
- 节点元信息展示
- hostname
- region
- os
- tags
- last_seen

Web 控制台

- `Console / Nodes / History / Settings` 四个一级页面
- 左侧 session 列表（仅存在于 `Console`）
- 主操作区
- 当前会话目标上下文 / 待确认目标提示
- 聊天式输入框
- Row-based 消息流展示
- 内联执行日志展示
- 快捷操作入口
- 节点列表页与独立节点详情页
- 任务历史查看
- 模型配置、账户安全与偏好设置

聊天式 Agent

- 连续上下文对话
- 自然语言解析
- 目标节点语义解析
- 目标节点确认
- assistant 文本回复
- 结构化 plan / approval / execution / summary row
- `tool_call_meta` / `tool_result_meta` 元信息展示

工具化执行

- `resolve_target_nodes`
- `request_target_confirmation`
- `propose_plan`
- `request_approval`
- `exec_on_nodes`
- 会话目标上下文更新

审批流

- approval row
- Approve / Reject / Cancel
- 审批状态记录

执行与结果

- 单节点执行
- 多节点执行
- 全部节点执行
- 多节点 fan-out 下发
- stdout / stderr / exit code 回传
- 多节点执行结果聚合
- AI 结果摘要

审计

- 发起人
- 目标节点范围
- 原始输入
- row 类型
- 审批动作
- 执行时间线
- 执行结果摘要

8.2 不包含

- 文件分发
- 交互式 TTY
- 会话回放
- 自动回滚
- 编排式部署流水线
- 节点分组权限体系
- 独立 admin 后台
- 多租户协作空间
- 独立 session 列表页
- 独立审计中心
- 插件生态

⸻

9. 功能需求

9.1 节点连接管理

描述

每个 VPS 上运行一个 Go 编写的 NodeAgent，启动后主动连接 Control Server。

用户价值

- 无需用户逐台 SSH
- 节点状态统一管理
- 适合多地域机器接入

需求点

- NodeAgent 启动后自动注册
- 定时 heartbeat
- 断线自动重连
- UI 显示在线 / 离线
- UI 支持查看节点基础信息

验收标准

- NodeAgent 启动后 10 秒内出现在节点列表
- 节点断开后 30 秒内 UI 显示 offline
- 恢复后自动回 online

⸻

9.2 线程式聊天输入

描述

用户在 UI 中进入一个 thread，与 Agent 连续对话，例如：

- 看看 nginx 状态
- 检查所有节点磁盘
- 只对东京和新加坡节点执行
- 看最近 100 行错误日志
- 重启东京节点的服务

需求点

- 支持中文输入
- 支持多轮上下文
- 支持线程历史显示
- 支持从自然语言中解析目标节点
- 支持目标节点确认与二次确认
- 支持会话内显示最近一次已确认目标
- 支持单节点、多节点、全部节点

验收标准

- 用户可在同一个会话里连续提问与执行
- 常见运维类中文输入可被正确识别
- 当输入中包含机器语义时，系统能先给出候选目标并请求确认
- 线程历史可恢复当前上下文

⸻

9.3 Agent 回复与结构化 Row

描述

Agent 不要求每轮都直接执行，也不要求每轮都生成 plan。
Agent 可以输出普通文本回复，也可以输出结构化 row。

Row 类型

- assistant text row
- target confirmation row
- tool_call_meta row
- tool_result_meta row
- plan row
- approval row
- execution row
- summary row

输入

- 用户消息
- 当前会话已确认目标上下文
- 当前轮目标解析结果
- 节点元信息
- 可用工具定义
- 风险策略
- 对话历史

需求点

- 支持 assistant 纯文本回复
- 支持结构化 plan row
- 支持结构化说明文本
- 支持 `tool_call_meta` / `tool_result_meta` 小字元信息
- 按钮触发的确认、审批默认不新增 user row
- 支持无法执行时给出原因和建议

验收标准

- assistant 可只回复文本，不强制产出执行计划
- plan row 能展示 target / steps / risk / impact
- 普通 tool 调用默认展示 `tool_call_meta` 与 `tool_result_meta`
- 用户点击确认或 Approve 后，消息流中只追加 `tool_result_meta`，而不是新增 user row
- 无法执行时可回退为“无法完成此操作”的解释性回复

⸻

9.4 工具化审批流

描述

高风险或写操作必须在执行前展示 approval row 并等待用户批准。

审批对象是一次 `request_approval` 工具调用。

审批粒度

- 一次批量执行意图一次审批
- 不按节点逐个审批

需求点

- 显示 approval row
- 显示 target / action / args / risk / impact
- 支持 Approve / Reject / Cancel
- 审批前不得进入执行状态
- 审批动作进入审计日志
- 审批按钮点击后，消息流中只追加一条弱化的 `tool_result_meta`

验收标准

- 未审批的危险操作不会下发
- 审批后 Agent 才能继续调用执行工具
- Reject 后本次执行意图结束，不会进入节点执行

⸻

9.5 节点执行工具与 fan-out 下发

描述

审批通过或无需审批后，Control Server 仅在目标节点已确认的前提下，通过 `exec_on_nodes` 工具将执行意图拆分并下发到目标节点。

NodeAgent 只负责执行受控动作，不负责规划、审批或聊天。

需求点

- 支持单节点执行
- 支持多节点执行
- 支持全部节点执行
- 支持由目标确认结果驱动执行
- 支持单次工具调用覆盖当前会话已确认目标
- 目标未确认时不得进入执行
- 支持节点级 fan-out 下发
- 支持执行状态流展示

执行投影状态

- queued
- dispatched
- running
- success
- failed
- timeout
- cancelled

验收标准

- 每次执行有唯一追踪 ID
- 广播执行可追踪每个节点子执行状态
- 执行投影可在 UI 中按节点展开

⸻

9.6 节点执行

描述

NodeAgent 接收受控动作后执行。

动作类型（MVP）

- system_status
- disk_usage
- memory_usage
- docker_ps
- service_status
- tail_log
- restart_service
- reload_service
- network_check

需求点

- 不接收任意 shell 字符串作为默认执行模型
- action + args 映射到固定命令模板
- 支持执行超时
- 支持 stdout / stderr 流式回传
- 支持 exit code 回传

验收标准

- 固定动作能稳定执行
- 超时后执行自动终止并标记 timeout
- 输出能在 UI 中实时看到

⸻

9.7 受限命令式输入

描述

产品只保留 AI Agent 主模式，不再单独暴露 `Manual Command` 模式切换。
但用户仍然可以在消息中使用更接近命令的表达方式，例如：

- 重启 nginx
- 看最近 100 行日志
- 检查 docker ps

系统会把这类输入当作 Agent 可理解的一类消息风格，而不是裸 shell 透传。

需求点

- 第一版不做真正交互式 shell
- 用户输入“命令式表达”后，后端仍先做策略解析
- 高风险表达必须拒绝或要求审批
- 默认不允许广播写命令

验收标准

- 不允许直接裸透传到 shell
- 策略命中时可拒绝执行
- 所有命令式输入也记录审计

⸻

9.8 执行结果聚合

描述

对于多节点执行，系统需要聚合结果并给出总结。

需求点

- 显示成功 / 失败 / 离线跳过数量
- 支持按节点展开结果
- 支持 AI 生成摘要
- 支持失败节点快速定位
- 支持在同一线程中继续追问结果

验收标准

- 多节点结果可视化完整
- 单节点结果和多节点结果展示统一
- 用户可在总结后继续追问下一步

⸻

9.9 审计日志

描述

记录每一次会话、plan / approval / execution row、tool call 与 tool result 事件。

审计字段

- thread_id
- execution_id
- user_id
- input_text
- target_resolution
- target_confirmation
- confirmed_targets
- tool_call
- tool_result
- approval_status
- approver_id
- execution_started_at
- execution_finished_at
- final_status
- result_summary

验收标准

- 每个执行意图都有完整时间线
- 可按 thread 或 execution 查询历史记录

⸻

10. 权限与风控

10.1 风险等级

low

可自动执行：

- 状态查询
- 资源查看
- 日志查看
- Docker 状态查看

medium

需用户 approve：

- 重启服务
- reload 服务
- 小范围写操作

high

需更高等级确认，MVP 中默认阻断或保留为策略位：

- 修改配置
- 执行脚本
- 杀进程
- 广播型写操作

forbidden

直接禁止：

- 修改 sudoers
- 修改 SSH 核心配置
- 读取敏感凭证文件
- 任意下载并执行脚本
- 明显破坏性命令模式

10.2 执行限制

- 必须 allowlist action
- 必须 allowlist 参数范围
- 日志路径受限
- 服务名受限
- 广播写操作默认关闭
- 任意 shell 默认关闭

10.3 审批规则

- 低风险只读动作可自动执行
- 写操作和高风险动作通过 `request_approval`
- 批量节点执行默认一次审批
- NodeAgent 不具备绕过审批直接执行的能力

⸻

11. 前端需求

11.1 产品信息架构

MVP 只包含以下 4 个一级页面：

- `/console`
  - 主控制台
  - 承载 session 列表、聊天时间线、目标确认、计划、审批、执行与总结
- `/nodes`
  - 全量节点列表页
  - 承载搜索、筛选、状态浏览和跳转入口
- `/nodes/:id`
  - 节点详情页
  - 承载完整节点信息、最近心跳、资源摘要与最近任务
- `/history`
  - 任务历史页
  - 承载 task 级历史、审批记录、执行聚合与关联审计信息
- `/settings`
  - 统一设置页
  - 承载模型配置、账户安全与偏好设置

边界约束：

- 不做多租户
- 不做 admin 后台
- 不做独立 session 列表页
- 不做独立审计中心
- `Direct shell` 仍不是可执行入口，只保留受限模式占位说明

11.2 Console 页面结构

顶部

- 当前会话目标上下文
- session 标题 / 状态
- 全局连接状态
- 模式切换：`AI Agent / Direct shell`

左侧

- session 列表
  - 支持滚动
  - 支持未读、运行中、需关注状态
- 当前会话节点上下文区
  - 候选节点
  - 已确认目标
  - 节点健康摘要
  - `All online nodes` 候选入口

中间主区

- system / assistant / user 消息
- target confirmation row
- tool_call_meta row
- tool_result_meta row
- plan row
- approval row
- execution row
- summary row

底部输入区

- 聊天输入框
- 发送按钮
- 快捷操作 chips

约束：

- `session` 列表只存在于 `Console`
- 节点视图在控制台中只承担当前上下文镜像，不承担全量资产管理
- `Direct shell` 不提供真实执行输入框

实时交互规则：

- `Console` 通过一条 `ws/ui` 连接承载 session 列表、session snapshot、消息提交和增量事件
- 连接建立后，前端先等待 `connection.ready`
- 前端进入或切换 session 时，必须请求 `session.snapshot`
- 前端需显式发送 `subscriptions.update` 指定当前 active session 与 watch sessions
- active session 接收完整 timeline 级事件，watch session 只接收 summary 级事件
- 历史更早 rows 通过 `session.rows.request` 分页获取，不依赖全量事件回放
- 断线重连后，前端应重新请求 `sessions.list`、当前 `session.snapshot` 和 `subscriptions.update`
- 模型原始 `thinking` 和 `content` 都属于前端可见交互面
- 后端需把 OpenAI 原始 SSE 事件透传给前端，前端按流式事件实时展示 reasoning 与 content
- 最终 `assistant_text / plan / approval / execution / summary` row 继续保留，用于稳定展示、审计与恢复

11.3 Nodes 页面

页面目标：

- 浏览全部节点资产
- 快速搜索与筛选
- 进入节点详情页
- 从节点结果回到控制台执行或追问

核心模块：

- 搜索框
- 状态筛选：online / offline / busy / idle
- region / tag 过滤
- 节点统计摘要卡
- 节点列表

列表字段：

- hostname
- region
- os
- tags
- status
- last_seen
- busy / idle
- 最近资源摘要

关键交互：

- 点击节点进入 `/nodes/:id`
- 次级操作支持“在控制台中打开该节点”

11.4 Node Detail 页面

页面目标：

- 提供单节点完整上下文
- 让用户在不进入时间线的情况下快速判断节点状态

核心模块：

- 基础信息：hostname / region / os / tags / version
- 当前状态：online / offline / busy / idle
- 最近心跳：CPU / 内存 / 磁盘摘要
- 最近任务：最近 3-10 条
- 风险提示与快捷入口

关键交互：

- 返回控制台
- 以当前节点作为控制台目标继续操作

11.5 History 页面

页面目标：

- 查看 task 级历史
- 追溯 plan、approval、execution 与聚合结果
- 在单页面内查看关联审计信息

核心模块：

- 任务列表
- 状态与审批筛选
- task 详情区

任务列表字段：

- 时间
- input_text 摘要
- target 范围
- status
- approval_status
- aggregate 摘要

详情区展示：

- plan
- approval 记录
- execution 聚合
- 节点级结果
- `tool_call_meta`
- `tool_result_meta`
- 审计字段

约束：

- `history` 只做任务历史页
- 不单独拆审计页
- 审计信息来自 task 详情或其关联审计数据

11.6 Settings 页面

页面目标：

- 管理模型接入
- 管理账户安全
- 管理个人偏好

信息结构：

- `model_config`
  - provider
  - model
  - endpoint
  - API key
  - temperature
  - max tokens
  - timeout
  - 连接测试
- `account_security`
  - 改密码
  - 当前登录信息
  - 登出其他会话
- `preferences`
  - 语言
  - 时间格式
  - 默认视图
  - 执行结果展开偏好

11.7 关键交互

消息流

- user message
- assistant text
- target confirmation row
- tool_call_meta row
- tool_result_meta row
- plan row
- approval row
- execution row
- summary row

审批操作

- Approve
- Reject
- Cancel

执行态展示

- queued
- running
- success
- failed
- timeout

多节点摘要

- total
- success
- failed
- offline skipped

节点范围交互

- 单节点确认
- 多节点确认
- 全节点确认
- 清除或沿用当前会话目标上下文

⸻

12. 非功能需求

12.1 性能

- 节点在线状态刷新 < 5 秒
- 单节点只读命令首响应 < 2 秒
- 广播 20 节点以内结果可在 10 秒内开始返回

12.2 可用性

- NodeAgent 自动重连
- Control Server 支持异常恢复
- 线程与执行状态持久化

12.3 安全

- 所有通信走 TLS
- 节点身份校验
- 服务端鉴权
- 审计日志不可轻易篡改
- 最小权限执行

12.4 兼容性

- 节点支持 Ubuntu / Debian 优先
- CentOS 兼容尽力支持
- UI 兼容桌面端主流浏览器

⸻

13. 技术约束

服务端

- Golang
- HTTP API + WebSocket

节点端

- Golang
- systemd 托管
- 开机自启

前端

- Vue 3
- Vite
- Pinia
- Vue Router

存储

- PostgreSQL：thread / node / execution / audit
- Redis：队列 / 临时状态 / pubsub

13.1 当前前后端交互 Schema

单独维护在：

- [api_contract.md](/Users/wentx/momaek/src/tolato/docs/api_contract.md)

内容包括：

- 后端当前真实 HTTP / WebSocket schema
- 前端当前 Zod 合同
- 当前前后端 contract mismatch

PRD 目标接口模型以 thread/chat 为准，具体落地 schema 允许分阶段演进。

⸻

14. 关键数据对象

Node

- id
- hostname
- region
- os
- version
- tags
- status
- last_seen

Thread

- id
- mode
- initiator
- active_target_context
- status
- created_at
- updated_at

ThreadMessage

- id
- thread_id
- role
- content
- kind
- created_at

ToolCall

- id
- thread_id
- kind
- input
- status
- created_at
- updated_at

ToolResult

- id
- thread_id
- tool_call_id
- status
- payload
- created_at

ExecutionProjection

- id
- thread_id
- node_id
- status
- started_at
- finished_at
- exit_code
- stdout_tail
- stderr_tail

AuditLog

- id
- thread_id
- actor_id
- action
- payload
- created_at

⸻

15. 成功指标（MVP）

使用指标

- 节点接入成功率 > 95%
- 只读执行成功率 > 95%
- 写操作审批通过后执行成功率 > 90%

体验指标

- 用户完成一次单节点查询时间 < 30 秒
- 用户完成一次审批后执行时间 < 60 秒
- 用户可在同一会话中连续完成“提问 -> 批准 -> 继续追问”

稳定性指标

- NodeAgent 异常断连自动恢复率 > 90%
- 多节点执行状态可追踪率 100%

⸻

16. 里程碑建议

M1：基础连通

- 节点注册
- WebSocket 长连接
- 节点列表
- 心跳与在线状态

M2：执行基础

- 受控 action 执行
- 单节点执行
- 日志回传
- 结果展示

M3：聊天式 Agent

- thread 创建
- 多轮消息
- assistant 文本回复
- 目标解析与确认

M4：结构化 Row 与审批

- plan row
- approval row
- Approve / Reject
- 审计日志

M5：多节点聚合

- All online nodes 确认与执行
- 多节点 fan-out
- 聚合结果展示

M6：对话式运维闭环

- 执行后 summary
- 继续追问
- 受限命令式输入

⸻

17. 风险与依赖

17.1 风险

- Agent 回复质量不稳定，需要强工具边界和后端兜底
- 命令式输入容易滑向裸 shell
- 多发行版命令兼容性存在差异
- 广播执行风险高
- 节点掉线会影响执行一致性
- 聊天式上下文若处理不当，可能导致错误继承已确认目标

17.2 依赖

- LLM API 可用性
- TLS 证书与节点身份体系
- 服务端数据库与 Redis
- 节点权限与 sudoers 配置

⸻

18. MVP 验收口径

满足以下条件视为 MVP 完成：

1. 至少 5 台节点可稳定接入
2. UI 可显示节点在线状态
3. 支持 thread/chat 形式的多轮会话
4. assistant 可输出文本、plan row 和 approval row
5. 支持审批后执行
6. 支持 5 个以上受控动作
7. 支持单节点、多节点与全部节点执行
8. 支持执行日志流展示
9. 支持会话历史与审计查看

⸻

19. 一句话版本

ToLaTo 是一个基于 Go + Vue 的 AI 多节点运维聊天工作台，通过持续会话、结构化 row、审批后执行和多节点结果聚合，完成可控、可审计的运维操作。
