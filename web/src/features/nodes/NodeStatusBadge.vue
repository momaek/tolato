<script setup lang="ts">
import { computed } from "vue"
import { Badge } from "@/shared/ui"
import { nodeStatusClassMap, nodeStatusLabelMap } from "@/shared/lib/task"
import type { NodeStatus } from "@/shared/api/contracts"

const props = defineProps<{
  status: NodeStatus
  busy?: boolean
}>()

const label = computed(() => {
  if (props.busy && props.status === "online") {
    return "Busy"
  }

  return nodeStatusLabelMap[props.status]
})

const badgeClass = computed(() => {
  if (props.busy && props.status === "online") {
    return "bg-sky-100 text-sky-900 border-sky-200"
  }

  return nodeStatusClassMap[props.status]
})
</script>

<template>
  <Badge variant="outline" :class="badgeClass">
    {{ label }}
  </Badge>
</template>
