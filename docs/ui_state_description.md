# ToLaTo UI 时间线快照说明

## 1. 文档目的

本文档用于补充 [`docs/ui_console_design.md`](./ui_console_design.md) 的交互落地说明，重点描述：

- row-based timeline 在不同阶段应该长什么样
- 顶部状态栏、左侧节点栏和主时间线如何协同
- `tool_call_meta` / `tool_result_meta` 在什么位置出现
- 为什么按钮型确认 / 审批不会新增 `user row`

本文档不再以旧版 Pencil 里的 `Empty / Plan / Approval / Execution` 整页状态为主，而是以“时间线快照”描述同一个控制台在不同阶段的样子。

当前 [`docs/tolato-ui-states.pen`](./tolato-ui-states.pen) 仍然是旧模型草图，只能作为视觉参考，不能继续作为交互语义基线。

---

## 2. 共用界面骨架

无论处于哪个快照，页面都保持同一套骨架：

- 顶部状态栏：品牌、连接状态、当前会话目标上下文、模式
- 左侧节点栏：候选节点 / 已确认目标 / 节点状态
- 右侧主工作区：按时间顺序追加的 row-based timeline
- 底部输入区：输入框、`发送` 按钮、快捷 chips

需要固定遵守的规则：

- 主工作区是时间线，不是会不断长高的单卡
- `plan / approval / execution / summary` 是全宽结构化 row
- 普通工具调用默认展示 `tool_call_meta` 和 `tool_result_meta`
- 用户点击确认 / Approve / Reject 后，只追加 `tool_result_meta`
- 只有用户手动输入确认文本时，才新增 `user_message`

---

## 3. 固定区域规则

### 3.1 顶部状态栏

顶部只表达上下文，不承担主流程推进。

固定元素：

- `ToLaTo`
- `Control Server connected`
- 节点健康摘要，如 `4 online · 1 offline`
- 当前目标上下文，如：
  - `Target context: unset`
  - `Target context: pending jp-tokyo-01`
  - `Target context: confirmed 2 nodes`

规则：

- 多节点目标时显示广播提醒
- `pending_confirmation` 时显示待确认 badge
- 不在顶部放“下一步请审批/请执行”这类主流程动作

### 3.2 左侧节点栏

左栏是上下文镜像，不是前置选范围入口。

推荐分区：

1. 节点总览
2. 最近匹配节点 / 候选目标
3. 已确认目标
4. 节点列表

规则：

- 候选节点可高亮，但真正的确认动作发生在主时间线的 `target_confirmation row`
- 若当前是多节点广播，左栏和顶部同时显示广播提醒
- 若当前已确认单节点，左栏应把该节点提到最上方

### 3.3 底部输入区

底部输入区在所有快照中都存在。

固定规则：

- 主按钮文案是 `发送`
- 占位文案强调“AI 会自行决定是否查节点、确认目标、生成计划或进入审批”
- 不再使用 `生成计划` 作为主动作
- `重新生成` 只能作为某个结构化 row 内的次级动作，不应出现在主 composer

---

## 4. 时间线快照

## 4.1 Idle

### 页面目的

用户刚进入会话，还没有发起任务。

### 顶部

- `workspace idle`
- `Target context: unset`
- `4 online · 1 offline`

### 左栏

- 节点总览卡
- 常规节点列表
- 无候选目标、无已确认目标

### 主时间线

按从上到下的顺序：

1. `assistant_text`
   - `Control server ready. 4 agents connected.`
2. `assistant_text`
   - `发送一个任务请求，AI 会自行决定是否查询节点、确认目标、生成计划或进入审批。`

### 输入区

- 输入框为空
- 主按钮：`发送`

---

## 4.2 After Target Resolution

### 页面目的

用户刚发送消息，Agent Loop 开始查节点并解析目标。

### 顶部

- `Target context: pending jp-tokyo-01` 或 `pending 2 nodes`

### 左栏

- 最近匹配节点区出现候选节点
- 候选节点高亮

### 主时间线

1. `user_message`
   - `重启东京节点的 nginx`
2. `tool_call_meta`
   - `calling list_nodes(status=online,stale)`
3. `tool_result_meta`
   - `list_nodes returned 4 online nodes`
4. `tool_call_meta`
   - `calling resolve_target_nodes("东京节点")`
5. `tool_result_meta`
   - `resolve_target_nodes matched jp-tokyo-01`
6. `target_confirmation`
   - 展示目标、匹配依据、确认按钮

### 关键点

- 普通工具调用必须可见
- 目标确认 row 是第一条需要用户决策的结构化 row

---

## 4.3 After Target Confirmation

### 页面目的

用户点击了确认目标，系统已经锁定目标，但尚未生成 plan。

### 顶部

- `Target context: confirmed jp-tokyo-01`

### 左栏

- 已确认目标区出现 `jp-tokyo-01`
- 候选节点区可弱化但不必立刻消失

### 主时间线

紧接上一快照，在 `target_confirmation` 之后追加：

7. `tool_result_meta`
   - `target_confirmation succeeded · jp-tokyo-01 confirmed`

### 关键点

- 这里不新增 `user_message`
- 这条 row 是一次用户按钮动作的系统结果，不是聊天文本

---

## 4.4 After Plan

### 页面目的

目标已确认，Agent Loop 生成了结构化计划。

### 顶部

- `plan ready`
- `Target context: confirmed jp-tokyo-01`

### 左栏

- 已确认目标仍然固定显示

### 主时间线

在确认结果之后继续追加：

8. `tool_call_meta`
   - `calling propose_plan`
9. `tool_result_meta`
   - `plan generated · low risk`
10. `plan`
   - 展示 target / summary / steps / risk / impact / requiresApproval

### 关键点

- `plan` 是一条大 row，但它只是时间线中的一项
- 不把审批、执行结果继续塞回这条 row

---

## 4.5 After Approval

### 页面目的

当前计划需要审批，用户已经完成批准。

### 顶部

- `approval required` 到 `approved`
- `Target context: confirmed jp-tokyo-01`

### 左栏

- 已确认目标固定展示
- 可加 `risk-gated` 标签提示当前任务是写操作

### 主时间线

在 `plan` 之后追加：

11. `approval`
   - 展示风险、影响、目标、按钮
12. `tool_result_meta`
   - `approval recorded · Approved by Alex at 14:32`

### 关键点

- 按钮点击后只出现 `tool_result_meta`
- 不出现新的 `user_message: Approve`

---

## 4.6 During Execution

### 页面目的

任务已进入执行，正在流式回传结果。

### 顶部

- `running`
- `Live task in progress`

### 左栏

- 节点列表变成执行态摘要：
  - `sg-prod-01 success`
  - `us-east-02 running`
  - `hk-01 offline`

### 主时间线

在审批结果或自动执行之后追加：

13. `tool_call_meta`
   - `calling exec_on_nodes`
14. `tool_result_meta`
   - `execution started · 1 task / 2 executions`
15. `execution`
   - 顶部状态线
   - 分节点输出
   - 当前异常节点默认展开

### 关键点

- 执行日志不单独占一个页面
- 执行过程仍然是时间线中的一条结构化 row

---

## 4.7 After Summary

### 页面目的

执行完成，系统给出聚合结果和 AI 总结。

### 顶部

- `success` / `partial_failed` / `failed`

### 左栏

- 保留节点执行态摘要

### 主时间线

在 `execution` 之后追加：

16. `tool_call_meta`
   - `calling summarize_execution`
17. `tool_result_meta`
   - `summary generated`
18. `summary`
   - 聚合结果
   - 异常节点
   - AI 建议

### 关键点

- `summary` 是收口 row
- 用户下一轮追问会继续在它下面追加新的 row，而不是覆盖已有内容

---

## 5. 典型完整时间线

### 5.1 单节点只读

```text
user_message
tool_call_meta(list_nodes)
tool_result_meta
tool_call_meta(resolve_target_nodes)
tool_result_meta
target_confirmation
tool_result_meta(confirm)
tool_call_meta(propose_plan)
tool_result_meta
plan
tool_call_meta(exec_on_nodes)
tool_result_meta
execution
tool_call_meta(summarize_execution)
tool_result_meta
summary
```

### 5.2 单节点写操作

```text
user_message
tool_call_meta(list_nodes)
tool_result_meta
tool_call_meta(resolve_target_nodes)
tool_result_meta
target_confirmation
tool_result_meta(confirm)
tool_call_meta(propose_plan)
tool_result_meta
plan
approval
tool_result_meta(approve)
tool_call_meta(exec_on_nodes)
tool_result_meta
execution
tool_call_meta(summarize_execution)
tool_result_meta
summary
```

### 5.3 沿用已确认目标

如果本轮沿用了上一轮已确认目标，可以省略 `target_confirmation`，但必须补一条明确的：

- `assistant_text`，如 `将沿用上一轮已确认的东京和新加坡节点`
或
- `tool_result_meta`，如 `context inherited · 2 nodes confirmed`

---

## 6. 实现对齐要点

- 主时间线默认展示所有 `tool_call_meta` 和 `tool_result_meta`
- `plan / approval / execution / summary` 采用时间线中的大卡片外观
- 顶部和左栏只做上下文镜像，不替代主时间线
- 普通工具调用必须可观察
- 按钮型确认 / 审批不得伪装成 `user_message`
- 旧版 Pencil 画布需要后续按新的快照模型重画或补充
