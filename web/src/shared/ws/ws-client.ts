import { t } from '@/app/i18n'
import { appEnv } from '@/shared/config/env'
import { createMockSessions, toSessionListItem } from '@/shared/mock/sessions'
import { mockNodeSummaries } from '@/shared/mock/nodes'
import type {
  ApprovalRow,
  AssistantTextRow,
  ExecutionNodeState,
  ExecutionRow,
  PlanRow,
  SessionSnapshot,
  SummaryRow,
  TargetCandidate,
  TargetConfirmationRow,
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
  SessionApprovalRequest,
  SessionMessageSubmitRequest,
  SessionTargetConfirmRequest,
  SubscriptionsUpdateRequest,
  WSClient,
  WSUIEvent,
} from '@/shared/ws/protocol'

interface DraftScenario {
  taskId: string
  writePlan: boolean
  targetContextScope: SessionSnapshot['targetContext']['scope']
  targetSource: PlanRow['targetSource']
  confirmedNodeIds: string[]
  blockedReason?: string
  targetRow: TargetConfirmationRow
  planRow: PlanRow
  approvalRow?: ApprovalRow
  executionTitle: string
  summaryMarkdown: string
}

function cloneSnapshot(snapshot: SessionSnapshot) {
  return structuredClone(snapshot)
}

function nowIso() {
  return new Date().toISOString()
}

class MockWSClient implements WSClient {
  private bus = createEventBus<WSUIEvent>()
  private sessions = new Map<string, SessionSnapshot>()
  private drafts = new Map<string, DraftScenario>()
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
      approvalStatus: 'not_required',
      targetContext: {
        state: 'unset',
        scope: 'unset',
        summary: 'Target context: unset',
        source: 'none',
        candidates: [],
        confirmedNodeIds: [],
      },
      rows: [],
      candidateNodes: [],
      highlightedNodes: [],
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
    }
    this.sessions.set(sessionId, snapshot)
    return { sessionId }
  }

  async deleteSession(request: SessionDeleteRequest) {
    this.sessions.delete(request.sessionId)
    this.drafts.delete(request.sessionId)
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

    if (snapshot.pendingActionType) {
      throw new Error(t('ws.sessionBusy'))
    }

    const scenario = this.buildScenario(snapshot, request.text)
    snapshot.status = 'running'
    snapshot.summary = t('mockWs.resolvingTarget')
    snapshot.targetContext = {
      state: 'pending_confirmation',
      scope: scenario.targetContextScope,
      summary: this.getPendingTargetSummary(scenario),
      source: scenario.targetSource === 'context_inherited' ? 'session_context' : 'resolver',
      candidates: scenario.targetRow.candidates,
      confirmedNodeIds: [],
    }
    snapshot.candidateNodes = mockNodeSummaries.filter(node =>
      scenario.targetRow.candidates.some(candidate => candidate.nodeId === node.id),
    )
    snapshot.highlightedNodes = snapshot.candidateNodes
    snapshot.pendingActionType = 'target_confirmation'
    this.bumpRevision(snapshot)
    this.drafts.set(snapshot.id, scenario)

    await this.emitMockAssistantStream(snapshot, {
      reasoning: scenario.targetSource === 'context_inherited'
        ? t('mockWs.reasoningNoExplicitTarget')
        : t('mockWs.reasoningResolveTarget'),
      content: scenario.targetSource === 'context_inherited'
        ? t('mockWs.contentReuseConfirmed', { targets: scenario.targetRow.candidates.map(candidate => candidate.label).join(' / ') })
        : t('mockWs.contentResolveAndCheck'),
    })

    const rows = [
      this.makeUserRow(request.text),
      ...(scenario.targetSource === 'context_inherited'
        ? [this.makeAssistantRow(t('mockWs.inheritConfirmedTargets', { targets: scenario.targetRow.candidates.map(candidate => candidate.label).join(' / ') }))]
        : []),
      this.makeToolCallRow('calling list_nodes(status=online,stale)'),
      this.makeToolResultRow(`list_nodes returned ${snapshot.nodeHealthSummary.online + snapshot.nodeHealthSummary.busy} candidate nodes`, 'neutral'),
      this.makeToolCallRow(`calling resolve_target_nodes("${request.text}")`),
      this.makeToolResultRow(
        `resolve_target_nodes matched ${scenario.targetRow.candidates.map(candidate => candidate.label).join(' / ')}`,
        'neutral',
      ),
      scenario.targetRow,
    ]

    for (const row of rows) {
      snapshot.rows.push(row)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      await delay(row.kind === 'target_confirmation' ? 120 : 90)
    }

    this.bus.emit({ type: 'session.state.updated', sessionId: snapshot.id, revision: snapshot.revision })
  }

  async confirmTarget(request: SessionTargetConfirmRequest) {
    const snapshot = this.requireSession(request.sessionId)
    const draft = this.drafts.get(request.sessionId)

    if (!draft) {
      return
    }

    if (request.action === 'reselect') {
      if (draft.targetRow.candidates.length > 1) {
        draft.targetRow.candidates = [...draft.targetRow.candidates.slice(1), draft.targetRow.candidates[0]]
        const rowIndex = snapshot.rows.findIndex(row => row.id === draft.targetRow.id)
        if (rowIndex >= 0) {
          snapshot.rows.splice(rowIndex, 1, draft.targetRow)
          this.bumpRevision(snapshot)
          this.bus.emit({
            type: 'timeline.row.updated',
            sessionId: snapshot.id,
            row: draft.targetRow,
            revision: snapshot.revision,
          })
        }
      }

      const result = this.makeToolResultRow(t('mockWs.candidatesRefreshed'), 'warning')
      snapshot.rows.push(result)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: result, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    if (request.action === 'clear') {
      snapshot.pendingActionType = undefined
      snapshot.status = 'idle'
      snapshot.summary = t('mockWs.targetContextCleared')
      snapshot.targetContext = {
        state: 'unset',
        scope: 'unset',
        summary: 'Target context: unset',
        source: 'none',
        candidates: [],
        confirmedNodeIds: [],
      }
      const result = this.makeToolResultRow('target context cleared', 'warning')
      snapshot.rows.push(result)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: result, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    const candidate = request.candidate ?? draft.targetRow.candidates[0]
    const confirmedNodeIds = draft.targetContextScope === 'single' ? [candidate.nodeId] : draft.confirmedNodeIds
    const targetLabel = this.getConfirmedTargetLabel(confirmedNodeIds, draft.targetContextScope, candidate.label)
    this.refreshDraftPlan(draft, targetLabel, confirmedNodeIds.length)
    snapshot.targetContext = {
      state: 'confirmed',
      scope: draft.targetContextScope,
      summary: `confirmed ${targetLabel}`,
      source: candidate.source,
      candidates: draft.targetRow.candidates,
      confirmedNodeIds,
    }
    snapshot.highlightedNodes = mockNodeSummaries.filter(node => confirmedNodeIds.includes(node.id))
    snapshot.pendingActionType = draft.blockedReason ? undefined : draft.approvalRow ? 'approval' : undefined
    snapshot.status = draft.blockedReason || draft.approvalRow ? 'attention' : 'running'
    snapshot.summary = draft.blockedReason ?? (draft.approvalRow ? t('mockWs.planGeneratedAwaitingApproval') : t('mockWs.lowRiskPlanGenerated'))
    snapshot.approvalStatus = draft.approvalRow ? 'pending' : 'not_required'

    await this.emitMockAssistantStream(snapshot, {
      reasoning: t('mockWs.reasoningGeneratePlan'),
      content: draft.blockedReason
        ? t('mockWs.contentBroadcastWriteBlocked')
        : draft.approvalRow
          ? t('mockWs.contentPlanNeedsApproval', { target: targetLabel })
          : t('mockWs.contentPlanAutoRun', { target: targetLabel }),
    })

    const resultRows = [
      this.makeToolResultRow(`target_confirmation succeeded · ${targetLabel} confirmed`, 'success'),
      this.makeToolCallRow('calling propose_plan'),
      this.makeToolResultRow(`plan generated · ${draft.planRow.risk} risk`, draft.planRow.requiresApproval ? 'warning' : 'success'),
      this.makeAssistantRow(
        draft.blockedReason
          ? t('mockWs.broadcastWriteMVPBlocked')
          : draft.planRow.requiresApproval
            ? t('mockWs.planForTargetExecution', { target: targetLabel })
            : t('mockWs.planForTargetReadOnly', { target: targetLabel }),
      ),
      draft.planRow,
    ] as const

    for (const row of resultRows) {
      snapshot.rows.push(row)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      await delay(110)
    }

    if (draft.blockedReason) {
      const blockedRow = this.makeToolResultRow(
        'broadcast write blocked in MVP · narrow target set before retrying',
        'warning',
      )
      snapshot.rows.push(blockedRow)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: blockedRow, revision: snapshot.revision })
      this.bus.emit({ type: 'session.requires_attention', sessionId: snapshot.id, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    if (draft.approvalRow) {
      snapshot.rows.push(draft.approvalRow)
      this.bumpRevision(snapshot)
      this.bus.emit({
        type: 'timeline.row.appended',
        sessionId: snapshot.id,
        row: draft.approvalRow,
        revision: snapshot.revision,
      })
      this.bus.emit({ type: 'session.requires_attention', sessionId: snapshot.id, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    await this.startExecution(snapshot, draft)
  }

  async submitApproval(request: SessionApprovalRequest) {
    const snapshot = this.requireSession(request.sessionId)
    const draft = this.drafts.get(request.sessionId)

    if (!draft) {
      return
    }

    if (request.action !== 'approve') {
      snapshot.pendingActionType = undefined
      snapshot.status = 'completed'
      snapshot.summary = request.action === 'reject' ? t('mockWs.approvalRejected') : t('mockWs.approvalCancelled')
      const result = this.makeToolResultRow(
        `approval recorded · ${request.action === 'reject' ? 'Rejected' : 'Cancelled'} by Alex`,
        'warning',
      )
      snapshot.rows.push(result)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: result, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    const result = this.makeToolResultRow('approval recorded · Approved by Alex', 'success')
    snapshot.rows.push(result)
    snapshot.pendingActionType = undefined
    snapshot.status = 'running'
    snapshot.summary = t('mockWs.approvalPassedExecuting')
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: result, revision: snapshot.revision })
    this.emitSessionSummary(snapshot)
    await delay(140)
    await this.startExecution(snapshot, draft)
  }

  private async startExecution(snapshot: SessionSnapshot, draft: DraftScenario) {
    const confirmedNodeIds = snapshot.targetContext.confirmedNodeIds
    const executionNodes: ExecutionNodeState[] = confirmedNodeIds.map(nodeId => {
      const node = mockNodeSummaries.find(item => item.id === nodeId)
      return {
        nodeId,
        label: node?.hostname ?? nodeId,
        region: node?.region ?? 'unknown',
        status: 'queued',
      }
    })

    const executionRow: ExecutionRow = {
      id: `row-execution-${draft.taskId}`,
      kind: 'execution',
      createdAt: nowIso(),
      taskId: draft.taskId,
      title: draft.executionTitle,
      status: 'queued',
      nodes: executionNodes,
    }

    snapshot.rows.push(executionRow)
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: executionRow, revision: snapshot.revision })
    this.emitSessionSummary(snapshot)
    await delay(180)

    executionRow.status = 'running'
    executionRow.nodes = executionRow.nodes.map(node => ({
      ...node,
      status: 'running',
      stdoutTail: 'Inspecting pre-checks...',
    }))
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'execution.chunk', sessionId: snapshot.id, row: structuredClone(executionRow), revision: snapshot.revision })
    await delay(260)

    executionRow.status = 'success'
    executionRow.nodes = executionRow.nodes.map(node => ({
      ...node,
      status: 'success',
      exitCode: 0,
      stdoutTail:
        draft.planRow.requiresApproval
          ? 'pre-check ok\nnginx reload ok\nupstream healthy'
          : 'read-only sampling ok\np95 latency 112ms',
    }))
    this.bumpRevision(snapshot)
    this.bus.emit({
      type: 'execution.finished',
      sessionId: snapshot.id,
      row: structuredClone(executionRow),
      revision: snapshot.revision,
    })

    const summaryRow: SummaryRow = {
      id: `row-summary-${draft.taskId}`,
      kind: 'summary',
      createdAt: nowIso(),
      taskId: draft.taskId,
      total: executionRow.nodes.length,
      success: executionRow.nodes.filter(node => node.status === 'success').length,
      failed: executionRow.nodes.filter(node => node.status === 'failed').length,
      skipped: executionRow.nodes.filter(node => node.status === 'skipped').length,
      markdown: draft.summaryMarkdown,
      nextSteps: draft.planRow.requiresApproval
        ? [t('mockWs.nextStepWatch5xx'), t('mockWs.nextStepRollback')]
        : [t('mockWs.nextStepUpgradeReload')],
    }

    snapshot.rows.push(summaryRow)
    await this.emitMockAssistantStream(snapshot, {
      reasoning: t('mockWs.reasoningSummarize'),
      content: draft.planRow.requiresApproval
        ? t('mockWs.contentWriteComplete')
        : t('mockWs.contentReadComplete'),
    })
    const assistantConclusion = this.makeAssistantRow(
      draft.planRow.requiresApproval
        ? t('mockWs.assistantWriteConclusion')
        : t('mockWs.assistantReadConclusion'),
    )
    snapshot.rows.push(assistantConclusion)
    snapshot.status = 'completed'
    snapshot.summary = draft.planRow.requiresApproval ? t('mockWs.summaryWriteComplete') : t('mockWs.summaryReadComplete')
    snapshot.approvalStatus = draft.planRow.requiresApproval ? 'approved' : 'not_required'
    this.bumpRevision(snapshot)
    this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: summaryRow, revision: snapshot.revision })
    this.bumpRevision(snapshot)
    this.bus.emit({
      type: 'timeline.row.appended',
      sessionId: snapshot.id,
      row: assistantConclusion,
      revision: snapshot.revision,
    })
    this.bus.emit({ type: 'session.finished', sessionId: snapshot.id, revision: snapshot.revision })
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

  private buildScenario(snapshot: SessionSnapshot, text: string): DraftScenario {
    const normalized = text.toLowerCase()
    const taskId = `task-${Math.floor(Math.random() * 900 + 100)}`
    const writePlan = /(重启|reload|restart|重载|停止|删除|deploy|升级)/.test(normalized)
    const allOnlineTarget = /(所有在线节点|全部在线节点|all online|all nodes|所有节点|全部节点)/.test(normalized)
    const hasExplicitTarget = allOnlineTarget || /(东京|tokyo|frankfurt|fra|singapore|sg|san francisco|sfo|节点|node|机器|agent|主机)/.test(normalized)
    const shouldInheritTarget = !hasExplicitTarget && snapshot.targetContext.state === 'confirmed' && snapshot.targetContext.confirmedNodeIds.length > 0

    let candidates: TargetCandidate[]
    let targetContextScope: DraftScenario['targetContextScope']
    let targetSource: DraftScenario['targetSource']
    let sourceLabel: string
    let scopeLabel: string
    let basis: string
    let originalTargetText: string
    let inheritedHint: string | undefined
    let confirmedNodeIds: string[]

    if (allOnlineTarget) {
      const onlineNodes = mockNodeSummaries.filter(node => node.status === 'online' || node.status === 'busy')
      candidates = onlineNodes.map(node => ({
        id: `candidate-${node.id}-${Date.now()}`,
        nodeId: node.id,
        label: node.hostname,
        region: node.region,
        scope: 'all_online',
        reason: node.status === 'busy' ? t('mockWs.candidateBusyReason') : t('mockWs.candidateOnlineReason'),
        source: 'resolver',
        tags: node.tags,
      }))
      targetContextScope = 'all_online'
      targetSource = 'assistant_resolved'
      sourceLabel = 'resolve_target_nodes("all online nodes")'
      scopeLabel = 'all online nodes'
      basis = t('mockWs.broadcastHitBasis', { text })
      originalTargetText = 'all online nodes'
      confirmedNodeIds = onlineNodes.map(node => node.id)
    } else if (shouldInheritTarget) {
      const inheritedNodes = mockNodeSummaries.filter(node => snapshot.targetContext.confirmedNodeIds.includes(node.id))
      candidates = inheritedNodes.map(node => ({
        id: `candidate-${node.id}-${Date.now()}`,
        nodeId: node.id,
        label: node.hostname,
        region: node.region,
        scope: snapshot.targetContext.scope === 'unset' ? 'single' : snapshot.targetContext.scope,
        reason: t('mockWs.inheritTargetReason'),
        source: 'session_context',
        tags: node.tags,
      }))
      targetContextScope = snapshot.targetContext.scope === 'unset' ? 'single' : snapshot.targetContext.scope
      targetSource = 'context_inherited'
      sourceLabel = 'reuse_confirmed_target_context'
      scopeLabel = targetContextScope === 'all_online' ? 'all online nodes' : targetContextScope === 'multi' ? 'multi-node target set' : 'single node'
      basis = t('mockWs.inheritTargetBasis')
      originalTargetText = snapshot.targetContext.summary
      inheritedHint = t('mockWs.inheritTargetHint')
      confirmedNodeIds = [...snapshot.targetContext.confirmedNodeIds]
    } else if (normalized.includes('东京') || normalized.includes('tokyo')) {
      candidates = mockNodeSummaries
        .filter(node => node.region === 'Tokyo')
        .map(node => ({
          id: `candidate-${node.id}-${Date.now()}`,
          nodeId: node.id,
          label: node.hostname,
          region: node.region,
          scope: 'single',
          reason: node.id === 'jp-tokyo-01' ? t('mockWs.tokyoActiveReason') : t('mockWs.tokyoBackupReason'),
          source: 'resolver',
          tags: node.tags,
        }))
      targetContextScope = 'single'
      targetSource = 'assistant_resolved'
      sourceLabel = `resolve_target_nodes(“${t('mockWs.tokyoTarget')}”)`
      scopeLabel = candidates.length > 1 ? 'single node from region cluster' : 'single node'
      basis = t('mockWs.resolvedBasis', { text })
      originalTargetText = t('mockWs.tokyoTarget')
      confirmedNodeIds = candidates.length > 0 ? [candidates[0].nodeId] : []
    } else {
      const defaultNode = mockNodeSummaries[2]
      candidates = [
        {
          id: `candidate-${defaultNode.id}-${Date.now()}`,
          nodeId: defaultNode.id,
          label: defaultNode.hostname,
          region: defaultNode.region,
          scope: 'single',
          reason: t('mockWs.defaultContextReason'),
          source: 'resolver',
          tags: defaultNode.tags,
        },
      ]
      targetContextScope = 'single'
      targetSource = 'assistant_resolved'
      sourceLabel = 'resolve_target_nodes'
      scopeLabel = 'single node'
      basis = t('mockWs.resolvedBasis', { text })
      originalTargetText = text
      confirmedNodeIds = [defaultNode.id]
    }

    const blockedReason = writePlan && targetContextScope !== 'single' ? t('mockWs.broadcastBlockedMessage') : undefined
    const targetLabel = this.getConfirmedTargetLabel(confirmedNodeIds, targetContextScope, candidates[0]?.label ?? 'selected node')
    const planPackage = this.createPlanPackage({
      taskId,
      inputText: text,
      targetLabel,
      targetSource,
      targetScope: targetContextScope,
      targetCount: confirmedNodeIds.length,
      writePlan,
      blockedReason,
    })

    return {
      taskId,
      writePlan,
      targetContextScope,
      targetSource,
      confirmedNodeIds,
      blockedReason,
      targetRow: {
        id: `row-target-${taskId}`,
        kind: 'target_confirmation',
        createdAt: nowIso(),
        title: targetContextScope === 'all_online' ? t('mockWs.confirmBroadcastTitle') : t('mockWs.confirmTargetTitle'),
        originalTargetText,
        basis,
        scope: scopeLabel,
        source: sourceLabel,
        inheritedHint,
        candidates,
      },
      ...planPackage,
    }
  }

  private refreshDraftPlan(draft: DraftScenario, targetLabel: string, targetCount: number) {
    const planPackage = this.createPlanPackage({
      taskId: draft.taskId,
      inputText: draft.planRow.inputText,
      targetLabel,
      targetSource: draft.targetSource,
      targetScope: draft.targetContextScope,
      targetCount,
      writePlan: draft.writePlan,
      blockedReason: draft.blockedReason,
    })
    draft.planRow = planPackage.planRow
    draft.approvalRow = planPackage.approvalRow
    draft.executionTitle = planPackage.executionTitle
    draft.summaryMarkdown = planPackage.summaryMarkdown
  }

  private createPlanPackage(options: {
    taskId: string
    inputText: string
    targetLabel: string
    targetSource: PlanRow['targetSource']
    targetScope: DraftScenario['targetContextScope']
    targetCount: number
    writePlan: boolean
    blockedReason?: string
  }) {
    const isBroadcast = options.targetScope === 'multi' || options.targetScope === 'all_online'
    const risk: PlanRow['risk'] = options.blockedReason ? 'high' : options.writePlan ? 'medium' : 'low'
    const requiresApproval = Boolean(options.blockedReason || options.writePlan)

    const planRow: PlanRow = {
      id: `row-plan-${options.taskId}`,
      kind: 'plan',
      createdAt: nowIso(),
      taskId: options.taskId,
      inputText: options.inputText,
      summary: options.writePlan
        ? isBroadcast
          ? t('mockWs.planWriteBroadcastSummary', { target: options.targetLabel })
          : t('mockWs.planWriteSingleSummary', { target: options.targetLabel })
        : isBroadcast
          ? t('mockWs.planReadBroadcastSummary', { target: options.targetLabel })
          : t('mockWs.planReadSingleSummary', { target: options.targetLabel }),
      impact: options.writePlan
        ? isBroadcast
          ? t('mockWs.impactWriteBroadcast', { count: options.targetCount })
          : t('mockWs.impactWriteSingle')
        : isBroadcast
          ? t('mockWs.impactReadBroadcast', { count: options.targetCount })
          : t('mockWs.impactReadSingle'),
      risk,
      requiresApproval,
      targetLabel: options.targetLabel,
      targetSource: options.targetSource,
      autoExecutionHint: !options.writePlan ? t('mockWs.autoExecutionHint') : undefined,
      steps: options.writePlan
        ? [
            { action: 'inspect_nginx', argsLabel: t('mockWs.stepInspectNginx'), risk: isBroadcast ? 'medium' : 'low', timeoutSec: 20, broadcastAllowed: false },
            { action: 'reload_nginx', argsLabel: t('mockWs.stepReloadNginx'), risk: isBroadcast ? 'high' : 'medium', timeoutSec: 30, broadcastAllowed: false },
            { action: 'verify_service', argsLabel: t('mockWs.stepVerifyService'), risk: isBroadcast ? 'medium' : 'low', timeoutSec: 45, broadcastAllowed: false },
          ]
        : [
            { action: 'inspect_node', argsLabel: t('mockWs.stepInspectNode'), risk: 'low', timeoutSec: 20, broadcastAllowed: true },
            { action: 'inspect_service', argsLabel: t('mockWs.stepInspectService'), risk: 'low', timeoutSec: 45, broadcastAllowed: true },
          ],
    }

    const approvalRow = options.blockedReason
      ? undefined
      : options.writePlan
        ? {
            id: `row-approval-${options.taskId}`,
            kind: 'approval' as const,
            createdAt: nowIso(),
            taskId: options.taskId,
            reason: t('mockWs.approvalReasonReload'),
            risk,
            impact: isBroadcast ? t('mockWs.approvalImpactBroadcast', { count: options.targetCount }) : t('mockWs.approvalImpactSingle'),
            targetLabel: options.targetLabel,
          }
        : undefined

    return {
      planRow,
      approvalRow,
      executionTitle: options.writePlan ? t('mockWs.executionWriteTitle') : t('mockWs.executionReadTitle'),
      summaryMarkdown: options.writePlan
        ? t('mockWs.summaryWriteMarkdown', { target: options.targetLabel })
        : t('mockWs.summaryReadMarkdown', { target: options.targetLabel }),
    }
  }

  private getPendingTargetSummary(scenario: DraftScenario) {
    if (scenario.targetContextScope === 'all_online') {
      return `pending all online nodes (${scenario.confirmedNodeIds.length})`
    }
    if (scenario.targetContextScope === 'multi') {
      return `pending ${scenario.confirmedNodeIds.length} nodes`
    }
    return `pending ${scenario.targetRow.candidates[0]?.label ?? 'target'}`
  }

  private getConfirmedTargetLabel(confirmedNodeIds: string[], scope: DraftScenario['targetContextScope'], fallbackLabel: string) {
    if (scope === 'all_online') {
      return `all online nodes (${confirmedNodeIds.length})`
    }
    if (scope === 'multi') {
      return `${confirmedNodeIds.length} selected nodes`
    }
    return fallbackLabel
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

  private async emitMockAssistantStream(snapshot: SessionSnapshot, input: { reasoning: string; content: string }) {
    const responseId = `resp-${crypto.randomUUID()}`
    snapshot.llmStreamState = {
      responseId,
      status: 'streaming',
      reasoningText: '',
      contentText: '',
      events: [],
    }

    const reasoningChunks = this.chunkText(input.reasoning, 10)
    const contentChunks = this.chunkText(input.content, 12)

    let sequenceNumber = 1
    for (const delta of reasoningChunks) {
      this.bus.emit({
        type: 'llm.sse.event',
        sessionId: snapshot.id,
        responseId,
        sequenceNumber,
        upstreamEventType: 'response.reasoning_text.delta',
        rawEvent: { delta },
      })
      sequenceNumber += 1
      await delay(90)
    }

    await delay(120)

    for (const delta of contentChunks) {
      this.bus.emit({
        type: 'llm.sse.event',
        sessionId: snapshot.id,
        responseId,
        sequenceNumber,
        upstreamEventType: 'response.output_text.delta',
        rawEvent: { delta },
      })
      sequenceNumber += 1
      await delay(75)
    }

    await delay(180)

    this.bus.emit({
      type: 'llm.response.completed',
      sessionId: snapshot.id,
      responseId,
      rawResponse: {
        id: responseId,
        reasoning_text: input.reasoning,
        output_text: input.content,
      },
    })
    snapshot.llmStreamState = {
      responseId,
      status: 'completed',
      reasoningText: input.reasoning,
      contentText: input.content,
      events: [],
    }
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
