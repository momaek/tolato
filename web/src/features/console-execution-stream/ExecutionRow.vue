<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { ExecutionRow as Row } from '@/shared/types/console'
import ExecutionNodePanel from '@/features/console-execution-stream/ExecutionNodePanel.vue'

defineProps<{
  row: Row
}>()

const { t } = useI18n()
</script>

<template>
  <div class="rounded-[1rem] border border-border/70 bg-background px-5 py-5 shadow-sm shadow-black/[0.03]">
    <div class="flex items-start justify-between gap-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">execution</p>
      <p class="text-sm font-semibold leading-6 text-muted-foreground">{{ row.status }}</p>
    </div>

    <div class="mt-4 flex flex-wrap items-center gap-2">
      <StatusBadge :label="row.status" :tone="row.status === 'running' ? 'info' : row.status === 'success' ? 'success' : 'neutral'" :dot="true" />
      <StatusBadge :label="t('common.metrics.nodes', { count: row.nodes.length })" />
    </div>

    <h3 class="mt-4 text-[1.6rem] font-semibold leading-tight tracking-[-0.03em] text-foreground">
      {{ row.title }}
    </h3>

    <div class="mt-4 space-y-3">
      <ExecutionNodePanel v-for="node in row.nodes" :key="node.nodeId" :node="node" />
    </div>
  </div>
</template>
