<script setup lang="ts">
import { storeToRefs } from "pinia"
import NodeStatusBadge from "@/features/nodes/NodeStatusBadge.vue"
import { useConsoleStore } from "@/entities/console/store"
import { useNodesStore } from "@/entities/nodes/store"
import { Button, Card, CardContent, CardHeader, CardTitle } from "@/shared/ui"
import { formatDateTime } from "@/shared/lib/format"

const consoleStore = useConsoleStore()
const nodesStore = useNodesStore()
const { items, onlineCount, offlineCount } = storeToRefs(nodesStore)
const { targetNodeId } = storeToRefs(consoleStore)

const selectNode = (nodeId: string) => {
  consoleStore.setTargetNodeId(nodeId)
  nodesStore.selectNode(nodeId === "all" ? null : nodeId)
}
</script>

<template>
  <Card class="h-full border-none shadow-sm">
    <CardHeader class="space-y-2">
      <p class="text-xs font-semibold uppercase tracking-[0.18em] text-muted-foreground">
        Workspace
      </p>
      <CardTitle class="text-2xl font-semibold">Nodes</CardTitle>
      <p class="text-sm text-muted-foreground">
        {{ onlineCount }} online · {{ offlineCount }} offline
      </p>
    </CardHeader>
    <CardContent class="space-y-3">
      <button
        class="w-full rounded-2xl border border-border bg-muted p-4 text-left transition hover:border-primary/30"
        :class="targetNodeId === 'all' ? 'border-primary/40 bg-secondary' : ''"
        type="button"
        @click="selectNode('all')"
      >
        <p class="text-sm font-semibold text-foreground">All nodes</p>
        <p class="text-sm text-muted-foreground">Broadcast · {{ onlineCount }} active</p>
      </button>

      <div class="space-y-2">
        <button
          v-for="node in items"
          :key="node.id"
          class="w-full rounded-2xl border border-border bg-card p-4 text-left transition hover:border-primary/30"
          :class="targetNodeId === node.id ? 'border-primary/40 bg-secondary/40' : ''"
          type="button"
          @click="selectNode(node.id)"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="space-y-1">
              <p class="font-semibold text-foreground">{{ node.hostname }}</p>
              <p class="text-sm text-muted-foreground">{{ node.region }} · {{ node.os }}</p>
              <p class="text-xs text-muted-foreground">
                Last seen {{ formatDateTime(node.lastSeen) }}
              </p>
            </div>
            <NodeStatusBadge :status="node.status" :busy="node.busy" />
          </div>
          <div class="mt-3 flex flex-wrap gap-2">
            <span
              v-for="tag in node.tags"
              :key="tag"
              class="rounded-full bg-muted px-2 py-1 text-xs text-muted-foreground"
            >
              {{ tag }}
            </span>
          </div>
        </button>
      </div>

      <Button class="w-full justify-start rounded-2xl" variant="outline">
        Read-only broadcast enforced
      </Button>
    </CardContent>
  </Card>
</template>
