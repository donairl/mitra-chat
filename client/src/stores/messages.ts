import { defineStore } from 'pinia'
import { ref } from 'vue'
import { apiStatus, messageApi, safeRefetch } from '@/api'
import { socket } from '@/ws/socket'
import { useAuthStore } from '@/stores/auth'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import type { Message } from '@/types'

// Messages store: message list for the open channel, infinite-scroll pagination,
// typing indicators, and realtime send/edit/delete over the socket.
interface Typer {
  username: string
  timer: number // setTimeout handle that auto-expires this typing indicator
}

export const useMessagesStore = defineStore('messages', () => {
  const auth = useAuthStore()
  const messages = ref<Message[]>([]) // oldest-first (ascending); newest at the end
  const channelId = ref<string>('')
  const loading = ref(false)
  const hasMore = ref(true) // false once a page returns fewer than the page size
  const typing = ref<Record<string, Typer>>({}) // keyed by user id
  // Why the server refused our last send/edit/delete (or that we were offline).
  const lastError = ref('')
  let inflight: ((ok: boolean) => void) | null = null // settles the awaiting send()
  let wired = false // one-time guard so socket handlers register only once

  // Switch channels: leave the old socket room, reset paging state, join the new
  // room (so the server streams events for it), then load the first page.
  async function open(id: string) {
    lastError.value = ''
    inflight?.(false) // the outcome of a send for the old channel is no longer observable
    if (channelId.value) socket.send('leave_room', { channel_id: channelId.value })
    channelId.value = id
    messages.value = []
    hasMore.value = true
    typing.value = {}
    socket.send('join_room', { channel_id: id })
    await load()
  }

  async function load() {
    loading.value = true
    try {
      // Page backwards using the oldest loaded message as the `before` cursor.
      const before = messages.value[0]?.id
      const { data } = await messageApi.history(channelId.value, before)
      if (data.length < 50) hasMore.value = false // short page => no older history left
      // Prepend the older page ahead of what we already have (keeps ascending order).
      messages.value = [...data, ...messages.value]
    } catch (e: unknown) {
      const status = apiStatus(e)
      if (status !== 403 && status !== 404) throw e
      hasMore.value = false
      refreshChannels()
    } finally {
      loading.value = false
    }
  }

  // The server says we can no longer see the open channel. Refetch the channel
  // list so the dashboard can move us to one we can see.
  function refreshChannels() {
    const servers = useServersStore()
    if (servers.currentServerId) safeRefetch(useChannelsStore().fetch(servers.currentServerId))
  }

  // Send a mutation frame, or report that we are offline (the socket drops it).
  function sendFrame(type: string, payload: object): boolean {
    if (socket.send(type, payload)) return true
    lastError.value = 'Not connected. Please try again in a moment.'
    return false
  }

  // Mutations go over the socket (not REST); the server echoes them back via the
  // `message`/`message_edited`/`message_deleted` events handled in wire().
  // send() resolves true once our message is echoed back, and false if the server
  // refused it, the socket was down, or nothing came back in time. Callers keep
  // the draft on false; the reason is in `lastError`.
  function send(content: string, attachmentIds?: string[]): Promise<boolean> {
    lastError.value = ''
    inflight?.(false) // one send in flight at a time
    const frame = { channel_id: channelId.value, content, attachment_ids: attachmentIds || [] }
    if (!sendFrame('send_message', frame)) return Promise.resolve(false)
    return new Promise((resolve) => {
      const timer = window.setTimeout(() => {
        lastError.value = 'No response from the server. Your message may not have been sent.'
        inflight?.(false)
      }, 10000)
      inflight = (ok) => {
        clearTimeout(timer)
        inflight = null
        resolve(ok)
      }
    })
  }

  async function edit(id: string, content: string) {
    lastError.value = ''
    sendFrame('edit_message', { message_id: id, content })
  }

  async function remove(id: string) {
    lastError.value = ''
    sendFrame('delete_message', { message_id: id })
  }

  function sendTyping(start: boolean) {
    socket.send(start ? 'typing_start' : 'typing_stop', { channel_id: channelId.value })
  }

  // Register realtime handlers once. Each ignores events for other channels so a
  // background channel's traffic never mutates the currently displayed list.
  function wire() {
    if (wired) return
    wired = true
    socket.on('message', (m: Message) => {
      if (m.channel_id !== channelId.value) return
      messages.value.push(m)
      if (m.user_id === auth.user?.id) inflight?.(true) // our own send went through
    })
    socket.on('message_edited', (p: any) => {
      if (p.channel_id !== channelId.value) return
      const m = messages.value.find((x) => x.id === p.message_id)
      if (m) {
        m.content = p.content
        m.is_edited = true
        m.edited_at = p.edited_at
      }
    })
    socket.on('message_deleted', (p: any) => {
      if (p.channel_id !== channelId.value) return
      messages.value = messages.value.filter((x) => x.id !== p.message_id)
    })
    socket.on('error', (p: { code: string; message?: string; channel_id?: string }) => {
      if (p.code === 'not_found' && p.channel_id && p.channel_id === channelId.value) {
        refreshChannels() // the channel list update moves us off it; no message needed
      } else {
        lastError.value = p.message || 'Request failed'
      }
      inflight?.(false)
    })
    // A new connection has no rooms, so join the open channel again.
    socket.on('_open', () => {
      if (channelId.value) socket.send('join_room', { channel_id: channelId.value })
    })
    const startTyping = (p: any) => {
      if (p.channel_id !== channelId.value) return
      clearTimeout(typing.value[p.user_id]?.timer) // debounce: drop the previous expiry
      typing.value[p.user_id] = {
        username: p.username,
        // Auto-clear after 4s in case the "stop typing" event never arrives.
        timer: window.setTimeout(() => delete typing.value[p.user_id], 4000),
      }
    }
    socket.on('typing', startTyping)
    socket.on('typing_stop', (p: any) => {
      const t = typing.value[p.user_id]
      if (t) {
        clearTimeout(t.timer) // cancel pending auto-clear before removing
        delete typing.value[p.user_id]
      }
    })
  }

  return {
    messages,
    channelId,
    loading,
    hasMore,
    typing,
    lastError,
    open,
    load,
    send,
    edit,
    remove,
    sendTyping,
    wire,
  }
})
