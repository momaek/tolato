<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowUp } from 'lucide-vue-next'

import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'

const props = defineProps<{
  disabled: boolean
  initialText?: string
}>()

const emit = defineEmits<{
  submit: [text: string]
}>()

const text = ref('')
const { t } = useI18n()

watch(
  () => props.initialText,
  value => {
    if (typeof value === 'string' && value !== text.value) {
      text.value = value
    }
  },
  { immediate: true },
)

function handleSubmit() {
  if (props.disabled) {
    return
  }

  const value = text.value.trim()
  if (!value) {
    return
  }

  emit('submit', value)
  text.value = ''
}

function usePreset(value: string) {
  text.value = value
}

function handleKeydown(event: KeyboardEvent) {
  if (event.isComposing || event.key !== 'Enter') {
    return
  }

  if (event.shiftKey) {
    return
  }

  event.preventDefault()
  handleSubmit()
}

defineExpose({ usePreset })
</script>

<template>
  <div class="rounded-[0.95rem] border border-border/70 bg-background/82 p-3 shadow-[inset_0_1px_0_rgba(255,255,255,0.72)]">
    <div class="flex gap-3">
      <div class="min-w-0 flex-1">
        <Textarea
          v-model="text"
          :disabled="props.disabled"
          rows="2"
          class="min-h-16 max-h-28 resize-none border-0 bg-transparent px-0 py-0 text-[15px] leading-7 shadow-none placeholder:text-muted-foreground/70 focus-visible:ring-0"
          :placeholder="t('console.composer.placeholder')"
          @keydown="handleKeydown"
        />
      </div>

      <Button
        :disabled="props.disabled"
        class="mt-auto h-12 w-12 shrink-0 rounded-[0.8rem] px-0 shadow-[0_12px_28px_rgba(88,58,32,0.18)]"
        @click="handleSubmit"
      >
        <ArrowUp class="size-4" />
      </Button>
    </div>
  </div>
</template>
