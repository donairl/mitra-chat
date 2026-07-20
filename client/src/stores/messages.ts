import { defineStore } from 'pinia'
import { ref } from 'vue'
import { messageApi } from '@/api'
import { socket } from '@/ws/socket'
import type { Message } from '@/types'

interface Typer {
  username: string
  timer: number
}

export const useMessagesStore = defineStore('messages', () => {
  const messages = ref<Message[]>([])
  const channelId = ref<string>('')
  const loading = ref(false)
  const hasMore = ref(true)
  const typing = ref<Record<string, Typer>>({})
  let wired = false

  async function open(id: string) {
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
      const before = messages.value[0]?.id
      const { data } = await messageApi.history(channelId.value, before)
      if (data.length < 50) hasMore.value = false
      messages.value = [...data, ...messages.value]
    } finally {
      loading.value = false
    }
  }

  async function send(content: string, attachmentIds?: string[]) {
    socket.send('send_message', {
      channel_id: channelId.value,
      content,
      attachment_ids: attachmentIds || [],
    })
  }

  async function edit(id: string, content: string) {
    socket.send('edit_message', { message_id: id, content })
  }

  async function remove(id: string) {
    socket.send('delete_message', { message_id: id })
  }

  function sendTyping(start: boolean) {
    socket.send(start ? 'typing_start' : 'typing_stop', { channel_id: channelId.value })
  }

  function wire() {
    if (wired) return
    wired = true
    socket.on('message', (m: Message) => {
      if (m.channel_id === channelId.value) messages.value.push(m)
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
    const startTyping = (p: any) => {
      if (p.channel_id !== channelId.value) return
      clearTimeout(typing.value[p.user_id]?.timer)
      typing.value[p.user_id] = {
        username: p.username,
        timer: window.setTimeout(() => delete typing.value[p.user_id], 4000),
      }
    }
    socket.on('typing', startTyping)
    socket.on('typing_stop', (p: any) => {
      const t = typing.value[p.user_id]
      if (t) {
        clearTimeout(t.timer)
        delete typing.value[p.user_id]
      }
    })
  }

  return { messages, channelId, loading, hasMore, typing, open, load, send, edit, remove, sendTyping, wire }
})
