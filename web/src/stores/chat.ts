import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getConversations, getConversation, createConversation, deleteConversation } from '@/services/api'
import { wsService } from '@/services/ws'
import type { ConversationSummary, CreateConversationRequest, MessageItem, ToolCallItem } from '@/types/api'
import type { WSMessage, WSReasoningEvent, WSContentEvent, WSToolCallEvent, WSToolResultEvent, WSConfirmRequestEvent, WSErrorEvent } from '@/types/ws'
import { WS_TYPE } from '@/types/ws'

export type ConversationStatus = 'idle' | 'streaming' | 'tool_exec' | 'confirming' | 'error'

export interface StreamingAssistant {
  reasoning: string
  content: string
  toolCalls: ToolCallItem[]
}

export interface ConfirmRequest {
  id: string
  tool: string
  args: Record<string, unknown>
}

export interface ConversationState {
  id: string
  title: string
  model: string
  defaultNodeId?: string
  messages: MessageItem[]
  streaming: StreamingAssistant | null
  status: ConversationStatus
  confirmRequest: ConfirmRequest | null
  error: string | null
}

export const useChatStore = defineStore('chat', () => {
  const conversations = ref<ConversationSummary[]>([])
  const activeConversationId = ref<string | null>(null)
  const loading = ref(false)

  // Per-conversation state
  const conversationStates = ref<Map<string, ConversationState>>(new Map())

  const activeState = computed<ConversationState | null>(() => {
    if (!activeConversationId.value) return null
    return conversationStates.value.get(activeConversationId.value) || null
  })

  // Register WS event handlers exactly once (store is a Pinia singleton, so this
  // runs on first useChatStore() call — no view needs to trigger it).
  function registerWSHandlers() {
    wsService.on(WS_TYPE.REASONING, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSReasoningEvent
      const state = getOrCreateState(convId)
      if (!state.streaming) {
        state.streaming = { reasoning: '', content: '', toolCalls: [] }
      }
      state.streaming.reasoning += payload.delta
      state.status = 'streaming'
    })

    wsService.on(WS_TYPE.CONTENT, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSContentEvent
      const state = getOrCreateState(convId)
      if (!state.streaming) {
        state.streaming = { reasoning: '', content: '', toolCalls: [] }
      }
      state.streaming.content += payload.delta
      state.status = 'streaming'
    })

    wsService.on(WS_TYPE.TOOL_CALL, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSToolCallEvent
      const state = getOrCreateState(convId)
      if (!state.streaming) {
        state.streaming = { reasoning: '', content: '', toolCalls: [] }
      }
      state.streaming.toolCalls.push({
        id: payload.id,
        tool: payload.tool,
        args: payload.args,
      })
      state.status = 'tool_exec'
    })

    wsService.on(WS_TYPE.TOOL_RESULT, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSToolResultEvent
      const state = getOrCreateState(convId)
      if (state.streaming) {
        const tc = state.streaming.toolCalls.find((t) => t.id === payload.id)
        if (tc) {
          tc.result = payload.result as any
        }
      }
    })

    wsService.on(WS_TYPE.CONFIRM_REQUEST, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSConfirmRequestEvent
      const state = getOrCreateState(convId)
      state.confirmRequest = {
        id: payload.id,
        tool: payload.tool,
        args: payload.args,
      }
      state.status = 'confirming'
    })

    wsService.on(WS_TYPE.DONE, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId) return
      const state = getOrCreateState(convId)
      // Finalize streaming message into messages list
      if (state.streaming) {
        const assistantMsg: MessageItem = {
          id: crypto.randomUUID(),
          role: 'assistant',
          content: state.streaming.content || undefined,
          reasoning: state.streaming.reasoning || undefined,
          tool_calls: state.streaming.toolCalls.length > 0 ? state.streaming.toolCalls : undefined,
          created_at: new Date().toISOString(),
        }
        state.messages.push(assistantMsg)
        state.streaming = null
      }
      state.confirmRequest = null
      state.status = 'idle'
      state.error = null
    })

    wsService.on(WS_TYPE.ERROR, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId) return
      const payload = msg.payload as WSErrorEvent | undefined
      const state = getOrCreateState(convId)
      state.error = payload?.message ?? 'Unknown error'
      state.status = 'error'
      state.streaming = null
    })
  }

  // On reconnect, any in-flight streaming on the old socket is dead: the server
  // may have finished (or errored) during the outage and we missed the events.
  // Clear stale streaming state and re-fetch the active conversation from the
  // server, which has the authoritative final messages.
  let wasConnected = false
  function watchConnectionRecovery() {
    wsService.onStateChange((s) => {
      if (s === 'connected') {
        if (wasConnected) {
          // reconnect — recover
          for (const st of conversationStates.value.values()) {
            if (st.streaming || st.status === 'streaming' || st.status === 'tool_exec') {
              st.streaming = null
              st.confirmRequest = null
              st.status = 'idle'
              loadConversation(st.id)
            }
          }
        }
        wasConnected = true
      }
    })
  }

  registerWSHandlers()
  watchConnectionRecovery()

  function getOrCreateState(convId: string): ConversationState {
    let state = conversationStates.value.get(convId)
    if (!state) {
      state = {
        id: convId,
        title: '',
        model: '',
        messages: [],
        streaming: null,
        status: 'idle',
        confirmRequest: null,
        error: null,
      }
      conversationStates.value.set(convId, state)
    }
    return state
  }

  async function fetchConversations() {
    loading.value = true
    try {
      conversations.value = await getConversations()
    } catch {
      // silently fail
    } finally {
      loading.value = false
    }
  }

  async function loadConversation(id: string) {
    try {
      const detail = await getConversation(id)
      const state = getOrCreateState(id)
      state.title = detail.title
      state.model = detail.model
      state.defaultNodeId = detail.default_node_id ?? undefined
      state.messages = detail.messages || []
    } catch {
      // silently fail
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
    conversationStates.value.delete(id)
    if (activeConversationId.value === id) {
      activeConversationId.value = null
    }
  }

  function setActive(id: string | null) {
    activeConversationId.value = id
  }

  function sendMessage(content: string) {
    const convId = activeConversationId.value
    if (!convId) return

    const state = getOrCreateState(convId)

    // Check WS connection before sending
    if (wsService.state !== 'connected') {
      state.error = 'WebSocket is not connected. Please wait for reconnection.'
      state.status = 'error'
      return
    }

    // Add user message to local state
    state.messages.push({
      id: crypto.randomUUID(),
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    })
    state.status = 'streaming'

    // Send via WebSocket
    wsService.send({
      type: WS_TYPE.USER_MESSAGE,
      conversation_id: convId,
      payload: {
        content,
        model: state.model || undefined,
        default_node_id: state.defaultNodeId || undefined,
      },
    })
  }

  function confirmAction(id: string, approved: boolean) {
    const convId = activeConversationId.value
    if (!convId) return

    const state = getOrCreateState(convId)
    state.confirmRequest = null
    if (approved) {
      state.status = 'tool_exec'
    }

    wsService.send({
      type: WS_TYPE.CONFIRM_RESPONSE,
      conversation_id: convId,
      payload: { id, approved },
    })
  }

  return {
    conversations,
    activeConversationId,
    activeState,
    loading,
    conversationStates,
    fetchConversations,
    loadConversation,
    addConversation,
    removeConversation,
    setActive,
    sendMessage,
    confirmAction,
  }
})
