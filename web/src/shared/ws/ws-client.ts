import { t } from '@/app/i18n'
import { appEnv } from '@/shared/config/env'
import { createMockSessions, toSessionListItem } from '@/shared/mock/sessions'
import type {
  AssistantTextRow,
  SessionSnapshot,
  ToolCallMetaRow,
  ToolResultMetaRow,
  UserMessageRow,
} from '@/shared/types/console'
import { createEventBus } from '@/shared/ws/event-bus'
import { RealWSClient } from '@/shared/ws/real-client'
import { delay } from '@/shared/ws/reconnect'
import type {
  SessionCreateRequest,
  SessionDeleteRequest,
  SessionMessageSubmitRequest,
  SubscriptionsUpdateRequest,
  WSClient,
  WSUIEvent,
} from '@/shared/ws/protocol'

function cloneSnapshot(snapshot: SessionSnapshot) {
  return structuredClone(snapshot)
}

function nowIso() {
  return new Date().toISOString()
}

class MockWSClient implements WSClient {
  private bus = createEventBus<WSUIEvent>()
  private sessions = new Map<string, SessionSnapshot>()
  private activeSessionId = ''
  private watchSessionIds = new Set<string>()

  constructor() {
    createMockSessions().forEach(session => {
      this.sessions.set(session.id, session)
    })
  }

  async connect() {
    this.bus.emit({ type: 'connection.ready', timestamp: nowIso() })
    await delay(120)
    this.bus.emit({ type: 'connection.synced', timestamp: nowIso() })
  }

  disconnect() {}

  subscribe(handler: (event: WSUIEvent) => void) {
    return this.bus.on(handler)
  }

  async requestSessionsList() {
    return Array.from(this.sessions.values())
      .sort((a, b) => +new Date(b.updatedAt) - +new Date(a.updatedAt))
      .map(toSessionListItem)
  }

  async createSession(request: SessionCreateRequest) {
    const sessionId = `session-ops-${Math.floor(Math.random() * 900 + 100)}`
    const title = request.title?.trim() || t('mockWs.newSession')
    const snapshot: SessionSnapshot = {
      id: sessionId,
      title,
      summary: t('mockWs.waitingInput'),
      status: 'idle',
      mode: 'ai_agent',
      revision: 1,
      updatedAt: nowIso(),
      unread: 0,
      rows: [],
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
    }
    this.sessions.set(sessionId, snapshot)
    return { sessionId }
  }

  async deleteSession(request: SessionDeleteRequest) {
    this.sessions.delete(request.sessionId)
    if (this.activeSessionId === request.sessionId) {
      this.activeSessionId = ''
    }
    this.watchSessionIds.delete(request.sessionId)
    return { sessionId: request.sessionId }
  }

  async requestSessionSnapshot(sessionId: string) {
    const snapshot = this.sessions.get(sessionId)
    if (!snapshot) {
      throw new Error(t('ws.unknownSession', { sessionId }))
    }
    return cloneSnapshot(snapshot)
  }

  async updateSubscriptions(request: SubscriptionsUpdateRequest) {
    this.activeSessionId = request.activeSessionId
    this.watchSessionIds = new Set(request.watchSessionIds)
    const activeSnapshot = this.sessions.get(request.activeSessionId)
    if (activeSnapshot && activeSnapshot.unread !== 0) {
      activeSnapshot.unread = 0
      this.bus.emit({ type: 'session.unread.updated', sessionId: activeSnapshot.id, unread: 0 })
    }
    this.bus.emit({ type: 'connection.synced', timestamp: nowIso() })
    if (request.watchSessionIds.length > 0) {
      request.watchSessionIds.forEach(sessionId => {
        const snapshot = this.sessions.get(sessionId)
        if (snapshot) {
          this.emitSessionSummary(snapshot, false)
        }
      })
    }
  }

  async submitMessage(request: SessionMessageSubmitRequest) {
    const snapshot = this.requireSession(request.sessionId)

    snapshot.status = 'running'
    snapshot.summary = 'Processing...'
    this.bumpRevision(snapshot)

    // 1. User message row
    const userRow = this.makeUserRow(request.text)
    snapshot.rows.push(userRow)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: userRow, revision: snapshot.revision })
    this.bus.emit({ type: 'session.state.updated', sessionId: snapshot.id, revision: snapshot.revision, status: 'running' })
    await delay(90)

    // 2. Stream thinking + first text
    const responseId1 = `resp-${crypto.randomUUID()}`
    await this.emitStreamDeltas(snapshot.id, responseId1, {
      reasoning: 'Analyzing the user request and determining what tools to call.',
      content: `I'll look into "${request.text}" for you.`,
    })

    // 3. First assistant text row (finalizes streamed text)
    const assistantRow1 = this.makeAssistantRow(`I'll look into "${request.text}" for you.`)
    snapshot.rows.push(assistantRow1)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: assistantRow1, revision: snapshot.revision })
    await delay(90)

    // 4. Tool call
    const toolCallRow = this.makeToolCallRow('inspect_nodes')
    snapshot.rows.push(toolCallRow)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: toolCallRow, revision: snapshot.revision })
    await delay(400)

    // 5. Tool result
    const toolResultRow = this.makeToolResultRow('inspect_nodes completed successfully', 'success')
    snapshot.rows.push(toolResultRow)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: toolResultRow, revision: snapshot.revision })
    await delay(90)

    // 6. Stream second text
    const responseId2 = `resp-${crypto.randomUUID()}`
    await this.emitStreamDeltas(snapshot.id, responseId2, {
      content: 'The operation completed. Let me know if you need anything else.',
    })

    // 7. Second assistant text row
    const assistantRow2 = this.makeAssistantRow('The operation completed. Let me know if you need anything else.')
    snapshot.rows.push(assistantRow2)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: assistantRow2, revision: snapshot.revision })
    await delay(90)

    // 8. Done
    snapshot.status = 'idle'
    snapshot.summary = 'Completed task.'
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'session.state.updated', sessionId: snapshot.id, revision: snapshot.revision, status: 'idle' })
    this.emitSessionSummary(snapshot)
  }

  private emitSessionSummary(snapshot: SessionSnapshot, markUnread = true) {
    snapshot.updatedAt = nowIso()
    if (markUnread && snapshot.id !== this.activeSessionId && this.watchSessionIds.has(snapshot.id)) {
      snapshot.unread += 1
      this.bus.emit({ type: 'session.unread.updated', sessionId: snapshot.id, unread: snapshot.unread })
    }
    this.bus.emit({ type: 'session.summary.updated', session: toSessionListItem(snapshot) })
  }

  private bumpRevision(snapshot: SessionSnapshot) {
    snapshot.revision += 1
    snapshot.updatedAt = nowIso()
  }

  private requireSession(sessionId: string) {
    const session = this.sessions.get(sessionId)
    if (!session) {
      throw new Error(t('ws.unknownSession', { sessionId }))
    }
    return session
  }

  private makeUserRow(text: string): UserMessageRow {
    return {
      id: `row-user-${crypto.randomUUID()}`,
      kind: 'user_message',
      createdAt: nowIso(),
      text,
    }
  }

  private makeToolCallRow(label: string): ToolCallMetaRow {
    return {
      id: `row-call-${crypto.randomUUID()}`,
      kind: 'tool_call_meta',
      createdAt: nowIso(),
      label,
    }
  }

  private makeToolResultRow(label: string, tone: ToolResultMetaRow['tone']): ToolResultMetaRow {
    return {
      id: `row-result-${crypto.randomUUID()}`,
      kind: 'tool_result_meta',
      createdAt: nowIso(),
      label,
      tone,
    }
  }

  private makeAssistantRow(markdown: string): AssistantTextRow {
    return {
      id: `row-assistant-${crypto.randomUUID()}`,
      kind: 'assistant_text',
      createdAt: nowIso(),
      markdown,
    }
  }

  private async emitStreamDeltas(sessionId: string, responseId: string, input: { reasoning?: string; content?: string }) {
    let sequenceNumber = 1

    if (input.reasoning) {
      const chunks = this.chunkText(input.reasoning, 10)
      for (const delta of chunks) {
        this.bus.emit({
          type: 'llm.sse.event',
          sessionId,
          responseId,
          sequenceNumber,
          upstreamEventType: 'response.reasoning_text.delta',
          rawEvent: { delta },
        })
        sequenceNumber += 1
        await delay(60)
      }
      await delay(80)
    }

    if (input.content) {
      const chunks = this.chunkText(input.content, 12)
      for (const delta of chunks) {
        this.bus.emit({
          type: 'llm.sse.event',
          sessionId,
          responseId,
          sequenceNumber,
          upstreamEventType: 'response.output_text.delta',
          rawEvent: { delta },
        })
        sequenceNumber += 1
        await delay(50)
      }
    }

    await delay(100)
    this.bus.emit({
      type: 'llm.response.completed',
      sessionId,
      responseId,
      rawResponse: {
        id: responseId,
        reasoning_text: input.reasoning ?? '',
        output_text: input.content ?? '',
      },
    })
  }

  private chunkText(text: string, size: number) {
    const chunks: string[] = []
    for (let index = 0; index < text.length; index += size) {
      chunks.push(text.slice(index, index + size))
    }
    return chunks
  }
}

let clientSingleton: WSClient | null = null

export function getWSClient(): WSClient {
  if (!clientSingleton) {
    clientSingleton = appEnv.useMock ? new MockWSClient() : new RealWSClient()
  }

  return clientSingleton
}

export function resetWSClient() {
  clientSingleton?.disconnect()
  clientSingleton = null
}

export function isMockWSClient() {
  return appEnv.useMock
}
