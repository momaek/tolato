<script setup lang="ts">
import { ref } from "vue"
import { storeToRefs } from "pinia"
import NodeStatusBadge from "@/features/nodes/NodeStatusBadge.vue"
import { useNodesStore } from "@/entities/nodes/store"
import { Button, Card, CardContent, CardHeader, CardTitle, Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui"
import { formatDateTime, formatPercent } from "@/shared/lib/format"

const nodesStore = useNodesStore()
const { items, selectedNode } = storeToRefs(nodesStore)
const isDrawerOpen = ref(false)

const openDetails = (nodeId: string) => {
  nodesStore.selectNode(nodeId)
  isDrawerOpen.value = true
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex items-center justify-between gap-4">
      <div>
        <p class="text-sm font-semibold uppercase tracking-[0.18em] text-muted-foreground">
          Inventory
        </p>
        <h1 class="mt-2 text-3xl font-semibold tracking-tight">Nodes</h1>
      </div>
      <Button variant="outline">Node details drawer</Button>
    </div>

    <Card class="border-none shadow-sm">
      <CardHeader>
        <CardTitle>Connected agents</CardTitle>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Hostname</TableHead>
              <TableHead>Region</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>CPU</TableHead>
              <TableHead>Memory</TableHead>
              <TableHead>Disk</TableHead>
              <TableHead>Last seen</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow
              v-for="node in items"
              :key="node.id"
              class="cursor-pointer"
              @click="openDetails(node.id)"
            >
              <TableCell class="font-medium">{{ node.hostname }}</TableCell>
              <TableCell>{{ node.region }}</TableCell>
              <TableCell>
                <NodeStatusBadge :status="node.status" :busy="node.busy" />
              </TableCell>
              <TableCell>{{ formatPercent(node.metrics.cpu) }}</TableCell>
              <TableCell>{{ formatPercent(node.metrics.memory) }}</TableCell>
              <TableCell>{{ formatPercent(node.metrics.disk) }}</TableCell>
              <TableCell>{{ formatDateTime(node.lastSeen) }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  </div>

  <Sheet v-model:open="isDrawerOpen">
    <SheetContent class="w-[440px] sm:max-w-[440px]">
      <SheetHeader v-if="selectedNode">
        <SheetTitle>{{ selectedNode.hostname }}</SheetTitle>
        <SheetDescription>
          {{ selectedNode.region }} · {{ selectedNode.os }} · {{ selectedNode.version }}
        </SheetDescription>
      </SheetHeader>

      <div v-if="selectedNode" class="mt-6 space-y-4">
        <div class="rounded-2xl border border-border p-4">
          <p class="text-sm font-medium text-foreground">Status</p>
          <div class="mt-2">
            <NodeStatusBadge :status="selectedNode.status" :busy="selectedNode.busy" />
          </div>
        </div>
        <div class="grid gap-3 sm:grid-cols-3">
          <div class="rounded-2xl border border-border p-4 text-sm text-muted-foreground">
            CPU · {{ formatPercent(selectedNode.metrics.cpu) }}
          </div>
          <div class="rounded-2xl border border-border p-4 text-sm text-muted-foreground">
            Memory · {{ formatPercent(selectedNode.metrics.memory) }}
          </div>
          <div class="rounded-2xl border border-border p-4 text-sm text-muted-foreground">
            Disk · {{ formatPercent(selectedNode.metrics.disk) }}
          </div>
        </div>
        <div class="rounded-2xl border border-border p-4">
          <p class="text-sm font-medium text-foreground">Tags</p>
          <div class="mt-3 flex flex-wrap gap-2">
            <span
              v-for="tag in selectedNode.tags"
              :key="tag"
              class="rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground"
            >
              {{ tag }}
            </span>
          </div>
        </div>
      </div>
    </SheetContent>
  </Sheet>
</template>
