<script setup lang="ts">
import { ref } from 'vue'
import { useServersStore } from '@/stores/servers'

const emit = defineEmits<{ close: []; joined: [id: string] }>()
const servers = useServersStore()
const code = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    const s = await servers.join(code.value.trim())
    emit('joined', s.id)
    emit('close')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Invalid invite'
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="emit('close')">
    <div class="w-full max-w-sm rounded-lg bg-bg-alt p-6">
      <h2 class="mb-4 text-lg font-bold text-white">Join a Server</h2>
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Invite code</label>
      <input
        v-model="code"
        placeholder="e.g. 56b9c3ccda"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />
      <p v-if="error" class="mb-2 text-sm text-red-400">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <button class="px-3 py-2 text-sm text-txt-muted hover:text-white" @click="emit('close')">Cancel</button>
        <button class="rounded bg-blurple px-4 py-2 text-sm font-medium text-white hover:bg-blurple-dark" @click="submit">
          Join
        </button>
      </div>
    </div>
  </div>
</template>
