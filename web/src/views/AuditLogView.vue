<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter, type LocationQuery } from 'vue-router'
import { Search, ChevronLeft, ChevronRight, ChevronDown, X } from 'lucide-vue-next'
import { toast } from 'vue-sonner'
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

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const nodesStore = useNodesStore()

const logs = ref<AuditLogItem[]>([])
const loading = ref(false)
const page = ref(1)
const totalPages = ref(1)
const total = ref(0)
const expandedRows = ref<Set<number>>(new Set())

const searchQuery = ref('')
const nodeFilter = ref('all')

// Filters that come from a link rather than the header controls: an API
// caller gets the audit row id back from execute and its own metadata tags,
// and links here with ?id=123 or ?metadata.ticket=T-42.
const idFilter = ref<number>()
const metadataFilter = ref<Record<string, string>>({})
const METADATA_PREFIX = 'metadata.'

const linkFilters = computed(() => [
  ...(idFilter.value !== undefined ? [{ key: 'id', label: `#${idFilter.value}` }] : []),
  ...Object.entries(metadataFilter.value).map(([k, v]) => ({ key: METADATA_PREFIX + k, label: `${k}=${v}` })),
])

function readLinkFilters(query: LocationQuery) {
  const first = (v: LocationQuery[string]) => (Array.isArray(v) ? v[0] : v) ?? undefined
  const id = Number(first(query.id))
  idFilter.value = Number.isInteger(id) && id > 0 ? id : undefined
  const meta: Record<string, string> = {}
  for (const [k, v] of Object.entries(query)) {
    const value = first(v)
    if (k.startsWith(METADATA_PREFIX) && value != null) meta[k.slice(METADATA_PREFIX.length)] = value
  }
  metadataFilter.value = meta
}

// The URL is the source of truth for link filters, so the filtered view can
// itself be copied and shared.
function writeLinkFilters() {
  const query: Record<string, string> = {}
  if (idFilter.value !== undefined) query.id = String(idFilter.value)
  for (const [k, v] of Object.entries(metadataFilter.value)) query[METADATA_PREFIX + k] = v
  router.replace({ query })
}

function addMetadataFilter(key: string, value: string) {
  metadataFilter.value = { ...metadataFilter.value, [key]: value }
  writeLinkFilters()
}

function removeLinkFilter(key: string) {
  if (key === 'id') {
    idFilter.value = undefined
  } else {
    const { [key.slice(METADATA_PREFIX.length)]: _, ...rest } = metadataFilter.value
    metadataFilter.value = rest
  }
  writeLinkFilters()
}

onMounted(() => {
  nodesStore.fetchNodes()
  readLinkFilters(route.query)
  fetchLogs()
})

watch([searchQuery, nodeFilter], () => {
  page.value = 1
  fetchLogs()
})

watch(() => route.query, (query) => {
  if (route.name !== 'audit') return // navigating away
  readLinkFilters(query)
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
      id: idFilter.value,
      metadata: metadataFilter.value,
    })
    logs.value = res.items
    totalPages.value = res.total_pages
    total.value = res.total
    expandedRows.value.clear()
    // A link to one row is a request to see it, output included.
    if (idFilter.value !== undefined && res.items.length === 1) {
      expandedRows.value.add(res.items[0].id)
    }
  } catch {
    toast.error(t('auditLog.failedToLoad'))
  } finally {
    loading.value = false
  }
}

function toggleRow(id: number) {
  if (expandedRows.value.has(id)) {
    expandedRows.value.delete(id)
  } else {
    expandedRows.value.add(id)
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
      <h1 class="text-lg font-semibold">{{ $t('auditLog.title') }}</h1>
      <Badge variant="secondary">{{ total }}</Badge>
      <div class="flex-1" />
      <Select v-model="nodeFilter">
        <SelectTrigger class="w-[160px]">
          <SelectValue :placeholder="$t('auditLog.allNodes')" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('auditLog.allNodes') }}</SelectItem>
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
          :placeholder="$t('auditLog.searchPlaceholder')"
          class="pl-9"
        />
      </div>
    </div>

    <!-- Filters that arrived by link -->
    <div v-if="linkFilters.length" class="flex flex-wrap items-center gap-2 border-b px-6 py-2">
      <Badge
        v-for="f in linkFilters"
        :key="f.key"
        variant="secondary"
        class="gap-1 font-mono text-xs"
      >
        {{ f.label }}
        <button
          type="button"
          class="rounded-sm opacity-60 hover:opacity-100"
          :aria-label="$t('auditLog.removeFilter')"
          @click="removeLinkFilter(f.key)"
        >
          <X class="h-3 w-3" />
        </button>
      </Badge>
    </div>

    <!-- Table -->
    <div class="flex-1 overflow-auto px-6 py-4">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead class="w-8" />
            <TableHead>{{ $t('common.time') }}</TableHead>
            <TableHead>{{ $t('auditLog.node') }}</TableHead>
            <TableHead>{{ $t('auditLog.actor') }}</TableHead>
            <TableHead>{{ $t('auditLog.command') }}</TableHead>
            <TableHead>{{ $t('common.status') }}</TableHead>
            <TableHead>{{ $t('auditLog.confirmed') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <template v-for="log in logs" :key="log.id">
            <TableRow
              class="cursor-pointer"
              @click="toggleRow(log.id)"
            >
              <TableCell class="w-8 px-2">
                <ChevronDown
                  class="h-3.5 w-3.5 transition-transform"
                  :class="{ 'rotate-[-90deg]': !expandedRows.has(log.id) }"
                  style="color: var(--muted-foreground)"
                />
              </TableCell>
              <TableCell class="whitespace-nowrap text-sm">
                {{ formatTime(log.created_at) }}
              </TableCell>
              <TableCell>{{ log.node_name }}</TableCell>
              <TableCell class="whitespace-nowrap text-sm">
                <span v-if="log.actor">{{ log.actor }}</span>
                <span v-else style="color: var(--muted-foreground)">—</span>
                <Badge v-if="log.source !== 'webui'" variant="outline" class="ml-1.5 text-[10px]">
                  {{ log.source }}
                </Badge>
              </TableCell>
              <TableCell class="max-w-md">
                <div class="truncate font-mono text-sm">{{ log.command }}</div>
                <div v-if="log.metadata" class="mt-1 flex flex-wrap gap-1">
                  <button
                    v-for="(value, key) in log.metadata"
                    :key="key"
                    type="button"
                    class="max-w-full truncate rounded border px-1.5 py-0.5 font-mono text-[10px] hover:bg-[var(--secondary)]"
                    style="color: var(--muted-foreground)"
                    :title="$t('auditLog.filterByMetadata')"
                    @click.stop="addMetadataFilter(String(key), value)"
                  >
                    {{ key }}={{ value }}
                  </button>
                </div>
              </TableCell>
              <TableCell>
                <Badge
                  :style="{
                    backgroundColor: log.exit_code === 0 ? 'var(--color-success)' : 'var(--color-error)',
                    color: log.exit_code === 0 ? 'var(--color-success-foreground)' : 'var(--color-error-foreground)',
                  }"
                >
                  {{ log.exit_code !== undefined ? `exit: ${log.exit_code}` : t('common.pending') }}
                </Badge>
                <Badge v-if="log.output_redacted" variant="outline" class="ml-1.5 text-[10px]" :title="$t('auditLog.outputRedacted')">
                  {{ $t('auditLog.redacted') }}
                </Badge>
              </TableCell>
              <TableCell>
                <Badge variant="outline">
                  {{ log.confirmed ? t('common.yes') : t('common.no') }}
                </Badge>
              </TableCell>
            </TableRow>
            <!-- Expanded output -->
            <TableRow v-if="expandedRows.has(log.id) && (log.stdout || log.stderr || log.output_redacted)">
              <TableCell :colspan="7" class="p-0">
                <div class="mx-6 my-2 rounded-lg p-3 text-xs font-mono" style="background-color: var(--secondary)">
                  <div v-if="log.stdout" class="whitespace-pre-wrap break-all" style="color: var(--foreground)">
                    <div class="mb-1 text-[10px] font-sans font-medium" style="color: var(--muted-foreground)">{{ $t('auditLog.stdout') }}</div>
                    {{ log.stdout }}
                  </div>
                  <div v-if="log.stderr" class="mt-2 whitespace-pre-wrap break-all" style="color: var(--color-error-foreground)">
                    <div class="mb-1 text-[10px] font-sans font-medium" style="color: var(--muted-foreground)">{{ $t('auditLog.stderr') }}</div>
                    {{ log.stderr }}
                  </div>
                  <div v-if="log.output_redacted">
                    <div class="mb-1 font-sans" style="color: var(--muted-foreground)">{{ $t('auditLog.outputRedacted') }}</div>
                    <div v-if="log.stdout_digest" class="break-all" style="color: var(--foreground)">
                      <span class="font-sans" style="color: var(--muted-foreground)">{{ $t('auditLog.stdout') }}</span>
                      {{ $t('auditLog.digest', log.stdout_digest) }}
                    </div>
                    <div v-if="log.stderr_digest" class="break-all" style="color: var(--foreground)">
                      <span class="font-sans" style="color: var(--muted-foreground)">{{ $t('auditLog.stderr') }}</span>
                      {{ $t('auditLog.digest', log.stderr_digest) }}
                    </div>
                  </div>
                  <div
                    v-if="log.duration_ms"
                    class="mt-2 text-[10px] font-sans"
                    style="color: var(--muted-foreground)"
                  >
                    {{ $t('auditLog.durationMs', { ms: log.duration_ms }) }}
                  </div>
                </div>
              </TableCell>
            </TableRow>
          </template>
          <TableRow v-if="logs.length === 0">
            <TableCell :colspan="7" class="py-8 text-center" style="color: var(--muted-foreground)">
              {{ loading ? $t('common.loading') : $t('auditLog.noLogs') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>

    <!-- Pagination -->
    <div class="flex items-center justify-between border-t px-6 py-3">
      <span class="text-sm" style="color: var(--muted-foreground)">
        {{ $t('common.page', { current: page, total: totalPages }) }}
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
