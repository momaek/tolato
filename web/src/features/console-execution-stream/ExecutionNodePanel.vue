<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import type { ExecutionNodeState } from '@/shared/types/console'

defineProps<{
  node: ExecutionNodeState
}>()

const { t } = useI18n()
</script>

<template>
  <div class="space-y-2">
    <div class="flex flex-wrap items-center justify-between gap-3 rounded-[0.9rem] border border-border/60 bg-background px-4 py-3">
      <p class="text-base font-semibold leading-6 text-foreground">{{ node.label }}</p>
      <p class="text-sm font-medium leading-6 text-muted-foreground">{{ node.status }} · {{ node.region }}</p>
    </div>

    <div class="rounded-[0.9rem] border border-border/60 bg-muted/20 px-4 py-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">stdout</p>
      <p class="mt-1 text-sm leading-6 text-muted-foreground">
        {{ node.stdoutTail || node.stderrTail || t('common.states.waitingForOutput') }}
      </p>
    </div>
  </div>
</template>
