<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import type { PlanRow } from '@/shared/types/console'

defineProps<{
  row: PlanRow
}>()

const { t } = useI18n()
</script>

<template>
  <Dialog>
    <DialogTrigger as-child>
      <Button variant="outline">{{ t('common.buttons.viewFullPlan') }}</Button>
    </DialogTrigger>
    <DialogContent class="max-w-3xl rounded-[1.5rem]">
      <DialogHeader>
        <DialogTitle>{{ t('console.plan.detailTitle') }}</DialogTitle>
        <DialogDescription>{{ row.summary }}</DialogDescription>
      </DialogHeader>
      <div class="space-y-3">
        <div
          v-for="(step, index) in row.steps"
          :key="`${row.id}-${step.action}`"
          class="rounded-2xl border border-border/70 bg-muted/50 p-4"
        >
          <p class="text-xs uppercase tracking-[0.24em] text-muted-foreground">{{ t('console.plan.step', { index: index + 1 }) }}</p>
          <p class="mt-2 text-sm font-semibold text-foreground">{{ step.action }}</p>
          <p class="mt-1 text-sm text-muted-foreground">{{ step.argsLabel }}</p>
          <div class="mt-3 flex gap-4 text-xs text-muted-foreground">
            <span>{{ t('common.metrics.risk', { value: step.risk }) }}</span>
            <span>{{ t('common.metrics.timeout', { value: step.timeoutSec }) }}</span>
            <span>{{ t('common.metrics.broadcast', { value: step.broadcastAllowed ? t('common.values.yes') : t('common.values.no') }) }}</span>
          </div>
        </div>
      </div>
    </DialogContent>
  </Dialog>
</template>
