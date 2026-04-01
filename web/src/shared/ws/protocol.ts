import type {
  SessionListItem,
  SessionSnapshot,
  TimelineRow,
} from '@/shared/types/console'

export type WSUIEvent =
  | { type: 'connection.ready'; timestamp: string }
  | { type: 'connection.synced'; timestamp: string }
  | { type: 'connection.reconnecting'; timestamp: string; attempt: number }
  | { type: 'connection.offline'; timestamp: string; reason?: string }
  | { type: 'sessions.replaced'; sessions: SessionListItem[] }
  | { type: 'session.snapshot.replaced'; snapshot: SessionSnapshot }
  | { type: 'session.summary.updated'; session: SessionListItem }
  | { type: 'session.unread.updated'; sessionId: string; unread: number }
  | {
      type: 'session.state.updated'
      sessionId: string
      revision: number
      status?: string
    }
  | {
      type: 'timeline.row.appended'
      sessionId: string
      row: TimelineRow
      revision: number
    }
  | {
      type: 'timeline.row.updated'
      sessionId: string
      row: TimelineRow
      revision: number
    }
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

export interface SessionsListRequest {
  type: 'sessions.list.request'
}

export interface SessionCreateRequest {
  type: 'session.create'
  title?: string
}

export interface SessionDeleteRequest {
  type: 'session.delete'
  sessionId: string
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

export interface SubscriptionsUpdateRequest {
  type: 'subscriptions.update'
  activeSessionId: string
  watchSessionIds: string[]
}

export type WSUIRequest =
  | SessionCreateRequest
  | SessionDeleteRequest
  | SessionsListRequest
  | SessionSnapshotRequest
  | SessionMessageSubmitRequest
  | SubscriptionsUpdateRequest

export interface WSClient {
  connect(): Promise<void>
  disconnect(): void
  subscribe(handler: (event: WSUIEvent) => void): () => void
  createSession(request: SessionCreateRequest): Promise<{ sessionId: string }>
  deleteSession(request: SessionDeleteRequest): Promise<{ sessionId: string }>
  requestSessionsList(): Promise<SessionListItem[]>
  requestSessionSnapshot(sessionId: string): Promise<SessionSnapshot>
  updateSubscriptions(request: SubscriptionsUpdateRequest): Promise<void>
  submitMessage(request: SessionMessageSubmitRequest): Promise<void>
}
