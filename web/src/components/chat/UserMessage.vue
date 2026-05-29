<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import { useRelativeTime } from '@/composables/useRelativeTime'

const props = defineProps<{
  content: string
  createdAt?: string
  deletable?: boolean
}>()

const emit = defineEmits<{
  (e: 'delete'): void
}>()

const timeAgo = useRelativeTime(() => props.createdAt)
</script>

<template>
  <div class="group flex flex-col items-end gap-2.5">
    <div
      class="max-w-[70%] whitespace-pre-wrap rounded-2xl rounded-br-md px-4 py-2.5 text-sm leading-relaxed"
      style="background-color: var(--secondary); color: var(--foreground)"
    >
      {{ content }}
    </div>
    <div class="-mt-1 flex h-6 items-center justify-end gap-1.5">
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
      <span
        v-if="timeAgo"
        class="text-xs tabular-nums"
        style="color: var(--muted-foreground)"
      >
        {{ timeAgo }}
      </span>
    </div>
  </div>
</template>
