<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus, Search, Copy, Check } from 'lucide-vue-next'
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

const nodesStore = useNodesStore()

const searchQuery = ref('')
const statusFilter = ref('all')
const dialogOpen = ref(false)
const nodeAlias = ref('')
const createdNode = ref<CreateNodeResponse | null>(null)
const addingNode = ref(false)
const copied = ref(false)

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
  if (!confirm('Are you sure you want to remove this node?')) return
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
      <h1 class="text-lg font-semibold">Nodes</h1>
      <Badge variant="secondary">{{ nodesStore.nodes.length }}</Badge>
      <div class="flex-1" />
      <div class="relative w-64">
        <Search class="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2" style="color: var(--muted-foreground)" />
        <Input
          v-model="searchQuery"
          placeholder="Search nodes..."
          class="pl-9"
        />
      </div>
      <Select v-model="statusFilter">
        <SelectTrigger class="w-[130px]">
          <SelectValue placeholder="Status" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="all">All Status</SelectItem>
          <SelectItem value="online">Online</SelectItem>
          <SelectItem value="offline">Offline</SelectItem>
        </SelectContent>
      </Select>
      <Dialog v-model:open="dialogOpen" @update:open="resetDialog">
        <DialogTrigger as-child>
          <Button>
            <Plus class="mr-2 h-4 w-4" />
            Add Node
          </Button>
        </DialogTrigger>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Node</DialogTitle>
            <DialogDescription>
              Enter an alias for the node, then run the install command on your server.
            </DialogDescription>
          </DialogHeader>

          <template v-if="!createdNode">
            <div class="space-y-4 py-4">
              <Input v-model="nodeAlias" placeholder="Node alias (optional)" />
            </div>
            <DialogFooter>
              <Button :disabled="addingNode" @click="handleAddNode">
                {{ addingNode ? 'Creating...' : 'Generate Install Command' }}
              </Button>
            </DialogFooter>
          </template>

          <template v-else>
            <div class="space-y-4 py-4">
              <p class="text-sm" style="color: var(--muted-foreground)">
                Run this command on your server to install the agent:
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
                Token expires: {{ formatTime(createdNode.token_expiry) }}
              </p>
            </div>
            <DialogFooter>
              <Button variant="secondary" @click="dialogOpen = false">Done</Button>
            </DialogFooter>
          </template>
        </DialogContent>
      </Dialog>
    </div>

    <!-- Table -->
    <div class="flex-1 overflow-auto px-6 py-4">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>IP</TableHead>
            <TableHead>Status</TableHead>
            <TableHead>OS</TableHead>
            <TableHead>CPU</TableHead>
            <TableHead>Memory</TableHead>
            <TableHead>Disk</TableHead>
            <TableHead>Last Heartbeat</TableHead>
            <TableHead class="w-[80px]">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-for="node in filteredNodes" :key="node.id">
            <TableCell class="font-medium">
              {{ node.alias || node.name }}
            </TableCell>
            <TableCell class="font-mono text-sm">{{ node.ip }}</TableCell>
            <TableCell>
              <Badge
                :style="{
                  backgroundColor: node.status === 'online' ? 'var(--color-success)' : 'var(--color-error)',
                  color: node.status === 'online' ? 'var(--color-success-foreground)' : 'var(--color-error-foreground)',
                }"
              >
                {{ node.status }}
              </Badge>
            </TableCell>
            <TableCell>{{ node.os || '-' }}</TableCell>
            <TableCell>{{ formatPercent(node.cpu) }}</TableCell>
            <TableCell>{{ formatPercent(node.memory) }}</TableCell>
            <TableCell>{{ formatPercent(node.disk) }}</TableCell>
            <TableCell>{{ formatTime(node.last_heartbeat) }}</TableCell>
            <TableCell>
              <Button
                variant="ghost"
                size="sm"
                class="text-destructive"
                @click="handleDeleteNode(node.id)"
              >
                Remove
              </Button>
            </TableCell>
          </TableRow>
          <TableRow v-if="filteredNodes.length === 0">
            <TableCell :colspan="9" class="py-8 text-center" style="color: var(--muted-foreground)">
              {{ nodesStore.loading ? 'Loading...' : 'No nodes found' }}
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>
