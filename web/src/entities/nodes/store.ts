import { defineStore } from "pinia"
import type { NodeSummary, UiWsEvent } from "@/shared/api/contracts"

export const useNodesStore = defineStore("nodes", {
  state: () => ({
    byId: {} as Record<string, NodeSummary>,
    order: [] as string[],
    selectedNodeId: null as string | null,
  }),
  getters: {
    items: (state) => state.order.map((id) => state.byId[id]).filter(Boolean),
    selectedNode(state) {
      return state.selectedNodeId ? state.byId[state.selectedNodeId] ?? null : null
    },
    onlineCount(): number {
      return this.items.filter((node) => node.status === "online").length
    },
    offlineCount(): number {
      return this.items.filter((node) => node.status === "offline").length
    },
  },
  actions: {
    setNodes(nodes: NodeSummary[]) {
      this.byId = Object.fromEntries(nodes.map((node) => [node.id, node]))
      this.order = nodes.map((node) => node.id)
      this.selectedNodeId ??= nodes[0]?.id ?? null
    },
    selectNode(nodeId: string | null) {
      this.selectedNodeId = nodeId
    },
    consumeWsEvent(event: UiWsEvent) {
      if (event.type === "node.updated") {
        this.byId[event.node.id] = event.node
        if (!this.order.includes(event.node.id)) {
          this.order.push(event.node.id)
        }
      }
    },
  },
})
