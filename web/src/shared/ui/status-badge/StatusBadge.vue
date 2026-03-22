<script setup lang="ts">
import { computed } from 'vue'

import { Badge } from '@/components/ui/badge'

const props = withDefaults(
  defineProps<{
    label: string
    tone?: 'neutral' | 'info' | 'success' | 'warning' | 'danger'
    dot?: boolean
  }>(),
  {
    tone: 'neutral',
    dot: false,
  },
)

const toneClass = computed(() => {
  switch (props.tone) {
    case 'info':
      return 'border-brand-info/30 bg-brand-info/10 text-brand-info'
    case 'success':
      return 'border-brand-success/30 bg-brand-success/10 text-brand-success'
    case 'warning':
      return 'border-brand-warning/30 bg-brand-warning/10 text-amber-800'
    case 'danger':
      return 'border-brand-danger/30 bg-brand-danger/10 text-brand-danger'
    default:
      return 'border-border bg-background/80 text-muted-foreground'
  }
})
</script>

<template>
  <Badge
    variant="outline"
    class="inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-[10px] font-medium tracking-[0.14em] uppercase"
    :class="toneClass"
  >
    <span v-if="dot" class="size-1.5 rounded-full bg-current" />
    {{ label }}
  </Badge>
</template>
