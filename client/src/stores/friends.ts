import { defineStore } from 'pinia'
import { ref } from 'vue'
import { friendApi } from '@/api'
import { socket } from '@/ws/socket'
import type { FriendRequest, User } from '@/types'

export const useFriendsStore = defineStore('friends', () => {
  const friends = ref<User[]>([])
  const requests = ref<FriendRequest[]>([])
  const online = ref<Set<string>>(new Set())
  const searchResults = ref<User[]>([])
  let wired = false

  async function fetch() {
    const [f, r] = await Promise.all([friendApi.list(), friendApi.requests()])
    friends.value = f.data
    requests.value = r.data
  }

  async function sendRequest(username: string) {
    await friendApi.request(username)
  }

  async function accept(id: string) {
    await friendApi.accept(id)
    await fetch()
  }

  async function reject(id: string) {
    await friendApi.reject(id)
    requests.value = requests.value.filter((r) => r.id !== id)
  }

  async function remove(id: string) {
    await friendApi.remove(id)
    await fetch()
  }

  async function search(q: string) {
    if (q.length < 2) {
      searchResults.value = []
      return
    }
    const { data } = await friendApi.search(q)
    searchResults.value = data
  }

  function wire() {
    if (wired) return
    wired = true
    socket.on('user_online', (p: any) => online.value.add(p.user_id))
    socket.on('user_offline', (p: any) => online.value.delete(p.user_id))
  }

  return { friends, requests, online, searchResults, fetch, sendRequest, accept, reject, remove, search, wire }
})
