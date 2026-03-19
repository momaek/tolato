import { mockNodes } from "@/shared/api/mock-data"
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
    const ws = new WebSocket(this.endpoint)

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

    ws.addEventListener("close", () => {
      onEvent(
        uiWsEventSchema.parse({
          type: "connection.synced",
          timestamp: new Date().toISOString(),
        }),
      )
    })

    return () => ws.close()
  }
}

const useMock = import.meta.env.VITE_USE_MOCK !== "false"
const wsBaseUrl = import.meta.env.VITE_WS_BASE_URL ?? "ws://localhost:8080/ws/ui"

export const controlWs: ControlWsAdapter = useMock
  ? new MockControlWs()
  : new RealControlWs(wsBaseUrl)
