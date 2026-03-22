<script setup lang="ts">
import { computed } from 'vue'

import type { ToolResultMetaRow as Row } from '@/shared/types/console'

const props = defineProps<{
  row: Row
}>()

const toneClass = computed(() => {
  switch (props.row.tone) {
    case 'success':
      return 'border-brand-success/25 bg-brand-success/10 text-brand-success'
    case 'warning':
      return 'border-brand-warning/30 bg-brand-warning/10 text-amber-800'
    default:
      return 'border-border/60 bg-muted/55 text-foreground/75'
  }
})
</script>

<template>
  <div class="rounded-[0.85rem] border px-3.5 py-1.5 font-mono text-[12px] font-medium leading-4 shadow-sm shadow-black/[0.02]" :class="toneClass">
    <span class="whitespace-pre-wrap break-words">tool_result_meta · {{ row.label }}</span>
  </div>
</template>
