<script setup lang="ts">
import type { Turn } from '@/shared/types/console'

import UserTurnCard from './UserTurnCard.vue'
import AssistantTurnCard from './AssistantTurnCard.vue'

defineProps<{
  turn: Turn
}>()
</script>

<template>
  <UserTurnCard v-if="turn.type === 'user'" :turn="turn" />
  <AssistantTurnCard v-else-if="turn.type === 'assistant'" :turn="turn" />
</template>
