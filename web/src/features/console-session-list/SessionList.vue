<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Trash2 } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Separator } from '@/components/ui/separator'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { SessionListItem } from '@/shared/types/console'
import { formatRelativeMinutes } from '@/shared/lib/format'

defineProps<{
  sessions: SessionListItem[]
  activeSessionId: string
}>()

const emit = defineEmits<{
  create: []
  delete: [sessionId: string]
  select: [sessionId: string]
}>()
const { t } = useI18n()

const statusTone = computed(() => {
  return (status: SessionListItem['status']) => {
    switch (status) {
      case 'running':
        return 'info'
      case 'attention':
        return 'warning'
      case 'completed':
        return 'success'
      default:
        return 'neutral'
    }
  }
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <div class="flex items-center justify-between">
      <div>
        <p class="text-xs font-semibold uppercase tracking-[0.3em] text-muted-foreground">{{ t('console.sessions.title') }}</p>
        <p class="mt-1 text-sm text-foreground">{{ t('console.sessions.description') }}</p>
      </div>
      <div class="flex items-center gap-2">
        <StatusBadge :label="t('console.sessions.total', { count: sessions.length })" />
        <Button
          type="button"
          variant="outline"
          size="icon"
          class="size-8 rounded-full"
          :aria-label="t('console.sessions.newSession')"
          @click="emit('create')"
        >
          <Plus class="size-4" />
        </Button>
      </div>
    </div>
    <Separator />
    <div class="relative min-h-0 flex-1">
      <ScrollArea class="h-full pr-2">
        <div class="space-y-2 pb-8">
          <div
            v-for="session in sessions"
            :key="session.id"
            class="flex items-start gap-2 rounded-[0.9rem] border px-3 py-3"
            :class="
              session.id === activeSessionId
                ? 'border-primary/30 bg-primary/10'
                : 'border-transparent bg-background/50 hover:border-border hover:bg-background/80'
            "
          >
            <button
              type="button"
              class="min-w-0 flex-1 text-left"
              @click="emit('select', session.id)"
            >
              <div class="w-full space-y-2">
                <div class="flex items-center justify-between gap-3">
                  <p class="truncate text-sm font-semibold text-foreground">
                    {{ session.title }}
                  </p>
                  <div class="flex items-center gap-2">
                    <span
                      v-if="session.unread > 0 && session.id !== activeSessionId"
                      class="inline-flex min-w-5 items-center justify-center rounded-full bg-primary px-1.5 py-0.5 text-[10px] font-semibold text-primary-foreground"
                    >
                      {{ session.unread > 9 ? '9+' : session.unread }}
                    </span>
                    <StatusBadge :label="session.status" :tone="statusTone(session.status)" :dot="true" />
                  </div>
                </div>
                <p class="line-clamp-2 text-xs leading-5 text-muted-foreground">
                  {{ session.summary }}
                </p>
                <div class="flex items-center justify-between text-[11px] uppercase tracking-[0.18em] text-muted-foreground">
                  <span>{{ session.targetSummary }}</span>
                  <span>{{ formatRelativeMinutes(session.updatedAt) }}</span>
                </div>
              </div>
            </button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              class="mt-0.5 size-8 shrink-0 rounded-full text-muted-foreground hover:text-foreground"
              :aria-label="t('console.sessions.deleteSession')"
              @click.stop="emit('delete', session.id)"
            >
              <Trash2 class="size-4" />
            </Button>
          </div>
        </div>
      </ScrollArea>
      <div class="pointer-events-none absolute inset-x-0 bottom-0 h-14 rounded-b-[0.9rem] bg-gradient-to-t from-brand-panel via-brand-panel/80 to-transparent" />
    </div>
  </div>
</template>
