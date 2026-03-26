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

    <div v-if="node.stdoutTail" class="rounded-[0.9rem] border border-border/60 bg-muted/20 px-4 py-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">stdout</p>
      <pre class="mt-1 whitespace-pre-wrap font-mono text-xs leading-5 text-muted-foreground">{{ node.stdoutTail }}</pre>
    </div>

    <div v-if="node.stderrTail" class="rounded-[0.9rem] border border-border/60 border-l-2 border-l-red-400 bg-muted/20 px-4 py-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-red-500">stderr</p>
      <pre class="mt-1 whitespace-pre-wrap font-mono text-xs leading-5 text-red-600/80">{{ node.stderrTail }}</pre>
    </div>

    <div v-if="!node.stdoutTail && !node.stderrTail" class="rounded-[0.9rem] border border-border/60 bg-muted/20 px-4 py-3">
      <p class="text-sm leading-6 text-muted-foreground">{{ t('common.states.waitingForOutput') }}</p>
    </div>

    <p
      v-if="node.exitCode != null"
      class="text-xs font-medium"
      :class="node.exitCode === 0 ? 'text-muted-foreground' : 'text-red-500'"
    >
      exit code: {{ node.exitCode }}
    </p>
  </div>
</template>
