<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Send, Square } from 'lucide-vue-next'
import { Textarea } from '@/components/ui/textarea'
import { Button } from '@/components/ui/button'
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

const isDisabled = computed(() => ['streaming', 'tool_exec', 'confirming'].includes(props.status))

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
  if (event.key === 'Enter' && !event.shiftKey) {
    event.preventDefault()
    if (!isDisabled) {
      handleSend()
    }
  }
}

function fillInput(text: string) {
  input.value = text
}

defineExpose({ fillInput })
</script>

<template>
  <div class="px-5 py-4">
    <div class="relative">
      <Textarea
        v-model="input"
        :placeholder="placeholder"
        :disabled="isDisabled"
        class="min-h-[52px] resize-none pr-14"
        :rows="1"
        @keydown="onKeydown"
      />
      <Button
        v-if="status === 'streaming' || status === 'tool_exec'"
        size="icon"
        variant="destructive"
        class="absolute bottom-2 right-2 h-8 w-8 rounded-full"
        @click="emit('stop')"
      >
        <Square class="h-3 w-3" />
      </Button>
      <Button
        v-else
        size="icon"
        class="absolute bottom-2 right-2 h-8 w-8 rounded-full"
        :disabled="isDisabled || !input.trim()"
        @click="handleSend"
      >
        <Send class="h-4 w-4" />
      </Button>
    </div>
  </div>
</template>
