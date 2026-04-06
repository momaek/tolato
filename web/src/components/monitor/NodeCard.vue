<script setup lang="ts">
import type { ProbeNode } from '@/stores/monitor'

defineProps<{
  node: ProbeNode
}>()

function statusColor(status: string) {
  if (status === 'online') return '#4ade80'
  return '#ef4444'
}
</script>

<template>
  <div
    class="flex items-center gap-2 rounded-lg border px-3 py-2 text-xs select-none cursor-move"
    style="background-color: var(--card); border-color: var(--border)"
  >
    <div
      class="h-2 w-2 rounded-full"
      :style="{ backgroundColor: statusColor(node.status) }"
    />
    <span class="font-medium" style="color: var(--foreground)">
      {{ node.alias || node.name }}
    </span>
    <span
      v-if="node.role"
      class="rounded px-1 py-0.5 text-[10px]"
      style="background-color: var(--secondary); color: var(--muted-foreground)"
    >
      {{ node.role }}
    </span>
  </div>
</template>
