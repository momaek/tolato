<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { Plus, Search, Copy, Check, Terminal as TerminalIcon, Eye, Pencil } from 'lucide-vue-next'
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
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { useNodesStore } from '@/stores/nodes'
import type { CreateNodeResponse } from '@/types/api'

const { t } = useI18n()
const router = useRouter()
const nodesStore = useNodesStore()

const searchQuery = ref('')
const statusFilter = ref('all')
const dialogOpen = ref(false)
const nodeAlias = ref('')
const createdNode = ref<CreateNodeResponse | null>(null)
const addingNode = ref(false)
const copied = ref(false)

// Inline alias editing state — only one row in edit mode at a time.
const editingId = ref<string | null>(null)
const editingDraft = ref('')
const aliasSaving = ref(false)

function flagEmoji(code?: string): string {
  if (!code || code.length !== 2) return ''
  const A = 0x1f1e6
  const c0 = code.toUpperCase().charCodeAt(0)
  const c1 = code.toUpperCase().charCodeAt(1)
  if (c0 < 65 || c0 > 90 || c1 < 65 || c1 > 90) return ''
  return String.fromCodePoint(A + c0 - 65, A + c1 - 65)
}

function startEditAlias(id: string, current?: string) {
  editingId.value = id
  editingDraft.value = current ?? ''
}

async function commitAlias() {
  if (!editingId.value) return
  const id = editingId.value
  const next = editingDraft.value.trim()
  const node = nodesStore.nodes.find((n) => n.id === id)
  if (node && (node.alias ?? '') === next) {
    editingId.value = null
    return
  }
  aliasSaving.value = true
  try {
    await nodesStore.renameNode(id, next)
    editingId.value = null
  } catch {
    alert(t('nodes.aliasSaveFailed'))
  } finally {
    aliasSaving.value = false
  }
}

function cancelEditAlias() {
  editingId.value = null
  editingDraft.value = ''
}

onMounted(() => {
  nodesStore.fetchNodes()
})

const filteredNodes = computed(() => {
  let result = nodesStore.nodes
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(
      (n) =>
        n.name.toLowerCase().includes(q) ||
        n.ip.toLowerCase().includes(q) ||
        (n.alias && n.alias.toLowerCase().includes(q))
    )
  }
  if (statusFilter.value !== 'all') {
    result = result.filter((n) => n.status === statusFilter.value)
  }
  return result
})

async function handleAddNode() {
  addingNode.value = true
  try {
    const res = await nodesStore.addNode({ alias: nodeAlias.value || undefined })
    createdNode.value = res
  } catch {
    // TODO: toast
  } finally {
    addingNode.value = false
  }
}

async function copyInstallCmd() {
  if (!createdNode.value) return
  await navigator.clipboard.writeText(createdNode.value.install_cmd)
  copied.value = true
  setTimeout(() => {
    copied.value = false
  }, 2000)
}

function resetDialog() {
  nodeAlias.value = ''
  createdNode.value = null
  copied.value = false
}

function formatPercent(val?: number) {
  if (val === undefined || val === null) return '-'
  return `${val.toFixed(1)}%`
}

function formatTime(iso?: string) {
  if (!iso) return '-'
  const d = new Date(iso)
  return d.toLocaleString()
}

async function handleDeleteNode(id: string) {
  if (!confirm(t('nodes.confirmRemove'))) return
  try {
    await nodesStore.removeNode(id)
  } catch {
    // TODO: toast
  }
}
</script>

<template>
  <div class="flex h-full flex-col" style="background-color: var(--background)">
    <!-- Header -->
    <div class="flex items-center gap-3 border-b px-6 py-4">
      <h1 class="text-lg font-semibold">{{ $t('nodes.title') }}</h1>
      <Badge variant="secondary">{{ nodesStore.nodes.length }}</Badge>
      <div class="flex-1" />
      <div class="relative w-64">
        <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--muted-foreground)" />
        <Input
          v-model="searchQuery"
          :placeholder="$t('nodes.searchPlaceholder')"
          class="pl-9"
        />
      </div>
      <Select v-model="statusFilter">
        <SelectTrigger class="w-[130px]">
          <SelectValue :placeholder="$t('common.status')" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">{{ $t('common.allStatus') }}</SelectItem>
          <SelectItem value="online">{{ $t('common.online') }}</SelectItem>
          <SelectItem value="offline">{{ $t('common.offline') }}</SelectItem>
        </SelectContent>
      </Select>
      <Dialog v-model:open="dialogOpen" @update:open="resetDialog">
        <DialogTrigger as-child>
          <Button>
            <Plus class="mr-2 h-4 w-4" />
            {{ $t('nodes.addNode') }}
          </Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{{ $t('nodes.addNode') }}</DialogTitle>
            <DialogDescription>
              {{ $t('nodes.addNodeDesc') }}
            </DialogDescription>
          </DialogHeader>

          <template v-if="!createdNode">
            <div class="space-y-4 py-4">
              <Input v-model="nodeAlias" :placeholder="$t('nodes.aliasPlaceholder')" />
            </div>
            <DialogFooter>
              <Button :disabled="addingNode" @click="handleAddNode">
                {{ addingNode ? $t('nodes.creating') : $t('nodes.generateCommand') }}
              </Button>
            </DialogFooter>
          </template>

          <template v-else>
            <div class="space-y-4 py-4">
              <p class="text-sm" style="color: var(--muted-foreground)">
                {{ $t('nodes.installInstruction') }}
              </p>
              <div
                class="relative rounded-lg p-3 font-mono text-sm"
                style="background-color: var(--secondary)"
              >
                <code class="block whitespace-pre-wrap break-all">{{ createdNode.install_cmd }}</code>
                <Button
                  variant="ghost"
                  size="icon"
                  class="absolute right-2 top-2 h-7 w-7"
                  @click="copyInstallCmd"
                >
                  <Check v-if="copied" class="h-4 w-4" style="color: var(--color-success-foreground)" />
                  <Copy v-else class="h-4 w-4" />
                </Button>
              </div>
              <p class="text-xs" style="color: var(--muted-foreground)">
                {{ createdNode.token_expiry
                  ? $t('nodes.tokenExpires', { expiry: createdNode.token_expiry })
                  : $t('nodes.tokenNeverExpires') }}
              </p>
            </div>
            <DialogFooter>
              <Button variant="secondary" @click="dialogOpen = false">{{ $t('common.done') }}</Button>
            </DialogFooter>
          </template>
        </DialogContent>
      </Dialog>
    </div>

    <!-- Table -->
    <div class="flex-1 overflow-auto px-6 py-4">
      <Table class="mb-3">
        <TableHeader>
          <TableRow>
            <TableHead>{{ $t('common.name') }}</TableHead>
            <TableHead>{{ $t('nodes.ip') }}</TableHead>
            <TableHead>{{ $t('nodes.region') }}</TableHead>
            <TableHead>{{ $t('common.status') }}</TableHead>
            <TableHead>{{ $t('nodes.os') }}</TableHead>
            <TableHead>{{ $t('nodes.cpu') }}</TableHead>
            <TableHead>{{ $t('nodes.memory') }}</TableHead>
            <TableHead>{{ $t('nodes.disk') }}</TableHead>
            <TableHead>{{ $t('nodes.lastHeartbeat') }}</TableHead>
            <TableHead class="w-[180px] text-right">{{ $t('common.actions') }}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="node in filteredNodes" :key="node.id" class="group">
            <TableCell class="font-medium">
              <div v-if="editingId === node.id" class="flex items-center gap-1">
                <input
                  v-model="editingDraft"
                  :disabled="aliasSaving"
                  autofocus
                  class="border-input h-7 w-44 rounded-md border bg-transparent px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/50"
                  @keydown.enter.prevent="commitAlias"
                  @keydown.esc.prevent="cancelEditAlias"
                  @blur="commitAlias"
                  @vue:mounted="(vnode: any) => { vnode.el?.focus(); vnode.el?.select() }"
                />
              </div>
              <div v-else class="flex items-center gap-1">
                <span>{{ node.alias || node.name }}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  class="h-6 w-6 opacity-0 group-hover:opacity-100"
                  :title="$t('nodes.editAlias')"
                  @click="startEditAlias(node.id, node.alias)"
                >
                  <Pencil class="h-3.5 w-3.5" />
                </Button>
              </div>
            </TableCell>
            <TableCell class="font-mono text-sm">{{ node.ip }}</TableCell>
            <TableCell>
              <div v-if="node.country_code" class="flex flex-col leading-tight">
                <span class="text-sm">
                  <span class="mr-1">{{ flagEmoji(node.country_code) }}</span>
                  {{ node.city || node.country_code }}
                </span>
                <span v-if="node.asn" class="text-xs" style="color: var(--muted-foreground)">
                  {{ node.asn }}
                </span>
              </div>
              <span v-else style="color: var(--muted-foreground)">-</span>
            </TableCell>
            <TableCell>
              <Badge
                :style="{
                  backgroundColor: node.status === 'online' ? 'var(--color-success)' : 'var(--color-error)',
                  color: node.status === 'online' ? 'var(--color-success-foreground)' : 'var(--color-error-foreground)',
                }"
              >
                {{ node.status === 'online' ? $t('common.online') : $t('common.offline') }}
              </Badge>
            </TableCell>
            <TableCell>{{ node.os || '-' }}</TableCell>
            <TableCell>{{ formatPercent(node.cpu) }}</TableCell>
            <TableCell>{{ formatPercent(node.memory) }}</TableCell>
            <TableCell>{{ formatPercent(node.disk) }}</TableCell>
            <TableCell>{{ formatTime(node.last_heartbeat) }}</TableCell>
            <TableCell>
              <div class="flex items-center justify-end gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  :title="$t('nodes.openTerminal')"
                  :disabled="node.status !== 'online'"
                  @click="router.push(`/nodes/${node.id}/terminal`)"
                >
                  <TerminalIcon class="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  :title="$t('nodes.viewDetail')"
                  @click="router.push(`/nodes/${node.id}`)"
                >
                  <Eye class="h-4 w-4" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="text-destructive"
                  @click="handleDeleteNode(node.id)"
                >
                  {{ $t('common.remove') }}
                </Button>
              </div>
            </TableCell>
          </TableRow>
          <TableRow v-if="filteredNodes.length === 0">
            <TableCell :colspan="10" class="py-8 text-center" style="color: var(--muted-foreground)">
              {{ nodesStore.loading ? $t('common.loading') : $t('nodes.noNodes') }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>

      <p class="px-1 text-xs" style="color: var(--muted-foreground)">
        {{ $t('nodes.geoipAttribution') }}
        <a
          href="https://www.maxmind.com"
          target="_blank"
          rel="noopener"
          class="underline underline-offset-2 hover:text-foreground"
        >MaxMind GeoLite2</a>
        ·
        <a
          href="https://github.com/P3TERX/GeoLite.mmdb"
          target="_blank"
          rel="noopener"
          class="underline underline-offset-2 hover:text-foreground"
        >P3TERX/GeoLite.mmdb</a>
      </p>
    </div>
  </div>
</template>
