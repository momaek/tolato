<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'

defineProps<{
  content: string
  deletable?: boolean
}>()

const emit = defineEmits<{
  (e: 'delete'): void
}>()
</script>

<template>
  <div class="group flex items-center justify-end gap-1.5">
    <button
      v-if="deletable"
      type="button"
      class="hidden h-6 w-6 items-center justify-center rounded transition-colors hover:bg-[var(--secondary)] group-hover:flex"
      style="color: var(--muted-foreground)"
      :title="$t('chat.deleteMessage')"
      :aria-label="$t('chat.deleteMessage')"
      @click="emit('delete')"
    >
      <Trash2 class="h-3.5 w-3.5" />
    </button>
    <div
      class="max-w-[70%] whitespace-pre-wrap rounded-[999px] px-4 py-2.5 text-sm leading-relaxed"
      style="background-color: var(--secondary); color: var(--foreground)"
    >
      {{ content }}
    </div>
  </div>
</template>
