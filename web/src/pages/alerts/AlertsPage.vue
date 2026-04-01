<script setup lang="ts">
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'

import { useAlertsStore } from '@/entities/probe/model/alerts.store'

const router = useRouter()
const store = useAlertsStore()

onMounted(() => {
  store.fetchAll()
})

const alertTypes = ['', 'latency', 'packet_loss', 'tcp', 'bandwidth', 'offline']
const statusOptions = ['all', 'open', 'resolved'] as const

function alertTypeLabel(type: string) {
  switch (type) {
    case 'latency': return 'Latency'
    case 'packet_loss': return 'Packet Loss'
    case 'tcp': return 'TCP'
    case 'bandwidth': return 'Bandwidth'
    case 'offline': return 'Offline'
    default: return 'All Types'
  }
}

function statusBadge(alert: { resolved_at: string | null }) {
  return alert.resolved_at
    ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
    : 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
}

function duration(alert: { triggered_at: string; resolved_at: string | null }) {
  const start = new Date(alert.triggered_at).getTime()
  const end = alert.resolved_at ? new Date(alert.resolved_at).getTime() : Date.now()
  const secs = Math.floor((end - start) / 1000)
  if (secs < 60) return `${secs}s`
  if (secs < 3600) return `${Math.floor(secs / 60)}m`
  return `${Math.floor(secs / 3600)}h ${Math.floor((secs % 3600) / 60)}m`
}

function goToLink(linkId: string) {
  router.push({ name: 'monitor-detail', params: { id: linkId } })
}
</script>

<template>
  <div class="flex flex-col h-full">
    <!-- Filter Bar -->
    <div class="flex items-center gap-3 p-4 border-b border-border">
      <select
        v-model="store.filterType"
        class="rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        @change="store.fetchAll()"
      >
        <option v-for="t in alertTypes" :key="t" :value="t">{{ alertTypeLabel(t) }}</option>
      </select>

      <div class="flex gap-1 rounded-lg border border-border p-0.5">
        <button
          v-for="s in statusOptions"
          :key="s"
          class="px-3 py-1 text-xs rounded-md transition-colors capitalize"
          :class="store.filterStatus === s
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'"
          @click="store.filterStatus = s; store.fetchAll()"
        >
          {{ s }}
        </button>
      </div>
    </div>

    <!-- Alert Table -->
    <div class="flex-1 overflow-auto p-4">
      <div v-if="store.loading" class="flex items-center justify-center h-64 text-muted-foreground">
        Loading...
      </div>
      <div v-else-if="store.error" class="flex items-center justify-center h-64 text-destructive">
        {{ store.error }}
      </div>
      <div v-else-if="!store.filteredItems.length" class="flex items-center justify-center h-64 text-muted-foreground">
        No alerts found
      </div>
      <table v-else class="w-full text-sm">
        <thead>
          <tr class="text-muted-foreground text-left border-b border-border">
            <th class="pb-2 font-medium">Time</th>
            <th class="pb-2 font-medium">Link</th>
            <th class="pb-2 font-medium">Type</th>
            <th class="pb-2 font-medium">Status</th>
            <th class="pb-2 font-medium">Duration</th>
            <th class="pb-2 font-medium">Message</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="alert in store.filteredItems"
            :key="alert.id"
            class="border-t border-border/50 hover:bg-muted/50 cursor-pointer"
            @click="goToLink(alert.link_id)"
          >
            <td class="py-2 text-xs text-muted-foreground">{{ new Date(alert.triggered_at).toLocaleString() }}</td>
            <td class="py-2 text-xs font-mono">{{ alert.link_id }}</td>
            <td class="py-2">
              <span class="text-xs px-1.5 py-0.5 rounded bg-muted">{{ alert.type }}</span>
            </td>
            <td class="py-2">
              <span class="text-xs px-1.5 py-0.5 rounded" :class="statusBadge(alert)">
                {{ alert.resolved_at ? 'Resolved' : 'Open' }}
              </span>
            </td>
            <td class="py-2 text-xs text-muted-foreground">{{ duration(alert) }}</td>
            <td class="py-2 text-xs">{{ alert.message }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>
