<script setup lang="ts">
import { ref, onMounted, watch } from 'vue'
import { Search, ChevronLeft, ChevronRight } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { getAuditLogs } from '@/services/api'
import { useNodesStore } from '@/stores/nodes'
import type { AuditLogItem, PaginatedResponse } from '@/types/api'

const nodesStore = useNodesStore()

const logs = ref<AuditLogItem[]>([])
const loading = ref(false)
const page = ref(1)
const totalPages = ref(1)
const total = ref(0)

const searchQuery = ref('')
const nodeFilter = ref('all')

onMounted(() => {
  nodesStore.fetchNodes()
  fetchLogs()
})

watch([searchQuery, nodeFilter], () => {
  page.value = 1
  fetchLogs()
})

async function fetchLogs() {
  loading.value = true
  try {
    const res: PaginatedResponse<AuditLogItem> = await getAuditLogs({
      page: page.value,
      page_size: 20,
      node_id: nodeFilter.value === 'all' ? undefined : nodeFilter.value,
      keyword: searchQuery.value || undefined,
    })
    logs.value = res.items
    totalPages.value = res.total_pages
    total.value = res.total
  } catch {
    // TODO: toast
  } finally {
    loading.value = false
  }
}

function prevPage() {
  if (page.value > 1) {
    page.value--
    fetchLogs()
  }
}

function nextPage() {
  if (page.value < totalPages.value) {
    page.value++
    fetchLogs()
  }
}

function formatTime(iso: string) {
  return new Date(iso).toLocaleString()
}
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 border-b px-6 py-4">
      <h1 class="text-lg font-semibold">Audit Log</h1>
      <Badge variant="secondary">{{ total }}</Badge>
      <div class="flex-1" />
      <Select v-model="nodeFilter">
        <SelectTrigger class="w-[160px]">
          <SelectValue placeholder="All Nodes" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Nodes</SelectItem>
          <SelectItem
            v-for="node in nodesStore.nodes"
            :key="node.id"
            :value="node.id"
          >
            {{ node.alias || node.name }}
          </SelectItem>
        </SelectContent>
      </Select>
      <div class="relative w-64">
        <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--muted-foreground)" />
        <Input
          v-model="searchQuery"
          placeholder="Search commands..."
          class="pl-9"
        />
      </div>
    </div>

    <!-- Table -->
    <div class="flex-1 overflow-auto px-6 py-4">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Time</TableHead>
            <TableHead>Node</TableHead>
            <TableHead>Command</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>Confirmed</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="log in logs" :key="log.id">
            <TableCell class="whitespace-nowrap text-sm">
              {{ formatTime(log.created_at) }}
            </TableCell>
            <TableCell>{{ log.node_name }}</TableCell>
            <TableCell class="max-w-md truncate font-mono text-sm">
              {{ log.command }}
            </TableCell>
            <TableCell>
              <Badge
                :style="{
                  backgroundColor: log.exit_code === 0 ? 'var(--color-success)' : 'var(--color-error)',
                  color: log.exit_code === 0 ? 'var(--color-success-foreground)' : 'var(--color-error-foreground)',
                }"
              >
                {{ log.exit_code !== undefined ? `exit: ${log.exit_code}` : 'pending' }}
              </Badge>
            </TableCell>
            <TableCell>
              <Badge variant="outline">
                {{ log.confirmed ? 'Yes' : 'No' }}
              </Badge>
            </TableCell>
          </TableRow>
          <TableRow v-if="logs.length === 0">
            <TableCell :colspan="5" class="py-8 text-center" style="color: var(--muted-foreground)">
              {{ loading ? 'Loading...' : 'No audit logs found' }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <!-- Pagination -->
    <div class="flex items-center justify-between border-t px-6 py-3">
      <span class="text-sm" style="color: var(--muted-foreground)">
        Page {{ page }} of {{ totalPages }}
      </span>
      <div class="flex gap-2">
        <Button variant="outline" size="sm" :disabled="page <= 1" @click="prevPage">
          <ChevronLeft class="h-4 w-4" />
        </Button>
        <Button variant="outline" size="sm" :disabled="page >= totalPages" @click="nextPage">
          <ChevronRight class="h-4 w-4" />
        </Button>
      </div>
    </div>
  </div>
</template>
