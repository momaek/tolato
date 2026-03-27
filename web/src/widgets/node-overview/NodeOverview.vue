<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  displayValue,
  formatPercent,
  formatRelativeMinutes,
  hasDisplayValue,
  joinDisplayParts,
  normalizePercentValue,
} from '@/shared/lib/format'
import { upgradeNode } from '@/shared/api/adapters/nodes'
import type { NodeDetail, NodeStatus } from '@/shared/types/node'
import StatusBadge from '@/shared/ui/status-badge/StatusBadge.vue'
import { cn } from '@/lib/utils'
import { ArrowUpCircle } from 'lucide-vue-next'

const props = defineProps<{
  node: NodeDetail
}>()

const { t } = useI18n()

const upgradeOpen = ref(false)
const upgradeUrl = ref('')
const upgradeVersion = ref('')
const upgradeLoading = ref(false)
const upgradeError = ref('')

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

function headerMeta(node: NodeDetail) {
  return joinDisplayParts([
    hasDisplayValue(node.region) ? node.region : null,
    hasDisplayValue(node.os) ? node.os : null,
    t('common.metrics.lastSeen', {
      value: formatRelativeMinutes(node.lastSeen),
    }),
  ])
}

async function handleUpgrade() {
  if (!upgradeUrl.value.trim()) return
  upgradeLoading.value = true
  upgradeError.value = ''
  try {
    await upgradeNode(props.node.id, {
      downloadUrl: upgradeUrl.value.trim(),
      targetVersion: upgradeVersion.value.trim(),
    })
    upgradeOpen.value = false
    upgradeUrl.value = ''
    upgradeVersion.value = ''
  } catch (err: any) {
    upgradeError.value = err?.message || t('nodeOverview.upgradeDialog.error')
  } finally {
    upgradeLoading.value = false
  }
}
</script>

<template>
  <Card class="glass-panel border-border/70 rounded-2xl">
    <CardHeader class="space-y-4">
      <div class="flex flex-wrap items-center gap-3">
        <span
          :class="cn('h-2.5 w-2.5 rounded-full', statusTone(node.status))"
        />
        <CardTitle class="text-2xl">{{ node.hostname }}</CardTitle>
        <StatusBadge
          :label="statusLabel(node.status)"
          :tone="statusBadgeTone(node.status)"
        />
        <Badge
          v-if="hasDisplayValue(node.region)"
          variant="secondary"
          class="rounded-full px-3 py-1"
        >
          {{ node.region }}
        </Badge>
      </div>
      <div
        class="flex flex-wrap items-center gap-2 text-sm text-muted-foreground"
      >
        {{ headerMeta(node) }}
      </div>
    </CardHeader>

    <CardContent class="space-y-6">
      <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <div class="rounded-xl border border-border/70 bg-background/70 p-4">
          <div
            class="text-xs uppercase tracking-[0.24em] text-muted-foreground"
          >
            {{ t('nodeOverview.ip') }}
          </div>
          <div class="mt-2 font-medium">
            {{ displayValue(node.ipAddress, t('common.labels.notAvailable')) }}
          </div>
        </div>
        <div class="rounded-xl border border-border/70 bg-background/70 p-4">
          <div
            class="text-xs uppercase tracking-[0.24em] text-muted-foreground"
          >
            {{ t('nodeOverview.provider') }}
          </div>
          <div class="mt-2 font-medium">
            {{ displayValue(node.provider, t('common.labels.notAvailable')) }}
          </div>
        </div>
        <div class="rounded-xl border border-border/70 bg-background/70 p-4">
          <div
            class="text-xs uppercase tracking-[0.24em] text-muted-foreground"
          >
            {{ t('nodeOverview.uptime') }}
          </div>
          <div class="mt-2 font-medium">
            {{ displayValue(node.uptime, t('common.labels.notAvailable')) }}
          </div>
        </div>
        <div class="rounded-xl border border-border/70 bg-background/70 p-4">
          <div
            class="text-xs uppercase tracking-[0.24em] text-muted-foreground"
          >
            {{ t('nodeOverview.kernel') }}
          </div>
          <div class="mt-2 font-medium">
            {{ displayValue(node.kernel, t('common.labels.notAvailable')) }}
          </div>
        </div>
      </div>

      <div v-if="props.node.tags.length" class="flex flex-wrap gap-2">
        <Badge v-for="tag in node.tags" :key="tag" variant="outline">{{
          tag
        }}</Badge>
      </div>

      <div class="grid gap-4 lg:grid-cols-[1.15fr_0.85fr]">
        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">
            {{ t('nodeOverview.resourceUsage') }}
          </div>
          <div class="mt-4 space-y-4 text-sm">
            <div>
              <div class="mb-2 flex items-center justify-between">
                <span>{{ t('nodesTable.cpu') }}</span>
                <span class="font-medium tabular-nums">{{
                  formatPercent(node.metrics.cpu)
                }}</span>
              </div>
              <div class="h-2 rounded-full bg-muted">
                <div
                  class="h-2 rounded-full bg-foreground/80"
                  :style="{ width: metricWidth(node.metrics.cpu) }"
                />
              </div>
            </div>
            <div>
              <div class="mb-2 flex items-center justify-between">
                <span>{{ t('nodesTable.memory') }}</span>
                <span class="font-medium tabular-nums">{{
                  formatPercent(node.metrics.memory)
                }}</span>
              </div>
              <div class="h-2 rounded-full bg-muted">
                <div
                  class="h-2 rounded-full bg-sky-500"
                  :style="{ width: metricWidth(node.metrics.memory) }"
                />
              </div>
            </div>
            <div>
              <div class="mb-2 flex items-center justify-between">
                <span>{{ t('nodesTable.disk') }}</span>
                <span class="font-medium tabular-nums">{{
                  formatPercent(node.metrics.disk)
                }}</span>
              </div>
              <div class="h-2 rounded-full bg-muted">
                <div
                  class="h-2 rounded-full bg-amber-500"
                  :style="{ width: metricWidth(node.metrics.disk) }"
                />
              </div>
            </div>
          </div>
        </div>

        <div class="rounded-2xl border border-border/70 bg-background/70 p-4">
          <div class="text-sm font-medium">
            {{ t('nodeOverview.agentStatus') }}
          </div>
          <dl class="mt-4 space-y-3 text-sm">
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">
                {{ t('nodeOverview.agentVersion') }}
              </dt>
              <dd class="flex items-center gap-2">
                {{
                  displayValue(
                    node.agentVersion,
                    t('common.labels.notAvailable'),
                  )
                }}
                <Dialog v-model:open="upgradeOpen">
                  <DialogTrigger as-child>
                    <Button
                      variant="ghost"
                      size="sm"
                      class="h-7 px-2"
                      :disabled="node.status === 'offline'"
                    >
                      <ArrowUpCircle class="mr-1 h-3.5 w-3.5" />
                      {{ t('nodeOverview.upgrade') }}
                    </Button>
                  </DialogTrigger>
                  <DialogContent class="max-w-md rounded-2xl">
                    <DialogHeader>
                      <DialogTitle>{{
                        t('nodeOverview.upgradeDialog.title')
                      }}</DialogTitle>
                      <DialogDescription>{{
                        t('nodeOverview.upgradeDialog.description')
                      }}</DialogDescription>
                    </DialogHeader>
                    <div class="space-y-4 py-2">
                      <div class="space-y-2">
                        <label
                          class="text-sm font-medium"
                          for="upgrade-url"
                        >
                          {{ t('nodeOverview.upgradeDialog.downloadUrl') }}
                        </label>
                        <input
                          id="upgrade-url"
                          v-model="upgradeUrl"
                          type="url"
                          :placeholder="
                            t(
                              'nodeOverview.upgradeDialog.downloadUrlPlaceholder',
                            )
                          "
                          class="flex h-9 w-full rounded-lg border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                        />
                      </div>
                      <div class="space-y-2">
                        <label
                          class="text-sm font-medium"
                          for="upgrade-version"
                        >
                          {{ t('nodeOverview.upgradeDialog.targetVersion') }}
                        </label>
                        <input
                          id="upgrade-version"
                          v-model="upgradeVersion"
                          type="text"
                          :placeholder="
                            t(
                              'nodeOverview.upgradeDialog.targetVersionPlaceholder',
                            )
                          "
                          class="flex h-9 w-full rounded-lg border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                        />
                      </div>
                      <p
                        v-if="upgradeError"
                        class="text-sm text-destructive"
                      >
                        {{ upgradeError }}
                      </p>
                    </div>
                    <DialogFooter>
                      <Button
                        variant="outline"
                        @click="upgradeOpen = false"
                      >
                        {{ t('nodeOverview.upgradeDialog.cancel') }}
                      </Button>
                      <Button
                        :disabled="!upgradeUrl.trim() || upgradeLoading"
                        @click="handleUpgrade"
                      >
                        {{ t('nodeOverview.upgradeDialog.confirm') }}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
              </dd>
            </div>
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">
                {{ t('nodeOverview.busy') }}
              </dt>
              <dd>
                {{ node.busy ? t('common.values.yes') : t('common.values.no') }}
              </dd>
            </div>
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">
                {{ t('nodeOverview.status') }}
              </dt>
              <dd>
                <StatusBadge
                  :label="statusLabel(node.status)"
                  :tone="statusBadgeTone(node.status)"
                />
              </dd>
            </div>
            <div class="flex items-center justify-between gap-4">
              <dt class="text-muted-foreground">
                {{ t('common.labels.lastSeen') }}
              </dt>
              <dd>{{ formatRelativeMinutes(node.lastSeen) }}</dd>
            </div>
          </dl>
        </div>
      </div>
    </CardContent>
  </Card>
</template>
