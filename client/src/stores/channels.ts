import { defineStore } from 'pinia'
import { ref } from 'vue'
import { channelApi } from '@/api'
import type { Channel } from '@/types'

// Channels store: channels for the active server plus the currently selected channel.
export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<Channel[]>([])
  const currentChannelId = ref<string>('')

  async function fetch(serverId: string) {
    const { data } = await channelApi.list(serverId)
    channels.value = data
  }

  async function create(serverId: string, b: { name: string; type?: string; topic?: string }) {
    const { data } = await channelApi.create(serverId, b)
    channels.value.push(data)
    return data
  }

  async function remove(id: string) {
    await channelApi.remove(id)
    channels.value = channels.value.filter((c) => c.id !== id)
  }

  function select(id: string) {
    currentChannelId.value = id
  }

  return { channels, currentChannelId, fetch, create, remove, select }
})
