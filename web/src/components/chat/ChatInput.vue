<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Send, Square } from 'lucide-vue-next'
import type { ConversationStatus } from '@/stores/chat'

const props = defineProps<{
  status: ConversationStatus
}>()

const emit = defineEmits<{
  (e: 'send', content: string): void
  (e: 'stop'): void
}>()

const { t } = useI18n()
const input = ref('')
// Tracks whether an IME composition (Chinese/Japanese/Korean) is in progress.
// While true, Enter is the IME's "commit candidate" key and must NOT trigger
// send — otherwise we'd either fire with half-composed text or (depending on
// the browser) block the IME commit.
const isComposing = ref(false)

const isDisabled = computed(() => ['streaming', 'tool_exec', 'confirming'].includes(props.status))
const isStreaming = computed(() => ['streaming', 'tool_exec'].includes(props.status))

const placeholderMap: Record<ConversationStatus, string> = {
  idle: 'chat.placeholder.idle',
  streaming: 'chat.placeholder.streaming',
  tool_exec: 'chat.placeholder.toolExec',
  confirming: 'chat.placeholder.confirming',
  error: 'chat.placeholder.error',
}

const placeholder = computed(() => t(placeholderMap[props.status]))

function handleSend() {
  const content = input.value.trim()
  if (!content) return
  emit('send', content)
  input.value = ''
}

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey) return
  // keyCode 229 is the legacy "IME in progress" signal; `isComposing` is the
  // modern one. Belt-and-braces — different browsers are inconsistent.
  if (event.isComposing || event.keyCode === 229 || isComposing.value) return
  event.preventDefault()
  if (!isDisabled.value) {
    handleSend()
  }
}

function fillInput(text: string) {
  input.value = text
}

defineExpose({ fillInput })
</script>

<template>
  <div class="px-5 py-4" style="border-top: 1px solid var(--border)">
    <div class="relative mx-auto w-full max-w-[780px]">
      <textarea
        v-model="input"
        :placeholder="placeholder"
        :disabled="isDisabled"
        :rows="1"
        class="chat-composer w-full resize-none rounded-[12px] py-3.5 pl-4 pr-14 text-sm leading-relaxed outline-none disabled:cursor-not-allowed disabled:opacity-60"
        style="background-color: var(--secondary); color: var(--foreground); min-height: 52px; border: none"
        @keydown="onKeydown"
        @compositionstart="isComposing = true"
        @compositionend="isComposing = false"
      />
      <button
        v-if="isStreaming"
        type="button"
        class="absolute bottom-2.5 right-2.5 flex h-8 w-8 items-center justify-center rounded-full transition-opacity hover:opacity-90"
        :style="{
          backgroundColor: 'var(--destructive)',
          color: 'var(--destructive-foreground)',
        }"
        @click="emit('stop')"
      >
        <Square class="h-3 w-3" fill="currentColor" />
      </button>
      <button
        v-else
        type="button"
        class="absolute bottom-2.5 right-2.5 flex h-8 w-8 items-center justify-center rounded-full transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
        :style="{
          backgroundColor: 'var(--primary)',
          color: 'var(--primary-foreground)',
        }"
        :disabled="isDisabled || !input.trim()"
        @click="handleSend"
      >
        <Send class="h-4 w-4" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.chat-composer::placeholder {
  color: var(--muted-foreground);
  opacity: 0.8;
}
</style>
