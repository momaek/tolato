import type { WSMessage } from '@/types/ws'

export type ConnectionState = 'connecting' | 'connected' | 'disconnected'
export type MessageHandler = (msg: WSMessage) => void

class WebSocketService {
  private ws: WebSocket | null = null
  private url: string = ''
  private handlers: Map<string, MessageHandler[]> = new Map()
  private stateListeners: ((state: ConnectionState) => void)[] = []
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private backoff = 1000
  private maxBackoff = 30000
  private shouldReconnect = true
  private token: string = ''
  private _state: ConnectionState = 'disconnected'

  get state(): ConnectionState {
    return this._state
  }

  connect(token: string) {
    // Idempotent: if we already have a live/pending socket with the same token,
    // don't open a second one. Callers (e.g. route remounts) can safely call this
    // on every mount without leaking sockets.
    if (this.ws && this.token === token) {
      const rs = this.ws.readyState
      if (rs === WebSocket.OPEN || rs === WebSocket.CONNECTING) {
        this.shouldReconnect = true
        return
      }
    }
    // Token changed or no live socket — tear down any existing one first.
    if (this.ws) {
      try { this.ws.close() } catch {}
      this.ws = null
    }
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    this.shouldReconnect = true
    this.token = token
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.url = `${protocol}//${window.location.host}/ws/chat`
    this.doConnect()
  }

  disconnect() {
    this.shouldReconnect = false
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
    if (this.ws) {
      this.ws.close()
      this.ws = null
    }
    this.setState('disconnected')
  }

  send(msg: WSMessage): boolean {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
      return true
    }
    return false
  }

  on(type: string, handler: MessageHandler) {
    const list = this.handlers.get(type) || []
    list.push(handler)
    this.handlers.set(type, list)
  }

  off(type: string, handler: MessageHandler) {
    const list = this.handlers.get(type) || []
    this.handlers.set(type, list.filter((h) => h !== handler))
  }

  onStateChange(listener: (state: ConnectionState) => void) {
    this.stateListeners.push(listener)
    return () => {
      this.stateListeners = this.stateListeners.filter((l) => l !== listener)
    }
  }

  private doConnect() {
    if (this.ws) {
      try { this.ws.close() } catch {}
      this.ws = null
    }
    this.setState('connecting')
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      // Send auth message as the first message (token is NOT in the URL)
      this.ws?.send(JSON.stringify({
        type: 'auth',
        payload: { token: this.token }
      }))
      // Connection state will be set to 'connected' when we receive 'auth_ok'
    }

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)

        // Handle auth success
        if (msg.type === 'auth_ok') {
          this.setState('connected')
          this.backoff = 1000
          return
        }

        // Dispatch to registered handlers
        const handlers = this.handlers.get(msg.type) || []
        for (const handler of handlers) {
          handler(msg)
        }

        // Also dispatch to wildcard handlers
        const wildcardHandlers = this.handlers.get('*') || []
        for (const handler of wildcardHandlers) {
          handler(msg)
        }
      } catch {
        // ignore parse errors
      }
    }

    this.ws.onclose = () => {
      if (this.shouldReconnect) {
        this.setState('disconnected')
        this.scheduleReconnect()
      }
    }

    this.ws.onerror = () => {
      // onclose will be called after onerror
    }
  }

  private scheduleReconnect() {
    if (!this.shouldReconnect) return
    this.reconnectTimer = setTimeout(() => {
      this.backoff = Math.min(this.backoff * 2, this.maxBackoff)
      this.doConnect()
    }, this.backoff)
  }

  private setState(state: ConnectionState) {
    this._state = state
    for (const listener of this.stateListeners) {
      listener(state)
    }
  }
}

// Singleton instance
export const wsService = new WebSocketService()

// In Vite dev mode, this module's singleton survives across HMR of *other*
// modules; that's intentional (we want one shared connection). But when this
// module ITSELF hot-updates, the old instance and any pending reconnect timer
// would leak — and components re-bound to the new instance might race with a
// stale socket from the old one. Disposing on hot-update keeps things clean.
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    wsService.disconnect()
  })
}
