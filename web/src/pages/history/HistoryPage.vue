<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { toast } from 'vue-sonner'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { useHistoryStore } from '@/entities/task/model/history.store'
import HistoryTaskDetail from '@/widgets/history-task-detail/HistoryTaskDetail.vue'
import HistoryTaskList from '@/widgets/history-task-list/HistoryTaskList.vue'
import { formatRelativeMinutes } from '@/shared/lib/format'

const historyStore = useHistoryStore()
const route = useRoute()
const router = useRouter()
const search = ref('')
const statusFilter = ref('all')
const { t } = useI18n()

onMounted(async () => {
  if (!historyStore.items.length) {
    await historyStore.fetchAll()
    if (historyStore.error) {
      toast.error(historyStore.error)
    }
  }
})

const filteredTasks = computed(() => {
  const query = search.value.trim().toLowerCase()
  return historyStore.items.filter(task => {
    const matchesStatus = statusFilter.value === 'all' || task.status === statusFilter.value
    const matchesSearch =
      !query ||
      task.title.toLowerCase().includes(query) ||
      task.summary.toLowerCase().includes(query) ||
      task.targetLabels.some(label => label.toLowerCase().includes(query))

    return matchesStatus && matchesSearch
  })
})

async function syncSelection(taskId?: string) {
  if (!taskId) {
    historyStore.detail = null
    historyStore.selectedTaskId = ''
    return
  }

  await historyStore.selectTask(taskId)
}

watch(
  () => [historyStore.items, route.params.taskId] as const,
  async ([items, taskId]) => {
    if (typeof taskId === 'string') {
      await syncSelection(taskId)
      return
    }

    if (!historyStore.selectedTaskId && items.length) {
      await syncSelection(items[0].id)
    }
  },
  { immediate: true },
)

const detail = computed(() => historyStore.detail)
</script>

<template>
  <div class="min-h-screen px-4 py-6 md:px-6 xl:px-8">
    <div class="mx-auto flex max-w-7xl flex-col gap-6">
      <section class="glass-panel border-border/70 rounded-[2rem] border p-6">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div class="space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <Badge>{{ t('pages.history.badge') }}</Badge>
              <Badge variant="secondary">{{ t('pages.history.badgeSecondary') }}</Badge>
            </div>
            <h1 class="text-3xl font-semibold tracking-tight md:text-4xl">{{ t('pages.history.title') }}</h1>
            <p class="max-w-3xl text-sm leading-6 text-muted-foreground md:text-base">{{ t('pages.history.description') }}</p>
          </div>

          <div class="flex flex-wrap gap-2">
            <Button variant="outline" @click="router.push({ name: 'console' })">{{ t('common.nav.console') }}</Button>
            <Button variant="outline" @click="router.push({ name: 'nodes' })">{{ t('common.nav.nodes') }}</Button>
          </div>
        </div>
      </section>

      <Card v-if="historyStore.error" class="border-brand-danger/25 bg-brand-danger/5">
        <CardContent class="flex items-center justify-between gap-3 p-4 text-sm text-brand-danger">
          <span>{{ historyStore.error }}</span>
          <Button size="sm" variant="outline" @click="historyStore.fetchAll()">{{ t('common.buttons.retry') }}</Button>
        </CardContent>
      </Card>

      <div class="grid gap-6 xl:grid-cols-[0.95fr_1.05fr]">
        <HistoryTaskList
          v-model:search="search"
          v-model:status-filter="statusFilter"
          :items="filteredTasks"
          :selected-task-id="historyStore.selectedTaskId"
          @select="syncSelection"
        />

        <div class="space-y-4">
          <Card class="glass-panel border-border/70 rounded-2xl">
            <CardContent class="grid gap-4 p-6 sm:grid-cols-2 xl:grid-cols-4">
              <div class="rounded-xl border border-border/70 bg-background/70 p-4">
                <div class="text-xs uppercase tracking-[0.2em] text-muted-foreground">{{ t('common.labels.selected') }}</div>
                <div class="mt-2 text-2xl font-semibold">{{ historyStore.selectedTaskId || t('common.labels.none') }}</div>
              </div>
              <div class="rounded-xl border border-border/70 bg-background/70 p-4">
                <div class="text-xs uppercase tracking-[0.2em] text-muted-foreground">{{ t('common.labels.total') }}</div>
                <div class="mt-2 text-2xl font-semibold">{{ historyStore.items.length }}</div>
              </div>
              <div class="rounded-xl border border-border/70 bg-background/70 p-4">
                <div class="text-xs uppercase tracking-[0.2em] text-muted-foreground">{{ t('common.labels.filtered') }}</div>
                <div class="mt-2 text-2xl font-semibold">{{ filteredTasks.length }}</div>
              </div>
              <div class="rounded-xl border border-border/70 bg-background/70 p-4">
                <div class="text-xs uppercase tracking-[0.2em] text-muted-foreground">{{ t('common.labels.updated') }}</div>
                <div class="mt-2 text-sm font-medium">{{ detail ? formatRelativeMinutes(detail.updatedAt) : t('common.labels.notAvailable') }}</div>
              </div>
            </CardContent>
          </Card>

          <HistoryTaskDetail :task="detail" @back-to-console="router.push({ name: 'console' })" />
        </div>
      </div>
    </div>
  </div>
</template>
