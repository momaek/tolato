import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getNodes, createNode, deleteNode, updateNode } from '@/services/api'
import type { NodeListItem, CreateNodeRequest, CreateNodeResponse } from '@/types/api'

export const useNodesStore = defineStore('nodes', () => {
  const nodes = ref<NodeListItem[]>([])
  const loading = ref(false)

  async function fetchNodes() {
    loading.value = true
    try {
      nodes.value = await getNodes()
    } catch {
      // silently fail for now
    } finally {
      loading.value = false
    }
  }

  async function addNode(data: CreateNodeRequest): Promise<CreateNodeResponse> {
    const res = await createNode(data)
    await fetchNodes()
    return res
  }

  async function removeNode(id: string) {
    await deleteNode(id)
    nodes.value = nodes.value.filter((n) => n.id !== id)
  }

  async function renameNode(id: string, alias: string) {
    await updateNode(id, { alias })
    const n = nodes.value.find((x) => x.id === id)
    if (n) n.alias = alias || undefined
  }

  return {
    nodes,
    loading,
    fetchNodes,
    addNode,
    removeNode,
    renameNode,
  }
})
