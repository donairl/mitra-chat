// Singleton WebSocket wrapper: auto-reconnect with backoff + a tiny pub/sub layer
// so stores can subscribe to server events by message `type`.
type Handler = (payload: any) => void

class Socket {
  private ws: WebSocket | null = null
  private token = ''
  private handlers = new Map<string, Set<Handler>>()
  private backoff = 1000 // reconnect delay (ms), doubles per failed attempt
  private closed = false // true only on intentional disconnect; suppresses reconnect
  private opened = false // true once this session has connected, so later opens are reconnects

  connect(token: string) {
    this.token = token
    this.closed = false
    this.opened = false
    this.open()
  }

  private open() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    this.ws = new WebSocket(`${proto}://${location.host}/ws?token=${this.token}`)

    this.ws.onopen = () => {
      this.backoff = 1000 // reset backoff after a successful connection
      // Let stores re-join rooms / re-sync. `reconnect` is false for the first open.
      this.emit('_open', { reconnect: this.opened })
      this.opened = true
    }
    this.ws.onmessage = (ev) => {
      try {
        const data = JSON.parse(ev.data)
        this.emit(data.type, data.payload)
      } catch {
        /* ignore malformed frames */
      }
    }
    this.ws.onclose = () => {
      if (this.closed) return // deliberate disconnect: do not reconnect
      // Exponential backoff, capped at 15s, to avoid hammering the server.
      setTimeout(() => this.open(), this.backoff)
      this.backoff = Math.min(this.backoff * 2, 15000)
    }
  }

  // Returns false when the socket is not open and the frame was dropped.
  send(type: string, payload: object): boolean {
    if (this.ws?.readyState !== WebSocket.OPEN) return false
    this.ws.send(JSON.stringify({ type, payload }))
    return true
  }

  on(type: string, handler: Handler) {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set())
    this.handlers.get(type)!.add(handler)
  }

  off(type: string, handler: Handler) {
    this.handlers.get(type)?.delete(handler)
  }

  // Fan out an incoming event to every handler registered for its type.
  private emit(type: string, payload: any) {
    this.handlers.get(type)?.forEach((h) => h(payload))
  }

  disconnect() {
    this.closed = true
    this.ws?.close()
    this.ws = null
  }
}

export const socket = new Socket()
