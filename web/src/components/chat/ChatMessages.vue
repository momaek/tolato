<script setup lang="ts">
import { ref, computed } from 'vue'
import { ArrowDown, AlertCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import UserMessage from './UserMessage.vue'
import AssistantMessage from './AssistantMessage.vue'
import ConfirmCard from './ConfirmCard.vue'
import StreamingIndicator from './StreamingIndicator.vue'
import EmptyState from './EmptyState.vue'
import { useAutoScroll } from '@/composables/useAutoScroll'
import type { MessageItem } from '@/types/api'
import type { StreamingAssistant, ConfirmRequest, ConversationStatus } from '@/stores/chat'

const props = defineProps<{
  messages: MessageItem[]
  streaming: StreamingAssistant | null
  status: ConversationStatus
  confirmRequest: ConfirmRequest | null
  error: string | null
}>()

const emit = defineEmits<{
  (e: 'quick-action', text: string): void
  (e: 'confirm', id: string, approved: boolean): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const { showScrollButton, scrollToBottom } = useAutoScroll(containerRef)

// Fold the streaming turn into the message list as a virtual "last message"
// that shares its id with the future finalized message. Rendering streaming
// and finalized states through the SAME keyed AssistantMessage lets Vue reuse
// the markstream-vue instance across the handoff — no re-parse, no flash.
const displayMessages = computed<MessageItem[]>(() => {
  if (!props.streaming) return props.messages
  return [
    ...props.messages,
    {
      id: props.streaming.id,
      role: 'assistant',
      reasoning: props.streaming.reasoning || undefined,
      segments: props.streaming.segments,
      created_at: new Date().toISOString(),
    },
  ]
})

const showEmptyIndicator = computed(() =>
  props.status === 'streaming' &&
  !props.streaming?.reasoning &&
  (props.streaming?.segments?.length ?? 0) === 0,
)
</script>

<template>
  <div ref="containerRef" class="flex flex-1 flex-col overflow-y-auto">
    <EmptyState
      v-if="messages.length === 0 && !streaming"
      @quick-action="emit('quick-action', $event)"
    />

    <div
      v-else
      class="mx-auto flex w-full flex-col gap-5 px-5 py-6 md:px-6 max-w-[clamp(720px,58vw,1400px)]"
    >
      <template v-for="msg in displayMessages" :key="msg.id">
        <UserMessage v-if="msg.role === 'user'" :content="msg.content || ''" />
        <AssistantMessage v-else-if="msg.role === 'assistant'" :message="msg" />
      </template>

      <!-- Confirm card -->
      <ConfirmCard
        v-if="confirmRequest"
        :id="confirmRequest.id"
        :tool="confirmRequest.tool"
        :args="confirmRequest.args"
        @confirm="(id, approved) => emit('confirm', id, approved)"
      />

      <!-- Streaming indicator: show only when nothing has arrived yet -->
      <StreamingIndicator v-if="showEmptyIndicator" />

      <!-- Error banner: LLM / tool / policy failures surfaced from the server.
           Persists after DONE so the user actually sees it; cleared on the
           next user message. -->
      <div
        v-if="error"
        class="flex items-start gap-2.5 rounded-lg border px-4 py-3 text-sm"
        style="background-color: color-mix(in oklab, var(--destructive) 12%, transparent); border-color: color-mix(in oklab, var(--destructive) 35%, transparent); color: var(--destructive-foreground, var(--foreground))"
        role="alert"
      >
        <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" style="color: var(--destructive)" />
        <div class="flex-1 whitespace-pre-wrap break-words leading-relaxed">
          {{ error }}
        </div>
      </div>
    </div>

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
