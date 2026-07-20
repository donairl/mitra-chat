type Handler = (payload: any) => void

class Socket {
  private ws: WebSocket | null = null
  private token = ''
  private handlers = new Map<string, Set<Handler>>()
  private backoff = 1000
  private closed = false

  connect(token: string) {
    this.token = token
    this.closed = false
    this.open()
  }

  private open() {
    const proto = location.protocol === 'https:' ? 'wss' : 'ws'
    this.ws = new WebSocket(`${proto}://${location.host}/ws?token=${this.token}`)

    this.ws.onopen = () => {
      this.backoff = 1000
      this.emit('_open', null)
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
      if (this.closed) return
      setTimeout(() => this.open(), this.backoff)
      this.backoff = Math.min(this.backoff * 2, 15000)
    }
  }

  send(type: string, payload: object) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ type, payload }))
    }
  }

  on(type: string, handler: Handler) {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set())
    this.handlers.get(type)!.add(handler)
  }

  off(type: string, handler: Handler) {
    this.handlers.get(type)?.delete(handler)
  }

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
