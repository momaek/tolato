<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'

import { useMonitorStore } from '@/entities/probe/model/monitor.store'
import type { LinkStatus, ProbeNode } from '@/shared/types/probe'

const router = useRouter()
const store = useMonitorStore()

onMounted(async () => {
  await store.fetchAll()
  store.startAutoRefresh()
})

onUnmounted(() => {
  store.stopAutoRefresh()
})

// Layout: 3 columns for entry / relay / landing
const COL_X = { entry: 80, relay: 380, landing: 680 }
const NODE_W = 160
const NODE_H = 56
const Y_START = 80
const Y_GAP = 80

interface LayoutNode {
  node: ProbeNode
  x: number
  y: number
}

const layoutNodes = computed<LayoutNode[]>(() => {
  const groups = store.nodesByRole
  const result: LayoutNode[] = []

  for (const role of ['entry', 'relay', 'landing'] as const) {
    const nodes = groups[role]
    nodes.forEach((node, i) => {
      result.push({
        node,
        x: COL_X[role],
        y: Y_START + i * Y_GAP,
      })
    })
  }
  return result
})

const layoutMap = computed(() => {
  const map = new Map<string, LayoutNode>()
  for (const ln of layoutNodes.value) {
    map.set(ln.node.id, ln)
  }
  return map
})

const svgHeight = computed(() => {
  const maxY = layoutNodes.value.reduce((max, ln) => Math.max(max, ln.y), 0)
  return Math.max(maxY + NODE_H + 60, 400)
})

function linkPath(link: LinkStatus) {
  const src = layoutMap.value.get(link.source_id)
  const tgt = layoutMap.value.get(link.target_id)
  if (!src || !tgt) return ''
  const x1 = src.x + NODE_W
  const y1 = src.y + NODE_H / 2
  const x2 = tgt.x
  const y2 = tgt.y + NODE_H / 2
  const cx = (x1 + x2) / 2
  return `M ${x1} ${y1} C ${cx} ${y1}, ${cx} ${y2}, ${x2} ${y2}`
}

function linkColor(status: string) {
  switch (status) {
    case 'ok': return '#22c55e'
    case 'warn': return '#eab308'
    case 'alert': return '#ef4444'
    default: return '#6b7280'
  }
}

function linkLabel(link: LinkStatus) {
  const parts: string[] = []
  if (link.latency_avg != null) parts.push(`${link.latency_avg.toFixed(0)}ms`)
  if (link.packet_loss != null && link.packet_loss > 0) parts.push(`${link.packet_loss.toFixed(1)}%`)
  return parts.join(' | ') || '-'
}

function linkLabelPos(link: LinkStatus) {
  const src = layoutMap.value.get(link.source_id)
  const tgt = layoutMap.value.get(link.target_id)
  if (!src || !tgt) return { x: 0, y: 0 }
  return {
    x: (src.x + NODE_W + tgt.x) / 2,
    y: (src.y + tgt.y + NODE_H) / 2 - 6,
  }
}

function statusDot(status: string) {
  switch (status) {
    case 'ok': return 'bg-green-500'
    case 'warn': return 'bg-yellow-500'
    case 'alert': return 'bg-red-500'
    default: return 'bg-gray-400'
  }
}

function roleBadge(role: string) {
  switch (role) {
    case 'entry': return 'bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300'
    case 'relay': return 'bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300'
    case 'landing': return 'bg-orange-100 text-orange-700 dark:bg-orange-900 dark:text-orange-300'
    default: return 'bg-gray-100 text-gray-700'
  }
}

function goToLink(link: LinkStatus) {
  router.push({ name: 'monitor-detail', params: { id: link.id } })
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Status Bar -->
    <div class="grid grid-cols-4 gap-4 p-4 border-b border-border">
      <div class="rounded-lg border bg-card p-3 text-center">
        <div class="text-2xl font-bold">{{ store.totalNodes }}</div>
        <div class="text-xs text-muted-foreground">Nodes</div>
      </div>
      <div class="rounded-lg border bg-card p-3 text-center">
        <div class="text-2xl font-bold">{{ store.totalLinks }}</div>
        <div class="text-xs text-muted-foreground">Links</div>
      </div>
      <div class="rounded-lg border bg-card p-3 text-center">
        <div class="text-2xl font-bold text-red-500">{{ store.alertLinks }}</div>
        <div class="text-xs text-muted-foreground">Alert Links</div>
      </div>
      <div class="rounded-lg border bg-card p-3 text-center">
        <div class="text-2xl font-bold text-yellow-500">{{ store.openAlerts }}</div>
        <div class="text-xs text-muted-foreground">Open Alerts</div>
      </div>
    </div>

    <!-- Topology Canvas -->
    <div class="flex-1 overflow-auto p-4">
      <div v-if="store.loading && !store.initialized" class="flex items-center justify-center h-64 text-muted-foreground">
        Loading...
      </div>
      <div v-else-if="store.error" class="flex items-center justify-center h-64 text-destructive">
        {{ store.error }}
      </div>
      <div v-else-if="!store.nodes.length" class="flex items-center justify-center h-64 text-muted-foreground">
        No probe data yet. Configure nodeagent with -probe-config to start monitoring.
      </div>
      <svg v-else :width="900" :height="svgHeight" class="mx-auto">
        <!-- Column labels -->
        <text :x="COL_X.entry + NODE_W / 2" y="30" text-anchor="middle" class="fill-muted-foreground text-sm font-medium">Entry</text>
        <text :x="COL_X.relay + NODE_W / 2" y="30" text-anchor="middle" class="fill-muted-foreground text-sm font-medium">Relay</text>
        <text :x="COL_X.landing + NODE_W / 2" y="30" text-anchor="middle" class="fill-muted-foreground text-sm font-medium">Landing</text>

        <!-- Links -->
        <g v-for="link in store.links" :key="link.id" class="cursor-pointer" @click="goToLink(link)">
          <path :d="linkPath(link)" fill="none" :stroke="linkColor(link.status)" stroke-width="2" stroke-opacity="0.7" />
          <text :x="linkLabelPos(link).x" :y="linkLabelPos(link).y" text-anchor="middle" class="fill-foreground text-[10px]">
            {{ linkLabel(link) }}
          </text>
        </g>

        <!-- Nodes -->
        <g v-for="ln in layoutNodes" :key="ln.node.id">
          <rect :x="ln.x" :y="ln.y" :width="NODE_W" :height="NODE_H" rx="8"
                class="fill-card stroke-border" stroke-width="1" />
          <circle :cx="ln.x + 16" :cy="ln.y + NODE_H / 2" r="5"
                  :class="statusDot('ok')" :fill="linkColor('ok')" />
          <text :x="ln.x + 28" :y="ln.y + 22" class="fill-foreground text-xs font-medium">
            {{ ln.node.name }}
          </text>
          <text :x="ln.x + 28" :y="ln.y + 38" class="fill-muted-foreground text-[10px]">
            {{ ln.node.role }}
          </text>
        </g>
      </svg>
    </div>

    <!-- Recent Alerts -->
    <div v-if="store.alerts.length > 0" class="border-t border-border p-4">
      <h3 class="text-sm font-medium mb-2">Recent Alerts</h3>
      <div class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-muted-foreground text-left">
              <th class="pb-2 font-medium">Time</th>
              <th class="pb-2 font-medium">Link</th>
              <th class="pb-2 font-medium">Type</th>
              <th class="pb-2 font-medium">Message</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="alert in store.alerts.slice(0, 10)" :key="alert.id" class="border-t border-border/50">
              <td class="py-1.5 text-muted-foreground text-xs">{{ new Date(alert.triggered_at).toLocaleString() }}</td>
              <td class="py-1.5 text-xs">{{ alert.link_id }}</td>
              <td class="py-1.5">
                <span class="text-xs px-1.5 py-0.5 rounded bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300">
                  {{ alert.type }}
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
