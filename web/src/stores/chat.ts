import { defineStore } from 'pinia'
import { ref } from 'vue'
import { getConversations, createConversation, deleteConversation } from '@/services/api'
import type { ConversationSummary, CreateConversationRequest } from '@/types/api'

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<ConversationSummary[]>([])
  const activeConversationId = ref<string | null>(null)
  const loading = ref(false)

  async function fetchConversations() {
    loading.value = true
    try {
      conversations.value = await getConversations()
    } catch {
      // silently fail for now
    } finally {
      loading.value = false
    }
  }

  async function addConversation(data: CreateConversationRequest) {
    const conv = await createConversation(data)
    conversations.value.unshift(conv)
    return conv
  }

  async function removeConversation(id: string) {
    await deleteConversation(id)
    conversations.value = conversations.value.filter((c) => c.id !== id)
    if (activeConversationId.value === id) {
      activeConversationId.value = null
    }
  }

  function setActive(id: string | null) {
    activeConversationId.value = id
  }

  return {
    conversations,
    activeConversationId,
    loading,
    fetchConversations,
    addConversation,
    removeConversation,
    setActive,
  }
})
