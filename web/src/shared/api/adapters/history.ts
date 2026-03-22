import { httpClient } from '@/shared/api/http-client'
import { appEnv } from '@/shared/config/env'
import { mockHistoryTasks } from '@/shared/mock/history'
import type { HistoryTaskDetail, HistoryTaskItem } from '@/shared/types/history'

export async function listHistoryTasks(): Promise<HistoryTaskItem[]> {
  if (appEnv.useMock) {
    return structuredClone(mockHistoryTasks)
  }

  return httpClient<HistoryTaskItem[]>('/api/v1/history/tasks')
}

export async function getHistoryTaskDetail(taskId: string): Promise<HistoryTaskDetail | null> {
  if (appEnv.useMock) {
    return structuredClone(mockHistoryTasks.find(task => task.id === taskId) ?? null)
  }

  return httpClient<HistoryTaskDetail>(`/api/v1/history/tasks/${taskId}`)
}
