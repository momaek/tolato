import type { WSMessage } from '@/types/ws'

export type TerminalWSState = 'connecting' | 'authenticated' | 'ready' | 'closed' | 'error'
export type TerminalMessageHandler = (msg: WSMessage) => void

/**
 * TerminalWebSocket — a per-session client for `/ws/terminal`.
 *
 * Lifecycle:
 *   1. connect(token) → opens the WS, sends the auth frame.
 *   2. on 'auth_ok' → state becomes 'authenticated', caller should send 'open'.
 *   3. on 'ready'   → state becomes 'ready', caller can send input / file_op.
 *   4. on 'exit' / 'error' or a close → state becomes 'closed' / 'error'.
 *
 * Unlike the chat WS singleton, each terminal tab owns its own instance and
 * does NOT auto-reconnect — a terminated PTY is terminated; users re-open.
 */
export class TerminalWebSocket {
  private ws: WebSocket | null = null
  private url: string
  private handlers = new Map<string, TerminalMessageHandler[]>()
  private stateListeners: ((s: TerminalWSState) => void)[] = []
  private _state: TerminalWSState = 'closed'

  constructor() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    this.url = `${protocol}//${window.location.host}/ws/terminal`
  }

  get state(): TerminalWSState {
    return this._state
  }

  connect(token: string) {
    this.setState('connecting')
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      this.ws?.send(JSON.stringify({ type: 'auth', payload: { token } }))
    }

    this.ws.onmessage = (event) => {
      let msg: WSMessage
      try {
        msg = JSON.parse(event.data)
      } catch {
        return
      }

      if (msg.type === 'auth_ok') {
        this.setState('authenticated')
        return
      }
      if (msg.type === 'ready') {
        this.setState('ready')
      }
      if (msg.type === 'exit') {
        this.setState('closed')
      }
      if (msg.type === 'error') {
        this.setState('error')
      }

      const list = this.handlers.get(msg.type) || []
      for (const h of list) h(msg)
      const wild = this.handlers.get('*') || []
      for (const h of wild) h(msg)
    }

    this.ws.onclose = () => {
      if (this._state !== 'error') this.setState('closed')
    }

    this.ws.onerror = () => {
      // Will be followed by onclose.
    }
  }

  send(msg: WSMessage) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  close() {
    this.ws?.close()
    this.ws = null
    this.setState('closed')
  }

  on(type: string, handler: TerminalMessageHandler) {
    const list = this.handlers.get(type) || []
    list.push(handler)
    this.handlers.set(type, list)
  }

  off(type: string, handler: TerminalMessageHandler) {
    const list = this.handlers.get(type) || []
    this.handlers.set(
      type,
      list.filter((h) => h !== handler),
    )
  }

  onStateChange(fn: (s: TerminalWSState) => void) {
    this.stateListeners.push(fn)
    return () => {
      this.stateListeners = this.stateListeners.filter((l) => l !== fn)
    }
  }

  private setState(s: TerminalWSState) {
    this._state = s
    for (const l of this.stateListeners) l(s)
  }
}

export function createTerminalWs() {
  return new TerminalWebSocket()
}
