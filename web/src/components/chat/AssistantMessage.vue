<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import ThinkingBlock from './ThinkingBlock.vue'
import ContentBlock from './ContentBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import { useRelativeTime } from '@/composables/useRelativeTime'
import type { MessageItem } from '@/types/api'

const props = defineProps<{
  message: MessageItem
  showMeta?: boolean
  deletable?: boolean
  /** This turn's reasoning is still streaming in. */
  reasoningLive?: boolean
}>()

const emit = defineEmits<{
  (e: 'delete'): void
}>()

const timeAgo = useRelativeTime(() => props.message.created_at)
</script>

<template>
  <div class="group flex flex-col gap-2.5">
    <ThinkingBlock v-if="message.reasoning" :reasoning="message.reasoning" :live="reasoningLive" />
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
    <div v-if="showMeta" class="-mt-1 flex h-6 items-center gap-1.5">
      <span
        v-if="timeAgo"
        class="text-xs tabular-nums"
        style="color: var(--muted-foreground)"
      >
        {{ timeAgo }}
      </span>
      <button
        v-if="deletable"
        type="button"
        class="flex h-6 w-6 items-center justify-center rounded opacity-0 transition hover:bg-[var(--secondary)] group-hover:opacity-100 focus-visible:opacity-100"
        style="color: var(--muted-foreground)"
        :title="$t('chat.deleteMessage')"
        :aria-label="$t('chat.deleteMessage')"
        @click="emit('delete')"
      >
        <Trash2 class="h-3.5 w-3.5" />
      </button>
    </div>
  </div>
</template>
