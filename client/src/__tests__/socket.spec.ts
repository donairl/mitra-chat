import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { socket } from '@/ws/socket'

// Minimal WebSocket stand-in: the test drives onopen/onclose by hand.
class FakeWebSocket {
  static OPEN = 1
  static instances: FakeWebSocket[] = []
  readyState = FakeWebSocket.OPEN
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((ev: { data: string }) => void) | null = null
  send = vi.fn<(data: string) => void>()
  close = vi.fn<() => void>()
  constructor(public url: string) {
    FakeWebSocket.instances.push(this)
  }
}

describe('socket _open event', () => {
  beforeEach(() => {
    FakeWebSocket.instances = []
    vi.stubGlobal('WebSocket', FakeWebSocket)
    vi.useFakeTimers()
  })
  afterEach(() => {
    socket.disconnect()
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('flags only opens after the first as reconnects', () => {
    const onOpen = vi.fn<(p: unknown) => void>()
    socket.on('_open', onOpen)

    socket.connect('t')
    FakeWebSocket.instances[0]!.onopen!()
    expect(onOpen).toHaveBeenLastCalledWith({ reconnect: false })

    // Connection drops: the socket reopens itself after the backoff delay.
    FakeWebSocket.instances[0]!.onclose!()
    vi.advanceTimersByTime(1000)
    expect(FakeWebSocket.instances).toHaveLength(2)
    FakeWebSocket.instances[1]!.onopen!()
    expect(onOpen).toHaveBeenLastCalledWith({ reconnect: true })

    // A fresh login (disconnect, then connect) starts a new session.
    socket.disconnect()
    socket.connect('t2')
    FakeWebSocket.instances[2]!.onopen!()
    expect(onOpen).toHaveBeenLastCalledWith({ reconnect: false })

    socket.off('_open', onOpen)
  })

  it('reports whether a frame was sent', () => {
    socket.connect('t')
    const ws = FakeWebSocket.instances[0]!
    expect(socket.send('typing_start', {})).toBe(true)
    expect(ws.send).toHaveBeenCalledOnce()
    ws.readyState = 0
    expect(socket.send('typing_start', {})).toBe(false)
    expect(ws.send).toHaveBeenCalledOnce()
  })
})
