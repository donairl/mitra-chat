<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, RouterLink } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const email = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  error.value = ''
  loading.value = true
  try {
    await auth.login({ email: email.value, password: password.value })
    router.push({ name: 'dashboard' })
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Login failed'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex h-full items-center justify-center bg-bg-dark px-4">
    <form
      @submit.prevent="submit"
      class="w-full max-w-md rounded-lg bg-bg-alt p-8 shadow-2xl"
    >
      <h1 class="mb-1 text-2xl font-bold text-white">Welcome back</h1>
      <p class="mb-6 text-sm text-txt-muted">Sign in to continue to MitraChat</p>

      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Email</label>
      <input
        v-model="email"
        type="email"
        required
        class="mb-4 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />

      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Password</label>
      <input
        v-model="password"
        type="password"
        required
        class="mb-4 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />

      <p v-if="error" class="mb-3 text-sm text-red-400">{{ error }}</p>

      <button
        :disabled="loading"
        class="w-full rounded bg-blurple py-2 font-medium text-white transition hover:bg-blurple-dark disabled:opacity-60"
      >
        {{ loading ? 'Signing in…' : 'Log In' }}
      </button>

      <p class="mt-4 text-sm text-txt-muted">
        Need an account?
        <RouterLink to="/register" class="text-blurple hover:underline">Register</RouterLink>
      </p>
    </form>
  </div>
</template>
