<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { Separator } from '@/components/ui/separator'
import PanelCard from '@/shared/ui/panel-card/PanelCard.vue'
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
