import { httpClient } from '@/shared/api/http-client'
import type {
  LinkStatus,
  MetricRow,
  ProbeAlert,
  ProbeNode,
} from '@/shared/types/probe'

export async function listProbeNodes(): Promise<ProbeNode[]> {
  return httpClient<ProbeNode[]>('/api/v1/probe/nodes')
}

export async function listProbeLinks(): Promise<LinkStatus[]> {
  return httpClient<LinkStatus[]>('/api/v1/probe/links')
}

export async function getLinkMetrics(
  linkId: string,
  from: string,
  to: string,
): Promise<MetricRow[]> {
  return httpClient<MetricRow[]>(
    `/api/v1/probe/links/${encodeURIComponent(linkId)}/metrics`,
    { query: { from, to } },
  )
}

export async function listProbeAlerts(params?: {
  link_id?: string
  type?: string
  status?: string
}): Promise<ProbeAlert[]> {
  return httpClient<ProbeAlert[]>('/api/v1/probe/alerts', { query: params })
}
