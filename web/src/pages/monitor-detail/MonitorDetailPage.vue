<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useLinkDetailStore, type TimeRange } from '@/entities/probe/model/link-detail.store'

const route = useRoute()
const router = useRouter()
const store = useLinkDetailStore()

const linkId = computed(() => route.params.id as string)

onMounted(() => {
  if (linkId.value) {
    store.fetch(linkId.value)
  }
})

watch(linkId, (id) => {
  if (id) store.fetch(id)
})

const timeRanges: { label: string; value: TimeRange }[] = [
  { label: '1H', value: '1h' },
  { label: '6H', value: '6h' },
  { label: '24H', value: '24h' },
  { label: '7D', value: '7d' },
]

// Chart data helpers
const latencyData = computed(() =>
  store.metrics.map((m) => ({
    time: new Date(m.timestamp).toLocaleTimeString(),
    min: m.latency_min,
    avg: m.latency_avg,
    max: m.latency_max,
  })),
)

const packetLossData = computed(() =>
  store.metrics.map((m) => ({
    time: new Date(m.timestamp).toLocaleTimeString(),
    value: m.packet_loss,
  })),
)

const tcpData = computed(() =>
  store.metrics.map((m) => ({
    time: new Date(m.timestamp).toLocaleTimeString(),
    value: m.tcp_connect_time,
  })),
)

const bandwidthData = computed(() =>
  store.metrics
    .filter((m) => m.bandwidth_mbps != null)
    .map((m) => ({
      time: new Date(m.timestamp).toLocaleTimeString(),
      value: m.bandwidth_mbps!,
    })),
)

function alertStatusClass(alert: { resolved_at: string | null }) {
  return alert.resolved_at
    ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
    : 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
}
</script>

<template>
  <div class="flex flex-col h-full overflow-auto">
    <!-- Header -->
    <div class="flex items-center justify-between p-4 border-b border-border">
      <div class="flex items-center gap-3">
        <button class="text-muted-foreground hover:text-foreground" @click="router.push({ name: 'monitor' })">
          &larr; Back
        </button>
        <h1 class="text-lg font-semibold">{{ linkId }}</h1>
      </div>
      <!-- Time range selector -->
      <div class="flex gap-1 rounded-lg border border-border p-0.5">
        <button
          v-for="tr in timeRanges"
          :key="tr.value"
          class="px-3 py-1 text-xs rounded-md transition-colors"
          :class="store.timeRange === tr.value
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'"
          @click="store.setTimeRange(tr.value)"
        >
          {{ tr.label }}
        </button>
      </div>
    </div>

    <div v-if="store.loading" class="flex items-center justify-center h-64 text-muted-foreground">
      Loading...
    </div>
    <div v-else-if="store.error" class="flex items-center justify-center h-64 text-destructive">
      {{ store.error }}
    </div>
    <div v-else class="p-4 space-y-6">
      <!-- Charts grid -->
      <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
        <!-- Latency Chart -->
        <div class="rounded-lg border bg-card p-4">
          <h3 class="text-sm font-medium mb-3">Latency (ms)</h3>
          <div v-if="!latencyData.length" class="h-40 flex items-center justify-center text-muted-foreground text-sm">
            No data
          </div>
          <div v-else class="h-40 overflow-x-auto">
            <div class="flex items-end gap-px h-32" :style="{ minWidth: latencyData.length * 8 + 'px' }">
              <div
                v-for="(d, i) in latencyData"
                :key="i"
                class="flex-1 min-w-[4px] bg-blue-500 rounded-t-sm"
                :style="{ height: Math.max(2, (d.avg / Math.max(...latencyData.map(x => x.max), 1)) * 100) + '%' }"
                :title="`${d.time}: min=${d.min.toFixed(1)} avg=${d.avg.toFixed(1)} max=${d.max.toFixed(1)}ms`"
              />
            </div>
            <div class="flex justify-between text-[10px] text-muted-foreground mt-1">
              <span>{{ latencyData[0]?.time }}</span>
              <span>{{ latencyData[latencyData.length - 1]?.time }}</span>
            </div>
          </div>
        </div>

        <!-- Packet Loss Chart -->
        <div class="rounded-lg border bg-card p-4">
          <h3 class="text-sm font-medium mb-3">Packet Loss (%)</h3>
          <div v-if="!packetLossData.length" class="h-40 flex items-center justify-center text-muted-foreground text-sm">
            No data
          </div>
          <div v-else class="h-40 overflow-x-auto">
            <div class="flex items-end gap-px h-32" :style="{ minWidth: packetLossData.length * 8 + 'px' }">
              <div
                v-for="(d, i) in packetLossData"
                :key="i"
                class="flex-1 min-w-[4px] rounded-t-sm"
                :class="d.value > 5 ? 'bg-red-500' : d.value > 0 ? 'bg-yellow-500' : 'bg-green-500'"
                :style="{ height: Math.max(2, (d.value / Math.max(...packetLossData.map(x => x.value), 1)) * 100) + '%' }"
                :title="`${d.time}: ${d.value.toFixed(1)}%`"
              />
            </div>
            <div class="flex justify-between text-[10px] text-muted-foreground mt-1">
              <span>{{ packetLossData[0]?.time }}</span>
              <span>{{ packetLossData[packetLossData.length - 1]?.time }}</span>
            </div>
          </div>
        </div>

        <!-- TCP Connect Chart -->
        <div class="rounded-lg border bg-card p-4">
          <h3 class="text-sm font-medium mb-3">TCP Connect (ms)</h3>
          <div v-if="!tcpData.length" class="h-40 flex items-center justify-center text-muted-foreground text-sm">
            No data
          </div>
          <div v-else class="h-40 overflow-x-auto">
            <div class="flex items-end gap-px h-32" :style="{ minWidth: tcpData.length * 8 + 'px' }">
              <div
                v-for="(d, i) in tcpData"
                :key="i"
                class="flex-1 min-w-[4px] bg-purple-500 rounded-t-sm"
                :style="{ height: Math.max(2, (d.value / Math.max(...tcpData.map(x => x.value), 1)) * 100) + '%' }"
                :title="`${d.time}: ${d.value.toFixed(1)}ms`"
              />
            </div>
            <div class="flex justify-between text-[10px] text-muted-foreground mt-1">
              <span>{{ tcpData[0]?.time }}</span>
              <span>{{ tcpData[tcpData.length - 1]?.time }}</span>
            </div>
          </div>
        </div>

        <!-- Bandwidth Chart -->
        <div class="rounded-lg border bg-card p-4">
          <h3 class="text-sm font-medium mb-3">Bandwidth (Mbps)</h3>
          <div v-if="!bandwidthData.length" class="h-40 flex items-center justify-center text-muted-foreground text-sm">
            No bandwidth data
          </div>
          <div v-else class="h-40 overflow-x-auto">
            <div class="flex items-end gap-px h-32" :style="{ minWidth: bandwidthData.length * 16 + 'px' }">
              <div
                v-for="(d, i) in bandwidthData"
                :key="i"
                class="flex-1 min-w-[8px] bg-emerald-500 rounded-t-sm"
                :style="{ height: Math.max(2, (d.value / Math.max(...bandwidthData.map(x => x.value), 1)) * 100) + '%' }"
                :title="`${d.time}: ${d.value.toFixed(1)} Mbps`"
              />
            </div>
            <div class="flex justify-between text-[10px] text-muted-foreground mt-1">
              <span>{{ bandwidthData[0]?.time }}</span>
              <span>{{ bandwidthData[bandwidthData.length - 1]?.time }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Alert History -->
      <div class="rounded-lg border bg-card p-4">
        <h3 class="text-sm font-medium mb-3">Alert History</h3>
        <div v-if="!store.alerts.length" class="text-muted-foreground text-sm py-4 text-center">
          No alerts for this link
        </div>
        <table v-else class="w-full text-sm">
          <thead>
            <tr class="text-muted-foreground text-left">
              <th class="pb-2 font-medium">Time</th>
              <th class="pb-2 font-medium">Type</th>
              <th class="pb-2 font-medium">Status</th>
              <th class="pb-2 font-medium">Message</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="alert in store.alerts" :key="alert.id" class="border-t border-border/50">
              <td class="py-1.5 text-xs text-muted-foreground">{{ new Date(alert.triggered_at).toLocaleString() }}</td>
              <td class="py-1.5 text-xs">{{ alert.type }}</td>
              <td class="py-1.5">
                <span class="text-xs px-1.5 py-0.5 rounded" :class="alertStatusClass(alert)">
                  {{ alert.resolved_at ? 'Resolved' : 'Open' }}
                </span>
              </td>
              <td class="py-1.5 text-xs">{{ alert.message }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
