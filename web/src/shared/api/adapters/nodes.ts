import { z } from 'zod'

import { httpClient } from '@/shared/api/http-client'
import { appEnv } from '@/shared/config/env'
import { normalizePercentValue } from '@/shared/lib/format'
import { mockNodes, mockNodeSummaries } from '@/shared/mock/nodes'
import type { NodeDetail, NodeSummary } from '@/shared/types/node'

const nodeSchema = z.object({
  id: z.string(),
  hostname: z.string(),
  region: z.string(),
  os: z.string(),
  version: z.string(),
  tags: z.array(z.string()),
  status: z.string(),
  busy: z.boolean(),
  last_seen_at: z.string(),
  metrics: z.object({
    cpu: z.number(),
    memory: z.number(),
    disk: z.number(),
  }),
  ip_address: z.string().optional(),
  provider: z.string().optional(),
  kernel: z.string().optional(),
  uptime: z.string().optional(),
  agent_version: z.string().optional(),
  risk_signals: z.array(z.string()).optional(),
  recent_tasks: z
    .array(
      z.object({
        id: z.string(),
        title: z.string(),
        status: z.string(),
        created_at: z.string(),
      }),
    )
    .optional(),
})

export async function listNodes(): Promise<NodeSummary[]> {
  if (appEnv.useMock) {
    return structuredClone(mockNodeSummaries)
  }

  const response = await httpClient<{ nodes: z.infer<typeof nodeSchema>[] }>(
    '/api/v1/nodes',
  )
  return response.nodes.map((node) => ({
    id: node.id,
    hostname: node.hostname,
    region: node.region,
    os: node.os,
    version: node.version,
    ipAddress: node.ip_address ?? '',
    provider: node.provider ?? '',
    tags: node.tags,
    status: node.status as NodeSummary['status'],
    busy: node.busy,
    lastSeen: node.last_seen_at,
    metrics: adaptMetrics(node.metrics),
  }))
}

export async function getNodeDetail(
  nodeId: string,
): Promise<NodeDetail | null> {
  if (appEnv.useMock) {
    return structuredClone(mockNodes.find((node) => node.id === nodeId) ?? null)
  }

  const response = await httpClient(`/api/v1/nodes/${nodeId}`)
  const parsed = nodeSchema.parse(response)

  return {
    id: parsed.id,
    hostname: parsed.hostname,
    region: parsed.region,
    os: parsed.os,
    version: parsed.version,
    ipAddress: parsed.ip_address ?? '',
    provider: parsed.provider ?? '',
    tags: parsed.tags,
    status: parsed.status as NodeDetail['status'],
    busy: parsed.busy,
    lastSeen: parsed.last_seen_at,
    metrics: adaptMetrics(parsed.metrics),
    kernel: parsed.kernel ?? '',
    uptime: parsed.uptime ?? '',
    agentVersion: parsed.agent_version ?? parsed.version ?? '',
    riskSignals: parsed.risk_signals ?? [],
    recentTasks: (parsed.recent_tasks ?? []).map((task) => ({
      id: task.id,
      title: task.title,
      status: task.status as NodeDetail['recentTasks'][number]['status'],
      createdAt: task.created_at,
    })),
  }
}

function adaptMetrics(metrics: z.infer<typeof nodeSchema>['metrics']) {
  return {
    cpu: normalizePercentValue(metrics.cpu),
    memory: normalizePercentValue(metrics.memory),
    disk: normalizePercentValue(metrics.disk),
  }
}
