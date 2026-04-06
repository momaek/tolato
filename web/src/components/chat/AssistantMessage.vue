<script setup lang="ts">
import ThinkingBlock from './ThinkingBlock.vue'
import ContentBlock from './ContentBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import type { MessageItem } from '@/types/api'

defineProps<{
  message: MessageItem
}>()
</script>

<template>
  <div class="px-5 py-3">
    <ThinkingBlock v-if="message.reasoning" :reasoning="message.reasoning" />
    <ContentBlock v-if="message.content" :content="message.content" />
    <ToolCallCard
      v-for="tc in message.tool_calls"
      :key="tc.id"
      :tool-call="tc"
    />
  </div>
</template>
