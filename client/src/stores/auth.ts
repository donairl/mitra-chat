import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api'
import { socket } from '@/ws/socket'
import type { User } from '@/types'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '')
  const user = ref<User | null>(null)
  const isAuthed = computed(() => !!token.value)

  function setSession(t: string, u: User) {
    token.value = t
    user.value = u
    localStorage.setItem('token', t)
    socket.connect(t)
  }

  async function register(b: { username: string; email: string; password: string }) {
    const { data } = await authApi.register(b)
    setSession(data.token, data.user)
  }

  async function login(b: { email: string; password: string }) {
    const { data } = await authApi.login(b)
    setSession(data.token, data.user)
  }

  async function fetchMe() {
    const { data } = await authApi.me()
    user.value = data
    if (token.value) socket.connect(token.value)
  }

  async function logout() {
    try {
      await authApi.logout()
    } catch {
      /* ignore */
    }
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
    socket.disconnect()
  }

  return { token, user, isAuthed, register, login, fetchMe, logout }
})
