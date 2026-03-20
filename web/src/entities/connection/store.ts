import { defineStore } from "pinia"
import type { UiWsEvent } from "@/shared/api/contracts"

type ConnectionState = "idle" | "connecting" | "connected"
type DetailedConnectionState = ConnectionState | "disconnected" | "error"

export const useConnectionStore = defineStore("connection", {
  state: () => ({
    state: "idle" as DetailedConnectionState,
    lastSyncAt: null as string | null,
    message: "" as string,
  }),
  actions: {
    markConnecting() {
      this.state = "connecting"
      this.message = ""
    },
    consumeWsEvent(event: UiWsEvent) {
      if (event.type === "connection.ready") {
        this.state = "connected"
        this.lastSyncAt = event.timestamp
        this.message = ""
      }

      if (event.type === "connection.synced") {
        this.lastSyncAt = event.timestamp
        this.message = ""
      }

      if (event.type === "connection.disconnected") {
        this.state = "disconnected"
        this.message = event.message ?? ""
      }

      if (event.type === "connection.error") {
        this.state = "error"
        this.message = event.message ?? ""
      }
    },
  },
})
