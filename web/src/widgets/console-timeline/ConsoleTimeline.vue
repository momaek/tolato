<script setup lang="ts">
import { nextTick, ref, watch } from 'vue'

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

watch(
  () => props.rows.length,
  async () => {
    await nextTick()
    const viewport = (rootRef.value?.$el as HTMLElement | undefined)?.querySelector('[data-radix-scroll-area-viewport]')
    if (viewport instanceof HTMLElement) {
      viewport.scrollTop = viewport.scrollHeight
    }
  },
)
</script>

<template>
  <ScrollArea ref="rootRef" class="h-full min-h-0 rounded-[1rem] bg-brand-panel/65 p-3">
    <div v-if="loading" class="space-y-4">
      <Skeleton v-for="index in 4" :key="index" class="h-24 rounded-[0.9rem]" />
    </div>
    <div v-else class="space-y-2">
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
