import { defineStore } from 'pinia'

import { useConsoleSessionListStore } from '@/entities/session/model/session-list.store'
import type {
  ApprovalRow,
  SessionSnapshot,
  TargetCandidate,
  TimelineRow,
} from '@/shared/types/console'
import { getWSClient } from '@/shared/ws/ws-client'
import type { WSUIEvent } from '@/shared/ws/protocol'

function applySnapshotRevision(
  current: SessionSnapshot | undefined,
  incoming: SessionSnapshot,
) {
  if (!current || incoming.revision >= current.revision) {
    return incoming
  }

  return current
}

function isSessionRowEvent(
  event: WSUIEvent,
): event is Extract<WSUIEvent, { sessionId: string }> {
  return 'sessionId' in event
}

function readTextDelta(rawEvent: Record<string, unknown>) {
  const delta = rawEvent.delta
  if (typeof delta === 'string') {
    return delta
  }

  const text = rawEvent.text
  if (typeof text === 'string') {
    return text
  }

  return ''
}

function readObject(rawEvent: Record<string, unknown>, key: string) {
  const value = rawEvent[key]
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>
  }
  return undefined
}

function readString(rawEvent: Record<string, unknown>, key: string) {
  const value = rawEvent[key]
  return typeof value === 'string' ? value : ''
}

function readPendingToolName(rawEvent: Record<string, unknown>) {
  const item = readObject(rawEvent, 'item') ?? readObject(rawEvent, 'output_item')
  const direct = readString(rawEvent, 'name')
  if (direct) {
    return direct
  }
  if (!item) {
    return ''
  }
  const type = readString(item, 'type')
  if (type && type !== 'function_call') {
    return ''
  }
  return readString(item, 'name')
}

function readPendingToolArguments(rawEvent: Record<string, unknown>) {
  const direct = readString(rawEvent, 'arguments')
  if (direct) {
    return direct
  }
  const item = readObject(rawEvent, 'item') ?? readObject(rawEvent, 'output_item')
  if (!item) {
    return ''
  }
  return readString(item, 'arguments')
}

function mapRuntimeSessionStatus(
  status: string | undefined,
): SessionSnapshot['status'] {
  switch (status) {
    case 'running':
    case 'waiting_async_execution':
      return 'running'
    case 'paused_wait_target_confirmation':
    case 'paused_wait_approval':
    case 'failed':
      return 'attention'
    case 'completed':
      return 'completed'
    default:
      return 'idle'
  }
}

function pendingActionTypeForStatus(
  status: string | undefined,
): SessionSnapshot['pendingActionType'] {
  switch (status) {
    case 'paused_wait_target_confirmation':
      return 'target_confirmation'
    case 'paused_wait_approval':
      return 'approval'
    default:
      return undefined
  }
}

export const useConsoleSessionViewStore = defineStore('console-session-view', {
  state: () => ({
    activeSessionId: '' as string,
    snapshots: {} as Record<string, SessionSnapshot>,
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
    activeRows(): TimelineRow[] {
      return this.activeSnapshot?.rows ?? []
    },
    isActiveMessageSubmitting(state) {
      return state.activeSessionId
        ? Boolean(state.submittingSessionIds[state.activeSessionId])
        : false
    },
  },
  actions: {
    async initialize() {
      if (this.initialized) {
        return
      }

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

        if (event.type === 'timeline.row.appended') {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot || event.revision < snapshot.revision) {
            return
          }
          if (snapshot.llmStreamState?.status === 'completed') {
            snapshot.llmStreamState = undefined
          }
          snapshot.revision = event.revision
          const existingIndex = snapshot.rows.findIndex(
            (row) => row.id === event.row.id,
          )
          if (existingIndex >= 0) {
            snapshot.rows = snapshot.rows.map((row, index) =>
              index === existingIndex ? event.row : row,
            )
            return
          }
          snapshot.rows = [...snapshot.rows, event.row]
        }

        if (
          event.type === 'execution.chunk' ||
          event.type === 'execution.finished' ||
          event.type === 'timeline.row.updated'
        ) {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot || event.revision < snapshot.revision) {
            return
          }
          snapshot.revision = event.revision
          const index = snapshot.rows.findIndex(
            (row) => row.id === event.row.id,
          )
          if (index >= 0) {
            snapshot.rows.splice(index, 1, event.row)
          }
        }

        if (event.type === 'session.state.updated') {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot || event.revision < snapshot.revision) {
            return
          }
          snapshot.revision = event.revision
          snapshot.status = mapRuntimeSessionStatus(event.status)
          snapshot.pendingActionType = pendingActionTypeForStatus(event.status)
        }

        if (
          event.type === 'thread.target.pending' ||
          event.type === 'thread.target.confirmed' ||
          event.type === 'thread.target.cleared'
        ) {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot || event.revision < snapshot.revision) {
            return
          }
          snapshot.revision = event.revision
          snapshot.targetContext = event.targetContext
          snapshot.pendingActionType =
            event.type === 'thread.target.pending'
              ? 'target_confirmation'
              : undefined
        }

        if (event.type === 'llm.sse.event') {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot) {
            return
          }

          const streamState = snapshot.llmStreamState ?? {
            responseId: event.responseId,
            status: 'streaming' as const,
            contentText: '',
            reasoningText: '',
            pendingToolName: '',
            pendingToolArguments: '',
            events: [],
          }

          streamState.responseId = event.responseId ?? streamState.responseId
          streamState.status = 'streaming'
          streamState.events.push({
            sequenceNumber: event.sequenceNumber,
            upstreamEventType: event.upstreamEventType,
            rawEvent: event.rawEvent,
          })

          if (event.upstreamEventType === 'response.output_text.delta') {
            streamState.contentText = `${streamState.contentText ?? ''}${readTextDelta(event.rawEvent)}`
          }

          if (event.upstreamEventType === 'response.reasoning_text.delta') {
            streamState.reasoningText = `${streamState.reasoningText ?? ''}${readTextDelta(event.rawEvent)}`
          }

          if (
            event.upstreamEventType === 'response.output_item.added' ||
            event.upstreamEventType === 'response.output_item.done'
          ) {
            const toolName = readPendingToolName(event.rawEvent)
            if (toolName) {
              streamState.pendingToolName = toolName
            }
            const toolArguments = readPendingToolArguments(event.rawEvent)
            if (toolArguments && !streamState.pendingToolArguments) {
              streamState.pendingToolArguments = toolArguments
            }
          }

          if (event.upstreamEventType === 'response.function_call_arguments.delta') {
            streamState.pendingToolArguments = `${streamState.pendingToolArguments ?? ''}${readTextDelta(event.rawEvent)}`
          }

          if (event.upstreamEventType === 'response.function_call_arguments.done') {
            const toolArguments = readPendingToolArguments(event.rawEvent)
            if (toolArguments) {
              streamState.pendingToolArguments = toolArguments
            }
            const toolName = readPendingToolName(event.rawEvent)
            if (toolName) {
              streamState.pendingToolName = toolName
            }
          }

          snapshot.llmStreamState = streamState
        }

        if (event.type === 'llm.response.completed') {
          const snapshot = this.snapshots[event.sessionId]
          if (!snapshot?.llmStreamState) {
            return
          }
          snapshot.llmStreamState.status = 'completed'
        }
      })

      this.initialized = true
    },
    async switchSession(sessionId: string) {
      if (!sessionId) {
        return
      }

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
    async confirmTarget(candidate?: TargetCandidate) {
      if (!this.activeSessionId) {
        return
      }
      await getWSClient().confirmTarget({
        type: 'session.target.confirm',
        sessionId: this.activeSessionId,
        action: 'confirm',
        candidate,
      })
    },
    async reselectTarget() {
      if (!this.activeSessionId) {
        return
      }
      await getWSClient().confirmTarget({
        type: 'session.target.confirm',
        sessionId: this.activeSessionId,
        action: 'reselect',
      })
    },
    async clearTargetContext() {
      if (!this.activeSessionId) {
        return
      }
      await getWSClient().confirmTarget({
        type: 'session.target.confirm',
        sessionId: this.activeSessionId,
        action: 'clear',
      })
    },
    async submitApproval(
      action: 'approve' | 'reject' | 'cancel',
      approvalRow?: ApprovalRow,
    ) {
      if (!this.activeSessionId) {
        return
      }
      await getWSClient().submitApproval({
        type: 'session.approval.action',
        sessionId: this.activeSessionId,
        action,
        approvalRow,
      })
    },
  },
})
