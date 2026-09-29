import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { messageApi } from '@/api'
import type * as Api from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useMessagesStore } from '@/stores/messages'
import type { Message } from '@/types'
import { emit, me, resetStoreTest, resp, sock } from './helpers'

vi.mock('@/ws/socket', async () => ({ socket: (await import('./helpers')).sock }))
vi.mock('@/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api')>()),
  messageApi: { history: vi.fn<typeof Api.messageApi.history>() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  resetStoreTest()
  useAuthStore().user = me
})
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

async function openChannel() {
  const messages = useMessagesStore()
  messages.wire()
  vi.mocked(messageApi.history).mockResolvedValue(resp([]))
  await messages.open('c1')
  return messages
}
const own = (id = 'm1'): Message => ({
  id,
  content: 'hi',
  user_id: 'me',
  channel_id: 'c1',
  is_edited: false,
  created_at: '',
})

// send() settles true/false so MessageInput can keep the draft when it fails.
describe('messages store send', () => {
  it('resolves true when the server echoes the message back', async () => {
    const messages = await openChannel()
    const p = messages.send('hi', ['a1'])
    expect(sock.sent[sock.sent.length - 1]).toEqual({
      type: 'send_message',
      payload: { channel_id: 'c1', content: 'hi', attachment_ids: ['a1'] },
    })
    // Someone else's message in the same channel is not our confirmation.
    emit('message', { ...own('m0'), user_id: 'other' })
    emit('message', own())
    expect(await p).toBe(true)
    expect(messages.lastError).toBe('')
  })

  it('resolves false and shows the server message when the send is refused', async () => {
    const messages = await openChannel()
    const p = messages.send('hi')
    emit('error', { code: 'forbidden', message: 'insufficient permissions', channel_id: 'c1' })
    expect(await p).toBe(false)
    expect(messages.lastError).toBe('insufficient permissions')
  })

  it('shows error frames for edits and deletes', async () => {
    const messages = await openChannel()
    emit('error', { code: 'not_found', message: 'message not found' })
    expect(messages.lastError).toBe('message not found')
  })

  it('resolves false when offline, and after a timeout with no reply', async () => {
    const messages = await openChannel()
    sock.online = false
    expect(await messages.send('hi')).toBe(false)
    expect(messages.lastError).toMatch(/Not connected/)

    sock.online = true
    vi.useFakeTimers()
    const p = messages.send('hi')
    vi.advanceTimersByTime(10000)
    expect(await p).toBe(false)
    expect(messages.lastError).toMatch(/No response/)
  })
})
