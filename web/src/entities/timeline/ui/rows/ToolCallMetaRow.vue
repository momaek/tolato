<script setup lang="ts">
import type { ToolCallMetaRow as Row } from '@/shared/types/console'

defineProps<{
  row: Row
}>()
</script>

<template>
  <div class="rounded-[0.85rem] border border-border/60 bg-muted/55 px-3.5 py-1.5 font-mono text-[12px] font-medium leading-4 text-foreground/75 shadow-sm shadow-black/[0.02]">
    <span class="whitespace-pre-wrap break-words">tool_call_meta · {{ row.label }}</span>
  </div>
</template>
