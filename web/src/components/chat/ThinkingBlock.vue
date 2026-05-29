<script setup lang="ts">
import { ref } from 'vue'
import { Brain, ChevronRight } from 'lucide-vue-next'

defineProps<{
  reasoning: string
  /** Reasoning is still being streamed — show the in-progress label + pulse. */
  live?: boolean
}>()

const isOpen = ref(false)
</script>

<template>
  <div>
    <button
      type="button"
      class="inline-flex w-fit cursor-pointer items-center gap-1.5 text-[11px] transition-opacity hover:opacity-100"
      :class="isOpen ? 'opacity-100' : 'opacity-70'"
      style="color: var(--muted-foreground)"
      @click="isOpen = !isOpen"
    >
      <Brain class="h-3 w-3" :class="{ 'animate-pulse': live }" />
      <span :class="{ 'animate-pulse': live }">
        {{ live ? $t('chat.thinking') : $t('chat.thoughtForAMoment') }}
      </span>
      <ChevronRight
        class="h-3 w-3 transition-transform"
        :class="{ 'rotate-90': isOpen }"
      />
    </button>
    <div
      v-if="isOpen"
      class="mt-1.5 whitespace-pre-wrap p-3 text-xs italic leading-relaxed"
      style="border-left: 2px solid var(--secondary); color: var(--muted-foreground)"
    >
      {{ reasoning }}
    </div>
  </div>
</template>
