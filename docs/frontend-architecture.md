# 前端架构设计 — Chat 界面

## 1. 概述

本文档描述 tolato 前端 Chat 界面的架构设计，涵盖：

- **整体布局与路由** — 页面结构、侧边栏、路由设计
- **WebSocket 服务** — 单连接多路复用、断线重连、Session 管理
- **Chat 状态管理** — 消息流、对话状态机、流式渲染
- **组件架构** — 消息类型组件拆分、可复用组件

### 1.1 技术栈

- Vue 3 + TypeScript + Composition API
- shadcn-vue 组件库
- Pinia 状态管理
- Vue Router 4
- VueUse 工具库
- [markstream-vue](https://github.com/Simon-He95/markstream-vue) — AI 流式 Markdown 渲染（增量解析、无闪烁、Virtual Window 内存控制）
- Shiki — 代码块语法高亮（作为 markstream-vue 的可选 peer）
- Dark 主题（当前仅 Dark，预留 Light 切换能力）

### 1.2 设计语言（来自设计稿）

| Token | 值 | 用途 |
|-------|-----|------|
| `--primary` | `#FF8400` | 主色调（按钮、高亮、品牌色） |
| `--background` | `#111111` | 页面背景 |
| `--card` | `#1A1A1A` | 卡片背景 |
| `--secondary` | `#2E2E2E` | 次级背景（输入框、气泡） |
| `--foreground` | `#FFFFFF` | 主文字 |
| `--muted-foreground` | `#B8B9B6` | 次要文字 |
| `--border` | `#2E2E2E` | 边框 |
| `--color-warning` | `#291C0F` | 警告背景（确认卡片） |
| `--color-warning-foreground` | `#FF8400` | 警告文字/边框 |
| `--color-error` | `#24100B` | 错误背景 |
| `--color-error-foreground` | `#FF5C33` | 错误文字 |
| `--color-success` | `#222924` | 成功背景 |
| `--color-success-foreground` | `#B6FFCE` | 成功文字 |
| `--sidebar` | `#18181b` | 侧边栏背景 |
| `--radius-m` | `16px` | 中等圆角 |
| `--radius-pill` | `999px` | 药丸形圆角 |
| 字体 | `Geist` / `Geist Mono` | 正文 / 代码 |

---

## 2. 整体布局

```
┌──────────────────────────────────────────────────────┐
│  App Shell (flex: row, 100vh)                        │
├─────────────┬────────────────────────────────────────┤
│             │                                        │
│  Sidebar    │          Router View                   │
│  (280px)    │    (flex: 1, 根据路由切换)               │
│  (可折叠)    │                                        │
│             │                                        │
│  ┌────────┐ │                                        │
│  │导航菜单  │ │                                        │
│  │- Chat  │ │                                        │
│  │- Nodes │ │                                        │
│  │- Audit │ │                                        │
│  │- Settings│                                        │
│  ├────────┤ │                                        │
│  │对话列表  │ │                                        │
│  │(Chat时) │ │                                        │
│  └────────┘ │                                        │
└─────────────┴────────────────────────────────────────┘
```

### 2.1 路由设计

```typescript
const routes = [
  {
    path: '/',
    component: AppLayout,           // 带 Sidebar 的外壳
    children: [
      { path: '', redirect: '/chat' },
      { path: 'chat', component: ChatView },             // 空状态 / 新对话
      { path: 'chat/:conversationId', component: ChatView },  // 具体对话
      { path: 'nodes', component: NodesView },
      { path: 'nodes/:nodeId', component: NodeDetailView },
      { path: 'audit', component: AuditLogView },
      { path: 'settings', component: SettingsView },
      // NodeProbe
      { path: 'monitor', component: MonitorView },
      { path: 'monitor/:linkId', component: LinkDetailView },
      { path: 'alerts', component: AlertsView },
    ]
  },
  { path: '/login', component: LoginView },
]
```

### 2.2 Sidebar 结构

Sidebar 分两个区域：

**上部 — 导航菜单**（固定）：
- Chat、Nodes、Audit Log、Settings
- 当前页高亮（橙色背景药丸形）
- 图标 + 文字，折叠后只显示图标

**下部 — 对话列表**（仅 Chat 页面显示）：
- 标题 "Conversations" + 新建按钮（`+`）
- 对话条目列表，当前对话高亮
- 点击切换路由到 `/chat/:conversationId`

---

## 3. WebSocket 服务

### 3.1 单连接多路复用

前端全局只维护**一条 WebSocket 连接**（`/ws`），所有 conversation 的消息通过 `conversation_id` 字段区分。

```typescript
// composables/useWebSocket.ts

interface WSMessage<T = unknown> {
  type: string
  conversation_id?: string  // Loop 事件带此字段
  payload?: T
}
```

### 3.2 连接管理

```typescript
class WebSocketService {
  private ws: WebSocket | null = null
  private url: string
  private token: string
  private reconnectAttempts = 0
  private maxReconnectAttempts = 10
  private handlers = new Map<string, Set<(msg: WSMessage) => void>>()

  // 连接状态
  state: 'connecting' | 'connected' | 'disconnected' | 'replaced' = 'disconnected'

  connect() {
    this.state = 'connecting'
    this.ws = new WebSocket(`${this.url}?token=${this.token}`)

    this.ws.onopen = () => {
      this.state = 'connected'
      this.reconnectAttempts = 0
    }

    this.ws.onmessage = (event) => {
      const msg: WSMessage = JSON.parse(event.data)

      // session_replaced → 被新 tab 踢掉
      if (msg.type === 'session_replaced') {
        this.state = 'replaced'
        this.showReplacedNotification()
        return
      }

      // 分发到注册的 handler
      this.dispatch(msg)
    }

    this.ws.onclose = () => {
      if (this.state !== 'replaced') {
        this.state = 'disconnected'
        this.reconnect()
      }
    }
  }

  // 断线重连（指数退避）
  private reconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return
    const delay = Math.min(1000 * 2 ** this.reconnectAttempts, 30000)
    this.reconnectAttempts++
    setTimeout(() => this.connect(), delay)
  }

  // 发送消息（自动附加 conversation_id）
  send(type: string, conversationId: string, payload: unknown) {
    this.ws?.send(JSON.stringify({
      type,
      conversation_id: conversationId,
      payload,
    }))
  }

  // 按 type 注册 handler
  on(type: string, handler: (msg: WSMessage) => void) { ... }
  off(type: string, handler: (msg: WSMessage) => void) { ... }

  // 按 conversation_id 分发
  private dispatch(msg: WSMessage) {
    const handlers = this.handlers.get(msg.type)
    handlers?.forEach(h => h(msg))
  }
}
```

### 3.3 生命周期

```
App 挂载
  → WebSocketService.connect()
  → 连接成功 → state = 'connected'

用户发消息 (conversation A)
  → ws.send({ type: 'user_message', conversation_id: 'A', payload: {...} })

收到事件
  → { type: 'content', conversation_id: 'A', payload: { delta: '...' } }
  → dispatch → chatStore 按 conversation_id 更新

新 tab 打开
  → 新 tab WS 连接 → Server 踢旧连接
  → 旧 tab 收到 session_replaced → 显示 "已在其他窗口打开" 遮罩
  → 旧 tab 不再尝试重连

WS 意外断线
  → state = 'disconnected' → 自动重连（指数退避）
  → 重连成功后，当前页面通过 REST API 刷新对话历史恢复状态
```

---

## 4. Chat 状态管理

### 4.1 Conversation 状态机

每个 conversation 有一个独立的状态：

```
                    ┌──────────────┐
                    │    idle      │ ← 初始 / Loop 结束后
                    └──────┬───────┘
                           │ 用户发送消息
                           ▼
                    ┌──────────────┐
              ┌────▶│  streaming   │ ← LLM 输出中（reasoning / content）
              │     └──────┬───────┘
              │            │ 收到 tool_calls
              │            ▼
              │     ┌──────────────┐
              │     │  tool_exec   │ ← Tool 执行中
              │     └──────┬───────┘
              │            │ tool results 全部返回
              │            │ → 继续下一轮 LLM
              │            │
              │     ┌──────┴───────┐
              ├────▶│  confirming  │ ← 等待用户确认敏感操作
              │     └──────┬───────┘
              │            │ 用户确认/拒绝
              │            ▼
              │       继续执行 / 返回 LLM
              │            │
              │     ┌──────────────┐
              │     │    done      │ ← 收到 done 事件
              │     └──────┬───────┘
              │            │ 自动转回
              └────────────┘ idle
```

### 4.2 Chat Store

```typescript
// stores/chat.ts

interface ChatMessage {
  id: string
  role: 'user' | 'assistant' | 'tool'
  content?: string
  reasoning?: string
  toolCalls?: ToolCallItem[]
  toolCallId?: string
  createdAt: string
}

// 流式渲染期间的"正在构建中"的 assistant 消息
interface StreamingAssistant {
  reasoning: string          // 累积的 reasoning 文本
  content: string            // 累积的 content 文本
  toolCalls: ToolCallItem[]  // 当前轮的 tool calls
}

interface ConversationState {
  id: string
  title: string
  model: string
  defaultNodeId?: string
  messages: ChatMessage[]              // 已持久化的历史消息
  streaming: StreamingAssistant | null // 当前正在流式输出的 assistant 消息
  status: 'idle' | 'streaming' | 'tool_exec' | 'confirming' | 'error'
  confirmRequest: ConfirmRequest | null
  error: string | null
}

export const useChatStore = defineStore('chat', () => {
  // 当前活跃的 conversation ID（对应路由 params）
  const activeConversationId = ref<string | null>(null)

  // 对话列表（侧边栏用）
  const conversations = ref<ConversationSummary[]>([])

  // 当前对话的完整状态
  const current = ref<ConversationState | null>(null)

  // --- Actions ---

  // 切换对话（路由变化时调用）
  async function loadConversation(id: string) {
    activeConversationId.value = id
    const detail = await api.getConversation(id)
    current.value = {
      id: detail.id,
      title: detail.title,
      model: detail.model,
      defaultNodeId: detail.default_node_id,
      messages: detail.messages,
      streaming: null,
      status: 'idle',
      confirmRequest: null,
      error: null,
    }
  }

  // 发送消息
  function sendMessage(content: string) {
    if (!current.value || current.value.status !== 'idle') return

    // 立即追加用户消息到 UI
    current.value.messages.push({
      id: crypto.randomUUID(),
      role: 'user',
      content,
      createdAt: new Date().toISOString(),
    })

    // 初始化 streaming 状态
    current.value.streaming = { reasoning: '', content: '', toolCalls: [] }
    current.value.status = 'streaming'

    // 通过 WS 发送
    wsService.send('user_message', current.value.id, {
      content,
      model: current.value.model,
      default_node_id: current.value.defaultNodeId,
    })
  }

  // 确认/拒绝敏感操作
  function confirmAction(toolCallId: string, approved: boolean) {
    if (!current.value) return
    wsService.send('confirm_response', current.value.id, {
      id: toolCallId,
      approved,
    })
    current.value.status = approved ? 'tool_exec' : 'streaming'
    current.value.confirmRequest = null
  }

  // 停止当前 Loop（发送 abort）
  function stopLoop() {
    if (!current.value) return
    wsService.send('abort', current.value.id, {})
  }

  return {
    activeConversationId,
    conversations,
    current,
    loadConversation,
    sendMessage,
    confirmAction,
    stopLoop,
  }
})
```

### 4.3 WS 事件处理

WebSocket 收到事件后，按 `conversation_id` 更新 ChatStore：

```typescript
// composables/useChatWebSocket.ts
// 在 App.vue 中调用一次，全局注册

function useChatWebSocket() {
  const chatStore = useChatStore()

  // 只处理当前活跃对话的事件
  function isActive(convId: string) {
    return chatStore.activeConversationId === convId
  }

  wsService.on('reasoning', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    const s = chatStore.current!.streaming!
    s.reasoning += msg.payload.delta
  })

  wsService.on('content', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    const s = chatStore.current!.streaming!
    s.content += msg.payload.delta
    chatStore.current!.status = 'streaming'
  })

  wsService.on('tool_call', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    chatStore.current!.status = 'tool_exec'
    chatStore.current!.streaming!.toolCalls.push({
      id: msg.payload.id,
      tool: msg.payload.tool,
      args: msg.payload.args,
      status: 'executing',       // executing → success | error
      result: null,
    })
  })

  wsService.on('tool_result', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    const tc = chatStore.current!.streaming!.toolCalls
      .find(t => t.id === msg.payload.id)
    if (tc) {
      tc.result = msg.payload.result
      tc.status = msg.payload.result.exit_code === 0 ? 'success' : 'error'
    }
  })

  wsService.on('confirm_request', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    chatStore.current!.status = 'confirming'
    chatStore.current!.confirmRequest = {
      id: msg.payload.id,
      tool: msg.payload.tool,
      args: msg.payload.args,
    }
  })

  wsService.on('done', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    // 把 streaming 内容合并到 messages
    finalizeStreaming()
    chatStore.current!.status = 'idle'
  })

  wsService.on('error', (msg) => {
    if (!isActive(msg.conversation_id!)) return
    chatStore.current!.status = 'error'
    chatStore.current!.error = msg.payload.message
  })
}
```

### 4.4 streaming → messages 合并

当收到 `done` 事件时，将 streaming 累积的内容合并成正式的 ChatMessage：

```typescript
function finalizeStreaming() {
  const c = chatStore.current!
  const s = c.streaming!

  // 1. 追加 assistant 消息
  c.messages.push({
    id: crypto.randomUUID(),
    role: 'assistant',
    content: s.content || undefined,
    reasoning: s.reasoning || undefined,
    toolCalls: s.toolCalls.length > 0 ? s.toolCalls : undefined,
    createdAt: new Date().toISOString(),
  })

  // 2. 追加 tool result 消息（每个 tool call 一条）
  for (const tc of s.toolCalls) {
    if (tc.result) {
      c.messages.push({
        id: crypto.randomUUID(),
        role: 'tool',
        toolCallId: tc.id,
        content: JSON.stringify(tc.result),
        createdAt: new Date().toISOString(),
      })
    }
  }

  // 3. 清空 streaming
  c.streaming = null
}
```

> **注意**：一次完整的 Loop 可能包含多轮 LLM 调用（LLM → tool → LLM → tool → LLM），
> 每轮 LLM 结束进入 tool 执行时，前端不做 finalize——一直累积到最终 `done` 事件才合并。
> 多轮 tool calls 全部存在 `streaming.toolCalls` 数组中。

---

## 5. 组件架构

### 5.1 Chat 页面组件树

```
ChatView
├── ChatTopBar                    # 顶部栏：标题 + 模型选择 + Node 选择
│   ├── EditableTitle             # 可编辑标题
│   ├── ModelSelector             # 模型下拉
│   └── NodeSelector              # 默认 Node 下拉
│
├── ChatMessages                  # 消息流（滚动容器）
│   ├── EmptyState                # 空状态（首次对话）
│   │   └── QuickActions          # 快捷操作按钮
│   │
│   ├── UserMessage               # 用户消息（靠右气泡）
│   │
│   ├── AssistantMessage          # AI 消息（靠左）
│   │   ├── ThinkingBlock         # 💭 思考过程（可折叠）
│   │   ├── ContentBlock          # 正文（markstream-vue 流式 Markdown 渲染）
│   │   ├── ToolCallCard          # 🔧 Tool 执行卡片（可折叠）
│   │   │   ├── ToolCallHeader    # 标题行：状态图标 + tool名 + node + 耗时
│   │   │   └── ToolCallOutput    # 命令 + stdout/stderr 输出
│   │   └── ConfirmCard           # ⚠️ 敏感操作确认卡片
│   │       ├── ConfirmHeader     # 警告标题
│   │       ├── CommandPreview    # 命令预览
│   │       └── ConfirmActions    # 确认/拒绝按钮
│   │
│   └── StreamingIndicator        # 流式输出光标/加载指示
│
└── ChatInput                     # 底部输入区域
    ├── TextArea                  # 输入框（Shift+Enter 换行）
    └── SendButton / StopButton   # 发送 / 停止按钮
```

### 5.2 核心组件设计

#### ChatMessages — 消息流容器

```
┌──────────────────────────────────────────────┐
│  (overflow-y: auto, flex: 1)                 │
│                                              │
│  遍历 messages + streaming 渲染：              │
│                                              │
│  for msg in messages:                        │
│    if msg.role === 'user'   → UserMessage    │
│    if msg.role === 'assistant' →             │
│      AssistantMessage (含 toolCalls 子组件)   │
│    // role === 'tool' 不单独渲染，             │
│    // 合并在 AssistantMessage 的 ToolCallCard │
│                                              │
│  if streaming !== null:                      │
│    StreamingAssistantMessage                 │
│    (实时渲染 reasoning + content + toolCalls) │
│                                              │
│  auto-scroll to bottom on new content        │
└──────────────────────────────────────────────┘
```

**自动滚动策略**：
- 新消息 / streaming delta 时自动滚到底部
- 用户手动向上滚动后暂停自动滚动
- 出现 "↓ 新消息" 浮动按钮，点击回到底部

#### UserMessage — 用户消息

```
┌─────────────────────────────────────────┐
│                  ┌──────────────────┐   │
│                  │  消息内容         │   │  靠右
│                  │  (--secondary bg) │   │  药丸圆角
│                  └──────────────────┘   │
└─────────────────────────────────────────┘
```

#### ThinkingBlock — AI 思考过程

```
┌─ 💭 Thinking...  ─────────────── ▶ 展开 ─┐
│  (默认折叠，显示前 50 字摘要)               │
│  斜体 / fontSize: 13 / --muted-foreground │
└───────────────────────────────────────────┘

展开后：
┌─ 💭 Thinking...  ─────────────── ▼ 收起 ─┐
│  完整 reasoning 文本                       │
│  斜体 / fontSize: 13 / --muted-foreground │
└───────────────────────────────────────────┘
```

- streaming 期间实时更新摘要文字
- 使用 `<Collapsible>` 组件

#### ContentBlock — Markdown 正文（markstream-vue）

使用 [markstream-vue](https://github.com/Simon-He95/markstream-vue) 渲染 AI 输出的 Markdown 内容，支持流式增量渲染。

```vue
<script setup lang="ts">
import MarkdownRender, { parseMarkdownToStructure, getMarkdown } from 'markstream-vue'
import 'markstream-vue/index.css'

const props = defineProps<{
  content: string        // Markdown 文本（streaming 时不断拼接）
  streaming?: boolean    // 是否正在流式输出
}>()

const md = getMarkdown()
const nodes = computed(() => parseMarkdownToStructure(props.content, md))
</script>

<template>
  <MarkdownRender :nodes="nodes" />
</template>
```

**两种渲染模式**：

| 场景 | 模式 | 说明 |
|------|------|------|
| 流式输出中 | 增量渲染 | `content` 随 WS delta 持续拼接，`parseMarkdownToStructure` 增量解析，无闪烁无重排 |
| 历史消息 | Virtual Window | 完整文本一次渲染，默认 320 节点窗口，长对话不会爆内存 |

**代码高亮**：通过 Shiki 作为 peer dependency 提供语法高亮，支持 shell、json、yaml、nginx 等运维常见语言。

#### ToolCallCard — 工具执行卡片

三种状态（来自设计稿）：

**执行中** (`status: 'executing'`)：
```
┌─ ◐ execute_command @ web-1 ──── Executing... ─┐
│  $ systemctl status nginx                      │
│  (流式 stdout 实时显示 / 加载动画)               │
└────────────────────────────────────────────────┘
  橙色左边框 / --card 背景 / --border 边框
```

**成功** (`status: 'success'`, 默认折叠)：
```
┌─ ✅ execute_command @ web-1 ─── exit:0  1.2s ▶ ─┐
│  $ systemctl status nginx                        │
└──────────────────────────────────────────────────┘
  展开后显示完整 stdout/stderr
```

**失败** (`status: 'error'`)：
```
┌─ ❌ execute_command @ web-1 ─── exit:1  0.3s ▶ ─┐
│  $ systemctl status nginx                        │
│  stderr 内容（展开）                               │
└──────────────────────────────────────────────────┘
  --color-error 边框 / 失败默认展开
```

#### ConfirmCard — 敏感操作确认

```
┌─ ⚠️ Sensitive Operation Confirmation ────────────┐
│  About to execute on web-1:                       │
│  ┌────────────────────────────────────────┐      │
│  │ $ systemctl restart nginx              │      │
│  └────────────────────────────────────────┘      │
│                                                   │
│  [Confirm]  [Reject]                              │
│                                                   │
│  (确认后按钮变为已确认/已拒绝，不可再操作)            │
└───────────────────────────────────────────────────┘
  --color-warning 背景 / --color-warning-foreground 边框
```

- `status === 'confirming'` 时显示按钮
- 用户操作后变为 `confirmed` 或 `rejected` 文字标签

#### ChatInput — 底部输入区

```
┌─────────────────────────────────────────────────┐
│  ┌──────────────────────────────────┐  ┌──┐    │
│  │ Send a message...               │  │🔶│    │  idle 状态
│  └──────────────────────────────────┘  └──┘    │
│                  输入框                  发送     │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  ┌──────────────────────────────────┐  ┌──┐    │
│  │ AI is responding...             │  │⏹ │    │  执行中状态
│  └──────────────────────────────────┘  └──┘    │
│              禁用输入框                  停止     │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│  ┌──────────────────────────────────┐           │  确认中状态
│  │ Waiting for your confirmation...│           │
│  └──────────────────────────────────┘           │
│              禁用输入框、无按钮                     │
└─────────────────────────────────────────────────┘
```

状态映射：
| conversation.status | 输入框 | 按钮 | placeholder |
|---|---|---|---|
| `idle` | 可用 | 🔶 发送 (Send) | "Send a message..." |
| `streaming` | 禁用 | 🔴 停止 (Stop) | "AI is responding..." |
| `tool_exec` | 禁用 | 🔴 停止 (Stop) | "AI is processing..." |
| `confirming` | 禁用 | 无 | "Waiting for your confirmation..." |
| `error` | 可用 | 🔶 发送 | "Send a message..." |

---

## 6. EmptyState — 空对话状态

设计稿显示的空状态（Chat - Empty State）：

```
┌─────────────────────────────────────────┐
│                                         │
│              💬 (图标)                   │
│                                         │
│   How can I help you manage your        │
│              servers?                   │
│                                         │
│  [Check nginx status] [View disk usage] │
│  [Update packages]                      │
│                                         │
└─────────────────────────────────────────┘
```

- 居中显示在消息区域
- Quick Action 按钮点击后等同于发送对应消息文本
- 有消息后自动隐藏

---

## 7. 非 Chat 页面消息的静默处理

由于是单 WS 多路复用，用户可能在 Nodes 页面时某个对话的 Loop 还在跑。处理策略：

- WS 事件照常接收和更新 Store，但不在当前页面渲染
- 用户切回 Chat 页面时，状态已经是最新的
- 如果 Loop 在用户不在 Chat 页面时结束（`done`），正常 finalize

---

## 8. 目录结构

```
web/src/
├── App.vue
├── main.ts
├── router/
│   └── index.ts                      # 路由定义
│
├── services/
│   ├── ws.ts                         # WebSocketService 单例
│   └── api.ts                        # REST API 封装（axios）
│
├── stores/
│   ├── chat.ts                       # Chat 状态管理
│   ├── nodes.ts                      # Nodes 状态
│   ├── settings.ts                   # 设置状态
│   └── app.ts                        # 全局状态（WS 连接状态、主题等）
│
├── composables/
│   ├── useChatWebSocket.ts           # Chat WS 事件处理（全局注册一次）
│   ├── useAutoScroll.ts              # 消息区自动滚动
│   └── useTheme.ts                   # 主题管理
│
├── views/
│   ├── ChatView.vue                  # 对话页（核心）
│   ├── NodesView.vue                 # 节点管理
│   ├── NodeDetailView.vue            # 节点详情
│   ├── AuditLogView.vue              # 审计日志
│   ├── SettingsView.vue              # 系统设置
│   ├── MonitorView.vue               # 链路监控
│   ├── LinkDetailView.vue            # 链路详情
│   └── AlertsView.vue               # 告警列表
│
├── components/
│   ├── layout/
│   │   ├── AppLayout.vue             # 整体布局（Sidebar + RouterView）
│   │   ├── AppSidebar.vue            # 侧边栏
│   │   └── ConversationList.vue      # 对话列表
│   │
│   ├── chat/
│   │   ├── ChatTopBar.vue            # 顶部栏
│   │   ├── ChatMessages.vue          # 消息流容器
│   │   ├── ChatInput.vue             # 底部输入
│   │   ├── EmptyState.vue            # 空状态
│   │   ├── UserMessage.vue           # 用户消息
│   │   ├── AssistantMessage.vue      # AI 消息（聚合组件）
│   │   ├── ThinkingBlock.vue         # 思考过程
│   │   ├── ContentBlock.vue          # Markdown 正文（markstream-vue 流式渲染）
│   │   ├── ToolCallCard.vue          # Tool 执行卡片
│   │   └── ConfirmCard.vue           # 敏感操作确认
│   │
│   └── common/                       # 通用组件（基于 shadcn-vue 封装）
│
├── types/
│   ├── api.ts                        # REST API 类型（从 schema 复制）
│   └── ws.ts                         # WebSocket 类型（从 schema 复制 + conversation_id 扩展）
│
└── assets/
    └── styles/
        └── variables.css             # CSS 变量（对应设计 Token）
```

---

## 9. 关键交互流

### 9.1 完整对话流程

```
1. 用户输入 "Check nginx status on web-1" → 点击发送
2. ChatStore.sendMessage()
   → 追加 UserMessage 到 messages
   → 初始化 streaming
   → status = 'streaming'
   → WS 发送 user_message
3. 收到 reasoning delta × N
   → 更新 streaming.reasoning
   → ThinkingBlock 实时渲染
4. 收到 content delta × N
   → 更新 streaming.content
   → ContentBlock 实时渲染
5. 收到 tool_call (execute_command)
   → status = 'tool_exec'
   → 追加到 streaming.toolCalls
   → ToolCallCard 显示 "Executing..."
6. 收到 tool_result
   → 更新对应 toolCall 的 result
   → ToolCallCard 更新为成功/失败
7. 可能继续收到 content delta（AI 分析结果）
   → status = 'streaming'
8. 收到 done
   → finalizeStreaming()
   → status = 'idle'
   → 输入框恢复可用
```

### 9.2 敏感操作确认流程

```
1. 收到 confirm_request
   → status = 'confirming'
   → ConfirmCard 渲染（Confirm / Reject 按钮）
   → 输入框显示 "Waiting for your confirmation..."
2. 用户点击 Confirm
   → ChatStore.confirmAction(id, true)
   → WS 发送 confirm_response
   → ConfirmCard 变为 "Confirmed" 标签
   → status = 'tool_exec'
3. 收到 tool_result → 正常流程继续
```

### 9.3 Session Replaced 流程

```
1. 用户在新 tab 打开 tolato
2. 旧 tab 收到 session_replaced
   → wsService.state = 'replaced'
   → 全屏遮罩："已在其他窗口打开，点击此处重新连接"
   → 不自动重连
3. 用户点击重新连接
   → wsService.connect()
   → 新 tab 被踢，当前 tab 恢复
```
