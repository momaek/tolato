<script setup lang="ts">
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Separator } from '@/components/ui/separator'
import type { NodeSummary } from '@/shared/types/node'
import { clampNumber } from '@/shared/lib/format'
import { formatRelativeMinutes } from '@/shared/lib/format'
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

function statusTone(status: NodeSummary['status']) {
  if (status === 'offline') return 'bg-stone-400'
  if (status === 'stale') return 'bg-amber-500'
  if (status === 'busy') return 'bg-sky-500'
  return 'bg-emerald-500'
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
        class="rounded-xl border border-border/70 bg-background/70 p-4 transition-colors hover:bg-background"
      >
        <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
          <div class="space-y-2">
            <div class="flex flex-wrap items-center gap-2">
              <div :class="cn('h-2.5 w-2.5 rounded-full', statusTone(node.status))" />
              <p class="font-medium">{{ node.hostname }}</p>
              <Badge variant="secondary">{{ node.region }}</Badge>
              <Badge :variant="node.busy ? 'default' : 'outline'">
                {{ node.busy ? 'busy' : node.status }}
              </Badge>
            </div>

            <div class="text-sm text-muted-foreground">
              {{ node.os }} · {{ node.version }} · {{ node.provider }} · {{ t('common.metrics.lastSeen', { value: formatRelativeMinutes(node.lastSeen) }) }}
            </div>

            <div class="flex flex-wrap gap-2">
              <Badge v-for="tag in node.tags" :key="tag" variant="outline">{{ tag }}</Badge>
            </div>
          </div>

          <div class="grid min-w-[260px] gap-2 text-sm xl:justify-end">
            <div class="flex items-center gap-3">
              <span class="w-16 text-muted-foreground">{{ t('nodesTable.cpu') }}</span>
              <div class="h-2 flex-1 rounded-full bg-muted">
                <div class="h-2 rounded-full bg-foreground/70" :style="{ width: `${clampNumber(node.metrics.cpu)}%` }" />
              </div>
              <span class="w-10 text-right tabular-nums">{{ node.metrics.cpu }}%</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="w-16 text-muted-foreground">{{ t('nodesTable.memory') }}</span>
              <div class="h-2 flex-1 rounded-full bg-muted">
                <div class="h-2 rounded-full bg-sky-500" :style="{ width: `${clampNumber(node.metrics.memory)}%` }" />
              </div>
              <span class="w-10 text-right tabular-nums">{{ node.metrics.memory }}%</span>
            </div>
            <div class="flex items-center gap-3">
              <span class="w-16 text-muted-foreground">{{ t('nodesTable.disk') }}</span>
              <div class="h-2 flex-1 rounded-full bg-muted">
                <div class="h-2 rounded-full bg-amber-500" :style="{ width: `${clampNumber(node.metrics.disk)}%` }" />
              </div>
              <span class="w-10 text-right tabular-nums">{{ node.metrics.disk }}%</span>
            </div>
          </div>
        </div>

        <Separator class="my-4" />

        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="text-xs text-muted-foreground">
            <span class="font-medium text-foreground">{{ node.ipAddress }}</span> · {{ t('common.metrics.version', { value: node.version }) }}
          </div>

          <div class="flex flex-wrap gap-2">
            <Button as-child variant="outline" size="sm">
              <RouterLink :to="{ name: 'node-detail', params: { id: node.id } }">
                {{ t('common.buttons.viewDetail') }}
                <ArrowRight class="h-4 w-4" />
              </RouterLink>
            </Button>

            <Button variant="secondary" size="sm" @click="emit('open-console', node.id)">
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
