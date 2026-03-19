import { defineStore } from "pinia"
import type { SessionInfo } from "@/shared/api/contracts"

export const useSessionStore = defineStore("session", {
  state: () => ({
    currentUser: null as SessionInfo | null,
  }),
  actions: {
    setSession(session: SessionInfo) {
      this.currentUser = session
    },
  },
})
