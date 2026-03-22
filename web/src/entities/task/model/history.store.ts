import { defineStore } from 'pinia'

import { getHistoryTaskDetail, listHistoryTasks } from '@/shared/api/adapters/history'
import { toErrorMessage } from '@/shared/lib/errors'
import type { HistoryTaskDetail, HistoryTaskItem } from '@/shared/types/history'

export const useHistoryStore = defineStore('history', {
  state: () => ({
    items: [] as HistoryTaskItem[],
    selectedTaskId: '' as string,
    detail: null as HistoryTaskDetail | null,
    loading: false,
    error: null as string | null,
  }),
  actions: {
    async fetchAll() {
      this.loading = true
      try {
        this.items = await listHistoryTasks()
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load history')
      } finally {
        this.loading = false
      }
    },
    async selectTask(taskId: string) {
      this.selectedTaskId = taskId
      this.loading = true
      try {
        this.detail = await getHistoryTaskDetail(taskId)
        this.error = null
      } catch (error) {
        this.error = toErrorMessage(error, 'Failed to load task detail')
      } finally {
        this.loading = false
      }
    },
  },
})
