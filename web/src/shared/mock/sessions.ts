import type {
  ApprovalRow,
  AssistantTextRow,
  PlanRow,
  SessionListItem,
  SessionSnapshot,
  SummaryRow,
  TargetConfirmationRow,
  ToolCallMetaRow,
  ToolResultMetaRow,
  UserMessageRow,
} from '@/shared/types/console'
import { mockNodeSummaries } from '@/shared/mock/nodes'

const baseTime = new Date('2026-03-22T08:40:00.000Z').getTime()

function minutesAgo(minutes: number) {
  return new Date(baseTime - minutes * 60_000).toISOString()
}

function makeUserRow(id: string, text: string, minutes: number): UserMessageRow {
  return { id, kind: 'user_message', text, createdAt: minutesAgo(minutes) }
}

function makeAssistantRow(id: string, markdown: string, minutes: number): AssistantTextRow {
  return { id, kind: 'assistant_text', markdown, createdAt: minutesAgo(minutes) }
}

function makeToolCallRow(id: string, label: string, minutes: number): ToolCallMetaRow {
  return { id, kind: 'tool_call_meta', label, createdAt: minutesAgo(minutes) }
}

function makeToolResultRow(
  id: string,
  label: string,
  tone: ToolResultMetaRow['tone'],
  minutes: number,
): ToolResultMetaRow {
  return { id, kind: 'tool_result_meta', label, tone, createdAt: minutesAgo(minutes) }
}

function makePlanRow(minutes: number): PlanRow {
  return {
    id: 'row-plan-401',
    kind: 'plan',
    createdAt: minutesAgo(minutes),
    taskId: 'task-401',
    inputText: '重启东京节点的 nginx',
    summary: '对东京边缘节点串行执行 nginx reload，并在前后采样 upstream 连接状态。',
    impact: '会短暂重载单节点 edge 进程，需要审批。',
    risk: 'medium',
    requiresApproval: true,
    targetLabel: 'jp-tokyo-01',
    targetSource: 'assistant_resolved',
    steps: [
      {
        action: 'inspect_nginx',
        argsLabel: '读取当前 worker / active connections',
        risk: 'low',
        timeoutSec: 20,
        broadcastAllowed: false,
      },
      {
        action: 'reload_nginx',
        argsLabel: '执行 nginx -s reload',
        risk: 'medium',
        timeoutSec: 30,
        broadcastAllowed: false,
      },
      {
        action: 'verify_upstreams',
        argsLabel: '检查 upstream 和 error.log tail',
        risk: 'low',
        timeoutSec: 45,
        broadcastAllowed: false,
      },
    ],
  }
}

function makeApprovalRow(minutes: number): ApprovalRow {
  return {
    id: 'row-approval-401',
    kind: 'approval',
    createdAt: minutesAgo(minutes),
    taskId: 'task-401',
    reason: '计划包含 Nginx reload，属于会改变线上流量承载状态的写操作。',
    risk: 'medium',
    impact: '影响单个东京 edge 节点。',
    targetLabel: 'jp-tokyo-01',
  }
}

function makeSummaryRow(minutes: number): SummaryRow {
  return {
    id: 'row-summary-398',
    kind: 'summary',
    createdAt: minutesAgo(minutes),
    taskId: 'task-398',
    total: 1,
    success: 1,
    failed: 0,
    skipped: 0,
    markdown:
      '东京入口当前 **p95 latency 112ms**，连接数稳定，没有发现明显的 upstream 退化。可以继续观察，不需要立刻执行自愈。',
    nextSteps: ['如需进一步确认，可执行 error log tail', '如延迟继续抬升，再进入 reload 计划'],
  }
}

const targetConfirmationRow: TargetConfirmationRow = {
  id: 'row-target-401',
  kind: 'target_confirmation',
  createdAt: minutesAgo(6),
  title: '需要确认目标节点',
  originalTargetText: '东京节点',
  basis: '解析“东京节点”后命中 2 台 edge 节点，按最近活跃度优先排序。',
  scope: 'single node',
  source: 'resolve_target_nodes("东京节点")',
  candidates: mockNodeSummaries
    .filter(node => node.region === 'Tokyo')
    .map(node => ({
      id: `candidate-${node.id}`,
      nodeId: node.id,
      label: node.hostname,
      region: node.region,
      scope: 'single',
      reason: node.id === 'jp-tokyo-01' ? '最近一次活跃，且当前会话摘要已在跟踪该节点' : '同区域备选节点',
      source: 'resolver',
      tags: node.tags,
    })),
}

export function createMockSessions(): SessionSnapshot[] {
  return [
    {
      id: 'session-ops-001',
      title: 'Tokyo nginx 风险处理',
      summary: '计划已生成，等待审批。',
      status: 'attention',
      mode: 'ai_agent',
      revision: 7,
      updatedAt: minutesAgo(5),
      unread: 0,
      approvalStatus: 'pending',
      targetContext: {
        state: 'confirmed',
        scope: 'single',
        summary: 'confirmed jp-tokyo-01',
        source: 'resolver',
        candidates: targetConfirmationRow.candidates,
        confirmedNodeIds: ['jp-tokyo-01'],
      },
      rows: [
        makeUserRow('row-user-401', '重启东京节点的 nginx', 9),
        makeToolCallRow('row-call-401-a', 'calling list_nodes(status=online,stale)', 8),
        makeToolResultRow('row-result-401-a', 'list_nodes returned 4 candidate nodes', 'neutral', 8),
        makeToolCallRow('row-call-401-b', 'calling resolve_target_nodes("东京节点")', 7),
        makeToolResultRow('row-result-401-b', 'resolve_target_nodes matched jp-tokyo-01 / jp-tokyo-02', 'neutral', 7),
        targetConfirmationRow,
        makeToolResultRow('row-result-401-c', 'target_confirmation succeeded · jp-tokyo-01 confirmed', 'success', 6),
        makeToolCallRow('row-call-401-c', 'calling propose_plan', 6),
        makeToolResultRow('row-result-401-d', 'plan generated · medium risk', 'warning', 5),
        makePlanRow(5),
        makeApprovalRow(4),
      ],
      candidateNodes: mockNodeSummaries.filter(node => node.region === 'Tokyo'),
      highlightedNodes: mockNodeSummaries.filter(node => ['jp-tokyo-01', 'jp-tokyo-02'].includes(node.id)),
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
      pendingActionType: 'approval',
    },
    {
      id: 'session-ops-002',
      title: 'Edge latency 巡检',
      summary: '上游延迟稳定，无需立即处理。',
      status: 'completed',
      mode: 'ai_agent',
      revision: 4,
      updatedAt: minutesAgo(18),
      unread: 1,
      approvalStatus: 'not_required',
      targetContext: {
        state: 'confirmed',
        scope: 'single',
        summary: 'confirmed jp-tokyo-01',
        source: 'session_context',
        candidates: [],
        confirmedNodeIds: ['jp-tokyo-01'],
      },
      rows: [
        makeAssistantRow('row-assistant-001', 'Control server ready. 4 agents connected.', 28),
        makeUserRow('row-user-398', '看一下东京边缘节点的 upstream latency', 25),
        makeToolCallRow('row-call-398-a', 'calling resolve_target_nodes("东京边缘节点")', 24),
        makeToolResultRow('row-result-398-a', 'resolved jp-tokyo-01', 'success', 24),
        makeToolCallRow('row-call-398-b', 'calling exec_on_nodes(inspect_upstream_latency)', 23),
        makeAssistantRow('row-assistant-002', '我已经为 jp-tokyo-01 生成只读检查计划，接下来会自动执行。', 23),
        {
          id: 'row-plan-398',
          kind: 'plan',
          createdAt: minutesAgo(23),
          taskId: 'task-398',
          inputText: '看一下东京边缘节点的 upstream latency',
          summary: '对 jp-tokyo-01 执行只读诊断，汇总 upstream latency 和健康状态。',
          impact: '只读，不改写任何节点状态。',
          risk: 'low',
          requiresApproval: false,
          targetLabel: 'jp-tokyo-01',
          targetSource: 'assistant_resolved',
          autoExecutionHint: '低风险只读计划将自动执行，无需审批。',
          steps: [
            {
              action: 'inspect_node',
              argsLabel: '采样 CPU / memory / disk',
              risk: 'low',
              timeoutSec: 20,
              broadcastAllowed: true,
            },
            {
              action: 'inspect_service',
              argsLabel: '读取服务健康和日志 tail',
              risk: 'low',
              timeoutSec: 45,
              broadcastAllowed: true,
            },
          ],
        },
        makeToolResultRow('row-result-398-b', 'execution finished · 1/1 success', 'success', 19),
        makeSummaryRow(18),
        makeAssistantRow('row-assistant-003', '目前没有发现需要立即处理的异常，建议继续观察延迟变化。', 17),
      ],
      candidateNodes: mockNodeSummaries.filter(node => node.region === 'Tokyo'),
      highlightedNodes: mockNodeSummaries.filter(node => node.id === 'jp-tokyo-01'),
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
    },
    {
      id: 'session-ops-003',
      title: '新会话',
      summary: '等待新的任务输入。',
      status: 'idle',
      mode: 'ai_agent',
      revision: 1,
      updatedAt: minutesAgo(1),
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
      rows: [
        makeAssistantRow('row-assistant-101', 'Control server ready. 4 agents connected.', 2),
        makeAssistantRow(
          'row-assistant-102',
          '发送一个任务请求，AI 会自行决定是否查询节点、确认目标、生成计划或进入审批。',
          1,
        ),
      ],
      candidateNodes: [],
      highlightedNodes: [],
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
    },
  ]
}

export function toSessionListItem(snapshot: SessionSnapshot): SessionListItem {
  return {
    id: snapshot.id,
    title: snapshot.title,
    summary: snapshot.summary,
    status: snapshot.status,
    unread: snapshot.unread,
    updatedAt: snapshot.updatedAt,
    targetSummary: snapshot.targetContext.summary,
  }
}
