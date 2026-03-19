<script setup lang="ts">
import { computed } from "vue"
import { Badge } from "@/shared/ui"
import { riskLevelClassMap, riskLevelLabelMap } from "@/shared/lib/task"
import type { RiskLevel } from "@/shared/api/contracts"

const props = defineProps<{
  level: RiskLevel
}>()

const badgeClass = computed(() => riskLevelClassMap[props.level])
const label = computed(() => riskLevelLabelMap[props.level])
</script>

<template>
  <Badge variant="outline" :class="badgeClass">
    {{ label }}
  </Badge>
</template>
