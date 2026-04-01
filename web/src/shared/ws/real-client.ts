import { t } from '@/app/i18n'
import { listNodes } from '@/shared/api/adapters/nodes'
import { getAccessToken } from '@/shared/auth/session'
import { appEnv } from '@/shared/config/env'
import type {
  SessionListItem,
  SessionSnapshot,
  TimelineRow,
  ToolResultMetaRow,
} from '@/shared/types/console'
import type { NodeSummary } from '@/shared/types/node'
import { createEventBus } from '@/shared/ws/event-bus'
import { delay } from '@/shared/ws/reconnect'
import type {
  SessionCreateRequest,
  SessionDeleteRequest,
  SessionMessageSubmitRequest,
  SubscriptionsUpdateRequest,
  WSClient,
  WSUIEvent,
} from '@/shared/ws/protocol'

type PendingRequest = {
  resolve: (value: unknown) => void
  reject: (error: Error) => void
}

type BackendEnvelope = {
  type: string
  requestId?: string
  payload?: unknown
  error?: {
    code?: string
    message?: string
  }
  sessionId?: string
  summary?: {
    title?: string
    status?: string
    unread?: number
    updatedAt?: string
  }
  responseId?: string
  sequenceNumber?: number
  upstreamEventType?: string
  rawEvent?: Record<string, unknown>
  rawResponse?: Record<string, unknown>
  status?: string
  revision?: number
  row?: BackendTimelineRow
  timestamp?: string
}

type BackendSessionListItem = {
  sessionId: string
  title: string
  status: string
  updatedAt: string
  unread?: number
}

type BackendTimelineRow = {
  ID?: string
  Kind?: string
  CreatedAt?: string
  Text?: string
  ToolName?: string
  ToolStatus?: string
  Source?: string
  ArgsPreview?: string
  TaskID?: string
  id?: string
  kind?: string
  createdAt?: string
  text?: string
  toolName?: string
  toolStatus?: string
  source?: string
  argsPreview?: string
  taskId?: string
}

type BackendSnapshot = {
  session: {
    id: string
    title: string
    status: string
    updatedAt: string
    revision: number
  }
  sidebarSummary?: {
    primaryText?: string
  }
  timeline?: {
    rows?: BackendTimelineRow[]
  }
}

function buildWSURL(path: string) {
  const baseURL = appEnv.apiBaseUrl
  const origin = /^https?:\/\//.test(baseURL)
    ? new URL(baseURL).origin
    : window.location.origin
  const url = new URL(path, origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  url.search = ''
  url.hash = ''
  const token = getAccessToken()
  if (token) {
    url.searchParams.set('access_token', token)
  }
  return url.toString()
}

function requestId(prefix: string) {
  return `${prefix}-${globalThis.crypto?.randomUUID?.() ?? Math.random().toString(16).slice(2)}`
}

function messageId() {
  return (
    globalThis.crypto?.randomUUID?.() ?? Math.random().toString(16).slice(2)
  )
}

function mapSessionStatus(status: string): SessionListItem['status'] {
  switch (status) {
    case 'running':
      return 'running'
    case 'failed':
      return 'failed'
    default:
      return 'idle'
  }
}

function toneForToolResult(row: BackendTimelineRow): ToolResultMetaRow['tone'] {
  const text = readRowText(row).toLowerCase()
  if (
    row.toolStatus === 'failed' ||
    text.includes('failed') ||
    text.includes('rejected') ||
    text.includes('cancel')
  ) {
    return 'warning'
  }
  if (
    text.includes('succeeded') ||
    text.includes('success') ||
    text.includes('completed')
  ) {
    return 'success'
  }
  return 'neutral'
}

function readRowID(row: BackendTimelineRow) {
  return row.id || row.ID || ''
}

function readRowKind(row: BackendTimelineRow) {
  return row.kind || row.Kind || ''
}

function readRowCreatedAt(row: BackendTimelineRow) {
  return row.createdAt || row.CreatedAt || new Date().toISOString()
}

function readRowText(row: BackendTimelineRow) {
  return row.text || row.Text || ''
}

function readRowToolName(row: BackendTimelineRow) {
  return row.toolName || row.ToolName || ''
}

function readRowTaskID(row: BackendTimelineRow) {
  return row.taskId || row.TaskID || ''
}

function readRowArgsPreview(row: BackendTimelineRow) {
  return row.argsPreview || row.ArgsPreview || ''
}

function mapTimelineRows(rows: BackendTimelineRow[]): TimelineRow[] {
  return rows.map((row) => {
    const kind = readRowKind(row)

    switch (kind) {
      case 'user_message':
        return {
          id: readRowID(row),
          kind: 'user_message',
          createdAt: readRowCreatedAt(row),
          text: readRowText(row),
        } satisfies TimelineRow

      case 'assistant_text':
        return {
          id: readRowID(row),
          kind: 'assistant_text',
          createdAt: readRowCreatedAt(row),
          markdown: readRowText(row),
        } satisfies TimelineRow

      case 'tool_call_meta': {
        const toolName = readRowToolName(row)
        const args = readRowArgsPreview(row)
        return {
          id: readRowID(row),
          kind: 'tool_call_meta',
          createdAt: readRowCreatedAt(row),
          label: args
            ? `${toolName}(${args})`
            : toolName || readRowText(row) || 'tool call',
        } satisfies TimelineRow
      }

      case 'tool_result_meta':
        return {
          id: readRowID(row),
          kind: 'tool_result_meta',
          createdAt: readRowCreatedAt(row),
          label: readRowText(row) || readRowToolName(row) || 'tool result',
          tone: toneForToolResult(row),
          taskId: readRowTaskID(row) || undefined,
        } satisfies TimelineRow

      default:
        return {
          id: readRowID(row) || `row-${Math.random().toString(16).slice(2)}`,
          kind: 'tool_result_meta',
          createdAt: readRowCreatedAt(row),
          label: readRowText(row) || kind || 'unsupported row',
          tone: 'neutral',
        } satisfies TimelineRow
    }
  })
}

function deriveSummary(item: {
  status: string
  title?: string
  primaryText?: string
}) {
  if (item.primaryText) {
    return item.primaryText
  }
  switch (item.status) {
    case 'running':
      return t('ws.derivedRunning', { target: item.title || 'current task' })
    case 'failed':
      return t('ws.derivedFailed')
    default:
      return t('ws.derivedWaitingInput')
  }
}

function mapSessionListItem(item: BackendSessionListItem): SessionListItem {
  return {
    id: item.sessionId,
    title: item.title,
    summary: deriveSummary({
      status: item.status,
      title: item.title,
    }),
    status: mapSessionStatus(item.status),
    unread: item.unread ?? 0,
    updatedAt: item.updatedAt,
  }
}

export class RealWSClient implements WSClient {
  private bus = createEventBus<WSUIEvent>()
  private socket: WebSocket | null = null
  private pending = new Map<string, PendingRequest>()
  private connectPromise: Promise<void> | null = null
  private snapshots = new Map<string, SessionSnapshot>()
  private nodesPromise: Promise<NodeSummary[]> | null = null
  private reconnectAttempt = 0
  private reconnectLoop: Promise<void> | null = null
  private restorePromise: Promise<void> | null = null
  private manualDisconnect = false
  private lastSubscriptions: SubscriptionsUpdateRequest | null = null

  async connect() {
    if (this.socket?.readyState === WebSocket.OPEN) {
      return
    }

    if (this.connectPromise) {
      return this.connectPromise
    }

    this.manualDisconnect = false
    this.connectPromise = this.openSocket()
    return this.connectPromise
  }

  disconnect() {
    this.manualDisconnect = true
    this.pending.forEach((request) => {
      request.reject(new Error(t('ws.disconnected')))
    })
    this.pending.clear()
    this.snapshots.clear()
    this.nodesPromise = null
    this.connectPromise = null
    this.reconnectLoop = null
    this.restorePromise = null
    this.reconnectAttempt = 0
    this.lastSubscriptions = null

    if (!this.socket) {
      return
    }

    const socket = this.socket
    this.socket = null
    socket.onopen = null
    socket.onmessage = null
    socket.onerror = null
    socket.onclose = null
    socket.close()
  }

  subscribe(handler: (event: WSUIEvent) => void) {
    return this.bus.on(handler)
  }

  async requestSessionsList() {
    return this.fetchSessionsList()
  }

  async createSession(request: SessionCreateRequest) {
    return this.sendRequest<{ sessionId: string }>('session.create', {
      title: request.title,
    })
  }

  async deleteSession(request: SessionDeleteRequest) {
    this.snapshots.delete(request.sessionId)
    return this.sendRequest<{ sessionId: string }>('session.delete', {
      sessionId: request.sessionId,
    })
  }

  async requestSessionSnapshot(sessionId: string) {
    const snapshot = await this.fetchSnapshot(sessionId)
    this.snapshots.set(sessionId, snapshot)
    this.bus.emit({
      type: 'connection.synced',
      timestamp: new Date().toISOString(),
    })
    return snapshot
  }

  async updateSubscriptions(request: SubscriptionsUpdateRequest) {
    this.lastSubscriptions = {
      ...request,
      watchSessionIds: [...request.watchSessionIds],
    }
    await this.sendRequest('subscriptions.update', {
      activeSessionId: request.activeSessionId,
      watchSessionIds: request.watchSessionIds,
    })
    this.bus.emit({
      type: 'connection.synced',
      timestamp: new Date().toISOString(),
    })
  }

  async submitMessage(request: SessionMessageSubmitRequest) {
    await this.sendRequest('session.message.submit', {
      sessionId: request.sessionId,
      text: request.text,
      clientMessageId: messageId(),
    })
  }

  private async sendRequest<T = unknown>(
    type: string,
    payload?: Record<string, unknown>,
  ) {
    await this.connect()

    const reqID = requestId(type)
    const socket = this.socket
    if (!socket) {
      throw new Error(t('ws.notConnected'))
    }

    const promise = new Promise<T>((resolve, reject) => {
      this.pending.set(reqID, {
        resolve: (value) => resolve(value as T),
        reject,
      })
    })

    socket.send(
      JSON.stringify({
        type,
        requestId: reqID,
        payload,
      }),
    )

    return promise
  }

  private openSocket() {
    return new Promise<void>((resolve, reject) => {
      let opened = false
      let settled = false
      const socket = new WebSocket(buildWSURL('/ws/ui'))
      this.socket = socket

      const settleReject = (error: Error) => {
        if (settled) {
          return
        }
        settled = true
        reject(error)
      }

      socket.onmessage = (event) => {
        void this.handleMessage(String(event.data))
      }
      socket.onerror = () => {
        settleReject(new Error(t('ws.connectionFailed')))
      }
      socket.onopen = () => {
        opened = true
        if (!settled) {
          settled = true
          resolve()
        }
      }
      socket.onclose = () => {
        this.handleSocketClosed(
          opened ? t('ws.disconnected') : t('ws.closedBeforeReady'),
        )
        if (!opened) {
          settleReject(new Error(t('ws.closedBeforeReady')))
        }
      }
    })
  }

  private handleSocketClosed(reason: string) {
    if (
      this.socket?.readyState === WebSocket.CLOSED ||
      this.socket?.readyState === WebSocket.CLOSING
    ) {
      this.socket = null
    }
    this.connectPromise = null

    this.pending.forEach((request) => {
      request.reject(new Error(reason))
    })
    this.pending.clear()

    if (this.manualDisconnect) {
      return
    }

    this.bus.emit({
      type: 'connection.offline',
      timestamp: new Date().toISOString(),
      reason,
    })
    this.scheduleReconnect()
  }

  private scheduleReconnect() {
    if (this.reconnectLoop || this.manualDisconnect) {
      return
    }

    this.reconnectLoop = (async () => {
      while (!this.manualDisconnect && !this.socket) {
        this.reconnectAttempt += 1
        const attempt = this.reconnectAttempt
        this.bus.emit({
          type: 'connection.reconnecting',
          timestamp: new Date().toISOString(),
          attempt,
        })
        await delay(Math.min(1000 * attempt, 5000))

        try {
          await this.connect()
          this.reconnectLoop = null
          return
        } catch {
          this.connectPromise = null
        }
      }

      this.reconnectLoop = null
    })()
  }

  private async fetchSessionsList() {
    const response = await this.sendRequest<{
      items: BackendSessionListItem[]
    }>('sessions.list.request')
    return response.items.map(mapSessionListItem)
  }

  private async fetchSnapshot(sessionId: string) {
    const response = await this.sendRequest<{ snapshot: BackendSnapshot }>(
      'session.snapshot.request',
      {
        sessionId,
      },
    )
    return this.mapSnapshot(response.snapshot)
  }

  private async restoreState() {
    if (this.restorePromise) {
      return this.restorePromise
    }

    this.restorePromise = (async () => {
      const sessions = await this.fetchSessionsList()
      this.bus.emit({ type: 'sessions.replaced', sessions })

      if (this.lastSubscriptions) {
        await this.sendRequest('subscriptions.update', {
          activeSessionId: this.lastSubscriptions.activeSessionId,
          watchSessionIds: this.lastSubscriptions.watchSessionIds,
        })

        if (this.lastSubscriptions.activeSessionId) {
          const snapshot = await this.fetchSnapshot(
            this.lastSubscriptions.activeSessionId,
          )
          this.snapshots.set(snapshot.id, snapshot)
          this.bus.emit({ type: 'session.snapshot.replaced', snapshot })
        }
      }

      this.reconnectAttempt = 0
      this.bus.emit({
        type: 'connection.synced',
        timestamp: new Date().toISOString(),
      })
    })()

    try {
      await this.restorePromise
    } finally {
      this.restorePromise = null
    }
  }

  private async handleMessage(raw: string) {
    const message = JSON.parse(raw) as BackendEnvelope
    const timestamp = message.timestamp ?? new Date().toISOString()

    if (message.requestId && this.pending.has(message.requestId)) {
      const pending = this.pending.get(message.requestId)
      this.pending.delete(message.requestId)
      if (message.type === 'error') {
        pending?.reject(
          new Error(
            message.error?.message ||
              message.error?.code ||
              'ws/ui request failed',
          ),
        )
      } else {
        pending?.resolve(message.payload)
      }
      return
    }

    if (message.type === 'connection.ready') {
      this.bus.emit({ type: 'connection.ready', timestamp })
      if (this.reconnectAttempt > 0 || this.lastSubscriptions) {
        void this.restoreState()
      }
      return
    }

    if (
      message.type === 'session.summary.updated' ||
      message.type === 'session.finished'
    ) {
      if (!message.sessionId) {
        return
      }

      this.bus.emit({
        type: 'session.summary.updated',
        session: {
          id: message.sessionId,
          title: message.summary?.title || message.sessionId,
          summary: deriveSummary({
            status: message.summary?.status || 'idle',
          }),
          status: mapSessionStatus(message.summary?.status || 'idle'),
          unread: message.summary?.unread || 0,
          updatedAt: message.summary?.updatedAt || timestamp,
        },
      })
      return
    }

    if (message.type === 'llm.sse.event') {
      if (!message.sessionId || !message.upstreamEventType) {
        return
      }

      this.bus.emit({
        type: 'llm.sse.event',
        sessionId: message.sessionId,
        responseId: message.responseId,
        sequenceNumber: message.sequenceNumber,
        upstreamEventType: message.upstreamEventType,
        rawEvent: message.rawEvent ?? {},
      })
      return
    }

    if (message.type === 'llm.response.completed') {
      if (!message.sessionId) {
        return
      }

      this.bus.emit({
        type: 'llm.response.completed',
        sessionId: message.sessionId,
        responseId: message.responseId,
        rawResponse: message.rawResponse ?? {},
      })
      return
    }

    if (message.type === 'session.state.updated') {
      if (!message.sessionId) {
        return
      }

      this.bus.emit({
        type: 'session.state.updated',
        sessionId: message.sessionId,
        revision: message.revision ?? this.bumpRevision(message.sessionId),
        status: message.status,
      })
      return
    }

    if (message.type === 'timeline.row.appended') {
      if (!message.sessionId || !message.row) {
        return
      }

      const row = mapTimelineRows([message.row])[0]
      const revision = message.revision ?? this.bumpRevision(message.sessionId)
      if (!row) {
        return
      }
      this.syncRow(message.sessionId, row, revision)
      this.bus.emit({
        type: 'timeline.row.appended',
        sessionId: message.sessionId,
        row,
        revision,
      })
      return
    }
  }

  private mapSnapshot(snapshot: BackendSnapshot): SessionSnapshot {
    const nodes = this.getCachedNodes()
    const rows = mapTimelineRows(snapshot.timeline?.rows ?? [])

    return {
      id: snapshot.session.id,
      title: snapshot.session.title,
      summary: deriveSummary({
        status: snapshot.session.status,
        primaryText: snapshot.sidebarSummary?.primaryText,
      }),
      status: mapSessionStatus(snapshot.session.status),
      mode: 'ai_agent',
      revision: snapshot.session.revision,
      updatedAt: snapshot.session.updatedAt,
      unread: 0,
      rows,
      nodeHealthSummary: {
        online: nodes.filter((node) => node.status === 'online').length,
        offline: nodes.filter((node) => node.status === 'offline').length,
        busy: nodes.filter((node) => node.status === 'busy').length,
      },
    }
  }

  private getCachedNodes(): NodeSummary[] {
    // Returns empty if nodes haven't been fetched yet;
    // health summary will update on next snapshot fetch.
    return []
  }

  private async getNodes() {
    if (!this.nodesPromise) {
      this.nodesPromise = listNodes()
    }
    return this.nodesPromise
  }

  private bumpRevision(sessionID: string) {
    const snapshot = this.snapshots.get(sessionID)
    if (!snapshot) {
      return 1
    }
    snapshot.revision += 1
    return snapshot.revision
  }

  private syncRow(sessionID: string, row: TimelineRow, revision: number) {
    const snapshot = this.snapshots.get(sessionID)
    if (!snapshot) {
      return
    }
    snapshot.revision = Math.max(snapshot.revision, revision)
    const index = snapshot.rows.findIndex((item) => item.id === row.id)
    if (index >= 0) {
      snapshot.rows.splice(index, 1, row)
      return
    }
    snapshot.rows.push(row)
  }
}
