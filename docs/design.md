# AI VPS Manager — 技术方案设计文档

## 1. 项目概述

### 1.1 项目定位
基于 AI Agent 的 VPS 服务器管理工具。用户通过类似 ChatGPT/Claude 的对话界面，以自然语言与 AI 交互，AI 自动编排并执行服务器运维操作。

### 1.2 核心特性
- 自然语言驱动的服务器管理
- 实时展示 AI 思考过程、文本输出、Tool Call 执行详情
- 支持多台 VPS 统一管理
- 敏感操作二次确认机制
- 单用户模式

### 1.3 技术栈
- 前端：Vue 3
- 后端：Golang
- LLM：OpenAI API（支持用户切换模型，包括 GPT-4o、o1、o3、o4-mini 等）
- 通信协议：WebSocket
- 数据库：PostgreSQL / SQLite（初期）

---

## 2. 系统架构

### 2.1 整体架构

```
用户浏览器 (Vue)
    ↕ WebSocket / REST
后端 API Server (Go)
    ├── API/WS Gateway        — 面向前端，推送流式数据
    ├── AI Agent Engine       — 对接 LLM，编排 Tool Call
    └── Node Agent Manager    — 管理各 VPS 的长连接
            ↕ WebSocket
    Node Agent (Go) × N      — 部署在每台 VPS 上
```

### 2.2 模块说明

**API/WS Gateway**
- 面向前端的 WebSocket 连接，推送 AI 流式输出、tool call 状态、node agent 上报数据
- REST API 处理用户登录、VPS 管理、对话历史、配置管理等

**AI Agent Engine**
- 对接 OpenAI API，管理对话上下文
- 解析 function calling / tool_calls 结果
- 编排多轮 tool call 执行流程（核心 loop）
- 动态注入 system prompt（含当前在线 node 列表）

**Node Agent Manager**
- 管理所有 VPS 上 Node Agent 的 WebSocket 长连接
- 处理 agent 注册、认证、心跳
- 指令下发与结果收集
- 连接状态监控

---

## 3. Node Agent 设计

### 3.1 核心职责
- 启动时向 Server 注册，建立 WebSocket 长连接
- 定时上报系统指标（CPU、内存、磁盘、网络、负载）
- 接收 `execute_command` 指令，执行 shell 命令
- 流式回传 stdout / stderr
- 断线自动重连（指数退避，初始 1s，最大 60s）
- 本地指令执行日志

### 3.2 注册与连接流程

```
1. 用户在 WebUI 创建节点，获取一次性注册 token
2. 在 VPS 上执行安装脚本，Node Agent 启动
3. Agent 携带 token 连接 WS: wss://server/ws/agent?token=xxx
4. Server 验证 token → 通过后颁发持久化的 agent_id 和 agent_secret
5. Agent 本地保存 agent_id + agent_secret
6. 后续重连时使用 agent_id + agent_secret 认证（不再需要 token）
7. 连接建立后，Agent 立即发送一次完整系统信息（OS、内核、IP、Agent 版本等）
8. 进入心跳循环
```

### 3.3 通信协议

Server 与 Node Agent 之间统一使用 JSON 消息：

**指令下发**
```json
{
  "type": "command",
  "id": "uuid-xxx",
  "payload": {
    "action": "execute_command",
    "command": "systemctl restart nginx",
    "timeout": 60
  }
}
```

**结果回传**
```json
{
  "type": "command_result",
  "id": "uuid-xxx",
  "payload": {
    "exit_code": 0,
    "stdout": "...",
    "stderr": "",
    "duration_ms": 1200
  }
}
```

**流式输出回传**
```json
{
  "type": "command_stream",
  "id": "uuid-xxx",
  "payload": {
    "stream": "stdout",
    "data": "Reading package lists..."
  }
}
```

**心跳上报**
```json
{
  "type": "heartbeat",
  "payload": {
    "cpu": 23.5,
    "memory": 61.2,
    "disk": 45.0,
    "uptime": 86400,
    "load_avg": [0.5, 0.3, 0.2]
  }
}
```

**初始注册信息（连接建立后首次发送）**
```json
{
  "type": "register",
  "payload": {
    "hostname": "web-1",
    "os": "Ubuntu 22.04.3 LTS",
    "kernel": "5.15.0-91-generic",
    "ip": "1.2.3.4",
    "agent_version": "0.1.0",
    "cpu_cores": 4,
    "memory_total_mb": 8192,
    "disk_total_gb": 80
  }
}
```

---

## 4. AI Agent Tool 定义

### 4.1 Tool 列表

**list_nodes** — 列出所有 VPS 及状态
```json
{
  "name": "list_nodes",
  "description": "获取所有已注册的 VPS 节点列表及在线状态、基础指标",
  "parameters": {}
}
```

**get_node_info** — 获取指定 VPS 实时状态
```json
{
  "name": "get_node_info",
  "description": "获取指定节点的详细系统信息（从心跳缓存读取）",
  "parameters": {
    "node_id": { "type": "string", "description": "节点 ID" }
  }
}
```

**execute_command** — 在指定 VPS 上执行 shell 命令
```json
{
  "name": "execute_command",
  "description": "在指定节点上执行 shell 命令",
  "parameters": {
    "node_id": { "type": "string", "description": "目标节点 ID" },
    "command": { "type": "string", "description": "要执行的 shell 命令" },
    "timeout": { "type": "integer", "description": "超时秒数，默认 60" }
  }
}
```

### 4.2 Node 选择机制
- 用户可在前端 UI 选择一个默认 node 作为当前上下文
- AI 也可以通过 `list_nodes` 自行选择或操作多台 VPS
- 两者结合，灵活使用

---

## 5. 核心 Loop 流程

### 5.1 多轮 Tool Call 循环

```
用户发消息
  → 构建 messages（system prompt + 历史 + 用户输入）
  → 进入 loop:
      → 调用 LLM API（流式）
      → 流式推送 reasoning（thinking）给前端
      → 流式推送 content（文本）给前端
      → 如果返回 tool_calls:
          → 推送 tool_call 信息给前端
          → 判断是否敏感操作
            → 是 → 推送确认请求，等待用户确认
            → 用户拒绝 → 把拒绝结果作为 tool result 喂给 LLM，continue
          → 执行 tool（并行下发给对应 node agent，等结果）
          → 把 tool result 追加到 messages
          → 推送 tool result 给前端
          → continue（回到 loop 顶部再调 LLM）
      → 如果没有 tool_calls → break，推送 done
      → 如果超过 max_rounds（如 20 轮）→ 强制终止
```

### 5.2 并行 Tool Call
OpenAI 支持一次返回多个 tool_calls。对于多个 tool call，后端应并行下发给对应的 node agent，等全部返回后一起作为 tool result 喂给 LLM 进入下一轮。

### 5.3 执行期间状态
Loop 执行期间，前端禁用输入框，等 `done` 事件后再开放，避免并发问题。

---

## 6. 前端 WebSocket 事件协议

### 6.1 Server → 前端 事件类型

```json
{ "type": "reasoning", "delta": "让我先看看这台服务器的状态..." }
```
```json
{ "type": "content", "delta": "我来检查一下 nginx 的运行状态" }
```
```json
{ "type": "tool_call", "id": "call_xxx", "tool": "execute_command", "args": { "node_id": "node-1", "command": "systemctl status nginx" } }
```
```json
{ "type": "tool_result", "id": "call_xxx", "result": { "exit_code": 0, "stdout": "active (running)..." } }
```
```json
{ "type": "confirm_request", "id": "call_xxx", "tool": "execute_command", "args": { "node_id": "node-1", "command": "rm -rf /tmp/old_data" } }
```
```json
{ "type": "done" }
```
```json
{ "type": "error", "message": "LLM API 调用失败" }
```

### 6.2 前端 → Server 事件类型

```json
{ "type": "user_message", "content": "检查一下 node-1 的 nginx 状态" }
```
```json
{ "type": "confirm_response", "id": "call_xxx", "approved": true }
```

### 6.3 LLM 流式输出说明

不同模型返回的字段不同：
- **推理模型（o1、o3、o4-mini 等）**：返回 `reasoning_content` 字段 → 对应前端 `reasoning` 事件
- **普通模型（gpt-4o 等）**：无 `reasoning_content`，只有 `content` 和 `tool_calls`

后端需做模型兼容处理，统一转换为前端事件协议推送。

---

## 7. 前端设计

### 7.1 技术选型
- Vue 3 + TypeScript
- shadcn-vue 组件库
- 支持 Light / Dark 主题切换（shadcn-vue 原生支持，CSS 变量方案）
- 纯桌面端，暂不考虑移动端适配

### 7.2 整体布局

左右结构：

```
┌──────────────────────────────────────────────────┐
│  顶部栏：Logo / 项目名称          主题切换 / 设置 │
├────────────┬─────────────────────────────────────┤
│            │                                     │
│  左侧边栏   │           主内容区                   │
│  (可折叠)   │     (根据导航切换页面)                │
│            │                                     │
│  - 对话     │                                     │
│  - Nodes   │                                     │
│  - 审计日志  │                                     │
│  - 系统设置  │                                     │
│            │                                     │
│  ──────── │                                     │
│  对话历史列表 │                                     │
│  [新建对话]  │                                     │
│  · 对话1    │                                     │
│  · 对话2    │                                     │
│  · ...     │                                     │
│            │                                     │
└────────────┴─────────────────────────────────────┘
```

左侧边栏：
- 上部：导航菜单（对话、Nodes、审计日志、系统设置）
- 下部：对话历史列表（仅在「对话」页面时显示），支持新建、搜索、删除
- 可折叠，折叠后只显示图标

### 7.3 对话页面（核心）

**顶部操作栏**
```
┌─────────────────────────────────────────────────┐
│  当前对话标题(可编辑)    [模型选择▾]  [默认Node▾]  │
└─────────────────────────────────────────────────┘
```
- 对话标题：点击可编辑
- 模型选择：下拉框，快速切换当前对话使用的模型
- 默认 Node 选择器：下拉选一个默认节点，AI 操作时优先使用该节点

**消息流区域**

不同消息类型的渲染方式：

**用户消息** — 靠右对齐，普通气泡样式

**AI Reasoning（thinking）** — 靠左，折叠区块
```
┌─ 💭 思考过程 ──────────────────── ▶ 展开 ─┐
│  让我先看看这台服务器的...                    │
└────────────────────────────────────────────┘
```
- 默认收起，只显示一行摘要（前30字 + "..."）
- 点击展开查看完整推理过程
- 浅灰背景、斜体、小字号，与正文明显区分
- 流式输出时实时更新摘要文字

**AI Content** — 靠左，正常消息气泡
- 支持 Markdown 渲染（代码块、表格、列表等）

**Tool Call 卡片** — 靠左，独立卡片组件，默认折叠

三种状态：

执行中：
```
┌─ 🔄 execute_command @ node-1 ──────────────┐
│  $ systemctl status nginx                   │
│  ░░░░ 执行中...                              │
│  (流式 stdout 实时显示)                       │
└─────────────────────────────────────────────┘
```

成功（默认折叠）：
```
┌─ ✅ execute_command @ node-1 ────── ▶ 展开 ─┐
│  $ systemctl status nginx    exit: 0  1.2s  │
└─────────────────────────────────────────────┘
```
展开后显示完整 stdout/stderr。

失败：
```
┌─ ❌ execute_command @ node-1 ────── ▶ 展开 ─┐
│  $ systemctl status nginx    exit: 1  0.3s  │
└─────────────────────────────────────────────┘
```
失败卡片用红色边框/背景醒目提示。

**敏感操作确认** — 内联在消息流中，不使用弹窗
```
┌─ ⚠️ 敏感操作确认 ───────────────────────────┐
│  即将在 node-1 上执行：                       │
│  $ rm -rf /tmp/old_data                     │
│                                             │
│  [确认执行]  [拒绝]                           │
└─────────────────────────────────────────────┘
```
- 黄色/橙色边框醒目提示
- 确认或拒绝后卡片变为不可操作状态，显示用户的选择结果
- 内联展示保持上下文连贯

**底部输入区域**
```
┌─────────────────────────────────────────────┐
│  [输入消息...]                     [发送]     │
└─────────────────────────────────────────────┘
```
- AI 处理中时：输入框禁用，显示"AI 正在执行..."，发送按钮变为「停止」按钮
- 支持 Shift+Enter 换行，Enter 发送

### 7.4 Nodes 管理页

**表格布局**
```
┌─────────────────────────────────────────────────────────┐
│  Nodes 管理                    [搜索🔍]  [状态筛选▾]  [添加节点] │
├─────────────────────────────────────────────────────────┤
│  名称      IP           状态    OS           CPU  内存  磁盘  最后心跳    操作   │
│  ─────────────────────────────────────────────────────  │
│  web-1    1.2.3.4      🟢在线  Ubuntu22.04  23%  61%  45%  10s前      ⋮    │
│  db-1     5.6.7.8      🟢在线  Debian12     45%  78%  52%  15s前      ⋮    │
│  cache-1  9.10.11.12   🔴离线  CentOS9       -    -    -   2h前       ⋮    │
└─────────────────────────────────────────────────────────┘
```

- CPU/内存/磁盘 用小型进度条 + 百分比数字展示
- 在线状态：绿色圆点（在线）、红色圆点（离线）
- 离线节点整行降低透明度
- 操作列（⋮ 菜单）：编辑别名、查看详情、删除
- 支持按名称搜索、按在线/离线状态筛选

**添加节点对话框**
点击「添加节点」按钮，弹出对话框：
1. 输入节点别名（可选）
2. 点击「生成」后显示安装命令（带一键复制按钮）：
```bash
curl -sSL https://your-server/install.sh | bash -s -- --token=<token> --server=wss://your-server/ws/agent
```
3. 底部提示：Token 有效期 24 小时

**节点详情页**
点击节点名称或「查看详情」进入：
- 基本信息区：主机名、OS、内核版本、IP、Agent 版本
- 实时指标区：CPU、内存、磁盘、负载（只展示最新值）
- 命令历史区：该节点上最近 N 条通过 AI 执行的命令记录（表格形式：时间、命令、exit_code）

### 7.5 审计日志页

```
┌──────────────────────────────────────────────────────────┐
│  操作审计日志            [节点筛选▾]  [时间范围]  [搜索🔍]    │
├──────────────────────────────────────────────────────────┤
│  时间               节点     命令                  状态  确认  │
│  ────────────────────────────────────────────────────── │
│  04-02 14:23:01    web-1   systemctl restart nginx ✅    -   │
│  04-02 14:22:45    web-1   systemctl status nginx  ✅    -   │
│  04-02 13:10:02    db-1    rm -rf /tmp/old_data    ✅   已确认 │
│  04-02 13:09:58    db-1    df -h                   ✅    -   │
└──────────────────────────────────────────────────────────┘
```

- 支持按节点、时间范围、关键词筛选
- 经过二次确认的操作在「确认」列标注
- 点击行可展开查看完整 stdout/stderr 输出
- 分页加载

### 7.6 系统设置页

左侧竖向 Tab 导航，右侧对应表单内容：

```
┌──────────┬──────────────────────────────────┐
│          │                                  │
│ LLM 配置  │  (对应 Tab 的表单内容)             │
│ 安全设置   │                                  │
│ Agent 设置 │                                  │
│ 对话设置   │                                  │
│          │                          [保存]   │
└──────────┴──────────────────────────────────┘
```

**Tab 1：LLM 配置**
- API Base URL 输入框（默认 `https://api.openai.com`）
- API Key 输入框（密码类型，脱敏显示 `sk-****abcd`）
- 「验证并获取模型」按钮 → 成功后模型下拉框自动刷新
- 默认模型：下拉框（从 `/v1/models` 获取，支持搜索过滤）+ 手动输入兜底
- Max Rounds：Tool Call 循环上限（默认 20）
- Temperature 等可选参数

**Tab 2：安全设置**
- 二次确认全局开关（Switch）
- 敏感操作关键词列表（Tag 输入组件，可增删）
  - 默认内置：`rm -rf`、`rm -r`、`reboot`、`shutdown`、`poweroff`、`halt`、`drop database`、`drop table`、`mkfs`、`dd if=`、`fdisk`、`parted`、`kill -9`、`systemctl disable`、`iptables -F`
- 命令黑名单（可选，匹配后直接拒绝执行）

**Tab 3：Node Agent 设置**
- 心跳上报间隔（默认 30s）
- 命令默认超时时间（默认 60s）
- 命令输出最大长度限制（默认 10000 行）

**Tab 4：对话设置**
- 历史消息保留轮数（默认 20）
- 命令输出截断行数（默认前后各 100 行）
- System Prompt 自定义：
  - 上方只读区域：展示系统内置基础 prompt（供参考）
  - 下方文本域：用户追加自定义指令，拼接在基础 prompt 之后

### 7.7 其他页面
- **对话历史列表**（集成在左侧边栏）：按时间倒序，支持搜索，点击切换对话

---

## 8. System Prompt 设计

System Prompt 动态构建，包含以下部分：

```
1. 角色定义
   你是一个专业的 VPS 服务器管理助手，通过 tool 来管理和运维用户的服务器。

2. Tool 使用指引
   - 执行命令前先通过 get_node_info 或 execute_command 了解当前状态
   - 危险操作（rm、reboot、drop 等）执行前要说明风险
   - 优先使用安全的查询命令确认环境

3. 当前在线节点（动态注入）
   当前可用节点：
   - node-1: Ubuntu 22.04, 4C8G, IP: x.x.x.x, 在线
   - node-2: Debian 12, 2C4G, IP: x.x.x.x, 在线

4. 输出格式偏好
   - 先说明要做什么，再执行
   - 执行后给出结果分析和建议
   - 遇到错误时给出排查思路

5. 安全规范
   - 不要在命令中硬编码密码
   - 修改配置前先备份
   - 批量操作先在一台上验证
```

---

## 9. 安全机制

### 9.1 Node Agent 认证
- Agent 首次注册时由 Server 颁发 token
- WebSocket 连接时携带 token 认证
- Token 支持轮换

### 9.2 API Key 加密存储
- 用户配置的 LLM API Key 加密后存入数据库
- 加密密钥通过服务端配置文件注入（如 `config.yaml` 中的 `encrypt_key` 字段）
- 使用 AES-GCM 对称加密
- 前端展示时脱敏显示（如 `sk-****abcd`）

### 9.3 敏感操作二次确认
后端维护敏感操作规则（可配置），匹配方式：
- 关键词匹配：`rm -rf`、`reboot`、`shutdown`、`drop database`、`mkfs`、`dd if=` 等
- 用户可自定义规则

匹配到敏感操作后暂停执行，推送确认请求到前端，等待用户审批。

### 9.4 操作审计
所有通过 AI 执行的命令记录到审计日志，包含：时间、目标节点、命令内容、执行结果、是否经过二次确认。

---

## 10. 对话上下文管理

### 10.1 Context Window 限制
LLM 上下文窗口有限，需要控制 messages 大小：
- 命令输出截断：超长 stdout 只保留前后各 100 行
- 历史消息：保留最近 N 轮对话（如 20 轮）
- 超长对话：旧消息摘要压缩，或直接截断

### 10.2 对话持久化
- 所有对话历史存数据库
- 支持查看历史对话
- 新建对话时上下文清空

---

## 11. 数据存储

| 数据类型 | 存储方案 | 说明 |
|---------|---------|------|
| 对话历史 | PostgreSQL / SQLite | 消息、tool call、tool result |
| VPS 注册信息 | PostgreSQL / SQLite | 节点 ID、token、别名、备注、tags（预留） |
| 操作审计日志 | PostgreSQL / SQLite | 命令执行记录，含调用来源（webui/api/mcp） |
| 系统指标 | 内存缓存 | 心跳数据只缓存最新值 |
| LLM 配置 | PostgreSQL / SQLite | API Base、API Key（AES-GCM 加密）、默认模型 |
| 系统设置 | PostgreSQL / SQLite | 敏感关键词、超时配置、对话设置、自定义 prompt |
| 外部 API Keys | PostgreSQL / SQLite | Key 值（加密）、权限级别、创建时间、状态 |

---

## 12. 后端 REST API 路由

除 WebSocket 外，后端还需提供以下 REST API：

### 12.1 对话管理
```
POST   /api/conversations              — 新建对话
GET    /api/conversations              — 获取对话列表
GET    /api/conversations/:id          — 获取对话详情（含完整消息历史）
PUT    /api/conversations/:id          — 更新对话（标题等）
DELETE /api/conversations/:id          — 删除对话
```

### 12.2 节点管理
```
GET    /api/nodes                      — 获取节点列表
POST   /api/nodes                      — 创建节点（生成注册 token）
GET    /api/nodes/:id                  — 获取节点详情
PUT    /api/nodes/:id                  — 更新节点（别名、备注）
DELETE /api/nodes/:id                  — 删除节点
GET    /api/nodes/:id/commands         — 获取节点命令历史
```

### 12.3 系统设置
```
GET    /api/settings/llm               — 获取 LLM 配置
PUT    /api/settings/llm               — 更新 LLM 配置
POST   /api/settings/llm/verify        — 验证 LLM 配置并获取模型列表
GET    /api/settings/security          — 获取安全设置
PUT    /api/settings/security          — 更新安全设置
GET    /api/settings/agent             — 获取 Agent 设置
PUT    /api/settings/agent             — 更新 Agent 设置
GET    /api/settings/chat              — 获取对话设置
PUT    /api/settings/chat              — 更新对话设置
```

### 12.4 审计日志
```
GET    /api/audit-logs                 — 获取审计日志（支持分页、筛选）
```

### 12.5 WebSocket 端点
```
WS     /ws/chat                        — 前端对话 WebSocket
WS     /ws/agent                       — Node Agent 接入 WebSocket
```

---

## 13. 服务端配置文件

`config.yaml` 示例：

```yaml
server:
  host: 0.0.0.0
  port: 8080

database:
  driver: sqlite           # sqlite 或 postgres
  dsn: data/app.db         # SQLite 路径或 PostgreSQL 连接串

security:
  encrypt_key: "your-32-byte-encryption-key-here"   # AES-GCM 加密密钥
  agent_token_expiry: 24h                            # Node Agent 注册 token 有效期

defaults:
  heartbeat_interval: 30   # 心跳间隔（秒）
  command_timeout: 60      # 命令默认超时（秒）
  max_rounds: 20           # Tool Call 循环上限
  context_rounds: 20       # 对话上下文保留轮数
  output_truncate_lines: 100  # 命令输出截断行数（前后各）
```

---

## 14. 项目目录结构

```
ai-vps-manager/
├── server/                          # Go 后端
│   ├── cmd/
│   │   └── server/
│   │       └── main.go              # 入口
│   ├── internal/
│   │   ├── config/                  # 配置加载
│   │   ├── handler/                 # HTTP/WS handler
│   │   │   ├── api.go               # REST API 路由注册
│   │   │   ├── chat_ws.go           # 前端对话 WebSocket
│   │   │   └── agent_ws.go          # Node Agent WebSocket
│   │   ├── agent/                   # AI Agent Engine
│   │   │   ├── engine.go            # 核心 loop
│   │   │   ├── tools.go             # Tool 定义与执行
│   │   │   └── prompt.go            # System Prompt 构建
│   │   ├── node/                    # Node Agent Manager
│   │   │   ├── manager.go           # 连接管理、指令下发
│   │   │   └── protocol.go          # 消息协议定义
│   │   ├── model/                   # 数据模型
│   │   ├── store/                   # 数据库操作
│   │   ├── security/                # 加密、敏感操作检测
│   │   └── openai/                  # OpenAI API 客户端
│   ├── config.yaml                  # 配置文件
│   ├── go.mod
│   └── go.sum
│
├── agent/                           # Node Agent（Go）
│   ├── cmd/
│   │   └── agent/
│   │       └── main.go              # 入口
│   ├── internal/
│   │   ├── client/                  # WebSocket 客户端、重连
│   │   ├── executor/                # 命令执行器
│   │   ├── collector/               # 系统指标采集
│   │   └── protocol/                # 消息协议（与 server 共享定义）
│   └── go.mod
│
├── web/                             # Vue 前端
│   ├── src/
│   │   ├── views/                   # 页面组件
│   │   │   ├── ChatView.vue         # 对话页
│   │   │   ├── NodesView.vue        # 节点管理页
│   │   │   ├── NodeDetailView.vue   # 节点详情页
│   │   │   ├── AuditLogView.vue     # 审计日志页
│   │   │   └── SettingsView.vue     # 系统设置页
│   │   ├── components/              # 通用组件
│   │   │   ├── chat/                # 对话相关组件
│   │   │   │   ├── MessageBubble.vue
│   │   │   │   ├── ThinkingBlock.vue
│   │   │   │   ├── ToolCallCard.vue
│   │   │   │   └── ConfirmCard.vue
│   │   │   ├── layout/              # 布局组件
│   │   │   │   ├── Sidebar.vue
│   │   │   │   └── TopBar.vue
│   │   │   └── common/              # 通用 UI 组件
│   │   ├── composables/             # 组合式函数
│   │   │   ├── useWebSocket.ts      # WebSocket 管理
│   │   │   └── useTheme.ts          # 主题切换
│   │   ├── stores/                  # Pinia 状态管理
│   │   ├── router/                  # Vue Router
│   │   ├── types/                   # TypeScript 类型定义
│   │   └── App.vue
│   ├── package.json
│   └── vite.config.ts
│
└── README.md
```

---

## 15. 外部 API 与 MCP 集成

### 15.1 设计目标
将内部 Tool 的能力对外暴露为标准 API，使 Claude Code、OpenClaw 等外部 AI 客户端可以通过 REST API 或 MCP 协议接入，复用同一套节点管理和命令执行能力。

### 15.2 架构分层

```
内部调用路径：
  WebUI → AI Agent Engine → Tool Handler → Node Agent Manager → Node Agent

外部调用路径：
  Claude Code / OpenClaw / 其他客户端
       ↓
  REST API (/api/v1/tools/*)  或  MCP Server
       ↓
  Tool Handler → Node Agent Manager → Node Agent（复用同一套逻辑）
```

Tool Handler 层是共用的，内部 AI Agent 和外部 API 走同一个执行路径，确保行为一致。

### 15.3 REST API 设计

```
GET    /api/v1/nodes                — 列出所有节点及状态
GET    /api/v1/nodes/:id            — 获取指定节点详细信息
POST   /api/v1/nodes/:id/execute    — 在指定节点上执行命令
```

**execute 请求体：**
```json
{
  "command": "systemctl status nginx",
  "timeout": 60,
  "confirm": true,
  "stream": false
}
```

**execute 响应体：**
```json
{
  "id": "exec-uuid",
  "node_id": "node-1",
  "command": "systemctl status nginx",
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "duration_ms": 1200
}
```

**流式输出：** 当 `stream: true` 时，响应使用 SSE（Server-Sent Events）逐步推送 stdout/stderr。

### 15.4 MCP Server 适配

实现 MCP（Model Context Protocol）Server 规范，将 Tool 注册为 MCP Tools：

```json
{
  "tools": [
    {
      "name": "list_nodes",
      "description": "List all registered VPS nodes with online status and metrics",
      "inputSchema": { "type": "object", "properties": {} }
    },
    {
      "name": "get_node_info",
      "description": "Get detailed system info for a specific node",
      "inputSchema": {
        "type": "object",
        "properties": {
          "node_id": { "type": "string", "description": "Node ID" }
        },
        "required": ["node_id"]
      }
    },
    {
      "name": "execute_command",
      "description": "Execute a shell command on a specific node",
      "inputSchema": {
        "type": "object",
        "properties": {
          "node_id": { "type": "string", "description": "Target node ID" },
          "command": { "type": "string", "description": "Shell command to execute" },
          "timeout": { "type": "integer", "description": "Timeout in seconds, default 60" }
        },
        "required": ["node_id", "command"]
      }
    }
  ]
}
```

MCP Server 底层调用同一套 Tool Handler，只是协议适配层不同。

MCP 传输方式支持：
- Streamable HTTP（推荐，适合远程接入）
- stdio（适合本地 Claude Code 直接调用）

### 15.5 认证机制

外部 API 使用独立的 API Key 认证，与 WebUI 的用户 session 分开：

- 在系统设置页增加「API Keys」管理 Tab
- 支持生成/查看/吊销 API Key
- 请求时通过 `Authorization: Bearer <api_key>` 头传递
- 每个 API Key 可设置权限级别：
  - **只读**：仅允许 `list_nodes`、`get_node_info`
  - **标准**：允许所有操作，敏感命令需 `confirm: true` 显式确认，不传则拒绝
  - **管理员**：允许所有操作，跳过敏感操作确认

### 15.6 敏感操作处理

外部 API 调用是无交互的，无法像 WebUI 那样弹确认卡片，处理方式：

- 标准权限的 API Key：敏感操作必须在请求体中传 `confirm: true` 显式确认，缺省则返回 `403` 并附带提示信息
- 管理员权限的 API Key：直接执行，不做二次确认
- 命令黑名单规则同样生效，无论什么权限级别

**403 响应示例：**
```json
{
  "error": "sensitive_operation",
  "message": "This command matches sensitive operation rules. Set confirm: true to proceed.",
  "matched_rule": "rm -rf"
}
```

### 15.7 审计日志

通过外部 API 执行的操作同样记录到审计日志，额外标记：
- 调用来源：`webui` / `api` / `mcp`
- 使用的 API Key 标识

---

## 16. 开发优先级

### Phase 1 — 最小可用闭环
1. Node Agent 基础框架：WS 连接、心跳、命令执行
2. 后端 Node Agent Manager：连接管理、指令下发
3. 后端 AI Agent Engine：对接 OpenAI、核心 loop
4. 前端对话界面：消息流、tool call 展示

### Phase 2 — 核心功能完善
5. 流式输出（reasoning + content + tool call）
6. 敏感操作二次确认
7. 前端 Nodes 管理页
8. 对话历史持久化

### Phase 3 — 增强功能
9. 操作审计日志
10. 命令输出流式回传
11. 上下文管理优化（截断、摘要）
12. 多模型切换 UI
13. 系统设置完整实现

### Phase 4 — 外部 API 与集成
14. REST API 层（复用 Tool Handler）
15. API Key 管理（生成/吊销/权限级别）
16. MCP Server 适配（Streamable HTTP + stdio）

### Phase 5 — 后续扩展
17. 更多 Tool（文件管理、Docker 管理等）
18. 定时任务 / 自动化工作流
19. 系统指标可视化仪表盘
20. 告警通知