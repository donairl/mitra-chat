import { defineStore } from 'pinia'
import { ref } from 'vue'
import { channelApi } from '@/api'
import type { Channel } from '@/types'

// Direct-message channels (1:1 conversations reusing the channel/message infra).
export const useDmStore = defineStore('dm', () => {
  const channels = ref<Channel[]>([])

  async function fetch() {
    const { data } = await channelApi.dmList()
    channels.value = data
  }

  // Get-or-create the DM channel with a user and ensure it is in the list.
  async function open(userId: string) {
    const { data } = await channelApi.dmOpen(userId)
    const idx = channels.value.findIndex((c) => c.id === data.id)
    if (idx === -1) channels.value.unshift(data)
    else channels.value[idx] = data
    return data
  }

  return { channels, fetch, open }
})
