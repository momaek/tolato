export type SessionStatus = 'idle' | 'running' | 'failed'
export type SessionMode = 'ai_agent' | 'direct_shell'

interface TimelineRowBase {
  id: string
  createdAt: string
  taskId?: string
}

export interface UserMessageRow extends TimelineRowBase {
  kind: 'user_message'
  text: string
}

export interface AssistantTextRow extends TimelineRowBase {
  kind: 'assistant_text'
  markdown: string
}

export interface ToolCallMetaRow extends TimelineRowBase {
  kind: 'tool_call_meta'
  label: string
  toolName?: string
  argsPreview?: string
}

export interface ToolResultMetaRow extends TimelineRowBase {
  kind: 'tool_result_meta'
  label: string
  tone: 'neutral' | 'success' | 'warning'
}

export type TimelineRow =
  | UserMessageRow
  | AssistantTextRow
  | ToolCallMetaRow
  | ToolResultMetaRow

export interface LlmStreamEvent {
  sequenceNumber?: number
  upstreamEventType: string
  rawEvent: Record<string, unknown>
}

export interface LlmStreamState {
  responseId?: string
  status: 'streaming' | 'completed'
  contentText?: string
  reasoningText?: string
  pendingToolName?: string
  pendingToolArguments?: string
  events: LlmStreamEvent[]
}

export interface SessionSnapshot {
  id: string
  title: string
  summary: string
  status: SessionStatus
  mode: SessionMode
  revision: number
  updatedAt: string
  unread: number
  rows: TimelineRow[]
  nodeHealthSummary: {
    online: number
    offline: number
    busy: number
  }
  llmStreamState?: LlmStreamState
}

export interface SessionListItem {
  id: string
  title: string
  summary: string
  status: SessionStatus
  unread: number
  updatedAt: string
}

// ── Turn-based conversation model ──

export interface ThinkingBlock {
  type: 'thinking'
  text: string
}

export interface TextBlock {
  type: 'text'
  text: string
  rowId?: string
}

export interface ToolUseBlock {
  type: 'tool_use'
  toolName: string
  argsPreview?: string
  callRowId?: string
  result?: {
    label: string
    tone: 'neutral' | 'success' | 'warning'
    rowId: string
  }
}

export type ContentBlock = ThinkingBlock | TextBlock | ToolUseBlock

export type TurnStatus = 'streaming' | 'completed'

export interface UserTurn {
  type: 'user'
  id: string
  createdAt: string
  text: string
}

export interface AssistantTurn {
  type: 'assistant'
  id: string
  createdAt: string
  status: TurnStatus
  blocks: ContentBlock[]
  responseId?: string
}

export type Turn = UserTurn | AssistantTurn
