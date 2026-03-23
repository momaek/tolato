<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import {
  formatPercent,
  formatRelativeMinutes,
  hasDisplayValue,
  joinDisplayParts,
  normalizePercentValue,
} from '@/shared/lib/format'
import type { NodeStatus, NodeSummary } from '@/shared/types/node'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import { cn } from '@/lib/utils'
import { ArrowRight, ExternalLink, Play } from 'lucide-vue-next'
import { RouterLink } from 'vue-router'

defineProps<{
  nodes: NodeSummary[]
}>()

const emit = defineEmits<{
  (e: 'open-console', nodeId: string): void
}>()

const { t } = useI18n()

function statusTone(status: NodeStatus) {
  if (status === 'offline') return 'bg-stone-400'
  if (status === 'stale') return 'bg-amber-500'
  if (status === 'busy') return 'bg-sky-500'
  return 'bg-emerald-500'
}

function statusBadgeTone(
  status: NodeStatus,
): 'success' | 'info' | 'warning' | 'danger' {
  if (status === 'offline') return 'danger'
  if (status === 'stale') return 'warning'
  if (status === 'busy') return 'info'
  return 'success'
}

function statusLabel(status: NodeStatus) {
  return t(`common.statuses.${status}`)
}

function metricWidth(value: number) {
  return `${normalizePercentValue(value)}%`
}

function regionLabel(node: NodeSummary) {
  return hasDisplayValue(node.region) ? node.region : ''
}

function summaryText(node: NodeSummary) {
  return joinDisplayParts([
    hasDisplayValue(node.os) ? node.os : null,
    hasDisplayValue(node.provider) ? node.provider : null,
    t('common.metrics.lastSeen', {
      value: formatRelativeMinutes(node.lastSeen),
    }),
  ])
}

function footerText(node: NodeSummary) {
  return joinDisplayParts([
    hasDisplayValue(node.ipAddress) ? node.ipAddress : null,
    hasDisplayValue(node.version)
      ? `${t('common.labels.agent')} ${node.version}`
      : null,
  ])
}
</script>

<template>
  <Card class="glass-panel border-border/70 overflow-hidden rounded-2xl">
    <CardHeader class="pb-3">
      <CardTitle class="text-lg">{{ t('nodesTable.title') }}</CardTitle>
    </CardHeader>

    <CardContent class="space-y-3">
      <div
        v-for="node in nodes"
        :key="node.id"
        class="rounded-2xl border border-border/70 bg-background/80 p-5 transition-colors hover:bg-background"
      >
        <div
          class="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between"
        >
          <div class="min-w-0 space-y-3">
            <div class="flex flex-wrap items-center gap-3">
              <div
                :class="cn('h-2.5 w-2.5 rounded-full', statusTone(node.status))"
              />
              <p class="text-lg font-semibold tracking-tight">
                {{ node.hostname }}
              </p>
              <StatusBadge
                :label="statusLabel(node.status)"
                :tone="statusBadgeTone(node.status)"
              />
              <StatusBadge
                v-if="node.busy && node.status !== 'busy'"
                :label="statusLabel('busy')"
                tone="info"
              />
              <Badge
                v-if="regionLabel(node)"
                variant="secondary"
                class="rounded-full px-3 py-1"
              >
                {{ regionLabel(node) }}
              </Badge>
            </div>

            <div class="text-sm text-muted-foreground">
              {{ summaryText(node) }}
            </div>

            <div v-if="node.tags.length" class="flex flex-wrap gap-2">
              <Badge v-for="tag in node.tags" :key="tag" variant="outline">{{
                tag
              }}</Badge>
            </div>
          </div>

          <div
            class="min-w-[280px] rounded-2xl border border-border/70 bg-background/70 p-4 xl:w-[340px]"
          >
            <div
              class="mb-4 text-xs font-medium tracking-[0.18em] text-muted-foreground uppercase"
            >
              {{ t('nodeOverview.resourceUsage') }}
            </div>
            <div class="grid gap-3 text-sm">
              <div class="flex items-center gap-3">
                <span class="w-16 text-muted-foreground">{{
                  t('nodesTable.cpu')
                }}</span>
                <div class="h-2 flex-1 rounded-full bg-muted">
                  <div
                    class="h-2 rounded-full bg-foreground/70"
                    :style="{ width: metricWidth(node.metrics.cpu) }"
                  />
                </div>
                <span class="w-12 text-right font-medium tabular-nums">{{
                  formatPercent(node.metrics.cpu)
                }}</span>
              </div>
              <div class="flex items-center gap-3">
                <span class="w-16 text-muted-foreground">{{
                  t('nodesTable.memory')
                }}</span>
                <div class="h-2 flex-1 rounded-full bg-muted">
                  <div
                    class="h-2 rounded-full bg-sky-500"
                    :style="{ width: metricWidth(node.metrics.memory) }"
                  />
                </div>
                <span class="w-12 text-right font-medium tabular-nums">{{
                  formatPercent(node.metrics.memory)
                }}</span>
              </div>
              <div class="flex items-center gap-3">
                <span class="w-16 text-muted-foreground">{{
                  t('nodesTable.disk')
                }}</span>
                <div class="h-2 flex-1 rounded-full bg-muted">
                  <div
                    class="h-2 rounded-full bg-amber-500"
                    :style="{ width: metricWidth(node.metrics.disk) }"
                  />
                </div>
                <span class="w-12 text-right font-medium tabular-nums">{{
                  formatPercent(node.metrics.disk)
                }}</span>
              </div>
            </div>
          </div>
        </div>

        <Separator class="my-4" />

        <div class="flex flex-wrap items-center justify-between gap-3">
          <div v-if="footerText(node)" class="text-xs text-muted-foreground">
            {{ footerText(node) }}
          </div>

          <div class="flex flex-wrap gap-2">
            <Button as-child variant="outline" size="sm">
              <RouterLink
                :to="{ name: 'node-detail', params: { id: node.id } }"
              >
                {{ t('common.buttons.viewDetail') }}
                <ArrowRight class="h-4 w-4" />
              </RouterLink>
            </Button>

            <Button
              variant="secondary"
              size="sm"
              @click="emit('open-console', node.id)"
            >
              <Play class="h-4 w-4" />
              {{ t('common.buttons.openInConsole') }}
            </Button>

            <Button as-child variant="ghost" size="sm">
              <RouterLink to="/console">
                <ExternalLink class="h-4 w-4" />
                {{ t('common.buttons.goToConsole') }}
              </RouterLink>
            </Button>
          </div>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
