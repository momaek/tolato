import { defineStore } from "pinia"
import type { SessionInfo } from "@/shared/api/contracts"
import { clearAuthToken, getAuthToken, setAuthToken } from "@/shared/api/auth-token"

export const useSessionStore = defineStore("session", {
  state: () => ({
    currentUser: null as SessionInfo | null,
    token: getAuthToken(),
  }),
  getters: {
    isAuthenticated(state) {
      return Boolean(state.token)
    },
  },
  actions: {
    setSession(session: SessionInfo, token?: string) {
      this.currentUser = session
      if (typeof token === "string") {
        this.token = token
        setAuthToken(token)
      }
    },
    clearSession() {
      this.currentUser = null
      this.token = ""
      clearAuthToken()
    },
  },
})
