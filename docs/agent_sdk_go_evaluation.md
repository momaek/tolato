# `agent-sdk-go` 技术评估

## 1. 背景与评估目标

本文档用于评估 [`Ingenimax/agent-sdk-go`](https://github.com/Ingenimax/agent-sdk-go) 是否适合作为 ToLaTo 当前 Agent 能力的基础组件，重点不是泛泛讨论一个通用 agent framework，而是对照 ToLaTo 当前已确定的运行模型进行能力对标。

本评估的现有基线来源如下：

- [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md)
  - 定义 ToLaTo 当前后端的核心结论：主运行模型是“自研显式状态机驱动的 Agent Loop Runtime”
- [internal/server/app/runtime/service.go](/Users/wentx/momaek/src/tolato/internal/server/app/runtime/service.go)
  - 体现当前 runtime 的输入输出、状态迁移与暂停/恢复路径
- [internal/server/infra/llm/openai/provider.go](/Users/wentx/momaek/src/tolato/internal/server/infra/llm/openai/provider.go)
  - 体现当前 OpenAI provider 对 tool calling、流式事件、reasoning/content 聚合的处理方式

本文档重点回答以下问题：

- `agent-sdk-go` 是否适合替代 ToLaTo 当前的 LLM Loop / Agent Runtime
- 它对人审暂停、异步恢复、持久化状态、timeline/audit 的覆盖程度如何
- 它是否能返回 thinking / reasoning / final content，调用方分别能拿到什么
- 如果未来想试用它，合理的接入边界应该在哪里

本文档不试图：

- 重写 ToLaTo 现有架构结论
- 把 `agent-sdk-go` 作为默认实现方向进行推销
- 讨论所有 agent framework 的优劣

---

## 2. 当前 ToLaTo 的关键需求边界

ToLaTo 当前后端架构的核心不是“会调工具的 agent”，而是“可暂停、可恢复、可审计、可绑定异步 execution 的控制面 runtime”。这一点在 [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md) 中已经明确。

当前 runtime 不可丢的能力包括：

- 显式 session 状态机
  - session 不是单纯的聊天历史，而是运行中的业务状态容器
  - runtime 需要识别 `running`、`paused_wait_target_confirmation`、`paused_wait_approval`、`waiting_async_execution`、`completed`、`failed` 等状态
- `PendingAction` 语义
  - 支持 target confirmation
  - 支持 approval
  - 模型或 tool 返回的不是“给用户一句话”，而是一个可恢复的暂停点
- 异步 execution 绑定与恢复
  - tool 可以启动异步执行并返回 `AsyncExecutionStarted`
  - session 会切到 `waiting_async_execution`
  - execution 完成后，runtime 需要在同一 session 上恢复 loop 并继续总结
- 强事实持久化
  - `sessions`、`thread_messages`、`timeline_rows`、`tool_calls`、`tool_results`、`tasks`、`executions`、`audits` 都是事实数据
  - `PostgreSQL` 是唯一事实源，`Redis` 只是临时能力
- timeline / audit 可追踪
  - 每次 assistant text、tool call、tool result、target confirmation、approval、execution summary 都有明确 timeline 行
  - 这不是“memory”，而是可恢复、可审计、可回放的系统事实
- provider continuation state / conversation state 可恢复
  - 当前 runtime 的模型输入中包含 `ProviderState`、`CurrentTask`、`PendingAction`、`ActiveTargetContext`
  - 服务重启后仍需能够通过事实数据重建上下文

从当前实现看，这些边界已经体现在 runtime 的核心数据结构中：

- `ModelTurnInput` 包含 `SessionID`、`Conversation`、`ActiveTargetContext`、`PendingAction`、`ProviderState`、`CurrentTask`
- `ToolResult` 包含 `WaitForUser`、`PendingActionType`、`PendingActionPayload`、`AsyncExecutionStarted`
- `continueLoop()` 中显式处理 tool call、tool result、暂停、继续和完成

因此，本次评估的判断标准不是“这个 SDK 能不能跑 tools”，而是“它能否覆盖 ToLaTo runtime 的业务状态语义”。

---

## 3. `agent-sdk-go` 能力概览

结合其公开源码能力，`agent-sdk-go` 确实提供了一套完整的通用 agent 能力集合，但这些能力和 ToLaTo 当前 runtime 需求只部分重叠。

### 3.1 原生支持的能力

- tool calling
  - 支持模型调用工具并在客户端内部做迭代 tool loop
- MCP
  - 支持直接接入 MCP server，含 lazy MCP tools
- sub-agent
  - 支持把子 agent 包装成 tools，由主 agent 调用
- streaming
  - 支持 LLM streaming 和 agent 级 streaming
  - 支持 `thinking`、`tool_call`、`tool_result` 等事件类型
- memory
  - 支持内存 buffer、Redis memory、vector retriever 等会话记忆能力
- execution plan approval
  - 在 `requirePlanApproval=true` 时，会先生成 execution plan，再等待人工批准
- provider support
  - 支持 OpenAI、Anthropic、Azure OpenAI、Gemini 等 provider

### 3.2 仅有相邻能力，但不能直接替代 ToLaTo runtime 的部分

- plan approval 不是 runtime pause
  - 它的人工介入主要是“执行前审批计划”
  - 不是 ToLaTo 当前的 `PendingAction` / `wait_for_user` 这类运行中暂停点
- memory 不是事实层
  - 它可以保存对话和 tool result
  - 但不能替代 ToLaTo 的 `timeline_rows`、`tool_calls`、`tool_results`、`audits`、`tasks`、`executions`
- internal tool loop 不是可恢复状态机
  - 它能在一次 agent 调用内处理多轮 tool 调用
  - 但没有 ToLaTo 当前 runtime 那种跨暂停点、跨 execution 生命周期的显式恢复语义
- execution plan store 不是持久化状态
  - 其 `executionplan.Store` 是进程内内存结构
  - 重启后不满足 ToLaTo 所需的恢复要求

### 3.3 对 ToLaTo 的意义

如果只看“模型侧 agent 能力”，`agent-sdk-go` 是一套成熟的通用组件；如果看“ToLaTo 主控制面的 runtime 语义”，它并不是一个可直接替换当前自研 runtime 的现成方案。

---

## 4. 对标结论

下表从 ToLaTo 当前需求出发，对 `agent-sdk-go` 做逐项对标。

| 能力项 | ToLaTo 当前需求 | `agent-sdk-go` 支持情况 | 差距 / 备注 |
| --- | --- | --- | --- |
| 单轮模型调用 | 需要 | 支持 | 可满足 |
| tool loop | 需要显式 `模型 -> tool -> 再续跑` | 支持内部 iterative tool loop | 可部分满足，但 loop 主要封装在 SDK 内部，不等价于 ToLaTo runtime 状态机 |
| 人工介入前置审批 | 需要 | 支持 | 通过 execution plan approval 支持“执行前审批” |
| tool 运行中暂停并恢复 | 需要 | 不支持 | 没有 ToLaTo 当前 `WaitForUser` / `PendingAction` 一等语义 |
| 异步执行绑定与恢复 | 需要 | 不支持 | 没有 `AsyncExecutionStarted`、execution finished 后恢复 loop 的标准机制 |
| 会话状态持久化 | 需要 durable state | 部分支持 | 有 memory，但 memory 不是 session runtime state |
| timeline / audit 持久化 | 需要 | 不支持 | 没有对应 ToLaTo 的事实表与审计语义 |
| provider-specific continuation state | 需要 | 不支持 | 没有 ToLaTo 当前 `ProviderState` 对应的外部恢复接口 |
| thinking / reasoning 可见性 | 需要区分 provider 和调用模式 | 部分支持 | 依赖 provider，且主要通过 streaming 暴露 |
| streaming 事件暴露 | 需要 | 支持 | 可暴露 content、thinking、tool_call、tool_result 等事件 |
| 重启恢复 | 需要 | 不满足 | execution plan store 为进程内内存，不满足主链路要求 |

### 4.1 结论解释

- 如果目标是替代“模型侧单轮 agent executor”，`agent-sdk-go` 有可用价值。
- 如果目标是替代“ToLaTo 主 runtime”，它不满足当前关键需求。
- 最大差距不在 tool calling，而在 HITL、异步恢复、持久化状态和审计事实层。

因此，对标结论是：

- `agent-sdk-go` 不适合直接替代当前自研 Runtime
- 它最多适合作为局部能力借用或实验性组件
- HITL 与恢复语义是主要不匹配点
- thinking 可见性依赖 provider 且主要通过 streaming 暴露

---

## 5. 人工介入与 thinking 能力专项说明

本节单独说明两个最容易被误判的点。

### 5.1 HITL：它支持的是 plan-level approval，不是 runtime-level pause

`agent-sdk-go` 确实支持人工介入，但它的核心路径是：

1. agent 发现当前请求需要工具执行
2. 先生成一个 execution plan
3. 向用户返回“请批准这个计划”
4. 用户批准后，再顺序执行 plan 中的工具

这属于“执行前审批”，可以概括为 plan-level approval。

它不等价于 ToLaTo 当前 runtime 中的以下语义：

- tool 执行到一半返回 `WaitForUser`
- runtime 设置 `PendingAction`
- session 进入 `paused_wait_target_confirmation` 或 `paused_wait_approval`
- 用户动作到达后，从同一个 session runtime 上恢复
- 异步 execution 完成后继续总结

因此，这里的关键判断是：

- `agent-sdk-go` 的 plan approval 归类为“执行前审批”
- 它不能等价于 ToLaTo 的 `PendingAction`
- 它不能替代当前 target confirmation / approval / waiting async execution 这三类暂停点

另外，其 `executionplan.Store` 目前是进程内内存存储，不满足 ToLaTo 对重启恢复和事实持久化的要求。

### 5.2 thinking / reasoning：调用方能拿到什么

`agent-sdk-go` 区分“最终内容”和“流式 reasoning / thinking 事件”，但不是所有 provider 都会把 reasoning 文本作为稳定的独立字段返回。

#### OpenAI

- `Run()` / `Generate()`：
  - 调用方能拿到最终 `content`
- `RunDetailed()` / `GenerateDetailed()`：
  - 能拿到最终 `Content`
  - 能拿到 token usage
  - reasoning model 下可以拿到 `ReasoningTokens` 数量
- `RunStream()` / `GenerateStream()`：
  - 当前 SDK 明确认为 OpenAI reasoning model 有内部 reasoning，但不会稳定暴露原始 thinking 文本
  - 因此通常拿不到独立 reasoning content

结论：

- OpenAI：能拿最终 content 和 reasoning token 数
- OpenAI：不能稳定拿 reasoning 文本

#### Anthropic

- `Run()` / `Generate()`：
  - 调用方拿到最终内容
- `RunDetailed()` / `GenerateDetailed()`：
  - 调用方拿到最终 `Content` 和 usage
  - 不提供独立 thinking 文本字段
- `RunStream()` / `GenerateStream()`：
  - 如果启用 extended thinking，SDK 会把 thinking block 解析为 `StreamEventThinking`
  - 调用方可以在 streaming 事件中看到 thinking 内容

结论：

- Anthropic：streaming 可拿 thinking
- Anthropic：非流式只返回 final content

#### Gemini

- `Run()` / `Generate()`：
  - 调用方拿到最终内容
- `RunDetailed()` / `GenerateDetailed()`：
  - SDK 在内部能识别 thinking content
  - 但非流式接口明确只返回 final response content
- `RunStream()` / `GenerateStream()`：
  - 如果启用了 thoughts，SDK 会发 `StreamEventThinking`
  - 调用方可以在 streaming 中拿到 thinking 内容

结论：

- Gemini：streaming 可拿 thinking
- Gemini：非流式只返回 final content

### 5.3 对 ToLaTo 的含义

如果 ToLaTo 需要像当前 OpenAI provider 一样，把 reasoning / content 流式分发到 UI，那么：

- `agent-sdk-go` 在 Anthropic 和 Gemini 上可以较好支持
- `agent-sdk-go` 在 OpenAI 上只能稳定提供 final content 和 reasoning token 数，不适合作为“原始 reasoning 文本可视化”的基础

这意味着它不能直接替代 ToLaTo 当前对 OpenAI 流式事件的细粒度处理思路。

---

## 6. 最终结论与建议

最终结论如下：

- 不建议用 `agent-sdk-go` 替代 ToLaTo 主 runtime
- 可作为模型侧 agent executor 的实验性组件
- 若试用，只能放在 `infra/llm` 或某个隔离的新 agent path，不能接管 session/runtime/execution 主链路

### 6.1 为什么不建议替代主 runtime

原因不是它“不够强”，而是它解决的问题和 ToLaTo 主控制面要解决的问题不一致。

`agent-sdk-go` 更擅长的是：

- 提供通用 agent 能力
- 封装多 provider、streaming、tools、MCP、sub-agent、memory
- 帮助快速构建一个会调工具的 agent

ToLaTo 当前 runtime 真正的复杂度则在：

- session 状态机
- user confirmation / approval 边界
- async execution 生命周期
- timeline / audit 持久化
- PostgreSQL 事实层恢复

这些能力目前都仍然需要 ToLaTo 自己控制。

### 6.2 可以考虑的试用边界

如果未来要试用 `agent-sdk-go`，合理的边界只有两种：

- 作为 `infra/llm` 层的实验性 provider adapter
  - 只把它当成模型侧 tool loop executor
  - session state、timeline、approval、execution 仍由 ToLaTo runtime 负责
- 作为隔离的 agent path
  - 用于研究型 agent、MCP-heavy agent、一次性流程 agent
  - 与 ToLaTo 主会话 runtime 隔离，不共享主控制面语义

### 6.3 不建议的接入方式

以下方向不建议推进：

- 用 `agent-sdk-go` 直接替代 `internal/server/app/runtime`
- 用其 memory 替代 ToLaTo 的事实层
- 用其 plan approval 替代 ToLaTo 当前的 `PendingAction` 语义
- 让其接管主 session 的暂停、恢复、execution 绑定和总结

当前阶段的技术建议应保持明确：

- `agent-sdk-go` 不适合直接替代当前自研 Runtime
- 它最多适合作为局部能力借用或实验性组件
- 主链路仍应坚持当前自研 runtime 边界

---

## 7. 附录：关键证据

以下证据用于支撑本文结论，便于后续复核。

### 7.1 ToLaTo 当前架构与 runtime 证据

- [docs/backend_architecture_manual_loop.md](/Users/wentx/momaek/src/tolato/docs/backend_architecture_manual_loop.md)
  - 明确 ToLaTo 后端核心是自研显式状态机驱动的 Agent Loop Runtime
  - 强调关键复杂度在 session 状态、审批边界、异步 execution 生命周期、timeline/audit 落库和重启恢复
- [internal/server/app/runtime/service.go](/Users/wentx/momaek/src/tolato/internal/server/app/runtime/service.go)
  - `ModelTurnInput` 包含 `ActiveTargetContext`、`PendingAction`、`ProviderState`、`CurrentTask`
  - `ToolResult` 包含 `WaitForUser`、`PendingActionType`、`PendingActionPayload`、`AsyncExecutionStarted`
  - `continueLoop()` 明确处理 tool call、tool result、暂停、恢复与完成
- [internal/server/infra/llm/openai/provider.go](/Users/wentx/momaek/src/tolato/internal/server/infra/llm/openai/provider.go)
  - 当前 provider 层会消费流式事件并区分 assistant text、reasoning text、function call arguments

### 7.2 `agent-sdk-go` 能力与限制证据

- `pkg/agent/agent.go`
  - agent 支持 tools、sub-agents、streaming、memory、MCP、execution plan
  - `requirePlanApproval` 控制 plan approval 行为
- `pkg/executionplan/store.go`
  - `executionplan.Store` 为进程内 map，不是 durable store
- `pkg/interfaces/tool.go`
  - tool 接口只暴露 `Run` / `Execute`，没有 ToLaTo 当前 `WaitForUser` 这类标准暂停结果
- `pkg/agent/streaming.go`
  - 支持 `content`、`thinking`、`tool_call`、`tool_result` streaming
  - 在 `requirePlanApproval` 场景下会回退到非流式 execution plan 路径
- `pkg/interfaces/llm.go`
  - `LLMResponse` 只有 `Content`、`Usage`、`Model`、`StopReason`、`Metadata`
  - 没有独立 reasoning text 字段

### 7.3 `agent-sdk-go` 在 thinking / reasoning 上的 provider 差异证据

- OpenAI
  - `pkg/llm/openai/client.go`
    - 支持 `ReasoningTokens` usage 统计
  - `pkg/llm/openai/streaming.go`
    - 明确指出 reasoning model 有内部 reasoning，但不暴露 raw thinking tokens
- Anthropic
  - `pkg/llm/anthropic/sse.go`
    - 会把 thinking block 转成 `StreamEventThinking`
  - `docs/extended-thinking.md`
    - 说明 extended thinking 通过 streaming events 暴露
- Gemini
  - `pkg/llm/gemini/client.go`
    - 内部能识别 thinking content，但非流式接口不返回
  - `pkg/llm/gemini/streaming.go`
    - 流式接口会发 `StreamEventThinking`

### 7.4 可直接复用的结论表述

为了后续在架构评审、技术同步或实现评估中保持口径一致，可以直接复用以下表述：

- `agent-sdk-go` 不适合直接替代当前自研 Runtime
- 它最多适合作为局部能力借用或实验性组件
- HITL 与恢复语义是主要不匹配点
- thinking 可见性依赖 provider 且主要通过 streaming 暴露

