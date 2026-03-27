import { t } from '@/app/i18n'
import { getHistoryTaskDetail } from '@/shared/api/adapters/history'
import { listNodes } from '@/shared/api/adapters/nodes'
import { getAccessToken } from '@/shared/auth/session'
import { appEnv } from '@/shared/config/env'
import type {
  ApprovalRow,
  ExecutionNodeState,
  ExecutionRow,
  PlanRow,
  SessionListItem,
  SessionSnapshot,
  SummaryRow,
  TargetCandidate,
  TargetContext,
  TimelineRow,
  ToolResultMetaRow,
} from '@/shared/types/console'
import type { HistoryTaskDetail } from '@/shared/types/history'
import type { NodeSummary } from '@/shared/types/node'
import { createEventBus } from '@/shared/ws/event-bus'
import { delay } from '@/shared/ws/reconnect'
import type {
  SessionCreateRequest,
  SessionDeleteRequest,
  SessionApprovalRequest,
  SessionMessageSubmitRequest,
  SessionTargetConfirmRequest,
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
    activeTargetSummary?: string
  }
  responseId?: string
  sequenceNumber?: number
  upstreamEventType?: string
  rawEvent?: Record<string, unknown>
  rawResponse?: Record<string, unknown>
  status?: string
  revision?: number
  targetContext?: BackendTargetContext
  row?: BackendTimelineRow
  taskId?: string
  executionId?: string
  nodeId?: string
  chunk?: {
    stream?: string
    text?: string
  }
  timestamp?: string
}

type BackendSessionListItem = {
  sessionId: string
  title: string
  status: string
  updatedAt: string
  activeTargetSummary?: string
  unread?: number
}

type BackendTargetCandidate = {
  nodeId: string
  hostname?: string
  region?: string
  matchedBy?: string
  reason?: string
}

type BackendTargetContext = {
  status?: string
  scope?: string
  nodeIds?: string[]
  displayLabel?: string
  source?: string
  candidates?: BackendTargetCandidate[]
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
  TargetContext?: BackendTargetContext
  id?: string
  kind?: string
  createdAt?: string
  text?: string
  toolName?: string
  toolStatus?: string
  source?: string
  argsPreview?: string
  taskId?: string
  targetContext?: BackendTargetContext
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
  activeTargetContext?: BackendTargetContext
  pendingAction?: {
    type?: string
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
    case 'waiting_async_execution':
      return 'running'
    case 'paused_wait_target_confirmation':
    case 'paused_wait_approval':
    case 'failed':
      return 'attention'
    case 'completed':
      return 'completed'
    default:
      return 'idle'
  }
}

function mapTargetSource(source: string | undefined): TargetContext['source'] {
  switch (source) {
    case 'assistant_resolved':
      return 'resolver'
    case 'context_inherited':
      return 'session_context'
    case 'user_explicit':
      return 'manual'
    default:
      return 'none'
  }
}

function mapTargetCandidate(
  candidate: BackendTargetCandidate,
  nodes: NodeSummary[],
): TargetCandidate {
  const matched = nodes.find((node) => node.id === candidate.nodeId)
  return {
    id: `candidate-${candidate.nodeId}`,
    nodeId: candidate.nodeId,
    label: candidate.hostname || matched?.hostname || candidate.nodeId,
    region: candidate.region || matched?.region || 'unknown',
    scope: 'single',
    reason: candidate.reason || 'resolved from backend target context',
    source: 'resolver',
    tags: matched?.tags ?? [],
  }
}

function mapTargetContext(
  raw: BackendTargetContext | undefined,
  nodes: NodeSummary[],
): TargetContext {
  const state =
    raw?.status === 'confirmed'
      ? 'confirmed'
      : raw?.status === 'pending_confirmation'
        ? 'pending_confirmation'
        : 'unset'
  const scope =
    raw?.scope === 'single' ||
    raw?.scope === 'multi' ||
    raw?.scope === 'all_online'
      ? raw.scope
      : 'unset'
  const candidates = (raw?.candidates ?? []).map((candidate) =>
    mapTargetCandidate(candidate, nodes),
  )
  const confirmedNodeIds = raw?.nodeIds ?? []

  let summary = 'Target context: unset'
  if (state === 'pending_confirmation') {
    summary = raw?.displayLabel
      ? `pending ${raw.displayLabel}`
      : 'pending target confirmation'
  }
  if (state === 'confirmed') {
    summary = raw?.displayLabel
      ? `confirmed ${raw.displayLabel}`
      : 'confirmed target'
  }

  return {
    state,
    scope,
    summary,
    source: mapTargetSource(raw?.source),
    candidates,
    confirmedNodeIds,
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

function readRowTargetContext(row: BackendTimelineRow) {
  return row.targetContext || row.TargetContext
}

async function fetchTaskDetails(taskIDs: string[]) {
  const entries = await Promise.all(
    [...new Set(taskIDs.filter(Boolean))].map(async (taskID) => {
      try {
        const detail = await getHistoryTaskDetail(taskID)
        return [taskID, detail] as const
      } catch {
        return [taskID, null] as const
      }
    }),
  )
  return new Map<string, HistoryTaskDetail | null>(entries)
}

function makeExecutionRow(
  taskID: string,
  detail: HistoryTaskDetail | null,
  nodes: NodeSummary[],
): ExecutionRow {
  const nodeStates: ExecutionNodeState[] =
    detail?.executions?.map((item) => ({
      nodeId: item.nodeId,
      label: item.label,
      region:
        nodes.find((node) => node.id === item.nodeId)?.region ?? 'unknown',
      status: item.status,
      stdoutTail: item.stdoutTail,
      stderrTail: item.stderrTail,
    })) ?? []

  return {
    id: `row-execution-${taskID}`,
    kind: 'execution',
    createdAt: detail?.updatedAt ?? new Date().toISOString(),
    taskId: taskID,
    title: detail?.title || t('ws.fallbackExecutionTitle'),
    status: detail?.status || 'queued',
    nodes: nodeStates,
  }
}

function makePlanRow(
  row: BackendTimelineRow,
  detail: HistoryTaskDetail | null,
  targetContext: TargetContext,
): PlanRow {
  const taskID = readRowTaskID(row)
  return {
    id: readRowID(row),
    kind: 'plan',
    createdAt: readRowCreatedAt(row),
    taskId: taskID || undefined,
    inputText: detail?.title || latestInputFallback(detail),
    summary: detail?.summary || readRowText(row) || t('ws.fallbackPlanSummary'),
    impact: detail?.impact || t('ws.fallbackPlanImpact'),
    risk: detail?.risk || 'low',
    requiresApproval:
      detail?.approvalStatus === 'pending' ||
      detail?.approvalStatus === 'approved',
    targetLabel: detail?.targetLabels?.[0] || targetContext.summary,
    targetSource:
      targetContext.source === 'session_context'
        ? 'context_inherited'
        : targetContext.source === 'manual'
          ? 'manual'
          : 'assistant_resolved',
    autoExecutionHint:
      detail?.approvalStatus === 'not_required'
        ? t('ws.fallbackAutoExecHint')
        : undefined,
    steps: (detail?.steps ?? []).map((step) => ({
      action: step,
      argsLabel: step,
      risk: detail?.risk || 'low',
      timeoutSec: 30,
      broadcastAllowed: false,
    })),
  }
}

function latestInputFallback(detail: HistoryTaskDetail | null) {
  return detail?.title || t('ws.fallbackConsoleTask')
}

function makeApprovalRow(
  row: BackendTimelineRow,
  detail: HistoryTaskDetail | null,
  targetContext: TargetContext,
): ApprovalRow {
  const taskID = readRowTaskID(row)
  return {
    id: readRowID(row),
    kind: 'approval',
    createdAt: readRowCreatedAt(row),
    taskId: taskID || undefined,
    reason:
      detail?.summary ||
      t('ws.fallbackApprovalReason'),
    risk: detail?.risk || 'medium',
    impact: detail?.impact || t('ws.fallbackApprovalImpact'),
    targetLabel: detail?.targetLabels?.[0] || targetContext.summary,
  }
}

function makeSummaryRow(
  row: BackendTimelineRow,
  detail: HistoryTaskDetail | null,
): SummaryRow {
  const executions = detail?.executions ?? []
  return {
    id: readRowID(row),
    kind: 'summary',
    createdAt: readRowCreatedAt(row),
    taskId: readRowTaskID(row) || undefined,
    total: executions.length,
    success: executions.filter((item) => item.status === 'success').length,
    failed: executions.filter((item) => item.status === 'failed').length,
    skipped: executions.filter((item) => item.status === 'skipped').length,
    markdown: detail?.aiSummary || readRowText(row) || t('ws.fallbackSummaryComplete'),
    nextSteps: detail?.steps?.slice(-2) ?? [],
  }
}

async function mapTimelineRows(
  rows: BackendTimelineRow[],
  nodes: NodeSummary[],
  targetContext: TargetContext,
) {
  const taskDetails = await fetchTaskDetails(rows.map(readRowTaskID))

  return rows.map((row) => {
    const kind = readRowKind(row)
    const detail = taskDetails.get(readRowTaskID(row)) ?? null

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

      case 'target_confirmation': {
        const rowTarget = mapTargetContext(readRowTargetContext(row), nodes)
        return {
          id: readRowID(row),
          kind: 'target_confirmation',
          createdAt: readRowCreatedAt(row),
          title: t('ws.fallbackConfirmTarget'),
          originalTargetText: rowTarget.summary,
          basis: t('ws.fallbackTargetBasis'),
          scope:
            rowTarget.scope === 'all_online'
              ? 'all online nodes'
              : rowTarget.scope === 'multi'
                ? 'multi node target set'
                : 'single node',
          source: rowTarget.source,
          candidates: rowTarget.candidates,
          inheritedHint:
            rowTarget.source === 'session_context'
              ? t('ws.fallbackInheritedHint')
              : undefined,
        } satisfies TimelineRow
      }

      case 'plan':
        return makePlanRow(row, detail, targetContext)

      case 'approval':
        return makeApprovalRow(row, detail, targetContext)

      case 'execution':
        return makeExecutionRow(readRowTaskID(row), detail, nodes)

      case 'summary':
        return makeSummaryRow(row, detail)

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

function executionEventRow(
  current: ExecutionRow | undefined,
  payload: Pick<
    BackendEnvelope,
    'taskId' | 'nodeId' | 'timestamp' | 'chunk' | 'status'
  >,
  nodes: NodeSummary[],
  statusOverride?: ExecutionNodeState['status'],
) {
  const taskID = payload.taskId ?? 'unknown-task'
  const nodeID = payload.nodeId ?? 'unknown-node'
  const next: ExecutionRow = current
    ? structuredClone(current)
    : {
        id: `row-execution-${taskID}`,
        kind: 'execution',
        createdAt: payload.timestamp || new Date().toISOString(),
        taskId: taskID,
        title: t('ws.fallbackExecutionTitle'),
        status: 'queued',
        nodes: [],
      }

  const region = nodes.find((node) => node.id === nodeID)?.region ?? 'unknown'
  const nodeIndex = next.nodes.findIndex((node) => node.nodeId === nodeID)
  const currentNode: ExecutionNodeState =
    nodeIndex >= 0
      ? next.nodes[nodeIndex]
      : {
          nodeId: nodeID,
          label: nodeID,
          region,
          status: 'queued' as const,
        }

  if (payload.chunk) {
    if (payload.chunk.stream === 'stdout') {
      currentNode.stdoutTail = `${currentNode.stdoutTail ?? ''}${payload.chunk.text}`
    } else if (payload.chunk.stream === 'stderr') {
      currentNode.stderrTail = `${currentNode.stderrTail ?? ''}${payload.chunk.text}`
    }
    currentNode.status = 'running'
    next.status = 'running'
  }

  if (statusOverride) {
    currentNode.status = statusOverride
    next.status =
      statusOverride === 'success'
        ? 'success'
        : statusOverride === 'skipped'
          ? 'cancelled'
          : 'failed'
  }

  if (nodeIndex >= 0) {
    next.nodes.splice(nodeIndex, 1, currentNode)
  } else {
    next.nodes.push(currentNode)
  }

  return next
}

function deriveSummary(item: {
  status: string
  target?: string
  title?: string
  primaryText?: string
}) {
  if (item.primaryText) {
    return item.primaryText
  }
  switch (item.status) {
    case 'running':
    case 'waiting_async_execution':
      return t('ws.derivedRunning', { target: item.target || item.title || 'current target' })
    case 'paused_wait_target_confirmation':
      return t('ws.derivedWaitingTarget')
    case 'paused_wait_approval':
      return t('ws.derivedWaitingApproval')
    case 'completed':
      return t('ws.derivedCompleted')
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
      target: item.activeTargetSummary,
      title: item.title,
    }),
    status: mapSessionStatus(item.status),
    unread: item.unread ?? 0,
    updatedAt: item.updatedAt,
    targetSummary: item.activeTargetSummary || 'Target context: unset',
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

  async confirmTarget(request: SessionTargetConfirmRequest) {
    if (request.action === 'clear') {
      await this.sendRequest('session.target.clear', {
        sessionId: request.sessionId,
        idempotencyKey: requestId('clear'),
      })
      return
    }
    if (request.action === 'reselect') {
      await this.sendRequest('session.target.reselect', {
        sessionId: request.sessionId,
        idempotencyKey: requestId('reselect'),
      })
      return
    }
    if (request.action !== 'confirm') {
      throw new Error(t('ws.unsupportedAction'))
    }
    const candidate = request.candidate
    if (!candidate) {
      throw new Error(t('ws.candidateRequired'))
    }
    await this.sendRequest('session.target.confirm', {
      sessionId: request.sessionId,
      nodeIds: [candidate.nodeId],
      scope: candidate.scope,
      idempotencyKey: requestId('confirm'),
    })
  }

  async submitApproval(request: SessionApprovalRequest) {
    if (!request.approvalRow?.taskId) {
      throw new Error(t('ws.approvalContextMissing'))
    }

    if (request.action === 'approve') {
      await this.sendRequest('session.approval.approve', {
        sessionId: request.sessionId,
        taskId: request.approvalRow.taskId,
        idempotencyKey: requestId('approve'),
      })
      return
    }

    if (request.action === 'reject') {
      await this.sendRequest('session.approval.reject', {
        sessionId: request.sessionId,
        taskId: request.approvalRow.taskId,
        idempotencyKey: requestId('reject'),
        reason: t('ws.rejectedFromUI'),
      })
      return
    }

    await this.sendRequest('session.operation.cancel', {
      sessionId: request.sessionId,
      taskId: request.approvalRow.taskId,
      idempotencyKey: requestId('cancel'),
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
            target: message.summary?.activeTargetSummary,
          }),
          status: mapSessionStatus(message.summary?.status || 'idle'),
          unread: message.summary?.unread || 0,
          updatedAt: message.summary?.updatedAt || timestamp,
          targetSummary:
            message.summary?.activeTargetSummary || 'Target context: unset',
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

    if (message.type === 'session.requires_attention') {
      if (!message.sessionId) {
        return
      }

      this.bus.emit({
        type: 'session.requires_attention',
        sessionId: message.sessionId,
        revision: this.bumpRevision(message.sessionId),
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

    if (
      message.type === 'thread.target.pending' ||
      message.type === 'thread.target.confirmed' ||
      message.type === 'thread.target.cleared'
    ) {
      if (!message.sessionId) {
        return
      }

      const nodes = await this.getNodes()
      this.bus.emit({
        type: message.type,
        sessionId: message.sessionId,
        revision: message.revision ?? this.bumpRevision(message.sessionId),
        targetContext: mapTargetContext(message.targetContext, nodes),
      })
      return
    }

    if (message.type === 'timeline.row.appended') {
      if (!message.sessionId || !message.row) {
        return
      }

      const snapshot = this.snapshots.get(message.sessionId)
      const nodes = await this.getNodes()
      const targetContext =
        snapshot?.targetContext ?? mapTargetContext(undefined, nodes)
      const row = (
        await mapTimelineRows([message.row], nodes, targetContext)
      )[0]
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

    if (
      message.type === 'execution.chunk' ||
      message.type === 'execution.finished'
    ) {
      if (!message.sessionId) {
        return
      }

      const nodes = await this.getNodes()
      const snapshot = this.snapshots.get(message.sessionId)
      const current = snapshot?.rows.find(
        (row) => row.kind === 'execution' && row.taskId === message.taskId,
      ) as ExecutionRow | undefined
      const status =
        message.type === 'execution.finished'
          ? message.status === 'success'
            ? 'success'
            : 'failed'
          : undefined
      const row = executionEventRow(current, message, nodes, status)
      const revision = this.bumpRevision(message.sessionId)
      this.syncRow(message.sessionId, row, revision)
      this.bus.emit({
        type: message.type,
        sessionId: message.sessionId,
        row,
        revision,
      })
    }
  }

  private async mapSnapshot(
    snapshot: BackendSnapshot,
  ): Promise<SessionSnapshot> {
    const nodes = await this.getNodes()
    const targetContext = mapTargetContext(snapshot.activeTargetContext, nodes)
    const rows = await mapTimelineRows(
      snapshot.timeline?.rows ?? [],
      nodes,
      targetContext,
    )
    const highlightedNodes = nodes.filter(
      (node) =>
        targetContext.confirmedNodeIds.includes(node.id) ||
        targetContext.candidates.some(
          (candidate) => candidate.nodeId === node.id,
        ),
    )

    return {
      id: snapshot.session.id,
      title: snapshot.session.title,
      summary: deriveSummary({
        status: snapshot.session.status,
        target: targetContext.summary,
        primaryText: snapshot.sidebarSummary?.primaryText,
      }),
      status: mapSessionStatus(snapshot.session.status),
      mode: 'ai_agent',
      revision: snapshot.session.revision,
      updatedAt: snapshot.session.updatedAt,
      unread: 0,
      approvalStatus:
        snapshot.pendingAction?.type === 'approval'
          ? 'pending'
          : 'not_required',
      targetContext,
      rows,
      candidateNodes: nodes.filter((node) =>
        targetContext.candidates.some(
          (candidate) => candidate.nodeId === node.id,
        ),
      ),
      highlightedNodes,
      nodeHealthSummary: {
        online: nodes.filter((node) => node.status === 'online').length,
        offline: nodes.filter((node) => node.status === 'offline').length,
        busy: nodes.filter((node) => node.status === 'busy').length,
      },
      pendingActionType:
        snapshot.pendingAction?.type === 'approval' ||
        snapshot.pendingAction?.type === 'target_confirmation'
          ? snapshot.pendingAction.type
          : undefined,
    }
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
