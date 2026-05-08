<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import ThinkingBlock from './ThinkingBlock.vue'
import ContentBlock from './ContentBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import type { MessageItem } from '@/types/api'

defineProps<{
  message: MessageItem
  deletable?: boolean
}>()

const emit = defineEmits<{
  (e: 'delete'): void
}>()
</script>

<template>
  <div class="group relative flex flex-col gap-2.5">
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
    <button
      v-if="deletable"
      type="button"
      class="absolute -bottom-7 left-0 hidden h-6 w-6 items-center justify-center rounded transition-colors hover:bg-[var(--secondary)] group-hover:flex"
      style="color: var(--muted-foreground)"
      :title="$t('chat.deleteMessage')"
      :aria-label="$t('chat.deleteMessage')"
      @click="emit('delete')"
    >
      <Trash2 class="h-3.5 w-3.5" />
    </button>
  </div>
</template>
