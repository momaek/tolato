import { defineStore } from 'pinia'

import { listProbeAlerts } from '@/shared/api/adapters/probe'
import { toErrorMessage } from '@/shared/lib/errors'
import type { ProbeAlert } from '@/shared/types/probe'

export const useAlertsStore = defineStore('probeAlerts', {
  state: () => ({
    items: [] as ProbeAlert[],
    filterLinkId: '',
    filterType: '',
    filterStatus: 'all' as 'all' | 'open' | 'resolved',
    loading: false,
    error: null as string | null,
  }),
  getters: {
    filteredItems(state) {
      return state.items.filter((alert) => {
        if (state.filterLinkId && alert.link_id !== state.filterLinkId) return false
        if (state.filterType && alert.type !== state.filterType) return false
        if (state.filterStatus === 'open' && alert.resolved_at) return false
        if (state.filterStatus === 'resolved' && !alert.resolved_at) return false
        return true
      })
    },
  },
  actions: {
    async fetchAll() {
      this.loading = true
      try {
        const params: Record<string, string> = {}
        if (this.filterStatus !== 'all') params.status = this.filterStatus
        this.items = await listProbeAlerts(params)
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load alerts')
      } finally {
        this.loading = false
      }
    },
  },
})
