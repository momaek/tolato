import { defineStore } from 'pinia'

import { getWSClient } from '@/shared/ws/ws-client'

export const useConnectionStore = defineStore('connection', {
  state: () => ({
    status: 'connecting' as 'connecting' | 'connected' | 'reconnecting' | 'offline',
    lastSyncedAt: '',
    initialized: false,
  }),
  actions: {
    async initialize() {
      if (this.initialized) {
        return
      }

      const client = getWSClient()
      client.subscribe(event => {
        if (event.type === 'connection.ready') {
          this.status = 'connected'
          this.lastSyncedAt = event.timestamp
        }

        if (event.type === 'connection.synced') {
          this.status = 'connected'
          this.lastSyncedAt = event.timestamp
        }
      })

      await client.connect()
      this.initialized = true
    },
  },
})
