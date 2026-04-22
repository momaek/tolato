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
  <div class="flex flex-col gap-2.5">
    <ThinkingBlock v-if="message.reasoning" :reasoning="message.reasoning" />
    <!-- If segments are present (from streaming), render in chronological order. -->
    <template v-if="message.segments && message.segments.length">
      <template v-for="(seg, i) in message.segments" :key="i">
        <ContentBlock
          v-if="seg.type === 'content' && seg.text"
          :content="seg.text"
        />
        <ToolCallCard
          v-else-if="seg.type === 'tool_call'"
          :tool-call="seg.toolCall"
        />
      </template>
    </template>
    <!-- Fallback for DB-loaded messages: one round per message, content then tools. -->
    <template v-else>
      <ContentBlock v-if="message.content" :content="message.content" />
      <ToolCallCard
        v-for="tc in message.tool_calls"
        :key="tc.id"
        :tool-call="tc"
      />
    </template>
  </div>
</template>
