<script setup lang="ts">
import { computed } from 'vue'

import type { ToolUseBlock } from '@/shared/types/console'

const props = defineProps<{
  block: ToolUseBlock
}>()

const isExecuting = computed(() => !props.block.result)

const resultToneClass = computed(() => {
  if (!props.block.result) return ''
  switch (props.block.result.tone) {
    case 'success':
      return 'border-brand-success/25 bg-brand-success/10'
    case 'warning':
      return 'border-brand-warning/30 bg-brand-warning/10'
    default:
      return 'border-border/50 bg-muted/35'
  }
})

const resultTextClass = computed(() => {
  if (!props.block.result) return ''
  switch (props.block.result.tone) {
    case 'success':
      return 'text-brand-success'
    case 'warning':
      return 'text-amber-800'
    default:
      return 'text-muted-foreground'
  }
})
</script>

<template>
  <div class="rounded-[0.8rem] border border-border/50 bg-muted/35 px-3 py-2">
    <div class="flex items-center gap-2">
      <span
        v-if="isExecuting"
        class="size-2 rounded-full bg-foreground/50 animate-pulse"
      />
      <span
        v-else-if="block.result?.tone === 'success'"
        class="size-2 rounded-full bg-brand-success"
      />
      <span
        v-else-if="block.result?.tone === 'warning'"
        class="size-2 rounded-full bg-brand-warning"
      />
      <span v-else class="size-2 rounded-full bg-muted-foreground/40" />
      <span class="font-mono text-[12px] font-semibold text-foreground/80">{{ block.toolName }}</span>
    </div>

    <pre
      v-if="block.argsPreview"
      class="mt-2 max-h-40 overflow-auto whitespace-pre-wrap break-all rounded-[0.7rem] bg-background/70 px-3 py-2 font-mono text-[11px] leading-5 text-muted-foreground"
    >{{ block.argsPreview }}</pre>

    <div
      v-if="block.result"
      class="mt-2 rounded-[0.7rem] border px-3 py-1.5"
      :class="[resultToneClass]"
    >
      <span class="font-mono text-[11px] font-medium leading-4" :class="[resultTextClass]">
        {{ block.result.label }}
      </span>
    </div>
  </div>
</template>
