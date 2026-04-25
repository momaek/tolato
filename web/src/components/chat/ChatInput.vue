<script setup lang="ts">
import { ref, computed, nextTick, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUp, Square } from 'lucide-vue-next'
import ChatComposerControls from './ChatComposerControls.vue'
import type { ConversationStatus } from '@/stores/chat'

const TEXTAREA_MAX_HEIGHT = 320

const props = defineProps<{
  status: ConversationStatus
  conversationId?: string
  model: string
  defaultNodeId?: string
}>()

const emit = defineEmits<{
  (e: 'send', content: string): void
  (e: 'stop'): void
  (e: 'update:model', value: string): void
  (e: 'update:defaultNodeId', value: string | undefined): void
}>()

const { t } = useI18n()
const input = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)

function autoResize() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  const next = Math.min(el.scrollHeight, TEXTAREA_MAX_HEIGHT)
  el.style.height = next + 'px'
  el.style.overflowY = el.scrollHeight > TEXTAREA_MAX_HEIGHT ? 'auto' : 'hidden'
}

watch(input, () => {
  nextTick(autoResize)
})
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
  <div class="px-5 py-4 md:px-6">
    <div class="mx-auto w-full max-w-[clamp(720px,58vw,1400px)]">
      <div
        class="chat-composer-shell flex flex-col rounded-[20px] px-2 pb-2 pt-1 transition-shadow focus-within:shadow-sm"
        :style="{
          backgroundColor: 'var(--secondary)',
          border: '1px solid color-mix(in oklab, var(--border) 60%, transparent)',
        }"
      >
        <textarea
          ref="textareaRef"
          v-model="input"
          :placeholder="placeholder"
          :disabled="isDisabled"
          :rows="1"
          class="chat-composer w-full resize-none bg-transparent px-3 pt-2.5 text-sm leading-relaxed outline-none disabled:cursor-not-allowed disabled:opacity-60"
          style="color: var(--foreground); border: none; overflow-y: hidden"
          @keydown="onKeydown"
          @compositionstart="isComposing = true"
          @compositionend="isComposing = false"
        />

        <div class="flex items-center justify-between gap-2 pt-1.5">
          <ChatComposerControls
            :conversation-id="conversationId"
            :model="model"
            :default-node-id="defaultNodeId"
            @update:model="(v) => emit('update:model', v)"
            @update:default-node-id="(v) => emit('update:defaultNodeId', v)"
          />

          <button
            v-if="isStreaming"
            type="button"
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-opacity hover:opacity-90"
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
            class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full transition-opacity hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-40"
            :style="{
              backgroundColor: 'var(--primary)',
              color: 'var(--primary-foreground)',
            }"
            :disabled="isDisabled || !input.trim()"
            @click="handleSend"
          >
            <ArrowUp class="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat-composer::placeholder {
  color: var(--muted-foreground);
  opacity: 0.8;
}
.chat-composer-shell:focus-within {
  border-color: color-mix(in oklab, var(--foreground) 18%, transparent);
}
</style>
