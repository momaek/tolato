import { defineStore } from 'pinia'

import type { SessionListItem } from '@/shared/types/console'
import { getWSClient } from '@/shared/ws/ws-client'

function sortSessions(items: SessionListItem[]) {
  items.sort(
    (left, right) => +new Date(right.updatedAt) - +new Date(left.updatedAt),
  )
}

export const useConsoleSessionListStore = defineStore('console-session-list', {
  state: () => ({
    sessions: [] as SessionListItem[],
    activeSessionId: '' as string,
    initialized: false,
  }),
  getters: {
    activeSession(state) {
      return (
        state.sessions.find(
          (session) => session.id === state.activeSessionId,
        ) ?? null
      )
    },
  },
  actions: {
    async refreshSessions() {
      this.sessions = await getWSClient().requestSessionsList()
      sortSessions(this.sessions)
      if (
        !this.sessions.some((session) => session.id === this.activeSessionId)
      ) {
        this.activeSessionId = this.sessions[0]?.id ?? ''
      }
      return this.sessions
    },
    async initialize() {
      if (this.initialized) {
        return
      }

      const client = getWSClient()
      client.subscribe((event) => {
        if (event.type === 'sessions.replaced') {
          this.sessions = [...event.sessions]
          sortSessions(this.sessions)
          if (
            !this.sessions.some(
              (session) => session.id === this.activeSessionId,
            )
          ) {
            this.activeSessionId = this.sessions[0]?.id ?? ''
          }
        }

        if (event.type === 'session.summary.updated') {
          const index = this.sessions.findIndex(
            (session) => session.id === event.session.id,
          )
          if (index === -1) {
            this.sessions.unshift({
              ...event.session,
              unread:
                event.session.id === this.activeSessionId
                  ? 0
                  : Math.max(1, event.session.unread),
            })
          } else {
            const current = this.sessions[index]
            const unread =
              event.session.id === this.activeSessionId
                ? 0
                : event.session.updatedAt !== current.updatedAt
                  ? Math.max(current.unread + 1, event.session.unread)
                  : Math.max(current.unread, event.session.unread)
            this.sessions.splice(index, 1, {
              ...event.session,
              unread,
            })
          }
          sortSessions(this.sessions)
        }

        if (event.type === 'session.requires_attention') {
          const session = this.sessions.find(
            (item) => item.id === event.sessionId,
          )
          if (session) {
            session.status = 'attention'
          }
        }

        if (event.type === 'session.unread.updated') {
          const session = this.sessions.find(
            (item) => item.id === event.sessionId,
          )
          if (session) {
            session.unread = event.unread
          }
        }
      })

      await this.refreshSessions()
      this.initialized = true
    },
    async createSession(title = '') {
      const { sessionId } = await getWSClient().createSession({
        type: 'session.create',
        title,
      })
      await this.refreshSessions()
      this.activeSessionId = sessionId
      return sessionId
    },
    async deleteSession(sessionId: string) {
      await getWSClient().deleteSession({
        type: 'session.delete',
        sessionId,
      })
      await this.refreshSessions()
      return this.activeSessionId
    },
    setActiveSession(sessionId: string) {
      this.activeSessionId = sessionId
      const session = this.sessions.find((item) => item.id === sessionId)
      if (session) {
        session.unread = 0
      }
    },
  },
})
