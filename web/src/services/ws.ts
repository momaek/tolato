import type { WSMessage } from '@/types/ws'

export type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'replaced'
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
  private _state: ConnectionState = 'disconnected'

  get state(): ConnectionState {
    return this._state
  }

  connect(token: string) {
    this.shouldReconnect = true
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.url = `${protocol}//${window.location.host}/ws/chat?token=${token}`
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

  send(msg: WSMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
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
    this.setState('connecting')
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      this.setState('connected')
      this.backoff = 1000
    }

    this.ws.onmessage = (event) => {
      try {
        const msg: WSMessage = JSON.parse(event.data)

        // Handle session replaced
        if (msg.type === 'session_replaced') {
          this.shouldReconnect = false
          this.setState('replaced')
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
      if (this._state !== 'replaced' && this.shouldReconnect) {
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
