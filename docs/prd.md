PRD：AI 驱动的多节点 VPS 运维控制台（MVP）

1. 产品名称

ToLaTo

2. 产品定位

一个面向多台 VPS / 服务器节点的 AI 运维控制台。
用户通过 Web UI 选择节点并输入自然语言任务，系统由 AI 生成结构化执行计划，经人工审批后，下发到目标节点执行，并实时回传执行结果。

⸻

3. 背景与问题

当前多台 VPS 的运维通常存在这些问题：
	•	节点分散，登录和操作成本高
	•	批量巡检、批量执行效率低
	•	运维知识依赖人工记忆
	•	缺少自然语言入口，非标准命令不易组织
	•	缺少统一审批、审计、结果聚合能力

传统 SSH 工具能解决“连上去执行”，但不能很好解决：
	•	多节点统一编排
	•	AI 辅助分析与建议
	•	计划审批
	•	执行过程可视化
	•	审计留痕

本产品目标是提供一个 AI + 多节点控制 + 审批执行 的统一控制台。

⸻

4. 产品目标

4.1 核心目标

让用户可以通过自然语言或受控命令方式，对多台 VPS 节点进行：
	•	状态查询
	•	日志查看
	•	服务操作
	•	批量任务执行
	•	AI 辅助分析与建议

4.2 MVP 目标

第一阶段只做“可控、可审计、可上线”的最小版本：
	•	支持节点在线管理
	•	支持 WebSocket 长连接
	•	支持 AI 生成执行计划
	•	支持人工 approve 后执行
	•	支持单节点 / 多节点广播
	•	支持实时日志回传
	•	支持执行结果聚合
	•	支持基础审计记录

4.3 非目标

MVP 暂不做：
	•	完整交互式终端
	•	任意裸 shell 直通执行
	•	自动执行高危操作
	•	文件上传分发
	•	自动回滚编排
	•	复杂工作流引擎
	•	多租户企业权限体系

⸻

5. 目标用户

5.1 主要用户
	•	个人开发者
	•	小团队运维人员
	•	独立站 / SaaS 维护者
	•	管理多地域 VPS 的技术负责人

5.2 用户特征
	•	熟悉 Linux 基本操作
	•	有多台服务器管理需求
	•	希望降低重复运维操作成本
	•	接受 AI 参与分析，但希望保留人工审批权

⸻

6. 典型使用场景

场景 1：查看某台机器状态

用户选择 sg-prod-01，输入：

帮我看看这台机器 CPU、内存、磁盘和 Docker 状态

系统：
	1.	AI 生成只读计划
	2.	自动通过策略检查
	3.	执行并回传结果
	4.	汇总展示

场景 2：诊断网站 502

用户输入：

看看 sg-prod-01 为什么 502

系统：
	1.	AI 规划检查 nginx 状态、应用状态、错误日志
	2.	执行只读检查
	3.	汇总原因
	4.	给出建议，如“建议重启 myapp”
	5.	用户 approve 后执行重启

场景 3：批量巡检

用户选择 All nodes，输入：

检查所有在线节点的系统负载和磁盘占用

系统：
	1.	生成广播型只读计划
	2.	执行到所有在线节点
	3.	聚合输出异常节点

场景 4：受控服务重启

用户输入：

重启东京节点的 nginx

系统：
	1.	生成 restart_service(nginx) 计划
	2.	显示风险等级与影响说明
	3.	用户 approve
	4.	下发执行
	5.	返回结果

⸻

7. 核心原则

7.1 安全优先

系统不以“任意 shell 执行”为设计中心，而以“受控动作执行”为中心。
OWASP 对 OS Command Injection 的建议是优先避免将外部输入直接交给 shell，采用参数化、输入校验和 allowlist。 ￼

7.2 审批优先

写操作、高风险操作必须经过 approve，不允许默认自动执行。

7.3 最小权限

节点 client 使用最小权限账号运行；涉及提权时，通过精确约束的 sudoers 策略执行有限命令。sudoers 本身就是用于定义谁可以以何种身份执行哪些命令的策略机制。 ￼

7.4 连接实时

节点与控制端使用 WebSocket 长连接，适合双向实时消息与执行结果流式回传。RFC 6455 将 WebSocket 定义为单个 TCP 连接上的双向通信协议。 ￼

7.5 隔离可选

后续版本支持将部分执行能力放在更受限环境中；例如 Docker rootless mode 允许以非 root 方式运行 daemon 和容器，降低宿主机风险。 ￼

⸻

8. 产品范围（MVP）

8.1 包含

节点管理
	•	节点注册
	•	节点上线 / 离线状态展示
	•	节点元信息展示
	•	hostname
	•	region
	•	os
	•	tags
	•	last_seen

Web 控制台
	•	左侧节点列表
	•	主操作区
	•	AI Agent / Manual Command 模式切换
	•	Target 节点切换
	•	聊天式输入框
	•	执行日志流展示
	•	快捷操作入口

AI 计划生成
	•	自然语言解析
	•	结构化计划生成
	•	风险等级识别
	•	操作说明生成

审批流
	•	计划预览
	•	Approve / Reject / Cancel
	•	审批状态记录

任务执行
	•	单节点执行
	•	广播执行（仅限允许场景）
	•	任务下发
	•	任务取消
	•	超时控制

结果处理
	•	stdout / stderr / exit code 回传
	•	多节点执行结果聚合
	•	失败节点识别
	•	AI 结果摘要

审计
	•	发起人
	•	目标节点
	•	原始输入
	•	生成计划
	•	审批人
	•	执行时间线
	•	执行结果摘要

⸻

8.2 不包含
	•	文件分发
	•	交互式 TTY
	•	会话回放
	•	自动回滚
	•	编排式部署流水线
	•	节点分组权限体系
	•	插件生态

⸻

9. 功能需求

9.1 节点连接管理

描述

每个 VPS 上运行一个 Go 编写的 node client，启动后主动连接 Control Server。

用户价值
	•	无需用户逐台 SSH
	•	节点状态统一管理
	•	适合多地域机器接入

需求点
	•	client 启动后自动注册
	•	定时 heartbeat
	•	断线自动重连
	•	UI 显示在线 / 离线
	•	UI 支持查看节点基础信息

验收标准
	•	client 启动后 10 秒内出现在节点列表
	•	节点断开后 30 秒内 UI 显示 offline
	•	恢复后自动回 online

⸻

9.2 自然语言任务输入

描述

用户可在 UI 中输入自然语言任务，例如：
	•	看看 nginx 状态
	•	检查所有节点磁盘
	•	重启东京节点的服务
	•	看最近 100 行错误日志

需求点
	•	支持中文输入
	•	支持节点上下文
	•	支持单节点和广播目标
	•	支持任务历史显示

验收标准
	•	常见运维类中文输入可被正确识别
	•	AI 可输出可执行计划结构

⸻

9.3 AI 计划生成

描述

AI 不直接执行命令，而是生成结构化计划。

输入
	•	用户自然语言
	•	当前 target
	•	节点元信息
	•	可用工具定义
	•	风险策略

输出示例

{
  "target": ["sg-prod-01"],
  "steps": [
    {
      "action": "restart_service",
      "args": { "service": "nginx" },
      "risk": "medium"
    }
  ],
  "summary": "重启 nginx 服务",
  "requires_approval": true
}

需求点
	•	输出统一 JSON 结构
	•	支持多步骤计划
	•	支持风险等级
	•	支持计划说明文本

验收标准
	•	输出符合 schema
	•	不输出不可识别自由文本作为执行指令
	•	支持失败回退为“无法生成计划”

⸻

9.4 审批流

描述

高风险或写操作必须在执行前展示计划并等待用户批准。

需求点
	•	显示计划卡片
	•	显示 target / action / args / risk / impact
	•	支持 Approve / Reject
	•	审批前不得进入执行状态
	•	审批动作进入审计日志

验收标准
	•	未审批任务不会下发
	•	审批后任务进入 queue
	•	Reject 后任务状态为 cancelled

⸻

9.5 任务队列与下发

描述

审批通过后，由 Control Server 将任务写入队列并分发到节点。

需求点
	•	支持单节点任务
	•	支持广播任务
	•	支持任务状态流转：
	•	planned
	•	waiting_approval
	•	approved
	•	queued
	•	dispatched
	•	running
	•	success
	•	failed
	•	timeout
	•	cancelled

验收标准
	•	任务有唯一 ID
	•	状态可查询
	•	广播任务可追踪每个子任务状态

⸻

9.6 节点执行

描述

node client 接收任务后执行受控动作。

动作类型（MVP）
	•	system_status
	•	disk_usage
	•	memory_usage
	•	docker_ps
	•	service_status
	•	tail_log
	•	restart_service
	•	reload_service
	•	network_check

需求点
	•	不接收任意 shell 字符串作为默认执行模型
	•	action + args 映射到固定命令模板
	•	支持执行超时
	•	支持 stdout / stderr 流式回传
	•	支持 exit code 回传

验收标准
	•	固定动作能稳定执行
	•	超时后任务自动终止并标记 timeout
	•	输出能在 UI 中实时看到

⸻

9.7 手动命令模式（受限）

描述

提供一个比 AI 模式更“接近命令”的入口，但仍受策略控制。

需求点
	•	第一版不做真正交互式 shell
	•	用户输入命令后，后端先做策略解析
	•	高风险命令必须拒绝或要求更高审批
	•	默认不允许广播写命令

验收标准
	•	不允许直接裸透传到 shell
	•	策略命中时可拒绝执行
	•	所有手动命令也记录审计

⸻

9.8 执行结果聚合

描述

对于多节点任务，系统需要聚合结果并给出总结。

需求点
	•	显示成功 / 失败 / 离线跳过数量
	•	支持按节点展开结果
	•	支持 AI 生成摘要
	•	支持失败节点快速定位

验收标准
	•	广播结果可视化完整
	•	单节点结果和多节点结果展示统一

⸻

9.9 审计日志

描述

记录每一次计划、审批与执行事件。

审计字段
	•	task_id
	•	user_id
	•	input_text
	•	target_nodes
	•	plan_json
	•	approval_status
	•	approver_id
	•	execution_started_at
	•	execution_finished_at
	•	final_status
	•	result_summary

验收标准
	•	每个任务都有完整时间线
	•	可按任务查询历史记录

⸻

10. 权限与风控

10.1 风险等级

low

可自动执行：
	•	状态查询
	•	资源查看
	•	日志查看
	•	Docker 状态查看

medium

需用户 approve：
	•	重启服务
	•	reload 服务
	•	小范围写操作

high

需管理员 approve：
	•	修改配置
	•	执行脚本
	•	杀进程
	•	广播型写操作

forbidden

直接禁止：
	•	修改 sudoers
	•	修改 SSH 核心配置
	•	读取敏感凭证文件
	•	任意下载并执行脚本
	•	明显破坏性命令模式

10.2 执行限制
	•	必须 allowlist action
	•	必须 allowlist 参数范围
	•	日志路径受限
	•	服务名受限
	•	广播写操作默认关闭
	•	任意 shell 默认关闭

⸻

11. 前端需求

11.1 页面结构

左侧
	•	节点列表
	•	online / offline 状态
	•	All nodes 入口

中间主区
	•	系统消息
	•	计划卡片
	•	审批卡片
	•	执行日志流
	•	结果摘要

底部输入区
	•	输入框
	•	执行按钮
	•	快捷命令 chips

顶部
	•	当前 Target
	•	模式切换：
	•	AI Agent
	•	Manual Command

⸻

11.2 关键交互

计划预览

展示：
	•	target
	•	steps
	•	risk
	•	estimated impact
	•	requires approval

审批操作
	•	Approve
	•	Reject
	•	Cancel

执行态展示
	•	queued
	•	running
	•	success
	•	failed
	•	timeout

多节点摘要
	•	total
	•	success
	•	failed
	•	offline skipped

⸻

12. 非功能需求

12.1 性能
	•	节点在线状态刷新 < 5 秒
	•	单节点只读命令首响应 < 2 秒
	•	广播 20 节点以内结果可在 10 秒内开始返回

12.2 可用性
	•	client 自动重连
	•	Control Server 支持异常恢复
	•	任务状态持久化

12.3 安全
	•	所有通信走 TLS
	•	节点身份校验
	•	服务端鉴权
	•	审计日志不可轻易篡改
	•	最小权限执行

12.4 兼容性
	•	节点支持 Ubuntu / Debian 优先
	•	CentOS 兼容尽力支持
	•	UI 兼容桌面端主流浏览器

⸻

13. 技术约束

服务端
	•	Golang
	•	HTTP API + WebSocket

节点端
	•	Golang
	•	systemd 托管
	•	开机自启

前端
	•	Vue 3
	•	Vite
	•	Pinia
	•	Vue Router

存储
	•	PostgreSQL：任务 / 节点 / 审计
	•	Redis：队列 / 临时状态 / pubsub

⸻

14. 关键数据对象

Node
	•	id
	•	hostname
	•	region
	•	os
	•	version
	•	tags
	•	status
	•	last_seen

Task
	•	id
	•	type
	•	mode
	•	initiator
	•	target
	•	input_text
	•	plan_json
	•	risk_level
	•	approval_status
	•	final_status
	•	created_at

TaskExecution
	•	id
	•	task_id
	•	node_id
	•	status
	•	started_at
	•	finished_at
	•	exit_code
	•	stdout_tail
	•	stderr_tail

AuditLog
	•	id
	•	task_id
	•	actor_id
	•	action
	•	payload
	•	created_at

⸻

15. 成功指标（MVP）

使用指标
	•	节点接入成功率 > 95%
	•	只读任务执行成功率 > 95%
	•	写操作审批通过后执行成功率 > 90%

体验指标
	•	用户完成一次单节点查询任务时间 < 30 秒
	•	用户完成一次审批后执行任务时间 < 60 秒

稳定性指标
	•	client 异常断连自动恢复率 > 90%
	•	广播任务状态可追踪率 100%

⸻

16. 里程碑建议

M1：基础连通
	•	节点注册
	•	WebSocket 长连接
	•	节点列表
	•	心跳与在线状态

M2：任务执行基础
	•	受控 action 执行
	•	单节点执行
	•	日志回传
	•	结果展示

M3：AI 计划
	•	自然语言转 plan
	•	计划预览
	•	风险等级

M4：审批流
	•	Approve / Reject
	•	状态流转
	•	审计日志

M5：广播与聚合
	•	All nodes
	•	多节点任务下发
	•	聚合结果展示

M6：手动命令模式
	•	受限 manual command
	•	策略校验
	•	审计补全

⸻

17. 风险与依赖

17.1 风险
	•	AI 计划不稳定，需要强 schema 和后端兜底
	•	“manual command” 容易滑向裸 shell
	•	多发行版命令兼容性存在差异
	•	广播执行风险高
	•	节点掉线会影响任务一致性

17.2 依赖
	•	LLM API 可用性
	•	TLS 证书与节点身份体系
	•	服务端数据库与 Redis
	•	节点权限与 sudoers 配置

⸻

18. MVP 验收口径

满足以下条件视为 MVP 完成：
	1.	至少 5 台节点可稳定接入
	2.	UI 可显示节点在线状态
	3.	支持自然语言生成计划
	4.	支持审批后执行
	5.	支持 5 个以上受控动作
	6.	支持单节点与广播只读任务
	7.	支持执行日志流展示
	8.	支持任务历史与审计查看

⸻

19. 一句话版本

ToLaTo 是一个基于 Go + Vue 的 AI 多节点运维控制台，通过 WebSocket 连接各地 VPS 节点，以“计划—审批—执行”的方式完成可控、可审计的运维操作。
