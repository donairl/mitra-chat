<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useServersStore } from '@/stores/servers'

const props = defineProps<{ serverId: string }>()
const emit = defineEmits<{ close: [] }>()
const servers = useServersStore()

const code = ref('')
const error = ref('')
const loading = ref(true)
const copied = ref(false)

onMounted(async () => {
  try {
    code.value = await servers.invite(props.serverId)
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Could not load invite'
  } finally {
    loading.value = false
  }
})

async function copy() {
  try {
    await navigator.clipboard.writeText(code.value)
    copied.value = true
    setTimeout(() => (copied.value = false), 1500)
  } catch {
    error.value = 'Copy failed'
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="emit('close')">
    <div class="w-full max-w-sm rounded-lg bg-bg-alt p-6">
      <h2 class="mb-4 text-lg font-bold text-white">Invite People</h2>
      <p class="mb-3 text-sm text-txt-muted">Share this code so others can join the server.</p>

      <p v-if="loading" class="text-sm text-txt-muted">Loading…</p>
      <template v-else>
        <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Invite code</label>
        <div class="mb-3 flex gap-2">
          <input
            :value="code"
            readonly
            class="w-full rounded bg-bg-input px-3 py-2 font-mono text-txt outline-none"
          />
          <button
            class="rounded bg-blurple px-4 py-2 text-sm font-medium text-white hover:bg-blurple-dark"
            @click="copy"
          >
            {{ copied ? 'Copied' : 'Copy' }}
          </button>
        </div>
      </template>

      <p v-if="error" class="mb-2 text-sm text-red-400">{{ error }}</p>
      <div class="flex justify-end">
        <button class="px-3 py-2 text-sm text-txt-muted hover:text-white" @click="emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>
