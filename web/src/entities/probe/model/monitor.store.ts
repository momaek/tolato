import { defineStore } from 'pinia'

import {
  listProbeAlerts,
  listProbeLinks,
  listProbeNodes,
} from '@/shared/api/adapters/probe'
import { toErrorMessage } from '@/shared/lib/errors'
import type { LinkStatus, ProbeAlert, ProbeNode } from '@/shared/types/probe'

export const useMonitorStore = defineStore('monitor', {
  state: () => ({
    nodes: [] as ProbeNode[],
    links: [] as LinkStatus[],
    alerts: [] as ProbeAlert[],
    loading: false,
    initialized: false,
    error: null as string | null,
    _refreshTimer: null as ReturnType<typeof setInterval> | null,
  }),
  getters: {
    nodesByRole(state) {
      const groups = { entry: [] as ProbeNode[], relay: [] as ProbeNode[], landing: [] as ProbeNode[] }
      for (const node of state.nodes) {
        if (node.role in groups) {
          groups[node.role].push(node)
        }
      }
      return groups
    },
    totalNodes(state) {
      return state.nodes.length
    },
    totalLinks(state) {
      return state.links.length
    },
    alertLinks(state) {
      return state.links.filter((l) => l.status === 'alert').length
    },
    warnLinks(state) {
      return state.links.filter((l) => l.status === 'warn').length
    },
    openAlerts(state) {
      return state.alerts.filter((a) => !a.resolved_at).length
    },
  },
  actions: {
    async fetchAll() {
      this.loading = true
      try {
        const [nodes, links, alerts] = await Promise.all([
          listProbeNodes(),
          listProbeLinks(),
          listProbeAlerts({ status: 'open' }),
        ])
        this.nodes = nodes
        this.links = links
        this.alerts = alerts
        this.error = null
        this.initialized = true
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load monitor data')
      } finally {
        this.loading = false
      }
    },
    startAutoRefresh() {
      this.stopAutoRefresh()
      this._refreshTimer = setInterval(() => {
        this.fetchAll()
      }, 30_000)
    },
    stopAutoRefresh() {
      if (this._refreshTimer) {
        clearInterval(this._refreshTimer)
        this._refreshTimer = null
      }
    },
  },
})
