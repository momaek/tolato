<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui/button'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import type { ApprovalRow as Row } from '@/shared/types/console'

const props = defineProps<{
  row: Row
}>()

const emit = defineEmits<{
  action: [action: 'approve' | 'reject' | 'cancel']
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
</script>

<template>
  <div class="rounded-[1rem] border border-border/70 bg-background px-4 py-4 shadow-sm shadow-black/[0.03]">
    <div class="flex items-start justify-between gap-3">
      <p class="text-[11px] font-semibold tracking-[0.14em] text-amber-800">approval</p>
      <p class="text-sm font-medium leading-6 text-muted-foreground">{{ row.impact }}</p>
    </div>

    <div class="mt-3 flex flex-wrap items-center gap-1.5">
      <StatusBadge :label="t('common.metrics.risk', { value: row.risk })" :tone="riskTone" />
      <StatusBadge :label="row.targetLabel" tone="info" />
    </div>

    <h3 class="mt-3 text-[1.4rem] font-semibold leading-[1.3] tracking-[-0.03em] text-foreground">
      {{ t('console.approval.required') }}
    </h3>

    <p class="mt-2.5 text-[15px] leading-6 text-muted-foreground">{{ row.reason }}</p>

    <div class="mt-4 flex flex-wrap gap-2">
      <Button class="h-10 rounded-lg px-4" @click="emit('action', 'approve')">{{ t('common.buttons.approve') }}</Button>
      <Button variant="outline" class="h-10 rounded-lg px-4 border-brand-danger/30 text-brand-danger" @click="emit('action', 'reject')">
          {{ t('common.buttons.reject') }}
        </Button>
      <Button variant="outline" class="h-10 rounded-lg px-4" @click="emit('action', 'cancel')">{{ t('common.buttons.cancel') }}</Button>
    </div>
  </div>
</template>
