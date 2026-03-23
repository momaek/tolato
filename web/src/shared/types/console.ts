import type { ApprovalStatus, TaskStatus } from '@/shared/types/history'
import type { NodeSummary } from '@/shared/types/node'

export type SessionStatus = 'idle' | 'running' | 'attention' | 'completed'
export type SessionMode = 'ai_agent' | 'direct_shell'
export type RiskLevel = 'low' | 'medium' | 'high'

export interface TargetCandidate {
  id: string
  nodeId: string
  label: string
  region: string
  scope: 'single' | 'multi' | 'all_online'
  reason: string
  source: 'resolver' | 'manual' | 'session_context'
  tags: string[]
}

export interface TargetContext {
  state: 'unset' | 'pending_confirmation' | 'confirmed'
  scope: 'single' | 'multi' | 'all_online' | 'unset'
  summary: string
  source: 'resolver' | 'manual' | 'session_context' | 'none'
  candidates: TargetCandidate[]
  confirmedNodeIds: string[]
}

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
}

export interface ToolResultMetaRow extends TimelineRowBase {
  kind: 'tool_result_meta'
  label: string
  tone: 'neutral' | 'success' | 'warning'
}

export interface TargetConfirmationRow extends TimelineRowBase {
  kind: 'target_confirmation'
  title: string
  originalTargetText: string
  basis: string
  scope: string
  source: string
  inheritedHint?: string
  candidates: TargetCandidate[]
}

export interface PlanStep {
  action: string
  argsLabel: string
  risk: RiskLevel
  timeoutSec: number
  broadcastAllowed: boolean
}

export interface PlanRow extends TimelineRowBase {
  kind: 'plan'
  inputText: string
  summary: string
  impact: string
  risk: RiskLevel
  requiresApproval: boolean
  targetLabel: string
  targetSource: 'assistant_resolved' | 'context_inherited' | 'manual'
  autoExecutionHint?: string
  steps: PlanStep[]
}

export interface ApprovalRow extends TimelineRowBase {
  kind: 'approval'
  reason: string
  risk: RiskLevel
  impact: string
  targetLabel: string
}

export interface ExecutionNodeState {
  nodeId: string
  label: string
  region: string
  status: 'queued' | 'running' | 'success' | 'failed' | 'skipped'
  stdoutTail?: string
  stderrTail?: string
  exitCode?: number
}

export interface ExecutionRow extends TimelineRowBase {
  kind: 'execution'
  title: string
  status: TaskStatus
  nodes: ExecutionNodeState[]
}

export interface SummaryRow extends TimelineRowBase {
  kind: 'summary'
  total: number
  success: number
  failed: number
  skipped: number
  markdown: string
  nextSteps: string[]
}

export type TimelineRow =
  | UserMessageRow
  | AssistantTextRow
  | ToolCallMetaRow
  | ToolResultMetaRow
  | TargetConfirmationRow
  | PlanRow
  | ApprovalRow
  | ExecutionRow
  | SummaryRow

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
  approvalStatus: ApprovalStatus
  targetContext: TargetContext
  rows: TimelineRow[]
  candidateNodes: NodeSummary[]
  highlightedNodes: NodeSummary[]
  nodeHealthSummary: {
    online: number
    offline: number
    busy: number
  }
  llmStreamState?: LlmStreamState
  pendingActionType?: 'target_confirmation' | 'approval'
}

export interface SessionListItem {
  id: string
  title: string
  summary: string
  status: SessionStatus
  unread: number
  updatedAt: string
  targetSummary: string
}
