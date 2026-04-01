<script setup lang="ts">
import { computed } from 'vue'

import type { AssistantTurn } from '@/shared/types/console'

import ThinkingSection from './ThinkingSection.vue'
import TextSection from './TextSection.vue'
import ToolUseSection from './ToolUseSection.vue'

const props = defineProps<{
  turn: AssistantTurn
}>()

const isStreaming = computed(() => props.turn.status === 'streaming')

function isLastTextBlock(index: number) {
  for (let i = props.turn.blocks.length - 1; i >= 0; i--) {
    if (props.turn.blocks[i].type === 'text') {
      return i === index
    }
  }
  return false
}
</script>

<template>
  <div class="rounded-[0.95rem] border border-border/60 bg-background/95 px-4 py-3 shadow-sm shadow-black/[0.03]">
    <!-- Header: always show for streaming, show completed label otherwise -->
    <div class="mb-2 flex items-center gap-2">
      <span
        v-if="isStreaming"
        class="size-2 rounded-full bg-foreground/65 animate-pulse"
      />
      <span
        v-else
        class="size-2 rounded-full bg-muted-foreground/30"
      />
      <span class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">assistant</span>
    </div>

    <!-- Empty streaming state: waiting for first token -->
    <div v-if="isStreaming && turn.blocks.length === 0" class="flex items-center gap-2 py-1">
      <span class="size-1.5 animate-pulse rounded-full bg-muted-foreground/50" />
      <span class="text-[11px] text-muted-foreground">thinking...</span>
    </div>

    <!-- Content blocks -->
    <div class="space-y-3">
      <template v-for="(block, i) in turn.blocks" :key="i">
        <ThinkingSection
          v-if="block.type === 'thinking'"
          :block="block"
          :streaming="isStreaming"
        />
        <TextSection
          v-else-if="block.type === 'text'"
          :block="block"
          :streaming="isStreaming && isLastTextBlock(i)"
        />
        <ToolUseSection
          v-else-if="block.type === 'tool_use'"
          :block="block"
        />
      </template>
    </div>
  </div>
</template>
