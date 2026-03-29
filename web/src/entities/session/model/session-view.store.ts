import { defineStore } from 'pinia'

import { useConsoleSessionListStore } from '@/entities/session/model/session-list.store'
import { assembleRowsIntoTurns } from '@/entities/session/model/turn-assembler'
import type {
  AssistantTurn,
  ContentBlock,
  SessionSnapshot,
  TimelineRow,
  ToolUseBlock,
  Turn,
} from '@/shared/types/console'
import { getWSClient } from '@/shared/ws/ws-client'
import type { WSUIEvent } from '@/shared/ws/protocol'

// ── Snapshot with turns ──

interface SnapshotWithTurns extends SessionSnapshot {
  turns: Turn[]
}

function applySnapshotRevision(
  current: SnapshotWithTurns | undefined,
  incoming: SessionSnapshot,
): SnapshotWithTurns {
  if (current && incoming.revision < current.revision) {
    return current
  }
  return {
    ...incoming,
    turns: assembleRowsIntoTurns(incoming.rows),
  }
}

function isSessionRowEvent(
  event: WSUIEvent,
): event is Extract<WSUIEvent, { sessionId: string }> {
  return 'sessionId' in event
}

function readTextDelta(rawEvent: Record<string, unknown>) {
  const delta = rawEvent.delta
  if (typeof delta === 'string') return delta
  const text = rawEvent.text
  if (typeof text === 'string') return text
  return ''
}

function readString(rawEvent: Record<string, unknown>, key: string) {
  const value = rawEvent[key]
  return typeof value === 'string' ? value : ''
}

function readObject(rawEvent: Record<string, unknown>, key: string) {
  const value = rawEvent[key]
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return undefined
}

function readPendingToolName(rawEvent: Record<string, unknown>) {
  const direct = readString(rawEvent, 'name')
  if (direct) return direct
  const item = readObject(rawEvent, 'item') ?? readObject(rawEvent, 'output_item')
  if (!item) return ''
  const type = readString(item, 'type')
  if (type && type !== 'function_call') return ''
  return readString(item, 'name')
}

function readPendingToolArguments(rawEvent: Record<string, unknown>) {
  const direct = readString(rawEvent, 'arguments')
  if (direct) return direct
  const item = readObject(rawEvent, 'item') ?? readObject(rawEvent, 'output_item')
  if (!item) return ''
  return readString(item, 'arguments')
}

function mapRuntimeSessionStatus(
  status: string | undefined,
): SessionSnapshot['status'] {
  switch (status) {
    case 'running':
      return 'running'
    case 'failed':
      return 'failed'
    default:
      return 'idle'
  }
}

// ── Turn mutation helpers ──

function getOrCreateAssistantTurn(turns: Turn[], createdAt?: string): AssistantTurn {
  const last = turns[turns.length - 1]
  if (last && last.type === 'assistant') return last
  const turn: AssistantTurn = {
    type: 'assistant',
    id: `turn-${Date.now()}-${Math.random().toString(16).slice(2, 8)}`,
    createdAt: createdAt ?? new Date().toISOString(),
    status: 'streaming',
    blocks: [],
  }
  turns.push(turn)
  return turn
}

function findLastPendingToolBlock(blocks: ContentBlock[]): ToolUseBlock | undefined {
  for (let i = blocks.length - 1; i >= 0; i--) {
    const b = blocks[i]
    if (b.type === 'tool_use' && !b.result) return b
  }
  return undefined
}

function findLastTextBlock(blocks: ContentBlock[]) {
  for (let i = blocks.length - 1; i >= 0; i--) {
    if (blocks[i].type === 'text') return blocks[i] as { type: 'text'; text: string; rowId?: string }
  }
  return undefined
}

function findOrCreateThinkingBlock(blocks: ContentBlock[]) {
  const existing = blocks[0]
  if (existing?.type === 'thinking') return existing
  const block = { type: 'thinking' as const, text: '' }
  blocks.unshift(block)
  return block
}

function getOrCreateStreamingTextBlock(blocks: ContentBlock[]) {
  // Find the last text block that is still streaming (no rowId)
  const last = blocks[blocks.length - 1]
  if (last?.type === 'text' && !last.rowId) return last
  const block = { type: 'text' as const, text: '' }
  blocks.push(block)
  return block
}

// ── Store ──

export const useConsoleSessionViewStore = defineStore('console-session-view', {
  state: () => ({
    activeSessionId: '' as string,
    snapshots: {} as Record<string, SnapshotWithTurns>,
    submittingSessionIds: {} as Record<string, boolean>,
    isLoadingSnapshot: false,
    initialized: false,
  }),
  getters: {
    activeSnapshot(state) {
      return state.activeSessionId
        ? (state.snapshots[state.activeSessionId] ?? null)
        : null
    },
    activeTurns(): Turn[] {
      return this.activeSnapshot?.turns ?? []
    },
    isActiveMessageSubmitting(state) {
      return state.activeSessionId
        ? Boolean(state.submittingSessionIds[state.activeSessionId])
        : false
    },
  },
  actions: {
    async initialize() {
      if (this.initialized) return

      const client = getWSClient()
      client.subscribe((event) => {
        if (event.type === 'session.snapshot.replaced') {
          this.snapshots[event.snapshot.id] = applySnapshotRevision(
            this.snapshots[event.snapshot.id],
            event.snapshot,
          )
          return
        }

        if (
          !this.activeSessionId ||
          !isSessionRowEvent(event) ||
          event.sessionId !== this.activeSessionId
        ) {
          return
        }

        const snapshot = this.snapshots[event.sessionId]
        if (!snapshot) return

        // ── timeline.row.appended → update both rows[] and turns[] ──
        if (event.type === 'timeline.row.appended') {
          if (event.revision < snapshot.revision) return
          snapshot.revision = event.revision

          // Keep rows[] in sync (for snapshot transport compat)
          const existingIndex = snapshot.rows.findIndex((r) => r.id === event.row.id)
          if (existingIndex >= 0) {
            snapshot.rows = snapshot.rows.map((r, i) => (i === existingIndex ? event.row : r))
          } else {
            snapshot.rows = [...snapshot.rows, event.row]
          }

          // Update turns[]
          this.applyRowToTurns(snapshot, event.row)
          return
        }

        // ── timeline.row.updated ──
        if (event.type === 'timeline.row.updated') {
          if (event.revision < snapshot.revision) return
          snapshot.revision = event.revision
          const idx = snapshot.rows.findIndex((r) => r.id === event.row.id)
          if (idx >= 0) snapshot.rows.splice(idx, 1, event.row)
          return
        }

        // ── session.state.updated ──
        if (event.type === 'session.state.updated') {
          if (event.revision < snapshot.revision) return
          snapshot.revision = event.revision
          snapshot.status = mapRuntimeSessionStatus(event.status)
          if (snapshot.status === 'idle' || snapshot.status === 'failed') {
            const last = snapshot.turns[snapshot.turns.length - 1]
            if (last?.type === 'assistant') {
              last.status = 'completed'
            }
          }
          // Clear legacy llmStreamState
          if (snapshot.status === 'idle' && snapshot.llmStreamState) {
            snapshot.llmStreamState = undefined
          }
          return
        }

        // ── llm.sse.event → update turns[] directly ──
        if (event.type === 'llm.sse.event') {
          this.applySSEToTurns(snapshot, event)
          return
        }

        // ── llm.response.completed → no-op on turn status ──
        if (event.type === 'llm.response.completed') {
          // Agent loop may continue with tool calls — don't mark completed.
          return
        }
      })

      this.initialized = true
    },

    applyRowToTurns(snapshot: SnapshotWithTurns, row: TimelineRow) {
      const turns = snapshot.turns

      if (row.kind === 'user_message') {
        turns.push({
          type: 'user',
          id: row.id,
          createdAt: row.createdAt,
          text: row.text,
        })
        return
      }

      const turn = getOrCreateAssistantTurn(turns, row.createdAt)

      if (row.kind === 'assistant_text') {
        // Dedup: if last TextBlock was built from streaming (no rowId), finalize it
        const streamedText = findLastTextBlock(turn.blocks)
        if (streamedText && !streamedText.rowId) {
          streamedText.rowId = row.id
          streamedText.text = row.markdown
        } else {
          turn.blocks.push({ type: 'text', text: row.markdown, rowId: row.id })
        }
        return
      }

      if (row.kind === 'tool_call_meta') {
        const toolName = row.label.replace(/\(.*$/, '').trim()
        const argsPreview = row.label.includes('(')
          ? row.label.slice(row.label.indexOf('(') + 1, -1)
          : undefined
        turn.blocks.push({
          type: 'tool_use',
          toolName: toolName || row.label,
          argsPreview,
          callRowId: row.id,
        })
        return
      }

      if (row.kind === 'tool_result_meta') {
        const pending = findLastPendingToolBlock(turn.blocks)
        if (pending) {
          pending.result = { label: row.label, tone: row.tone, rowId: row.id }
        }
        return
      }
    },

    applySSEToTurns(
      snapshot: SnapshotWithTurns,
      event: Extract<WSUIEvent, { type: 'llm.sse.event' }>,
    ) {
      const turns = snapshot.turns
      const turn = getOrCreateAssistantTurn(turns)
      turn.status = 'streaming'
      if (event.responseId) {
        turn.responseId = event.responseId
      }

      if (event.upstreamEventType === 'response.reasoning_text.delta') {
        const block = findOrCreateThinkingBlock(turn.blocks)
        block.text += readTextDelta(event.rawEvent)
        return
      }

      if (event.upstreamEventType === 'response.output_text.delta') {
        const block = getOrCreateStreamingTextBlock(turn.blocks)
        block.text += readTextDelta(event.rawEvent)
        return
      }

      if (
        event.upstreamEventType === 'response.output_item.added' ||
        event.upstreamEventType === 'response.output_item.done'
      ) {
        const toolName = readPendingToolName(event.rawEvent)
        if (toolName) {
          turn.blocks.push({
            type: 'tool_use',
            toolName,
            argsPreview: readPendingToolArguments(event.rawEvent) || undefined,
          })
        }
        return
      }

      if (event.upstreamEventType === 'response.function_call_arguments.delta') {
        const pending = findLastPendingToolBlock(turn.blocks)
        if (pending) {
          pending.argsPreview = (pending.argsPreview ?? '') + readTextDelta(event.rawEvent)
        }
        return
      }

      if (event.upstreamEventType === 'response.function_call_arguments.done') {
        const pending = findLastPendingToolBlock(turn.blocks)
        if (pending) {
          const args = readPendingToolArguments(event.rawEvent)
          if (args) pending.argsPreview = args
          const name = readPendingToolName(event.rawEvent)
          if (name) pending.toolName = name
        }
        return
      }
    },

    async switchSession(sessionId: string) {
      if (!sessionId) return

      const client = getWSClient()
      const listStore = useConsoleSessionListStore()
      this.isLoadingSnapshot = true
      this.activeSessionId = sessionId
      const watchSessionIds = listStore.sessions
        .filter((session) => session.id !== sessionId)
        .map((session) => session.id)
      await client.updateSubscriptions({
        type: 'subscriptions.update',
        activeSessionId: sessionId,
        watchSessionIds,
      })
      const snapshot = await client.requestSessionSnapshot(sessionId)
      this.snapshots[sessionId] = applySnapshotRevision(
        this.snapshots[sessionId],
        snapshot,
      )
      this.isLoadingSnapshot = false
    },
    clearActiveSession() {
      this.activeSessionId = ''
      this.isLoadingSnapshot = false
    },
    removeSession(sessionId: string) {
      delete this.snapshots[sessionId]
      delete this.submittingSessionIds[sessionId]
      if (this.activeSessionId === sessionId) {
        this.clearActiveSession()
      }
    },
    async submitMessage(text: string) {
      if (!this.activeSessionId) {
        throw new Error('No active session selected.')
      }

      if (this.submittingSessionIds[this.activeSessionId]) {
        return
      }

      const sessionId = this.activeSessionId
      const snapshot = this.snapshots[sessionId]
      const previousStatus = snapshot?.status

      this.submittingSessionIds[sessionId] = true
      if (snapshot) {
        snapshot.status = 'running'
      }

      try {
        await getWSClient().submitMessage({
          type: 'session.message.submit',
          sessionId,
          text,
        })
      } catch (error) {
        if (snapshot && previousStatus) {
          snapshot.status = previousStatus
        }
        throw error
      } finally {
        this.submittingSessionIds[sessionId] = false
      }
    },
  },
})
