<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useMessagesStore } from '@/stores/messages'
import type { Message } from '@/types'

const props = defineProps<{ message: Message; prev?: Message }>()
const auth = useAuthStore()
const messages = useMessagesStore()

const isOwn = computed(() => props.message.user_id === auth.user?.id)
const grouped = computed(
  () =>
    props.prev?.user_id === props.message.user_id &&
    new Date(props.message.created_at).getTime() - new Date(props.prev.created_at).getTime() <
      5 * 60 * 1000,
)

const editing = ref(false)
const draft = ref('')

function time(ts: string) {
  return new Date(ts).toLocaleString([], { hour: '2-digit', minute: '2-digit', month: 'short', day: 'numeric' })
}

function startEdit() {
  editing.value = true
  draft.value = props.message.content
}
function saveEdit() {
  if (draft.value.trim()) messages.edit(props.message.id, draft.value.trim())
  editing.value = false
}
function isImage(t: string) {
  return t?.startsWith('image/')
}
</script>

<template>
  <div :class="['group relative flex gap-3 px-2 hover:bg-white/[0.02]', grouped ? 'mt-0.5' : 'mt-4']">
    <div class="w-10 shrink-0">
      <div
        v-if="!grouped"
        class="flex h-10 w-10 items-center justify-center rounded-full bg-secondary text-sm font-semibold text-white"
      >
        {{ message.user?.username?.slice(0, 1).toUpperCase() }}
      </div>
    </div>

    <div class="min-w-0 flex-1">
      <div v-if="!grouped" class="flex items-baseline gap-2">
        <span class="font-medium text-white">{{ message.user?.username || 'Unknown' }}</span>
        <span class="text-xs text-txt-muted">{{ time(message.created_at) }}</span>
      </div>

      <div v-if="!editing" class="whitespace-pre-wrap break-words text-txt">
        {{ message.content }}
        <span v-if="message.is_edited" class="ml-1 text-xs text-txt-muted">(edited)</span>
      </div>
      <div v-else class="mt-1">
        <textarea
          v-model="draft"
          @keydown.enter.prevent="saveEdit"
          @keydown.esc="editing = false"
          class="w-full rounded bg-bg-input px-2 py-1 text-txt outline-none"
          rows="2"
        ></textarea>
        <div class="mt-1 text-xs text-txt-muted">
          escape to <button class="text-blurple" @click="editing = false">cancel</button> • enter to
          <button class="text-blurple" @click="saveEdit">save</button>
        </div>
      </div>

      <div v-for="a in message.attachments" :key="a.id" class="mt-1">
        <img
          v-if="isImage(a.file_type)"
          :src="a.file_path"
          :alt="a.file_name"
          class="max-h-80 max-w-md rounded"
        />
        <a v-else :href="a.file_path" target="_blank" class="text-blurple hover:underline">
          📎 {{ a.file_name }}
        </a>
      </div>
    </div>

    <div
      v-if="isOwn && !editing"
      class="absolute right-2 top-0 hidden gap-2 rounded bg-bg-dark px-2 py-1 text-xs text-txt-muted group-hover:flex"
    >
      <button @click="startEdit" class="hover:text-white">Edit</button>
      <button @click="messages.remove(message.id)" class="hover:text-red-400">Delete</button>
    </div>
  </div>
</template>
