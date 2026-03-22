import { defineStore } from 'pinia'

import { getNodeDetail } from '@/shared/api/adapters/nodes'
import { toErrorMessage } from '@/shared/lib/errors'
import type { NodeDetail } from '@/shared/types/node'

export const useNodeDetailStore = defineStore('node-detail', {
  state: () => ({
    item: null as NodeDetail | null,
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetch(nodeId: string) {
      this.loading = true
      try {
        this.item = await getNodeDetail(nodeId)
        this.error = this.item ? null : 'Node not found'
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load node detail')
      } finally {
        this.loading = false
      }
    },
  },
})
