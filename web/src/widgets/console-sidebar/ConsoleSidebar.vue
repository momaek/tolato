<script setup lang="ts">
import PanelCard from '@/shared/ui/panel-card/PanelCard.vue'
import SessionList from '@/features/console-session-list/SessionList.vue'
import type { SessionListItem } from '@/shared/types/console'

defineProps<{
  sessions: SessionListItem[]
  activeSessionId: string
}>()

const emit = defineEmits<{
  selectSession: [sessionId: string]
}>()
</script>

<template>
  <aside class="h-full min-h-0 pr-1">
    <PanelCard
      compact
      body-class="flex-1 min-h-0"
      class="flex h-full min-h-0 flex-col overflow-hidden"
    >
      <SessionList :sessions="sessions" :active-session-id="activeSessionId" @select="emit('selectSession', $event)" />
    </PanelCard>
  </aside>
</template>
