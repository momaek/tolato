<script setup lang="ts">
import { computed } from "vue"
import { Badge } from "@/shared/ui"
import { taskStatusClassMap, taskStatusLabelMap } from "@/shared/lib/task"
import type { TaskStatus } from "@/shared/api/contracts"

const props = defineProps<{
  status: TaskStatus
}>()

const badgeClass = computed(() => taskStatusClassMap[props.status])
const label = computed(() => taskStatusLabelMap[props.status])
</script>

<template>
  <Badge variant="outline" :class="badgeClass">
    {{ label }}
  </Badge>
</template>
