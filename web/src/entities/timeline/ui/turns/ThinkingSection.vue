<script setup lang="ts">
import { computed } from 'vue'

import type { ThinkingBlock } from '@/shared/types/console'

const props = defineProps<{
  block: ThinkingBlock
  streaming: boolean
}>()

const isThinkingActive = computed(() => props.streaming && props.block.text.length > 0)
</script>

<template>
  <details v-if="block.text" class="rounded-[0.8rem] border border-border/50 bg-muted/35 px-3 py-2">
    <summary class="cursor-pointer list-none text-[11px] font-semibold tracking-[0.14em] text-muted-foreground select-none">
      thinking
      <span v-if="isThinkingActive" class="ml-1 inline-block size-1.5 animate-pulse rounded-full bg-muted-foreground/50" />
    </summary>
    <p class="mt-2 max-h-80 overflow-auto whitespace-pre-wrap text-xs leading-5 text-muted-foreground">{{ block.text }}</p>
  </details>
</template>
