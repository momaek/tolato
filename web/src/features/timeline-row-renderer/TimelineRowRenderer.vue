<script setup lang="ts">
import type { ApprovalRow, TargetCandidate, TimelineRow } from '@/shared/types/console'
import ApprovalRowView from '@/features/console-approval/ApprovalRow.vue'
import ExecutionRowView from '@/features/console-execution-stream/ExecutionRow.vue'
import PlanPreviewRow from '@/features/console-plan-preview/PlanPreviewRow.vue'
import SummaryRowView from '@/features/console-summary/SummaryRow.vue'
import TargetConfirmationRow from '@/features/console-target-confirm/TargetConfirmationRow.vue'
import AssistantTextRow from '@/entities/timeline/ui/rows/AssistantTextRow.vue'
import ToolCallMetaRow from '@/entities/timeline/ui/rows/ToolCallMetaRow.vue'
import ToolResultMetaRow from '@/entities/timeline/ui/rows/ToolResultMetaRow.vue'
import UserMessageRow from '@/entities/timeline/ui/rows/UserMessageRow.vue'

defineProps<{
  row: TimelineRow
}>()

const emit = defineEmits<{
  confirmTarget: [candidate: TargetCandidate]
  reselectTarget: []
  clearTarget: []
  approvalAction: [action: 'approve' | 'reject' | 'cancel', row: ApprovalRow]
}>()
</script>

<template>
  <UserMessageRow v-if="row.kind === 'user_message'" :row="row" />
  <AssistantTextRow v-else-if="row.kind === 'assistant_text'" :row="row" />
  <ToolCallMetaRow v-else-if="row.kind === 'tool_call_meta'" :row="row" />
  <ToolResultMetaRow v-else-if="row.kind === 'tool_result_meta'" :row="row" />
  <TargetConfirmationRow
    v-else-if="row.kind === 'target_confirmation'"
    :row="row"
    @confirm="emit('confirmTarget', $event)"
    @reselect="emit('reselectTarget')"
    @clear="emit('clearTarget')"
  />
  <PlanPreviewRow v-else-if="row.kind === 'plan'" :row="row" />
  <ApprovalRowView
    v-else-if="row.kind === 'approval'"
    :row="row"
    @action="emit('approvalAction', $event, row)"
  />
  <ExecutionRowView v-else-if="row.kind === 'execution'" :row="row" />
  <SummaryRowView v-else-if="row.kind === 'summary'" :row="row" />
</template>
