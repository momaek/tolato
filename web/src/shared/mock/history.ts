import type { HistoryTaskDetail } from '@/shared/types/history'

const baseTime = new Date('2026-03-22T08:40:00.000Z').getTime()

function minutesAgo(minutes: number) {
  return new Date(baseTime - minutes * 60_000).toISOString()
}

export const mockHistoryTasks: HistoryTaskDetail[] = [
  {
    id: 'task-401',
    title: 'Restart nginx safely',
    summary: '先确认东京边缘节点，再生成审批计划并串行执行 reload。',
    status: 'waiting_approval',
    approvalStatus: 'pending',
    risk: 'medium',
    targetLabels: ['jp-tokyo-01'],
    createdAt: minutesAgo(7),
    updatedAt: minutesAgo(5),
    impact: '会触发 edge 入口 Nginx reload，预计 1-2 秒内完成。',
    steps: ['识别东京节点', '生成 reload 计划', '等待审批', '执行 nginx -s reload'],
    executions: [{ id: 'exec-401-1', taskId: 'task-401', nodeId: 'jp-tokyo-01', label: 'jp-tokyo-01', status: 'queued' }],
    auditEvents: [
      { id: 'audit-901', actor: 'alex', eventType: 'plan_generated', description: '生成结构化计划并标记为需要审批。', createdAt: minutesAgo(6) },
      { id: 'audit-902', actor: 'system', eventType: 'approval_pending', description: '风险等级 medium，等待显式批准。', createdAt: minutesAgo(5) },
    ],
    toolMeta: ['list_nodes(status=online,stale)', 'resolve_target_nodes("东京节点")', 'propose_plan'],
    aiSummary: '计划已经就绪，但由于属于写操作，仍需审批后才能继续。建议先确认当前连接数和 upstream 状态。',
  },
  {
    id: 'task-398',
    title: 'Inspect upstream latency',
    summary: '对东京边缘节点执行只读诊断，聚合 upstream 延迟和连接数。',
    status: 'success',
    approvalStatus: 'not_required',
    risk: 'low',
    targetLabels: ['jp-tokyo-01'],
    createdAt: minutesAgo(22),
    updatedAt: minutesAgo(18),
    impact: '只读采样，不改写节点状态。',
    steps: ['确认目标节点', '执行 latency 采样', '聚合结果'],
    executions: [
      {
        id: 'exec-398-1',
        taskId: 'task-398',
        nodeId: 'jp-tokyo-01',
        label: 'jp-tokyo-01',
        status: 'success',
        stdoutTail: 'p95 latency 112ms\nactive connections 382',
      },
    ],
    auditEvents: [
      { id: 'audit-890', actor: 'alex', eventType: 'target_confirmed', description: '确认东京节点作为目标。', createdAt: minutesAgo(21) },
      { id: 'audit-891', actor: 'system', eventType: 'summary_emitted', description: '写入只读诊断总结。', createdAt: minutesAgo(18) },
    ],
    toolMeta: ['list_nodes', 'resolve_target_nodes', 'exec_on_nodes(inspect_latency)'],
    aiSummary: '东京入口延迟处于可接受区间，p95 未超过 120ms，暂时无需干预。',
  },
  {
    id: 'task-377',
    title: 'Check docker containers',
    summary: '检查 API 节点容器健康情况并返回异常实例。',
    status: 'partial_failed',
    approvalStatus: 'not_required',
    risk: 'low',
    targetLabels: ['us-sfo-01', 'eu-fra-01'],
    createdAt: minutesAgo(108),
    updatedAt: minutesAgo(100),
    impact: '只读检查两个 API / batch 节点上的容器状态。',
    steps: ['解析两个节点', '执行 docker ps', '归并失败节点'],
    executions: [
      { id: 'exec-377-1', taskId: 'task-377', nodeId: 'us-sfo-01', label: 'us-sfo-01', status: 'success', stdoutTail: 'api: healthy\nworker: healthy' },
      { id: 'exec-377-2', taskId: 'task-377', nodeId: 'eu-fra-01', label: 'eu-fra-01', status: 'failed', stderrTail: 'heartbeat stale, command skipped' },
    ],
    auditEvents: [
      { id: 'audit-850', actor: 'system', eventType: 'task_dispatched', description: '两个节点开始只读容器检查。', createdAt: minutesAgo(106) },
      { id: 'audit-851', actor: 'system', eventType: 'task_finished', description: '一个节点成功，一个节点因 stale 被跳过。', createdAt: minutesAgo(100) },
    ],
    toolMeta: ['session.message.submit', 'session.snapshot.request', 'execution.finished'],
    aiSummary: 'San Francisco 节点容器健康，Frankfurt 节点因心跳过期未执行，需要先恢复连接。',
  },
]
