<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, Server, Cpu, HardDrive, Activity, Terminal as TerminalIcon, Info, Calendar } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { getNode, getNodeCommands } from '@/services/api'
import type { NodeDetail, NodeCommandItem } from '@/types/api'

const route = useRoute()
const router = useRouter()
const nodeId = route.params.nodeId as string

// Terminal opens in a new browser tab — see NodesView.openTerminal for rationale.
function openTerminal() {
  const href = router.resolve(`/nodes/${nodeId}/terminal`).href
  window.open(href, '_blank', 'noopener')
}

const node = ref<NodeDetail | null>(null)
const commands = ref<NodeCommandItem[]>([])
const loading = ref(true)

onMounted(async () => {
  try {
    const [n, cmds] = await Promise.all([
      getNode(nodeId),
      getNodeCommands(nodeId, { page: 1, page_size: 50 }),
    ])
    node.value = n
    commands.value = cmds.items || []
  } catch {
    // handle error
  } finally {
    loading.value = false
  }
})

function formatTime(ts: string) {
  return new Date(ts).toLocaleString()
}

// Conventional keys we know how to label nicely; everything else is rendered
// with its raw key as label.
const EXTRA_LABELS: Record<string, string> = {
  provider: 'Provider',
  plan: 'Plan',
  expires_at: 'Expires',
  monthly_cost: 'Monthly Cost',
  currency: 'Currency',
  billing_cycle: 'Billing Cycle',
  renewal_url: 'Renewal URL',
  account_email: 'Account Email',
  notes: 'Notes',
}

const extraEntries = computed(() => {
  const e = node.value?.extra
  if (!e) return []
  return Object.entries(e).map(([key, value]) => ({
    key,
    label: EXTRA_LABELS[key] ?? key,
    value,
    isUrl: typeof value === 'string' && /^https?:\/\//i.test(value),
    isLong: typeof value === 'string' && value.length > 80,
  }))
})

const expiresInfo = computed(() => {
  const raw = node.value?.extra?.expires_at
  if (typeof raw !== 'string' || !raw) return null
  const t = Date.parse(raw)
  if (Number.isNaN(t)) return null
  const diffMs = t - Date.now()
  const days = Math.ceil(diffMs / (1000 * 60 * 60 * 24))
  let tone: 'normal' | 'soon' | 'expired' = 'normal'
  if (days < 0) tone = 'expired'
  else if (days <= 14) tone = 'soon'
  return { days, tone, raw }
})
</script>

<template>
  <div class="flex h-full flex-col overflow-y-auto p-6" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 mb-6">
      <Button variant="ghost" size="icon" @click="router.push('/nodes')">
        <ArrowLeft class="h-4 w-4" />
      </Button>
      <Server class="h-5 w-5" style="color: var(--primary)" />
      <h1 class="text-lg font-semibold">{{ node?.alias || node?.name || $t('common.loading') }}</h1>
      <Badge v-if="node" :variant="node.status === 'online' ? 'default' : 'secondary'">
        {{ node.status === 'online' ? $t('common.online') : $t('common.offline') }}
      </Badge>
      <Badge
        v-if="expiresInfo"
        :style="{
          backgroundColor: expiresInfo.tone === 'expired' ? 'var(--color-error)'
            : expiresInfo.tone === 'soon' ? 'var(--color-warning, #d97706)'
            : 'var(--secondary)',
          color: expiresInfo.tone === 'normal' ? 'var(--secondary-foreground)' : 'var(--color-error-foreground)',
        }"
      >
        <Calendar class="mr-1 h-3 w-3 inline" />
        <template v-if="expiresInfo.tone === 'expired'">
          {{ $t('nodeDetail.expired', { days: -expiresInfo.days }) }}
        </template>
        <template v-else>
          {{ $t('nodeDetail.expiresInDays', { days: expiresInfo.days }) }}
        </template>
      </Badge>
      <div class="flex-1" />
      <Button
        v-if="node"
        variant="outline"
        size="sm"
        :disabled="node.status !== 'online'"
        @click="openTerminal()"
      >
        <TerminalIcon class="h-4 w-4 mr-1" />
        Open Terminal
      </Button>
    </div>

    <div v-if="node" class="space-y-6">
      <!-- System Info -->
      <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.hostname') }}</div>
          <div class="mt-1 text-sm font-medium">{{ node.name }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.ip') }}</div>
          <div class="mt-1 text-sm font-medium font-mono">{{ node.ip }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.os') }}</div>
          <div class="mt-1 text-sm font-medium">{{ node.os }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.kernel') }}</div>
          <div class="mt-1 text-sm font-medium">{{ node.kernel }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.agentVersion') }}</div>
          <div class="mt-1 text-sm font-medium">{{ node.agent_version }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="flex items-center gap-1 text-xs" style="color: var(--muted-foreground)">
            <Cpu class="h-3 w-3" /> {{ $t('nodeDetail.cpu') }}
          </div>
          <div class="mt-1 text-sm font-medium">{{ $t('nodeDetail.cores', { count: node.cpu_cores }) }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="flex items-center gap-1 text-xs" style="color: var(--muted-foreground)">
            <HardDrive class="h-3 w-3" /> {{ $t('nodeDetail.memory') }}
          </div>
          <div class="mt-1 text-sm font-medium">{{ $t('nodeDetail.gb', { value: (node.memory_total_mb / 1024).toFixed(1) }) }}</div>
        </div>
        <div class="rounded-lg p-4" style="background-color: var(--card)">
          <div class="flex items-center gap-1 text-xs" style="color: var(--muted-foreground)">
            <HardDrive class="h-3 w-3" /> {{ $t('nodeDetail.disk') }}
          </div>
          <div class="mt-1 text-sm font-medium">{{ $t('nodeDetail.gb', { value: node.disk_total_gb }) }}</div>
        </div>
      </div>

      <!-- Real-time Metrics -->
      <div v-if="node.metrics">
        <h2 class="flex items-center gap-2 text-sm font-medium mb-3">
          <Activity class="h-4 w-4" style="color: var(--primary)" />
          {{ $t('nodeDetail.realtimeMetrics') }}
        </h2>
        <div class="grid grid-cols-3 gap-4">
          <div class="rounded-lg p-4" style="background-color: var(--card)">
            <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.cpuUsage') }}</div>
            <div class="mt-1 text-2xl font-bold">{{ node.metrics.cpu.toFixed(1) }}%</div>
          </div>
          <div class="rounded-lg p-4" style="background-color: var(--card)">
            <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.memoryUsage') }}</div>
            <div class="mt-1 text-2xl font-bold">{{ node.metrics.memory.toFixed(1) }}%</div>
          </div>
          <div class="rounded-lg p-4" style="background-color: var(--card)">
            <div class="text-xs" style="color: var(--muted-foreground)">{{ $t('nodeDetail.diskUsage') }}</div>
            <div class="mt-1 text-2xl font-bold">{{ node.metrics.disk.toFixed(1) }}%</div>
          </div>
        </div>
      </div>

      <!-- Subscription / Notes (free-form metadata) -->
      <div v-if="extraEntries.length > 0">
        <h2 class="flex items-center gap-2 text-sm font-medium mb-3">
          <Info class="h-4 w-4" style="color: var(--primary)" />
          {{ $t('nodeDetail.subscriptionInfo') }}
        </h2>
        <div class="rounded-lg p-4 space-y-2" style="background-color: var(--card)">
          <div
            v-for="entry in extraEntries"
            :key="entry.key"
            class="grid grid-cols-[140px_1fr] gap-3 items-start text-sm"
          >
            <div class="text-xs pt-0.5" style="color: var(--muted-foreground)">{{ entry.label }}</div>
            <a
              v-if="entry.isUrl"
              :href="String(entry.value)"
              target="_blank"
              rel="noopener"
              class="break-all underline underline-offset-2 hover:text-foreground"
              style="color: var(--primary)"
            >{{ entry.value }}</a>
            <div v-else-if="entry.isLong" class="whitespace-pre-wrap break-words">{{ entry.value }}</div>
            <div v-else class="break-all">{{ entry.value }}</div>
          </div>
        </div>
      </div>

      <Separator />

      <!-- Command History -->
      <div>
        <h2 class="text-sm font-medium mb-3">{{ $t('nodeDetail.commandHistory') }}</h2>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{{ $t('common.time') }}</TableHead>
              <TableHead>{{ $t('nodeDetail.command') }}</TableHead>
              <TableHead>{{ $t('nodeDetail.exitCode') }}</TableHead>
              <TableHead>{{ $t('common.duration') }}</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="cmd in commands" :key="cmd.id">
              <TableCell class="text-xs">{{ formatTime(cmd.created_at) }}</TableCell>
              <TableCell class="font-mono text-xs max-w-[300px] truncate">{{ cmd.command }}</TableCell>
              <TableCell>
                <Badge :variant="cmd.exit_code === 0 ? 'default' : 'secondary'">
                  {{ cmd.exit_code ?? '-' }}
                </Badge>
              </TableCell>
              <TableCell class="text-xs">{{ cmd.duration_ms ? cmd.duration_ms + 'ms' : '-' }}</TableCell>
            </TableRow>
            <TableRow v-if="commands.length === 0">
              <TableCell :colspan="4" class="text-center text-sm" style="color: var(--muted-foreground)">
                {{ $t('nodeDetail.noCommandHistory') }}
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </div>

    <div v-else-if="loading" class="flex flex-1 items-center justify-center">
      <span style="color: var(--muted-foreground)">{{ $t('common.loading') }}</span>
    </div>
  </div>
</template>
