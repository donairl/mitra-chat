import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises } from '@vue/test-utils'
import { AxiosError, type AxiosResponse } from 'axios'
import { channelApi, messageApi, serverApi } from '@/api'
import type * as Api from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useChannelsStore } from '@/stores/channels'
import { useMessagesStore } from '@/stores/messages'
import { useServersStore } from '@/stores/servers'
import type { Channel, ServerMember } from '@/types'
import {
  chan,
  deferred,
  emit,
  httpError,
  me,
  member,
  resetStoreTest,
  resp,
  sock,
  srv,
} from './helpers'

vi.mock('@/ws/socket', async () => ({ socket: (await import('./helpers')).sock }))
// Keep apiError/apiStatus/safeRefetch real; stub only the network calls.
vi.mock('@/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/api')>()),
  serverApi: {
    list: vi.fn<typeof Api.serverApi.list>(),
    members: vi.fn<typeof Api.serverApi.members>(),
  },
  channelApi: { list: vi.fn<typeof Api.channelApi.list>() },
  messageApi: { history: vi.fn<typeof Api.messageApi.history>() },
}))

beforeEach(() => {
  setActivePinia(createPinia())
  resetStoreTest()
  useAuthStore().user = me
})
afterEach(() => {
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
})

describe('channels store', () => {
  it('drops a stale fetch that resolves after a newer one', async () => {
    const channels = useChannelsStore()
    const a = deferred<AxiosResponse<Channel[]>>()
    const b = deferred<AxiosResponse<Channel[]>>()
    vi.mocked(channelApi.list).mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise)

    const pA = channels.fetch('A')
    const pB = channels.fetch('B')
    b.resolve(resp([chan('b1')]))
    await pB
    a.resolve(resp([chan('a1')]))
    // The stale fetch still hands its own list to the caller, but does not store it.
    expect(await pA).toEqual([chan('a1')])
    expect(channels.channels).toEqual([chan('b1')])
  })
})

describe('servers store', () => {
  it('clears the previous server members on select and ignores stale member loads', async () => {
    const servers = useServersStore()
    servers.currentServerId = 'A'
    servers.members = [member('me', 'owner')]
    const a1 = deferred<AxiosResponse<ServerMember[]>>()
    const b = deferred<AxiosResponse<ServerMember[]>>()
    const a2 = deferred<AxiosResponse<ServerMember[]>>()
    vi.mocked(serverApi.members)
      .mockReturnValueOnce(b.promise)
      .mockReturnValueOnce(a1.promise)
      .mockReturnValueOnce(a2.promise)

    const pB = servers.selectServer('B')
    // Gating must not see server A's members (and so A's owner role) while B loads.
    expect(servers.members).toEqual([])
    expect(servers.myRole).toBeUndefined()

    // Quick B -> A -> A again: the first A load resolves last and must not win.
    const pA1 = servers.selectServer('A')
    const pA2 = servers.selectServer('A')
    a2.resolve(resp([member('me', 'admin')]))
    await pA2
    a1.resolve(resp([member('me', 'member')]))
    b.resolve(resp([member('me', 'moderator')]))
    await Promise.all([pB, pA1])
    expect(servers.currentServerId).toBe('A')
    expect(servers.myRole).toBe('admin')
  })

  it('drops the server instead of rejecting when select gets a 403', async () => {
    const servers = useServersStore()
    servers.servers = [srv('A')]
    vi.mocked(serverApi.members).mockRejectedValue(httpError(403))

    await expect(servers.selectServer('A')).resolves.toBeUndefined()
    expect(servers.currentServerId).toBe('')
    expect(servers.servers).toEqual([])
    expect(servers.notice).toBe('You no longer have access to Server A.')
  })

  it('does not resync on the first connect', async () => {
    const servers = useServersStore()
    servers.wire()
    emit('_open', { reconnect: false })
    await flushPromises()
    expect(serverApi.list).not.toHaveBeenCalled()
  })

  it('reloads servers, members and channels after a reconnect', async () => {
    const servers = useServersStore()
    servers.wire()
    servers.servers = [srv('A')]
    servers.currentServerId = 'A'
    vi.mocked(serverApi.list).mockResolvedValue(resp([srv('A'), srv('B')]))
    vi.mocked(serverApi.members).mockResolvedValue(resp([member('me', 'admin')]))
    vi.mocked(channelApi.list).mockResolvedValue(resp([chan('c1')]))

    emit('_open', { reconnect: true })
    await flushPromises()
    expect(servers.servers.map((s) => s.id)).toEqual(['A', 'B'])
    expect(servers.myRole).toBe('admin')
    expect(useChannelsStore().channels).toEqual([chan('c1')])
  })

  it('clears the selection after a reconnect if the server is gone', async () => {
    const servers = useServersStore()
    servers.wire()
    servers.servers = [srv('A')]
    servers.currentServerId = 'A'
    servers.members = [member('me', 'member')]
    vi.mocked(serverApi.list).mockResolvedValue(resp([srv('B')]))

    emit('_open', { reconnect: true })
    await flushPromises()
    expect(servers.currentServerId).toBe('')
    expect(servers.members).toEqual([])
    expect(servers.notice).toBe('You no longer have access to Server A.')
    expect(serverApi.members).not.toHaveBeenCalled()
  })

  it('clears the selection after a reconnect if the member load returns 403', async () => {
    const servers = useServersStore()
    servers.wire()
    servers.servers = [srv('A')]
    servers.currentServerId = 'A'
    vi.mocked(serverApi.list).mockResolvedValue(resp([srv('A')]))
    vi.mocked(serverApi.members).mockRejectedValue(httpError(403))
    vi.mocked(channelApi.list).mockRejectedValue(httpError(403))

    emit('_open', { reconnect: true })
    await flushPromises()
    expect(servers.currentServerId).toBe('')
    expect(servers.servers).toEqual([])
  })

  it('logs a failed channels refetch from a socket event instead of leaving it unhandled', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const servers = useServersStore()
    servers.wire()
    servers.currentServerId = 'A'
    vi.mocked(channelApi.list).mockRejectedValue(new AxiosError('Network Error'))

    emit('channels_changed', { server_id: 'A' })
    await flushPromises()
    expect(warn).toHaveBeenCalledOnce()
  })
})

describe('messages store reconnect', () => {
  it('joins the open channel again after a reconnect', async () => {
    const messages = useMessagesStore()
    messages.wire()
    emit('_open', { reconnect: true })
    expect(sock.sent).toEqual([]) // nothing open yet

    vi.mocked(messageApi.history).mockResolvedValue(resp([]))
    await messages.open('c1')
    sock.sent = []
    emit('_open', { reconnect: true })
    expect(sock.sent).toEqual([{ type: 'join_room', payload: { channel_id: 'c1' } }])
  })
})
