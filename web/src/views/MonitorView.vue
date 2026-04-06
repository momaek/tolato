<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, AlertTriangle, CheckCircle, Link } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import TopologyCanvas from '@/components/monitor/TopologyCanvas.vue'
import { useMonitorStore } from '@/stores/monitor'
import api from '@/services/api'

const router = useRouter()
const monitorStore = useMonitorStore()

onMounted(async () => {
  await Promise.all([
    monitorStore.fetchNodes(),
    monitorStore.fetchLinks(),
    monitorStore.fetchAlerts({ status: 'unresolved' }),
  ])
})

function handleUpdatePosition(nodeId: string, x: number, y: number) {
  monitorStore.updateNodePosition(nodeId, x, y)
}

function handleClickLink(linkId: string) {
  router.push(`/monitor/${linkId}`)
}

async function handleCreateLink(sourceId: string, targetId: string) {
  await monitorStore.createLink(sourceId, targetId)
}

async function handleDeleteLink(linkId: string) {
  await monitorStore.deleteLink(linkId)
}

async function handleUpdateRole(nodeId: string, role: string) {
  try {
    await api.put(`/api/v1/probe/nodes/${nodeId}`, { role })
    await monitorStore.fetchNodes()
  } catch { /* silent */ }
}

function formatTime(ts: string) {
  return new Date(ts).toLocaleString()
}
</script>

<template>
  <div class="flex h-full flex-col overflow-hidden" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 border-b px-5 py-3">
      <Activity class="h-5 w-5" style="color: var(--primary)" />
      <h1 class="text-lg font-semibold">{{ $t('monitor.linkMonitor') }}</h1>
    </div>

    <!-- Stats cards -->
    <div class="flex gap-3 px-5 py-3">
      <div class="flex items-center gap-2 rounded-lg px-4 py-2" style="background-color: var(--card)">
        <Link class="h-4 w-4" style="color: var(--muted-foreground)" />
        <span class="text-sm">{{ $t('monitor.linksCount', { count: monitorStore.links.length }) }}</span>
      </div>
      <div class="flex items-center gap-2 rounded-lg px-4 py-2" style="background-color: var(--color-success)">
        <CheckCircle class="h-4 w-4" style="color: var(--color-success-foreground)" />
        <span class="text-sm" style="color: var(--color-success-foreground)">
          {{ $t('monitor.normalCount', { count: monitorStore.links.filter((l) => {
            const m = l.latest_metric
            return m && (!m.packet_loss || m.packet_loss < 5) && (!m.latency_avg || m.latency_avg < 200)
          }).length }) }}
        </span>
      </div>
      <div class="flex items-center gap-2 rounded-lg px-4 py-2" style="background-color: var(--color-error)">
        <AlertTriangle class="h-4 w-4" style="color: var(--color-error-foreground)" />
        <span class="text-sm" style="color: var(--color-error-foreground)">
          {{ $t('monitor.alertsCount', { count: monitorStore.alerts.length }) }}
        </span>
      </div>
    </div>

    <!-- Topology Canvas -->
    <div class="flex-1 border-t">
      <TopologyCanvas
        :nodes="monitorStore.nodes"
        :links="monitorStore.links"
        @update-position="handleUpdatePosition"
        @click-link="handleClickLink"
        @create-link="handleCreateLink"
        @delete-link="handleDeleteLink"
        @update-role="handleUpdateRole"
      />
    </div>

    <!-- Recent Alerts -->
    <div v-if="monitorStore.alerts.length > 0" class="border-t px-5 py-3">
      <h3 class="text-xs font-medium mb-2" style="color: var(--muted-foreground)">{{ $t('monitor.recentAlerts') }}</h3>
      <div class="space-y-1 max-h-32 overflow-y-auto">
        <div
          v-for="alert in monitorStore.alerts.slice(0, 10)"
          :key="alert.id"
          class="flex items-center gap-2 text-xs"
        >
          <Badge variant="secondary" class="text-[10px]">{{ alert.type }}</Badge>
          <span class="truncate" style="color: var(--foreground)">{{ alert.message }}</span>
          <span class="ml-auto shrink-0" style="color: var(--muted-foreground)">
            {{ formatTime(alert.triggered_at) }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
