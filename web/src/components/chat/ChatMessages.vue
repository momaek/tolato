<script setup lang="ts">
import { ref } from 'vue'
import { ArrowDown } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import UserMessage from './UserMessage.vue'
import AssistantMessage from './AssistantMessage.vue'
import ThinkingBlock from './ThinkingBlock.vue'
import ContentBlock from './ContentBlock.vue'
import ToolCallCard from './ToolCallCard.vue'
import ConfirmCard from './ConfirmCard.vue'
import StreamingIndicator from './StreamingIndicator.vue'
import EmptyState from './EmptyState.vue'
import { useAutoScroll } from '@/composables/useAutoScroll'
import type { MessageItem } from '@/types/api'
import type { StreamingAssistant, ConfirmRequest, ConversationStatus } from '@/stores/chat'

defineProps<{
  messages: MessageItem[]
  streaming: StreamingAssistant | null
  status: ConversationStatus
  confirmRequest: ConfirmRequest | null
}>()

const emit = defineEmits<{
  (e: 'quick-action', text: string): void
  (e: 'confirm', id: string, approved: boolean): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const { showScrollButton, scrollToBottom } = useAutoScroll(containerRef)
</script>

<template>
  <div ref="containerRef" class="flex flex-1 flex-col overflow-y-auto">
    <EmptyState
      v-if="messages.length === 0 && !streaming"
      @quick-action="emit('quick-action', $event)"
    />

    <template v-for="msg in messages" :key="msg.id">
      <UserMessage v-if="msg.role === 'user'" :content="msg.content || ''" />
      <AssistantMessage v-else-if="msg.role === 'assistant'" :message="msg" />
    </template>

    <!-- Streaming content -->
    <div v-if="streaming" class="px-5 py-3">
      <ThinkingBlock v-if="streaming.reasoning" :reasoning="streaming.reasoning" />
      <ContentBlock v-if="streaming.content" :content="streaming.content" />
      <ToolCallCard
        v-for="tc in streaming.toolCalls"
        :key="tc.id"
        :tool-call="tc"
      />
    </div>

    <!-- Confirm card -->
    <div v-if="confirmRequest" class="px-5">
      <ConfirmCard
        :id="confirmRequest.id"
        :tool="confirmRequest.tool"
        :args="confirmRequest.args"
        @confirm="(id, approved) => emit('confirm', id, approved)"
      />
    </div>

    <!-- Streaming indicator -->
    <StreamingIndicator
      v-if="status === 'streaming' && !streaming?.content && !streaming?.reasoning"
    />

    <!-- Scroll to bottom button -->
    <Button
      v-if="showScrollButton"
      size="icon"
      variant="secondary"
      class="fixed bottom-24 right-8 z-10 h-8 w-8 rounded-full shadow-lg"
      @click="scrollToBottom"
    >
      <ArrowDown class="h-4 w-4" />
    </Button>
  </div>
</template>
