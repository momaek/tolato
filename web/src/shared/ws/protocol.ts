import type { ApprovalRow, ExecutionRow, SessionListItem, SessionSnapshot, TargetCandidate, TargetContext, TimelineRow } from '@/shared/types/console'

export type WSUIEvent =
  | { type: 'connection.ready'; timestamp: string }
  | { type: 'connection.synced'; timestamp: string }
  | { type: 'session.summary.updated'; session: SessionListItem }
  | { type: 'session.unread.updated'; sessionId: string; unread: number }
  | { type: 'session.state.updated'; sessionId: string; revision: number; status?: string }
  | { type: 'timeline.row.appended'; sessionId: string; row: TimelineRow; revision: number }
  | { type: 'timeline.row.updated'; sessionId: string; row: TimelineRow; revision: number }
  | { type: 'thread.target.pending'; sessionId: string; revision: number; targetContext: TargetContext }
  | { type: 'thread.target.confirmed'; sessionId: string; revision: number; targetContext: TargetContext }
  | { type: 'thread.target.cleared'; sessionId: string; revision: number; targetContext: TargetContext }
  | {
      type: 'llm.sse.event'
      sessionId: string
      responseId?: string
      sequenceNumber?: number
      upstreamEventType: string
      rawEvent: Record<string, unknown>
    }
  | {
      type: 'llm.response.completed'
      sessionId: string
      responseId?: string
      rawResponse: Record<string, unknown>
    }
  | { type: 'execution.chunk'; sessionId: string; row: ExecutionRow; revision: number }
  | { type: 'execution.finished'; sessionId: string; row: ExecutionRow; revision: number }
  | { type: 'session.requires_attention'; sessionId: string; revision: number }
  | { type: 'session.finished'; sessionId: string; revision: number }

export interface SessionsListRequest {
  type: 'sessions.list.request'
}

export interface SessionSnapshotRequest {
  type: 'session.snapshot.request'
  sessionId: string
}

export interface SessionMessageSubmitRequest {
  type: 'session.message.submit'
  sessionId: string
  text: string
}

export interface SessionTargetConfirmRequest {
  type: 'session.target.confirm'
  sessionId: string
  action: 'confirm' | 'reselect' | 'clear'
  candidate?: TargetCandidate
}

export interface SessionApprovalRequest {
  type: 'session.approval.action'
  sessionId: string
  action: 'approve' | 'reject' | 'cancel'
  approvalRow?: ApprovalRow
}

export interface SubscriptionsUpdateRequest {
  type: 'subscriptions.update'
  activeSessionId: string
  watchSessionIds: string[]
}

export type WSUIRequest =
  | SessionsListRequest
  | SessionSnapshotRequest
  | SessionMessageSubmitRequest
  | SessionTargetConfirmRequest
  | SessionApprovalRequest
  | SubscriptionsUpdateRequest

export interface WSClient {
  connect(): Promise<void>
  subscribe(handler: (event: WSUIEvent) => void): () => void
  requestSessionsList(): Promise<SessionListItem[]>
  requestSessionSnapshot(sessionId: string): Promise<SessionSnapshot>
  updateSubscriptions(request: SubscriptionsUpdateRequest): Promise<void>
  submitMessage(request: SessionMessageSubmitRequest): Promise<void>
  confirmTarget(request: SessionTargetConfirmRequest): Promise<void>
  submitApproval(request: SessionApprovalRequest): Promise<void>
}
