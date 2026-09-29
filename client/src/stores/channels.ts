import { defineStore } from 'pinia'
import { ref } from 'vue'
import { channelApi, type ChannelBody } from '@/api'
import type { Channel } from '@/types'

// Channels store: channels for the active server plus the currently selected channel.
// The list comes from the server already filtered to what the user may view.
export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<Channel[]>([])
  const currentChannelId = ref<string>('')

  async function fetch(serverId: string) {
    const { data } = await channelApi.list(serverId)
    channels.value = data
  }

  async function create(serverId: string, b: ChannelBody) {
    const { data } = await channelApi.create(serverId, b)
    // A channels_changed refetch may have added it already.
    if (!channels.value.some((c) => c.id === data.id)) channels.value.push(data)
    return data
  }

  async function update(id: string, b: ChannelBody) {
    const { data } = await channelApi.update(id, b)
    const i = channels.value.findIndex((c) => c.id === id)
    if (i !== -1) channels.value[i] = data
    return data
  }

  async function remove(id: string) {
    await channelApi.remove(id)
    channels.value = channels.value.filter((c) => c.id !== id)
  }

  function select(id: string) {
    currentChannelId.value = id
  }

  return { channels, currentChannelId, fetch, create, update, remove, select }
})
