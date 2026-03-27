import { defineStore } from 'pinia'

import { useNodeDetailStore } from '@/entities/node/model/node-detail.store'
import { useNodesStore } from '@/entities/node/model/nodes.store'
import { useConnectionStore } from '@/entities/session/model/connection.store'
import { useConsoleSessionListStore } from '@/entities/session/model/session-list.store'
import { useConsoleSessionViewStore } from '@/entities/session/model/session-view.store'
import { useSettingsStore } from '@/entities/settings/model/settings.store'
import { useHistoryStore } from '@/entities/task/model/history.store'
import { t } from '@/app/i18n'
import { login } from '@/shared/api/adapters/auth'
import { clearStoredAuthSession, getAuthSession, setAuthSession } from '@/shared/auth/session'
import { appEnv } from '@/shared/config/env'
import { toErrorMessage } from '@/shared/lib/errors'
import type { AuthSession } from '@/shared/types/auth'
import { resetWSClient } from '@/shared/ws/ws-client'

function resetProtectedState() {
  resetWSClient()
  useConnectionStore().$reset()
  useConsoleSessionListStore().$reset()
  useConsoleSessionViewStore().$reset()
  useNodesStore().$reset()
  useNodeDetailStore().$reset()
  useHistoryStore().$reset()
  useSettingsStore().$reset()
}

export const useAuthStore = defineStore('auth', {
  state: () => ({
    session: getAuthSession() as AuthSession | null,
    loggingIn: false,
    error: null as string | null,
  }),
  getters: {
    isAuthenticated(state) {
      return appEnv.useMock || Boolean(state.session?.token)
    },
    userLabel(state) {
      return state.session?.userId || 'admin'
    },
  },
  actions: {
    hydrate() {
      this.session = getAuthSession()
    },
    async login(username: string, password: string) {
      this.loggingIn = true
      try {
        const session = await login({ username, password })
        setAuthSession(session)
        resetProtectedState()
        this.session = getAuthSession()
        this.error = null
        return session
      } catch (error) {
        this.error = toErrorMessage(error, t('errors.failedToSignIn'))
        return null
      } finally {
        this.loggingIn = false
      }
    },
    logout() {
      clearStoredAuthSession()
      resetProtectedState()
      this.session = getAuthSession()
      this.error = null
    },
  },
})
