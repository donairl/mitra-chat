import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi } from '@/api'
import { socket } from '@/ws/socket'
import type { User } from '@/types'

// Auth store: holds the JWT + current user, persists the token, and drives the
// socket lifecycle (connect on login/session, disconnect on logout).
export const useAuthStore = defineStore('auth', () => {
  const token = ref<string>(localStorage.getItem('token') || '') // rehydrate from storage
  const user = ref<User | null>(null)
  const isAuthed = computed(() => !!token.value)

  // Establish an authenticated session and open the realtime connection.
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
    // On app boot with a stored token, reconnect the socket for the restored session.
    if (token.value) socket.connect(token.value)
  }

  async function logout() {
    try {
      await authApi.logout()
    } catch {
      /* ignore: clear local session even if the server call fails */
    }
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
    socket.disconnect()
  }

  return { token, user, isAuthed, register, login, fetchMe, logout }
})
