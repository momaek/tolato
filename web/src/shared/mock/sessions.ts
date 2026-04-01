import type {
  AssistantTextRow,
  SessionListItem,
  SessionSnapshot,
  ToolCallMetaRow,
  ToolResultMetaRow,
  UserMessageRow,
} from '@/shared/types/console'

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

export function createMockSessions(): SessionSnapshot[] {
  return [
    {
      id: 'session-ops-001',
      title: 'Tokyo nginx 风险处理',
      summary: '正在分析东京节点状态。',
      status: 'running',
      mode: 'ai_agent',
      revision: 7,
      updatedAt: minutesAgo(5),
      unread: 0,
      rows: [
        makeUserRow('row-user-401', '重启东京节点的 nginx', 9),
        makeToolCallRow('row-call-401-a', 'calling list_nodes(status=online,stale)', 8),
        makeToolResultRow('row-result-401-a', 'list_nodes returned 4 candidate nodes', 'neutral', 8),
        makeToolCallRow('row-call-401-b', 'calling resolve_target_nodes("东京节点")', 7),
        makeToolResultRow('row-result-401-b', 'resolve_target_nodes matched jp-tokyo-01 / jp-tokyo-02', 'neutral', 7),
        makeAssistantRow('row-assistant-401', '正在检查东京节点的 nginx 状态...', 6),
      ],
      nodeHealthSummary: { online: 3, offline: 1, busy: 1 },
    },
    {
      id: 'session-ops-002',
      title: 'Edge latency 巡检',
      summary: '上游延迟稳定，无需立即处理。',
      status: 'idle',
      mode: 'ai_agent',
      revision: 4,
      updatedAt: minutesAgo(18),
      unread: 1,
      rows: [
        makeAssistantRow('row-assistant-001', 'Control server ready. 4 agents connected.', 28),
        makeUserRow('row-user-398', '看一下东京边缘节点的 upstream latency', 25),
        makeToolCallRow('row-call-398-a', 'calling resolve_target_nodes("东京边缘节点")', 24),
        makeToolResultRow('row-result-398-a', 'resolved jp-tokyo-01', 'success', 24),
        makeToolCallRow('row-call-398-b', 'calling exec_on_nodes(inspect_upstream_latency)', 23),
        makeToolResultRow('row-result-398-b', 'execution finished · 1/1 success', 'success', 19),
        makeAssistantRow('row-assistant-003', '目前没有发现需要立即处理的异常，建议继续观察延迟变化。', 17),
      ],
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
      rows: [
        makeAssistantRow('row-assistant-101', 'Control server ready. 4 agents connected.', 2),
        makeAssistantRow(
          'row-assistant-102',
          '发送一个任务请求，AI 会自行决定是否查询节点并执行操作。',
          1,
        ),
      ],
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
  }
}
