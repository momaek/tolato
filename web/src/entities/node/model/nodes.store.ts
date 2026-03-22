import { defineStore } from 'pinia'

import { listNodes } from '@/shared/api/adapters/nodes'
import { toErrorMessage } from '@/shared/lib/errors'
import type { NodeSummary } from '@/shared/types/node'

export const useNodesStore = defineStore('nodes', {
  state: () => ({
    items: [] as NodeSummary[],
    search: '',
    status: 'all' as 'all' | NodeSummary['status'],
    region: 'all',
    tag: 'all',
    busyOnly: false,
    loading: false,
    error: null as string | null,
  }),
  getters: {
    filteredItems(state) {
      return state.items.filter(node => {
        const matchesSearch =
          !state.search ||
          node.hostname.toLowerCase().includes(state.search.toLowerCase()) ||
          node.region.toLowerCase().includes(state.search.toLowerCase()) ||
          node.tags.some(tag => tag.toLowerCase().includes(state.search.toLowerCase()))
        const matchesStatus = state.status === 'all' || node.status === state.status
        const matchesRegion = state.region === 'all' || node.region === state.region
        const matchesTag = state.tag === 'all' || node.tags.includes(state.tag)
        const matchesBusy = !state.busyOnly || node.busy
        return matchesSearch && matchesStatus && matchesRegion && matchesTag && matchesBusy
      })
    },
  },
  actions: {
    async fetchAll() {
      this.loading = true
      try {
        this.items = await listNodes()
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load nodes')
      } finally {
        this.loading = false
      }
    },
  },
})
