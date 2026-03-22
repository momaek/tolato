<script setup lang="ts">
import { computed } from 'vue'

import type { LlmStreamState } from '@/shared/types/console'

const props = defineProps<{
  state: LlmStreamState
}>()

const streamStatus = computed(() => (props.state.status === 'completed' ? 'completed' : 'streaming'))
const isStreaming = computed(() => props.state.status === 'streaming')
</script>

<template>
  <div class="rounded-[0.95rem] border border-border/60 bg-background/95 px-4 py-3 shadow-sm shadow-black/[0.03]">
    <div class="flex flex-wrap items-center justify-between gap-2">
      <div class="flex items-center gap-2">
        <span v-if="isStreaming" class="size-2 rounded-full bg-foreground/65 animate-pulse" />
        <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">assistant_stream</p>
      </div>
      <span class="rounded-md bg-muted px-2 py-1 text-[11px] font-semibold tracking-[0.12em] text-muted-foreground">
        {{ streamStatus }}
      </span>
    </div>

    <div v-if="state.contentText" class="mt-2">
      <p class="whitespace-pre-wrap text-[15px] leading-7 text-foreground">
        {{ state.contentText }}<span v-if="isStreaming" class="ml-0.5 inline-block h-[1em] w-[0.5ch] translate-y-[2px] animate-pulse rounded-[1px] bg-foreground/70 align-baseline" />
      </p>
    </div>

    <details v-if="state.reasoningText" class="mt-3 rounded-[0.8rem] border border-border/50 bg-muted/35 px-3 py-2">
      <summary class="cursor-pointer list-none text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">
        thinking
      </summary>
      <p class="mt-2 whitespace-pre-wrap text-xs leading-5 text-muted-foreground">{{ state.reasoningText }}</p>
    </details>
  </div>
</template>
