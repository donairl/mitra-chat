import { vi } from 'vitest'
import { AxiosError, type AxiosResponse } from 'axios'
import type { Channel, Role, Server, ServerMember, User } from '@/types'

// Fake socket for store tests: records what stores send and lets tests fire
// incoming events. Specs install it with vi.mock('@/ws/socket', ...).
export const sock = {
  handlers: {} as Record<string, Array<(p: unknown) => void>>,
  sent: [] as Array<{ type: string; payload: unknown }>,
  online: true,
  on(type: string, h: (p: unknown) => void) {
    ;(this.handlers[type] ??= []).push(h)
  },
  send(type: string, payload: unknown) {
    if (!this.online) return false
    this.sent.push({ type, payload })
    return true
  },
}

export const emit = (type: string, payload: unknown) =>
  sock.handlers[type]?.forEach((h) => h(payload))

// The signed-in user in store tests. Specs put it in the auth store themselves:
// importing that store here would deadlock the async vi.mock of '@/ws/socket'.
export const me = { id: 'me', username: 'me' } as User

// Call before each test: fresh socket state and mocks.
export function resetStoreTest() {
  // Node's experimental localStorage shadows jsdom's and is unusable here; the auth store reads it.
  vi.stubGlobal('localStorage', { getItem: () => null, setItem: () => {}, removeItem: () => {} })
  for (const k of Object.keys(sock.handlers)) delete sock.handlers[k]
  sock.sent = []
  sock.online = true
  vi.resetAllMocks()
}

export const resp = <T>(data: T) => ({ data }) as AxiosResponse<T>

export const httpError = (status: number) =>
  new AxiosError('failed', 'ERR_BAD_REQUEST', undefined, undefined, {
    status,
    data: {},
  } as AxiosResponse)

export function deferred<T>() {
  let resolve!: (v: T) => void
  const promise = new Promise<T>((r) => (resolve = r))
  return { promise, resolve }
}

export const chan = (id: string): Channel => ({ id, name: id, type: 'text', server_id: 's' })
export const srv = (id: string): Server => ({ id, name: `Server ${id}`, owner_id: 'owner' })
export const member = (userId: string, role: Role): ServerMember => ({
  id: `m-${userId}`,
  server_id: 's',
  user_id: userId,
  role,
})
