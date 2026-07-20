<script setup lang="ts">
import { ref } from 'vue'
import { useChannelsStore } from '@/stores/channels'

const props = defineProps<{ serverId: string }>()
const emit = defineEmits<{ close: [] }>()
const channels = useChannelsStore()
const name = ref('')
const topic = ref('')
const error = ref('')

async function submit() {
  error.value = ''
  try {
    await channels.create(props.serverId, { name: name.value, topic: topic.value })
    emit('close')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed'
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="emit('close')">
    <div class="w-full max-w-sm rounded-lg bg-bg-alt p-6">
      <h2 class="mb-4 text-lg font-bold text-white">Create Channel</h2>
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Channel name</label>
      <input
        v-model="name"
        placeholder="new-channel"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Topic (optional)</label>
      <input
        v-model="topic"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />
      <p v-if="error" class="mb-2 text-sm text-red-400">{{ error }}</p>
      <div class="flex justify-end gap-2">
        <button class="px-3 py-2 text-sm text-txt-muted hover:text-white" @click="emit('close')">Cancel</button>
        <button class="rounded bg-blurple px-4 py-2 text-sm font-medium text-white hover:bg-blurple-dark" @click="submit">
          Create
        </button>
      </div>
    </div>
  </div>
</template>
