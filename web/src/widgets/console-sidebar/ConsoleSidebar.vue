<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Separator } from '@/components/ui/separator'
import PanelCard from '@/shared/ui/panel-card/PanelCard.vue'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import SessionList from '@/features/console-session-list/SessionList.vue'
import type { SessionListItem, SessionSnapshot } from '@/shared/types/console'

const props = defineProps<{
  sessions: SessionListItem[]
  activeSessionId: string
  snapshot: SessionSnapshot | null
}>()

const emit = defineEmits<{
  createSession: []
  deleteSession: [sessionId: string]
  selectSession: [sessionId: string]
}>()

const { t } = useI18n()

const hasTarget = computed(() => {
  return props.snapshot?.targetContext.state !== 'unset'
})

const scopeTone = computed(() => {
  const scope = props.snapshot?.targetContext.scope
  if (scope === 'all_online') return 'warning' as const
  if (scope === 'multi') return 'info' as const
  return 'neutral' as const
})

const confirmedNodes = computed(() => {
  if (!props.snapshot) return []
  const confirmedIds = new Set(props.snapshot.targetContext.confirmedNodeIds)
  return props.snapshot.candidateNodes.filter((n) => confirmedIds.has(n.id))
})

function statusTone(status: string) {
  if (status === 'online') return 'success' as const
  if (status === 'busy') return 'warning' as const
  if (status === 'offline') return 'danger' as const
  return 'neutral' as const
}
</script>

<template>
  <aside class="h-full min-h-0 pr-1">
    <PanelCard
      compact
      body-class="flex-1 min-h-0"
      class="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <SessionList
        :sessions="sessions"
        :active-session-id="activeSessionId"
        @create="emit('createSession')"
        @delete="emit('deleteSession', $event)"
        @select="emit('selectSession', $event)"
      />

      <Separator class="my-2" />

      <div class="shrink-0 space-y-3 px-1 pb-2">
        <p class="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
          {{ t('console.sidebar.currentTargets') }}
        </p>

        <template v-if="hasTarget && snapshot">
          <div class="space-y-1.5">
            <p class="text-sm font-medium leading-5 text-foreground">
              {{ snapshot.targetContext.summary }}
            </p>
            <StatusBadge :label="snapshot.targetContext.scope" :tone="scopeTone" />
          </div>

          <div v-if="confirmedNodes.length" class="space-y-1.5">
            <p class="text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground">
              {{ t('console.sidebar.confirmed') }}
            </p>
            <div
              v-for="node in confirmedNodes"
              :key="node.id"
              class="flex items-center justify-between gap-2 rounded-lg border border-border/40 bg-background/60 px-3 py-1.5"
            >
              <span class="truncate text-xs font-semibold text-foreground">{{ node.hostname }}</span>
              <StatusBadge :label="node.status" :tone="statusTone(node.status)" dot />
            </div>
          </div>
        </template>

        <p v-else class="text-xs text-muted-foreground">
          {{ t('console.sidebar.noConfirmed') }}
        </p>

        <div v-if="snapshot" class="flex flex-wrap gap-x-3 gap-y-1 text-[11px] text-muted-foreground">
          <span class="inline-flex items-center gap-1">
            <span class="size-1.5 rounded-full bg-brand-success" />
            {{ snapshot.nodeHealthSummary.online }} {{ t('console.sidebar.onlineNodes') }}
          </span>
          <span class="inline-flex items-center gap-1">
            <span class="size-1.5 rounded-full bg-brand-warning" />
            {{ snapshot.nodeHealthSummary.busy }} {{ t('console.sidebar.busyNodes') }}
          </span>
          <span class="inline-flex items-center gap-1">
            <span class="size-1.5 rounded-full bg-muted-foreground/40" />
            {{ snapshot.nodeHealthSummary.offline }} {{ t('console.sidebar.offlineNodes') }}
          </span>
        </div>
      </div>
    </PanelCard>
  </aside>
</template>
