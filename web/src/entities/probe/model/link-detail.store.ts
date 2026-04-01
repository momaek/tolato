import { defineStore } from 'pinia'

import { getLinkMetrics, listProbeAlerts } from '@/shared/api/adapters/probe'
import { toErrorMessage } from '@/shared/lib/errors'
import type { MetricRow, ProbeAlert } from '@/shared/types/probe'

export type TimeRange = '1h' | '6h' | '24h' | '7d'

function timeRangeToMs(range: TimeRange): number {
  switch (range) {
    case '1h':
      return 60 * 60 * 1000
    case '6h':
      return 6 * 60 * 60 * 1000
    case '24h':
      return 24 * 60 * 60 * 1000
    case '7d':
      return 7 * 24 * 60 * 60 * 1000
  }
}

export const useLinkDetailStore = defineStore('linkDetail', {
  state: () => ({
    linkId: '',
    timeRange: '1h' as TimeRange,
    metrics: [] as MetricRow[],
    alerts: [] as ProbeAlert[],
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetch(linkId: string) {
      this.linkId = linkId
      this.loading = true
      try {
        const now = new Date()
        const from = new Date(now.getTime() - timeRangeToMs(this.timeRange))

        const [metrics, alerts] = await Promise.all([
          getLinkMetrics(linkId, from.toISOString(), now.toISOString()),
          listProbeAlerts({ link_id: linkId }),
        ])
        this.metrics = metrics
        this.alerts = alerts
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load link detail')
      } finally {
        this.loading = false
      }
    },
    setTimeRange(range: TimeRange) {
      this.timeRange = range
      if (this.linkId) {
        this.fetch(this.linkId)
      }
    },
  },
})
