import { defineStore } from 'pinia'
import { ref } from 'vue'
import { serverApi } from '@/api'
import type { Server, ServerMember } from '@/types'

// Servers store: the user's server list, the selected server, and its member list.
export const useServersStore = defineStore('servers', () => {
  const servers = ref<Server[]>([])
  const currentServerId = ref<string>('')
  const members = ref<ServerMember[]>([])

  async function fetch() {
    const { data } = await serverApi.list()
    servers.value = data
  }

  async function create(b: { name: string; description?: string; icon?: string }) {
    const { data } = await serverApi.create(b)
    servers.value.push(data)
    return data
  }

  async function join(inviteCode: string) {
    const { data } = await serverApi.join(inviteCode)
    // Avoid duplicates when re-joining a server already in the list.
    if (!servers.value.find((s) => s.id === data.id)) servers.value.push(data)
    return data
  }

  async function invite(id: string) {
    const { data } = await serverApi.invite(id)
    const s = servers.value.find((x) => x.id === id)
    if (s) s.invite_code = data.invite_code
    return data.invite_code
  }

  // Switch active server and load its members.
  async function selectServer(id: string) {
    currentServerId.value = id
    const { data } = await serverApi.members(id)
    members.value = data
  }

  async function remove(id: string) {
    await serverApi.remove(id)
    servers.value = servers.value.filter((s) => s.id !== id)
    if (currentServerId.value === id) currentServerId.value = '' // clear stale selection
  }

  return { servers, currentServerId, members, fetch, create, join, invite, selectServer, remove }
})
