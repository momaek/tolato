<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import TurnRenderer from '@/entities/timeline/ui/turns/TurnRenderer.vue'
import type { Turn } from '@/shared/types/console'

const props = defineProps<{
  turns: Turn[]
  loading: boolean
}>()

const rootRef = ref<InstanceType<typeof ScrollArea> | null>(null)
const contentRef = ref<HTMLElement | null>(null)
let resizeObserver: ResizeObserver | null = null

function viewportElement() {
  return (rootRef.value?.$el as HTMLElement | undefined)?.querySelector(
    '[data-radix-scroll-area-viewport]',
  ) as HTMLElement | null
}

function scrollToBottom() {
  const viewport = viewportElement()
  if (!(viewport instanceof HTMLElement)) {
    return
  }
  viewport.scrollTop = viewport.scrollHeight
}

async function syncScrollToBottom() {
  await nextTick()
  scrollToBottom()
  requestAnimationFrame(() => {
    scrollToBottom()
  })
}

onMounted(() => {
  resizeObserver = new ResizeObserver(() => {
    void syncScrollToBottom()
  })

  if (contentRef.value) {
    resizeObserver.observe(contentRef.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})

watch(
  () => {
    const last = props.turns[props.turns.length - 1]
    return [
      props.loading,
      props.turns.length,
      last?.type === 'assistant' ? last.blocks.length : 0,
      last?.type === 'assistant' ? last.status : '',
    ]
  },
  () => {
    void syncScrollToBottom()
  },
)
</script>

<template>
  <ScrollArea ref="rootRef" class="h-full min-h-0 rounded-[1rem] bg-brand-panel/65 p-3">
    <div v-if="loading" class="space-y-4">
      <Skeleton v-for="index in 4" :key="index" class="h-24 rounded-[0.9rem]" />
    </div>
    <div v-else ref="contentRef" class="space-y-2">
      <TurnRenderer
        v-for="turn in turns"
        :key="turn.id"
        :turn="turn"
      />
    </div>
  </ScrollArea>
</template>
