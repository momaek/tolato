import { defineStore } from "pinia"
import type { UiWsEvent } from "@/shared/api/contracts"

type ConnectionState = "idle" | "connecting" | "connected"

export const useConnectionStore = defineStore("connection", {
  state: () => ({
    state: "idle" as ConnectionState,
    lastSyncAt: null as string | null,
  }),
  actions: {
    markConnecting() {
      this.state = "connecting"
    },
    consumeWsEvent(event: UiWsEvent) {
      if (event.type === "connection.ready") {
        this.state = "connected"
        this.lastSyncAt = event.timestamp
      }

      if (event.type === "connection.synced") {
        this.lastSyncAt = event.timestamp
      }
    },
  },
})
