<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { PlanRow } from '@/shared/types/console'
import PlanDetailDialog from '@/features/console-plan-preview/PlanDetailDialog.vue'

const props = defineProps<{
  row: PlanRow
}>()

const { t } = useI18n()

const riskTone = computed(() => {
  switch (props.row.risk) {
    case 'low':
      return 'success'
    case 'high':
      return 'danger'
    default:
      return 'warning'
  }
})

const targetSourceLabel = computed(() => props.row.targetSource.replaceAll('_', ' '))
</script>

<template>
  <div class="rounded-[1rem] border border-border/70 bg-background px-4 py-4 shadow-sm shadow-black/[0.03]">
    <div class="flex items-start justify-between gap-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-muted-foreground">plan</p>
      <StatusBadge :label="t('common.metrics.risk', { value: row.risk })" :tone="riskTone" />
    </div>

    <div class="mt-3 flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h3 class="text-[1.4rem] font-semibold leading-[1.3] tracking-[-0.03em] text-foreground">
          {{ row.summary }}
        </h3>
      </div>
      <PlanDetailDialog :row="row" />
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-1.5">
      <StatusBadge :label="row.requiresApproval ? t('console.plan.approvalRequired') : t('console.plan.autoRunReady')" :tone="row.requiresApproval ? 'warning' : 'success'" />
      <StatusBadge :label="row.targetLabel" tone="info" />
      <StatusBadge :label="targetSourceLabel" />
    </div>

    <div class="mt-3 rounded-[0.9rem] border border-border/60 bg-muted/20 px-3.5 py-2.5">
      <p class="text-[11px] font-semibold uppercase tracking-[0.18em] text-muted-foreground">{{ t('console.plan.input') }}</p>
      <p class="mt-1 text-[15px] leading-6 text-foreground">{{ row.inputText }}</p>
      <p v-if="row.autoExecutionHint" class="mt-1 text-sm leading-6 text-muted-foreground">
        {{ row.autoExecutionHint }}
      </p>
    </div>

    <p class="mt-3 text-sm leading-6 text-muted-foreground">{{ row.impact }}</p>

    <div class="mt-3 space-y-1.5">
      <div
        v-for="(step, index) in row.steps"
        :key="`${row.id}-${step.action}`"
        class="rounded-[0.9rem] border border-border/60 bg-background px-3.5 py-2.5"
      >
        <div class="flex flex-wrap items-baseline gap-2">
          <p class="text-[11px] font-semibold tracking-[0.18em] text-muted-foreground">
            {{ String(index + 1).padStart(2, '0') }}
          </p>
          <p class="text-[15px] font-semibold leading-6 text-foreground">{{ step.action }}</p>
        </div>
        <p class="mt-0.5 text-sm leading-6 text-muted-foreground">{{ step.argsLabel }}</p>
      </div>
    </div>
  </div>
</template>
