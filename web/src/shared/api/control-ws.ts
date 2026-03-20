import { mockNodes } from "@/shared/api/mock-data"
import { getAuthToken } from "@/shared/api/auth-token"
import { rawWelcomeWsEventSchema } from "@/shared/api/backend-contracts"
import { uiWsEventSchema, type UiWsEvent } from "@/shared/api/contracts"

export interface ControlWsAdapter {
  connect(onEvent: (event: UiWsEvent) => void): () => void
}

class MockControlWs implements ControlWsAdapter {
  connect(onEvent: (event: UiWsEvent) => void) {
    const readyEvent = uiWsEventSchema.parse({
      type: "connection.ready",
      timestamp: new Date().toISOString(),
    })
    onEvent(readyEvent)

    const syncTimer = window.setInterval(() => {
      onEvent(
        uiWsEventSchema.parse({
          type: "connection.synced",
          timestamp: new Date().toISOString(),
        }),
      )
    }, 12000)

    const nodePulseTimer = window.setInterval(() => {
      const node = mockNodes[0]
      onEvent(
        uiWsEventSchema.parse({
          type: "node.updated",
          node: {
            ...node,
            lastSeen: new Date().toISOString(),
          },
        }),
      )
    }, 18000)

    return () => {
      window.clearInterval(syncTimer)
      window.clearInterval(nodePulseTimer)
    }
  }
}

class RealControlWs implements ControlWsAdapter {
  private readonly endpoint: string

  constructor(endpoint: string) {
    this.endpoint = endpoint
  }

  connect(onEvent: (event: UiWsEvent) => void) {
    let ws: WebSocket | null = null
    let reconnectTimer: number | null = null
    let closedByCaller = false

    const connectSocket = () => {
      const token = getAuthToken()
      const endpoint = new URL(this.endpoint)
      if (token) {
        endpoint.searchParams.set("token", token)
      }

      ws = new WebSocket(endpoint)

      ws.addEventListener("open", () => {
        onEvent(
          uiWsEventSchema.parse({
            type: "connection.ready",
            timestamp: new Date().toISOString(),
          }),
        )
      })

      ws.addEventListener("message", (event) => {
        try {
          const payload = JSON.parse(String(event.data))
          const welcome = rawWelcomeWsEventSchema.safeParse(payload)

          if (welcome.success) {
            onEvent(
              uiWsEventSchema.parse({
                type: "connection.synced",
                timestamp: new Date().toISOString(),
              }),
            )
            return
          }

          onEvent(uiWsEventSchema.parse(payload))
        } catch {
          // Ignore unknown events until the backend contract is finalized.
        }
      })

      ws.addEventListener("error", () => {
        onEvent(
          uiWsEventSchema.parse({
            type: "connection.error",
            timestamp: new Date().toISOString(),
            message: "WebSocket transport error",
          }),
        )
      })

      ws.addEventListener("close", () => {
        onEvent(
          uiWsEventSchema.parse({
            type: "connection.disconnected",
            timestamp: new Date().toISOString(),
            message: closedByCaller ? "Disconnected" : "Connection dropped, retrying...",
          }),
        )

        if (!closedByCaller) {
          reconnectTimer = window.setTimeout(connectSocket, 1500)
        }
      })
    }

    connectSocket()

    return () => {
      closedByCaller = true
      if (reconnectTimer !== null) {
        window.clearTimeout(reconnectTimer)
      }
      ws?.close()
    }
  }
}

const useMock = import.meta.env.VITE_USE_MOCK === "true"
const wsBaseUrl = import.meta.env.VITE_WS_BASE_URL ?? "ws://localhost:8080/ws/ui"

export const controlWs: ControlWsAdapter = useMock
  ? new MockControlWs()
  : new RealControlWs(wsBaseUrl)
