import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '@/services/api'

export interface ProbeNode {
  id: string
  name: string
  alias?: string
  ip: string
  status: string
  role?: string
  canvas_x?: number
  canvas_y?: number
}

export interface ProbeLink {
  id: string
  source_id: string
  target_id: string
  source_name?: string
  target_name?: string
  latest_metric?: ProbeMetric
}

export interface ProbeMetric {
  id: number
  link_id: string
  timestamp: string
  latency_min?: number
  latency_avg?: number
  latency_max?: number
  packet_loss?: number
  tcp_connect_time?: number
  bandwidth_mbps?: number
}

export interface ProbeAlert {
  id: number
  link_id: string
  type: string
  message: string
  triggered_at: string
  resolved_at?: string
}

export const useMonitorStore = defineStore('monitor', () => {
  const nodes = ref<ProbeNode[]>([])
  const links = ref<ProbeLink[]>([])
  const alerts = ref<ProbeAlert[]>([])
  const loading = ref(false)

  async function fetchNodes() {
    loading.value = true
    try {
      const res = await api.get('/v1/probe/nodes')
      nodes.value = res.data
    } catch { /* silent */ } finally {
      loading.value = false
    }
  }

  async function fetchLinks() {
    loading.value = true
    try {
      const res = await api.get('/v1/probe/links')
      links.value = res.data
    } catch { /* silent */ } finally {
      loading.value = false
    }
  }

  async function fetchAlerts(params?: { link_id?: string; type?: string; status?: string }) {
    loading.value = true
    try {
      const res = await api.get('/v1/probe/alerts', { params })
      alerts.value = res.data
    } catch { /* silent */ } finally {
      loading.value = false
    }
  }

  async function updateNodePosition(id: string, x: number, y: number) {
    try {
      await api.put(`/v1/probe/nodes/${id}`, { canvas_x: x, canvas_y: y })
      const node = nodes.value.find((n) => n.id === id)
      if (node) {
        node.canvas_x = x
        node.canvas_y = y
      }
    } catch { /* silent */ }
  }

  async function createLink(sourceId: string, targetId: string) {
    try {
      await api.post('/v1/probe/links', { source_id: sourceId, target_id: targetId })
      await fetchLinks()
    } catch { /* silent */ }
  }

  async function deleteLink(id: string) {
    try {
      await api.delete(`/v1/probe/links/${id}`)
      links.value = links.value.filter((l) => l.id !== id)
    } catch { /* silent */ }
  }

  async function fetchLinkMetrics(linkId: string, from?: string, to?: string): Promise<ProbeMetric[]> {
    try {
      const res = await api.get(`/v1/probe/links/${linkId}/metrics`, { params: { from, to } })
      return res.data
    } catch {
      return []
    }
  }

  return {
    nodes,
    links,
    alerts,
    loading,
    fetchNodes,
    fetchLinks,
    fetchAlerts,
    updateNodePosition,
    createLink,
    deleteLink,
    fetchLinkMetrics,
  }
})
