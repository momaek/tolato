<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { ArrowLeft, Clock } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import MetricChart from '@/components/monitor/MetricChart.vue'
import { useMonitorStore, type ProbeMetric, type ProbeAlert } from '@/stores/monitor'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const monitorStore = useMonitorStore()
const linkId = route.params.linkId as string

const metrics = ref<ProbeMetric[]>([])
const alerts = ref<ProbeAlert[]>([])
const timeRange = ref('24h')
const loading = ref(true)

const timeRanges = [
  { label: '1h', hours: 1 },
  { label: '6h', hours: 6 },
  { label: '24h', hours: 24 },
  { label: '7d', hours: 168 },
]

onMounted(() => {
  loadData()
  monitorStore.fetchAlerts({ link_id: linkId }).then(() => {
    alerts.value = monitorStore.alerts
  })
})

async function loadData() {
  loading.value = true
  const hours = timeRanges.find((r) => r.label === timeRange.value)?.hours || 24
  const from = new Date(Date.now() - hours * 3600000).toISOString()
  metrics.value = await monitorStore.fetchLinkMetrics(linkId, from)
  loading.value = false
}

function selectRange(label: string) {
  timeRange.value = label
  loadData()
}

function formatTime(ts: string) {
  return new Date(ts).toLocaleString()
}

const chartLabels = computed(() =>
  metrics.value.map((m) => new Date(m.timestamp).toLocaleTimeString()).reverse()
)
</script>

<template>
  <div class="flex h-full flex-col overflow-y-auto p-6" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 mb-6">
      <Button variant="ghost" size="icon" @click="router.push('/monitor')">
        <ArrowLeft class="h-4 w-4" />
      </Button>
      <h1 class="text-lg font-semibold">{{ $t('linkDetail.linkTitle', { id: linkId }) }}</h1>
    </div>

    <!-- Time range selector -->
    <div class="flex gap-2 mb-6">
      <Button
        v-for="range_ in timeRanges"
        :key="range_.label"
        :variant="timeRange === range_.label ? 'default' : 'outline'"
        size="sm"
        @click="selectRange(range_.label)"
      >
        {{ range_.label }}
      </Button>
    </div>

    <!-- Metrics Charts -->
    <div class="grid grid-cols-2 gap-4 mb-6">
      <MetricChart
        :title="$t('linkDetail.latency')"
        :labels="chartLabels"
        :datasets="[
          { label: t('linkDetail.min'), data: metrics.map(m => m.latency_min ?? null).reverse(), borderColor: '#4ade80' },
          { label: t('linkDetail.avg'), data: metrics.map(m => m.latency_avg ?? null).reverse(), borderColor: '#FF8400' },
          { label: t('linkDetail.max'), data: metrics.map(m => m.latency_max ?? null).reverse(), borderColor: '#ef4444' },
        ]"
        y-axis-label="ms"
      />
      <MetricChart
        :title="$t('linkDetail.packetLoss')"
        :labels="chartLabels"
        :datasets="[
          { label: t('linkDetail.loss'), data: metrics.map(m => m.packet_loss ?? null).reverse(), borderColor: '#ef4444', backgroundColor: 'rgba(239,68,68,0.1)', fill: true },
        ]"
        y-axis-label="%"
      />
      <MetricChart
        :title="$t('linkDetail.tcpConnectTime')"
        :labels="chartLabels"
        :datasets="[
          { label: t('linkDetail.tcp'), data: metrics.map(m => m.tcp_connect_time ?? null).reverse(), borderColor: '#60a5fa' },
        ]"
        y-axis-label="ms"
      />
      <MetricChart
        :title="$t('linkDetail.bandwidth')"
        :labels="chartLabels"
        :datasets="[
          { label: t('linkDetail.bandwidth'), data: metrics.map(m => m.bandwidth_mbps ?? null).reverse(), borderColor: '#a78bfa', backgroundColor: 'rgba(167,139,250,0.1)', fill: true },
        ]"
        y-axis-label="Mbps"
      />
    </div>

    <!-- Alert History -->
    <h3 class="text-sm font-medium mb-3 flex items-center gap-2">
      <Clock class="h-4 w-4" />
      {{ $t('linkDetail.alertHistory') }}
    </h3>
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{{ $t('common.time') }}</TableHead>
          <TableHead>{{ $t('common.type') }}</TableHead>
          <TableHead>{{ $t('common.status') }}</TableHead>
          <TableHead>{{ $t('common.message') }}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow v-for="alert in alerts" :key="alert.id">
          <TableCell class="text-xs">{{ formatTime(alert.triggered_at) }}</TableCell>
          <TableCell><Badge variant="secondary">{{ alert.type }}</Badge></TableCell>
          <TableCell>
            <Badge :variant="alert.resolved_at ? 'default' : 'secondary'">
              {{ alert.resolved_at ? $t('common.resolved') : $t('common.active') }}
            </Badge>
          </TableCell>
          <TableCell class="text-xs">{{ alert.message }}</TableCell>
        </TableRow>
        <TableRow v-if="alerts.length === 0">
          <TableCell :colspan="4" class="text-center text-sm" style="color: var(--muted-foreground)">
            {{ $t('linkDetail.noAlerts') }}
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>
