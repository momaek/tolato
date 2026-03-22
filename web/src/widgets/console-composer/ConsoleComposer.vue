<script setup lang="ts">
import { ref } from 'vue'

import ComposerBox from '@/features/console-composer/ComposerBox.vue'

const props = defineProps<{
  disabled: boolean
  initialText?: string
}>()

const emit = defineEmits<{
  submit: [text: string]
}>()

const composerRef = ref<InstanceType<typeof ComposerBox> | null>(null)
</script>

<template>
  <div class="glass-panel rounded-[1rem] border border-white/65 px-4 py-3 shadow-[0_20px_45px_rgba(58,40,18,0.08)]">
    <ComposerBox ref="composerRef" :disabled="props.disabled" :initial-text="props.initialText" @submit="emit('submit', $event)" />
  </div>
</template>
