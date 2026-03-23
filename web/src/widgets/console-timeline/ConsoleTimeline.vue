<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { ScrollArea } from '@/components/ui/scroll-area'
import { Skeleton } from '@/components/ui/skeleton'
import LlmStreamRow from '@/entities/timeline/ui/rows/LlmStreamRow.vue'
import TimelineRowRenderer from '@/features/timeline-row-renderer/TimelineRowRenderer.vue'
import type { ApprovalRow, LlmStreamState, TargetCandidate, TimelineRow } from '@/shared/types/console'

const props = defineProps<{
  rows: TimelineRow[]
  loading: boolean
  llmStreamState?: LlmStreamState | null
}>()

const emit = defineEmits<{
  confirmTarget: [candidate: TargetCandidate]
  reselectTarget: []
  clearTarget: []
  approvalAction: [action: 'approve' | 'reject' | 'cancel', row: ApprovalRow]
}>()

function handleApprovalAction(action: 'approve' | 'reject' | 'cancel', row: ApprovalRow) {
  emit('approvalAction', action, row)
}

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
  () => [
    props.loading,
    props.rows.map((row) => row.id).join('|'),
    props.llmStreamState?.status ?? '',
    props.llmStreamState?.contentText ?? '',
    props.llmStreamState?.reasoningText ?? '',
  ],
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
      <TimelineRowRenderer
        v-for="row in rows"
        :key="row.id"
        :row="row"
        @confirm-target="emit('confirmTarget', $event)"
        @reselect-target="emit('reselectTarget')"
        @clear-target="emit('clearTarget')"
        @approval-action="handleApprovalAction"
      />
      <LlmStreamRow v-if="props.llmStreamState && (props.llmStreamState.reasoningText || props.llmStreamState.contentText)" :state="props.llmStreamState" />
    </div>
  </ScrollArea>
</template>
