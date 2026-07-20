<script setup lang="ts">
import { ref } from 'vue'
import { useMessagesStore } from '@/stores/messages'
import { attachmentApi } from '@/api'
import type { Channel, Attachment } from '@/types'

const props = defineProps<{ channel: Channel }>()
const messages = useMessagesStore()

const text = ref('')
const pending = ref<Attachment[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
let typingTimer: number | undefined
let isTyping = false

function onInput() {
  if (!isTyping) {
    isTyping = true
    messages.sendTyping(true)
  }
  clearTimeout(typingTimer)
  typingTimer = window.setTimeout(() => {
    isTyping = false
    messages.sendTyping(false)
  }, 2000)
}

async function pickFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  try {
    const { data } = await attachmentApi.upload(file)
    pending.value.push(data)
  } catch {
    alert('Upload failed (max 10MB).')
  }
  if (fileInput.value) fileInput.value.value = ''
}

async function send() {
  const content = text.value.trim()
  if (!content && pending.value.length === 0) return
  await messages.send(content, pending.value.map((a) => a.id))
  text.value = ''
  pending.value = []
  isTyping = false
  messages.sendTyping(false)
}
</script>

<template>
  <div class="px-4 pb-4">
    <div v-if="pending.length" class="mb-2 flex gap-2">
      <div
        v-for="a in pending"
        :key="a.id"
        class="flex items-center gap-1 rounded bg-bg-input px-2 py-1 text-xs text-txt"
      >
        📎 {{ a.file_name }}
        <button class="text-red-400" @click="pending = pending.filter((p) => p.id !== a.id)">✕</button>
      </div>
    </div>

    <div class="flex items-end gap-2 rounded-lg bg-bg-input px-3 py-2">
      <button @click="fileInput?.click()" class="text-xl text-txt-muted hover:text-white" title="Attach">
        ＋
      </button>
      <input ref="fileInput" type="file" class="hidden" @change="pickFile" />
      <textarea
        v-model="text"
        @input="onInput"
        @keydown.enter.exact.prevent="send"
        :placeholder="`Message #${channel.name}`"
        rows="1"
        class="max-h-40 flex-1 resize-none bg-transparent text-txt outline-none placeholder:text-txt-muted"
      ></textarea>
      <button @click="send" class="font-medium text-blurple hover:text-white">Send</button>
    </div>
  </div>
</template>
