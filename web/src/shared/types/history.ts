export type TaskStatus =
  | 'planned'
  | 'waiting_approval'
  | 'approved'
  | 'queued'
  | 'running'
  | 'success'
  | 'failed'
  | 'partial_failed'
  | 'cancelled'

export type ApprovalStatus = 'not_required' | 'pending' | 'approved' | 'rejected' | 'cancelled'

export interface HistoryAuditEvent {
  id: string
  actor: string
  eventType: string
  description: string
  createdAt: string
}

export interface HistoryExecutionSummary {
  id: string
  taskId: string
  nodeId: string
  label: string
  status: 'queued' | 'running' | 'success' | 'failed' | 'skipped'
  startedAt?: string
  finishedAt?: string
  exitCode?: number
  stdoutTail?: string
  stderrTail?: string
  streamSummary?: string
}

export interface HistoryPlanStep {
  id: string
  action: string
  args?: Record<string, unknown>
  risk: string
}

export interface HistoryPlanDetail {
  targetNodes: string[]
  summary: string
  estimatedImpact: string
  riskLevel: string
  requiresApproval: boolean
  steps: HistoryPlanStep[]
  sourceToolResultId?: string
}

export interface HistoryApprovalDetail {
  status: ApprovalStatus
  riskLevel: 'low' | 'medium' | 'high'
  requiresApproval: boolean
  latestDecision?: string
  latestReason?: string
  latestActor?: string
  latestTimestamp?: string
}

export interface HistoryToolCall {
  id: string
  toolName: string
  source: string
  argsPreview?: string
  arguments?: Record<string, unknown>
  createdAt: string
}

export interface HistoryToolResult {
  id: string
  toolName: string
  status: string
  text?: string
  source: string
  payload?: Record<string, unknown>
  createdAt: string
}

export interface HistoryTimelineMetaRow {
  id: string
  kind: string
  text?: string
  toolName?: string
  toolStatus?: string
  source?: string
  argsPreview?: string
  createdAt: string
}

export interface HistoryTaskItem {
  id: string
  title: string
  summary: string
  status: TaskStatus
  approvalStatus: ApprovalStatus
  risk: 'low' | 'medium' | 'high'
  targetLabels: string[]
  createdAt: string
  updatedAt: string
}

export interface HistoryTaskDetail extends HistoryTaskItem {
  mode?: 'ai_agent' | 'direct_shell'
  inputText?: string
  target?: string[]
  impact: string
  steps: string[]
  plan?: HistoryPlanDetail
  approval?: HistoryApprovalDetail
  executions: HistoryExecutionSummary[]
  auditEvents: HistoryAuditEvent[]
  toolMeta: string[]
  toolCalls?: HistoryToolCall[]
  toolResults?: HistoryToolResult[]
  planRows?: HistoryTimelineMetaRow[]
  approvalRows?: HistoryTimelineMetaRow[]
  executionRows?: HistoryTimelineMetaRow[]
  summaryRows?: HistoryTimelineMetaRow[]
  aiSummary: string
}
