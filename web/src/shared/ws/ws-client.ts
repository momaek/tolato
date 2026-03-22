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

  subscribe(handler: (event: WSUIEvent) => void) {
    return this.bus.on(handler)
  }

  async requestSessionsList() {
    return Array.from(this.sessions.values())
      .sort((a, b) => +new Date(b.updatedAt) - +new Date(a.updatedAt))
      .map(toSessionListItem)
  }

  async requestSessionSnapshot(sessionId: string) {
    const snapshot = this.sessions.get(sessionId)
    if (!snapshot) {
      throw new Error(`unknown session: ${sessionId}`)
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
      throw new Error('session_busy')
    }

    const scenario = this.buildScenario(snapshot, request.text)
    snapshot.status = 'running'
    snapshot.summary = '正在解析目标节点。'
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
        ? '当前输入没有显式目标，先检查会话上下文里是否已有已确认目标。'
        : '先列出在线节点并解析用户输入里的目标表达，确认本轮应该作用到哪台机器。',
      content: scenario.targetSource === 'context_inherited'
        ? `我会先沿用上一轮已确认的 ${scenario.targetRow.candidates.map(candidate => candidate.label).join(' / ')}，然后继续这轮分析。`
        : '我先帮你解析目标节点，并检查当前可用节点范围。',
    })

    const rows = [
      this.makeUserRow(request.text),
      ...(scenario.targetSource === 'context_inherited'
        ? [this.makeAssistantRow(`将沿用上一轮已确认的 ${scenario.targetRow.candidates.map(candidate => candidate.label).join(' / ')}。`)]
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

      const result = this.makeToolResultRow('候选节点顺序已刷新，请重新确认目标。', 'warning')
      snapshot.rows.push(result)
      this.bumpRevision(snapshot)
      this.bus.emit({ type: 'timeline.row.appended', sessionId: snapshot.id, row: result, revision: snapshot.revision })
      this.emitSessionSummary(snapshot)
      return
    }

    if (request.action === 'clear') {
      snapshot.pendingActionType = undefined
      snapshot.status = 'idle'
      snapshot.summary = '目标上下文已清空。'
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
    snapshot.summary = draft.blockedReason ?? (draft.approvalRow ? '计划已生成，等待审批。' : '低风险计划已生成，开始执行。')
    snapshot.approvalStatus = draft.approvalRow ? 'pending' : 'not_required'

    await this.emitMockAssistantStream(snapshot, {
      reasoning: '目标已经确认，接下来生成结构化计划，并判断风险等级以及是否需要审批。',
      content: draft.blockedReason
        ? '我已经完成计划生成，但这次操作属于广播写操作，默认不会继续执行。'
        : draft.approvalRow
          ? `我已经生成针对 ${targetLabel} 的计划，这次操作需要你明确审批。`
          : `我已经生成针对 ${targetLabel} 的只读计划，接下来会继续自动执行。`,
    })

    const resultRows = [
      this.makeToolResultRow(`target_confirmation succeeded · ${targetLabel} confirmed`, 'success'),
      this.makeToolCallRow('calling propose_plan'),
      this.makeToolResultRow(`plan generated · ${draft.planRow.risk} risk`, draft.planRow.requiresApproval ? 'warning' : 'success'),
      this.makeAssistantRow(
        draft.blockedReason
          ? '这次计划涉及广播写操作，MVP 默认不会继续执行，请先缩小目标范围。'
          : draft.planRow.requiresApproval
            ? `我已经为 ${targetLabel} 生成执行计划，这次操作会影响线上状态，需要你先审批。`
            : `我已经为 ${targetLabel} 生成只读计划，风险较低，将继续自动执行。`,
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
      snapshot.summary = request.action === 'reject' ? '审批已拒绝。' : '审批已取消。'
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
    snapshot.summary = '审批已通过，任务执行中。'
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
        ? ['观察 5 分钟内的 upstream 5xx', '如需回滚，可重新生成 restore 计划']
        : ['如果延迟继续抬升，可升级为 reload 计划'],
    }

    snapshot.rows.push(summaryRow)
    await this.emitMockAssistantStream(snapshot, {
      reasoning: '汇总节点执行结果，生成一条更适合用户直接阅读的自然语言结论。',
      content: draft.planRow.requiresApproval
        ? '执行已经完成，重载后的健康检查正常，建议继续观察短时间内的 upstream 和 5xx 变化。'
        : '只读诊断已经完成，目前没有发现需要立刻处理的异常，可以继续观察。',
    })
    const assistantConclusion = this.makeAssistantRow(
      draft.planRow.requiresApproval
        ? '执行已经完成，重载后的健康检查正常。接下来建议短时间观察 upstream 和 5xx 变化。'
        : '只读诊断已经完成，目前没有发现需要立刻处理的异常，可以继续观察。',
    )
    snapshot.rows.push(assistantConclusion)
    snapshot.status = 'completed'
    snapshot.summary = draft.planRow.requiresApproval ? '执行完成，Nginx reload 成功。' : '只读诊断完成。'
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
        reason: node.status === 'busy' ? '在线但忙碌，广播执行时需要谨慎观察。' : '当前在线，可加入广播目标集合。',
        source: 'resolver',
        tags: node.tags,
      }))
      targetContextScope = 'all_online'
      targetSource = 'assistant_resolved'
      sourceLabel = 'resolve_target_nodes("all online nodes")'
      scopeLabel = 'all online nodes'
      basis = `输入“${text}”命中了广播范围，候选列表展示当前所有在线节点。`
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
        reason: '当前输入没有显式目标，沿用上一轮已确认目标。',
        source: 'session_context',
        tags: node.tags,
      }))
      targetContextScope = snapshot.targetContext.scope === 'unset' ? 'single' : snapshot.targetContext.scope
      targetSource = 'context_inherited'
      sourceLabel = 'reuse_confirmed_target_context'
      scopeLabel = targetContextScope === 'all_online' ? 'all online nodes' : targetContextScope === 'multi' ? 'multi-node target set' : 'single node'
      basis = '当前输入未提供新的目标表达，系统将沿用上一轮已经确认的目标集合。'
      originalTargetText = snapshot.targetContext.summary
      inheritedHint = '沿用上一轮已确认目标'
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
          reason: node.id === 'jp-tokyo-01' ? '最近活跃，且会话摘要正在关注该节点。' : '同区域备选节点。',
          source: 'resolver',
          tags: node.tags,
        }))
      targetContextScope = 'single'
      targetSource = 'assistant_resolved'
      sourceLabel = 'resolve_target_nodes("东京节点")'
      scopeLabel = candidates.length > 1 ? 'single node from region cluster' : 'single node'
      basis = `根据输入“${text}”解析得到以下候选目标。`
      originalTargetText = '东京节点'
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
          reason: '文本中提到了 API / Docker 相关上下文。',
          source: 'resolver',
          tags: defaultNode.tags,
        },
      ]
      targetContextScope = 'single'
      targetSource = 'assistant_resolved'
      sourceLabel = 'resolve_target_nodes'
      scopeLabel = 'single node'
      basis = `根据输入“${text}”解析得到以下候选目标。`
      originalTargetText = text
      confirmedNodeIds = [defaultNode.id]
    }

    const blockedReason = writePlan && targetContextScope !== 'single' ? '广播写操作在 MVP 中默认阻断，请缩小目标范围后重试。' : undefined
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
        title: targetContextScope === 'all_online' ? '需要确认广播目标' : '需要确认目标节点',
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
          ? `对 ${options.targetLabel} 执行串行写操作前置检查，并评估是否允许继续。`
          : `对 ${options.targetLabel} 进行 Nginx reload，并在前后做健康验证。`
        : isBroadcast
          ? `对 ${options.targetLabel} 执行广播只读诊断，汇总逐节点指标与异常线索。`
          : `对 ${options.targetLabel} 执行只读诊断，汇总当前指标与异常线索。`,
      impact: options.writePlan
        ? isBroadcast
          ? `涉及 ${options.targetCount} 个节点的写操作，默认阻断，不进入执行阶段。`
          : '影响单节点入口流量承载，预计 1-2 秒。'
        : isBroadcast
          ? `只读执行，会串行采样 ${options.targetCount} 个在线节点，不改写任何节点状态。`
          : '只读，不改写任何节点状态。',
      risk,
      requiresApproval,
      targetLabel: options.targetLabel,
      targetSource: options.targetSource,
      autoExecutionHint: !options.writePlan ? '低风险只读计划将自动执行，无需审批。' : undefined,
      steps: options.writePlan
        ? [
            { action: 'inspect_nginx', argsLabel: '读取活跃连接和 worker', risk: isBroadcast ? 'medium' : 'low', timeoutSec: 20, broadcastAllowed: false },
            { action: 'reload_nginx', argsLabel: 'nginx -s reload', risk: isBroadcast ? 'high' : 'medium', timeoutSec: 30, broadcastAllowed: false },
            { action: 'verify_service', argsLabel: '检查 error log 和 upstream', risk: isBroadcast ? 'medium' : 'low', timeoutSec: 45, broadcastAllowed: false },
          ]
        : [
            { action: 'inspect_node', argsLabel: '采样 CPU / memory / disk', risk: 'low', timeoutSec: 20, broadcastAllowed: true },
            { action: 'inspect_service', argsLabel: '读取服务健康和日志 tail', risk: 'low', timeoutSec: 45, broadcastAllowed: true },
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
            reason: '计划包含线上服务重载，需要显式批准。',
            risk,
            impact: isBroadcast ? `${options.targetCount} 个节点的写操作` : '单节点写操作',
            targetLabel: options.targetLabel,
          }
        : undefined

    return {
      planRow,
      approvalRow,
      executionTitle: options.writePlan ? '执行 nginx reload' : '执行只读诊断',
      summaryMarkdown: options.writePlan
        ? `${options.targetLabel} 已完成 **nginx reload**，前后健康检查都通过，未发现新的 5xx 激增。`
        : `${options.targetLabel} 的只读检查已完成，目前没有发现需要立即处理的异常。`,
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
      throw new Error(`unknown session: ${sessionId}`)
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

export function isMockWSClient() {
  return appEnv.useMock
}
