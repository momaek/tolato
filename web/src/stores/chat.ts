import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getConversations, getConversation, createConversation, deleteConversation } from '@/services/api'
import { wsService } from '@/services/ws'
import type { ConversationSummary, CreateConversationRequest, MessageItem, MessageSegment } from '@/types/api'
import type { WSMessage, WSReasoningEvent, WSContentEvent, WSToolCallEvent, WSToolResultEvent, WSConfirmRequestEvent, WSErrorEvent } from '@/types/ws'
import { WS_TYPE } from '@/types/ws'

export type ConversationStatus = 'idle' | 'streaming' | 'tool_exec' | 'confirming' | 'error'

export interface StreamingAssistant {
  /**
   * Stable id reused when the streaming turn is committed to `messages`. Lets
   * the renderer key the same AssistantMessage instance across the handoff so
   * markdown isn't re-parsed (no flash of re-hydrated content).
   */
  id: string
  reasoning: string
  /**
   * Segments in the order events arrived on the wire. A single agent turn may
   * span multiple LLM rounds (content → tool_call → content → …), so we can't
   * flatten into separate `content` and `toolCalls` buckets without losing
   * the interleaving.
   */
  segments: MessageSegment[]
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

  // Conversations whose next loadConversation() should be skipped. Set by
  // addConversation(): we already seeded local state from the create response,
  // and the server has no messages yet — letting the route-watcher's fetch run
  // would race the first sendMessage() and overwrite state.messages = [],
  // eating the user's first message.
  const skipNextLoad = new Set<string>()

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
      ensureStreaming(state)
      state.streaming!.reasoning += payload.delta
      state.status = 'streaming'
    })

    wsService.on(WS_TYPE.CONTENT, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSContentEvent
      const state = getOrCreateState(convId)
      ensureStreaming(state)
      const segs = state.streaming!.segments
      const last = segs[segs.length - 1]
      // Coalesce consecutive content deltas into the last content segment so
      // markdown parses as one block; start a fresh segment after a tool_call.
      if (last && last.type === 'content') {
        last.text += payload.delta
      } else {
        segs.push({ type: 'content', text: payload.delta })
      }
      state.status = 'streaming'
    })

    wsService.on(WS_TYPE.TOOL_CALL, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSToolCallEvent
      const state = getOrCreateState(convId)
      ensureStreaming(state)
      state.streaming!.segments.push({
        type: 'tool_call',
        toolCall: { id: payload.id, tool: payload.tool, args: payload.args },
      })
      state.status = 'tool_exec'
    })

    wsService.on(WS_TYPE.TOOL_RESULT, (msg: WSMessage) => {
      const convId = msg.conversation_id
      if (!convId || !msg.payload) return
      const payload = msg.payload as WSToolResultEvent
      const state = getOrCreateState(convId)
      if (!state.streaming) return
      for (const seg of state.streaming.segments) {
        if (seg.type === 'tool_call' && seg.toolCall.id === payload.id) {
          seg.toolCall.result = payload.result as any
          break
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
      // Note: do NOT touch state.error here. On some paths (e.g. blacklisted
      // tool call) the loop emits ERROR *before* DONE; the error must survive
      // until the next user message clears it.
      // Finalize streaming: keep the interleaved segments on the message so the
      // renderer preserves chronological order (content → tool_call → content).
      if (state.streaming) {
        const { id, reasoning, segments } = state.streaming
        // Drop trailing empty content segment if any.
        const cleaned = segments.filter((s) => s.type !== 'content' || s.text.length > 0)
        if (cleaned.length > 0 || reasoning) {
          const assistantMsg: MessageItem = {
            // Reuse the streaming id so ChatMessages can key the same
            // AssistantMessage across the streaming → finalized swap.
            id,
            role: 'assistant',
            reasoning: reasoning || undefined,
            segments: cleaned,
            created_at: new Date().toISOString(),
          }
          state.messages.push(assistantMsg)
        }
        state.streaming = null
      }
      state.confirmRequest = null
      if (state.status !== 'error') {
        state.status = 'idle'
      }
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

  function ensureStreaming(state: ConversationState) {
    if (!state.streaming) {
      state.streaming = { id: crypto.randomUUID(), reasoning: '', segments: [] }
    }
  }

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
    if (skipNextLoad.has(id)) {
      skipNextLoad.delete(id)
      return
    }
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
    // Seed local state so the first sendMessage() carries the selected model
    // and node. Without this, sendMessage races loadConversation's network
    // round-trip and ships default_node_id: undefined — the AI then can't tell
    // which node "this machine" refers to.
    const state = getOrCreateState(conv.id)
    state.title = conv.title
    state.model = conv.model || data.model || ''
    state.defaultNodeId = data.default_node_id ?? undefined
    skipNextLoad.add(conv.id)
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

    // Reject if a turn is already in flight. Without this, double-Enter or a
    // racey UI lets two user_message frames hit the backend, which overwrites
    // the loop registry — the first runner becomes a zombie that interleaves
    // events with the second one's stream.
    if (state.status !== 'idle' && state.status !== 'error') return

    // Check WS connection before sending
    if (wsService.state !== 'connected') {
      state.error = 'WebSocket is not connected. Please wait for reconnection.'
      state.status = 'error'
      return
    }

    // New turn → clear any error from a previous turn so the banner goes away.
    state.error = null

    // Add user message to local state
    state.messages.push({
      id: crypto.randomUUID(),
      role: 'user',
      content,
      created_at: new Date().toISOString(),
    })
    state.status = 'streaming'

    const ok = wsService.send({
      type: WS_TYPE.USER_MESSAGE,
      conversation_id: convId,
      payload: {
        content,
        model: state.model || undefined,
        default_node_id: state.defaultNodeId || undefined,
      },
    })
    if (!ok) {
      // wsService.send no-ops if the socket flipped to non-OPEN between our
      // state check and the actual send. Roll back so the UI doesn't wedge in
      // 'streaming' forever waiting for events that will never arrive.
      state.messages.pop()
      state.status = 'error'
      state.error = 'Failed to send: connection lost. Please retry.'
    }
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
