<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { useConnectionStore } from '@/entities/session/model/connection.store'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { SessionSnapshot } from '@/shared/types/console'
import { formatRelativeMinutes } from '@/shared/lib/format'

const props = defineProps<{
  snapshot: SessionSnapshot | null
}>()

const connectionStore = useConnectionStore()
const { t } = useI18n()

const healthSummary = computed(() => {
  if (!props.snapshot) {
    return t('console.header.healthSummary', { online: 0, offline: 0 })
  }

  return t('console.header.healthSummary', {
    online: props.snapshot.nodeHealthSummary.online + props.snapshot.nodeHealthSummary.busy,
    offline: props.snapshot.nodeHealthSummary.offline,
  })
})
</script>

<template>
  <div class="glass-panel rounded-[1rem] border border-white/60 p-5">
    <div class="flex flex-col gap-4 xl:flex-row xl:items-center xl:justify-between">
      <div class="space-y-3">
        <div>
          <div class="mt-1 flex flex-wrap items-center gap-3">
            <h1 class="text-2xl font-semibold tracking-tight text-foreground">
              {{ snapshot?.title ?? t('console.header.targetUnset') }}
            </h1>
            <p class="text-sm text-muted-foreground">
              {{ t('common.connection.syncedRecently', { value: connectionStore.lastSyncedAt ? formatRelativeMinutes(connectionStore.lastSyncedAt) : t('common.labels.noSync') }) }}
            </p>
            <StatusBadge :label="healthSummary" />
          </div>
        </div>
      </div>

      <div class="flex flex-col items-start gap-3 xl:items-end">
        <div class="flex items-center gap-2">
          <Button size="sm" class="rounded-lg px-4">{{ t('console.header.agent') }}</Button>
          <Dialog>
            <DialogTrigger as-child>
              <Button size="sm" variant="outline" class="rounded-lg px-4">{{ t('console.header.directShell') }}</Button>
            </DialogTrigger>
            <DialogContent class="max-w-lg rounded-[1rem]">
              <DialogHeader>
                <DialogTitle>{{ t('console.header.directShellTitle') }}</DialogTitle>
                <DialogDescription>{{ t('console.header.directShellDescription') }}</DialogDescription>
              </DialogHeader>
            </DialogContent>
          </Dialog>
        </div>
        <p class="max-w-sm text-right text-sm leading-6 text-muted-foreground">
          {{ t('console.header.plannerHint') }}
        </p>
      </div>
    </div>
  </div>
</template>
