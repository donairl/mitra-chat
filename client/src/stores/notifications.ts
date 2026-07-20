import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { notificationApi } from '@/api'
import { socket } from '@/ws/socket'
import type { Notification } from '@/types'

export const useNotificationsStore = defineStore('notifications', () => {
  const items = ref<Notification[]>([])
  const unread = computed(() => items.value.filter((n) => !n.read).length)
  let wired = false

  async function fetch() {
    const { data } = await notificationApi.list()
    items.value = data
  }

  async function markRead(id: string) {
    await notificationApi.markRead(id)
    const n = items.value.find((x) => x.id === id)
    if (n) n.read = true
  }

  async function markAll() {
    await notificationApi.markAll()
    items.value.forEach((n) => (n.read = true))
  }

  function wire() {
    if (wired) return
    wired = true
    socket.on('notification', (n: Notification) => items.value.unshift(n))
  }

  return { items, unread, fetch, markRead, markAll, wire }
})
