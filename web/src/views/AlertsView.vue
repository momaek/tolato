<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AlertTriangle } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMonitorStore } from '@/stores/monitor'

const { t } = useI18n()
const router = useRouter()
const monitorStore = useMonitorStore()

const statusFilter = ref('all')
const typeFilter = ref('all')

onMounted(() => {
  loadAlerts()
})

watch([statusFilter, typeFilter], () => {
  loadAlerts()
})

async function loadAlerts() {
  const params: any = {}
  if (statusFilter.value !== 'all') params.status = statusFilter.value
  if (typeFilter.value !== 'all') params.type = typeFilter.value
  await monitorStore.fetchAlerts(params)
}

function formatTime(ts: string) {
  return new Date(ts).toLocaleString()
}

function duration(alert: any) {
  if (alert.resolved_at) {
    const ms = new Date(alert.resolved_at).getTime() - new Date(alert.triggered_at).getTime()
    const mins = Math.floor(ms / 60000)
    if (mins < 60) return `${mins}m`
    return `${(mins / 60).toFixed(1)}h`
  }
  const ms = Date.now() - new Date(alert.triggered_at).getTime()
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return `${mins}m ${t('alerts.ongoing')}`
  return `${(mins / 60).toFixed(1)}h ${t('alerts.ongoing')}`
}
</script>

<template>
  <div class="flex h-full flex-col p-6" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 mb-6">
      <AlertTriangle class="h-5 w-5" style="color: var(--color-warning-foreground)" />
      <h1 class="text-lg font-semibold">{{ $t('alerts.title') }}</h1>
      <Badge>{{ monitorStore.alerts.length }}</Badge>
    </div>

    <!-- Filters -->
    <div class="flex gap-3 mb-4">
      <Select v-model="statusFilter">
        <SelectTrigger class="w-[160px]">
          <SelectValue :placeholder="$t('common.status')" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('common.allStatus') }}</SelectItem>
          <SelectItem value="unresolved">{{ $t('alerts.unresolved') }}</SelectItem>
          <SelectItem value="resolved">{{ $t('common.resolved') }}</SelectItem>
        </SelectContent>
      </Select>
      <Select v-model="typeFilter">
        <SelectTrigger class="w-[160px]">
          <SelectValue :placeholder="$t('common.type')" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('alerts.allTypes') }}</SelectItem>
          <SelectItem value="latency">{{ $t('alerts.latency') }}</SelectItem>
          <SelectItem value="packet_loss">{{ $t('alerts.packetLoss') }}</SelectItem>
          <SelectItem value="tcp">{{ $t('alerts.tcp') }}</SelectItem>
          <SelectItem value="bandwidth">{{ $t('alerts.bandwidth') }}</SelectItem>
          <SelectItem value="offline">{{ $t('common.offline') }}</SelectItem>
        </SelectContent>
      </Select>
    </div>

    <!-- Alert Table -->
    <div class="flex-1 overflow-y-auto">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{{ $t('common.time') }}</TableHead>
            <TableHead>{{ $t('linkDetail.link') }}</TableHead>
            <TableHead>{{ $t('common.type') }}</TableHead>
            <TableHead>{{ $t('common.status') }}</TableHead>
            <TableHead>{{ $t('common.duration') }}</TableHead>
            <TableHead>{{ $t('common.message') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow
            v-for="alert in monitorStore.alerts"
            :key="alert.id"
            class="cursor-pointer"
            @click="router.push(`/monitor/${alert.link_id}`)"
          >
            <TableCell class="text-xs">{{ formatTime(alert.triggered_at) }}</TableCell>
            <TableCell class="text-xs font-mono">{{ alert.link_id }}</TableCell>
            <TableCell><Badge variant="secondary">{{ alert.type }}</Badge></TableCell>
            <TableCell>
              <Badge :variant="alert.resolved_at ? 'default' : 'secondary'">
                {{ alert.resolved_at ? $t('common.resolved') : $t('common.active') }}
              </Badge>
            </TableCell>
            <TableCell class="text-xs">{{ duration(alert) }}</TableCell>
            <TableCell class="text-xs max-w-[200px] truncate">{{ alert.message }}</TableCell>
          </TableRow>
          <TableRow v-if="monitorStore.alerts.length === 0">
            <TableCell :colspan="6" class="text-center py-8 text-sm" style="color: var(--muted-foreground)">
              {{ $t('alerts.noAlerts') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
