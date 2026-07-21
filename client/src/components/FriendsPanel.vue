<script setup lang="ts">
import { ref } from 'vue'
import { useFriendsStore } from '@/stores/friends'

const emit = defineEmits<{ openDm: [userId: string] }>()
const friends = useFriendsStore()
const tab = ref<'friends' | 'requests' | 'add'>('friends')
const addName = ref('')
const addMsg = ref('')

async function submitAdd() {
  addMsg.value = ''
  try {
    await friends.sendRequest(addName.value.trim())
    addMsg.value = 'Friend request sent!'
    addName.value = ''
  } catch (e: any) {
    addMsg.value = e.response?.data?.error || 'Failed to send request'
  }
}
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col p-6">
    <div class="mb-4 flex gap-2 border-b border-white/10 pb-3">
      <button
        v-for="t in (['friends', 'requests', 'add'] as const)"
        :key="t"
        @click="tab = t"
        :class="[
          'rounded px-3 py-1 text-sm capitalize',
          tab === t ? 'bg-blurple text-white' : 'text-txt-muted hover:bg-white/5 hover:text-white',
        ]"
      >
        {{ t }}
        <span
          v-if="t === 'requests' && friends.requests.length"
          class="ml-1 rounded-full bg-red-500 px-1.5 text-xs text-white"
        >
          {{ friends.requests.length }}
        </span>
      </button>
    </div>

    <div v-if="tab === 'friends'" class="min-h-0 flex-1 overflow-y-auto">
      <p v-if="!friends.friends.length" class="text-sm text-txt-muted">No friends yet. Add some!</p>
      <div
        v-for="f in friends.friends"
        :key="f.id"
        class="flex items-center gap-3 rounded px-2 py-2 hover:bg-white/5"
      >
        <div class="relative">
          <div class="flex h-9 w-9 items-center justify-center rounded-full bg-secondary font-semibold text-white">
            {{ f.username.slice(0, 1).toUpperCase() }}
          </div>
          <span
            class="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-bg"
            :class="friends.online.has(f.id) ? 'bg-green-500' : 'bg-gray-500'"
          ></span>
        </div>
        <button
          @click="emit('openDm', f.id)"
          class="text-txt hover:underline"
          title="Open direct message"
        >
          {{ f.username }}
        </button>
        <button
          @click="friends.remove(f.id)"
          class="ml-auto text-xs text-txt-muted hover:text-red-400"
        >
          Remove
        </button>
      </div>
    </div>

    <div v-else-if="tab === 'requests'" class="min-h-0 flex-1 overflow-y-auto">
      <p v-if="!friends.requests.length" class="text-sm text-txt-muted">No pending requests.</p>
      <div
        v-for="r in friends.requests"
        :key="r.id"
        class="flex items-center gap-3 rounded px-2 py-2 hover:bg-white/5"
      >
        <div class="flex h-9 w-9 items-center justify-center rounded-full bg-secondary font-semibold text-white">
          {{ r.user?.username?.slice(0, 1).toUpperCase() }}
        </div>
        <span class="text-txt">{{ r.user?.username }}</span>
        <div class="ml-auto flex gap-2">
          <button
            @click="friends.accept(r.id)"
            class="rounded bg-green-600 px-2 py-1 text-xs text-white hover:bg-green-500"
          >
            Accept
          </button>
          <button
            @click="friends.reject(r.id)"
            class="rounded bg-white/10 px-2 py-1 text-xs text-txt hover:bg-white/20"
          >
            Reject
          </button>
        </div>
      </div>
    </div>

    <div v-else class="max-w-md">
      <p class="mb-2 text-sm text-txt-muted">Add a friend by their username.</p>
      <div class="flex gap-2">
        <input
          v-model="addName"
          @keydown.enter="submitAdd"
          placeholder="username"
          class="flex-1 rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
        />
        <button
          @click="submitAdd"
          class="rounded bg-blurple px-4 py-2 font-medium text-white hover:bg-blurple-dark"
        >
          Send
        </button>
      </div>
      <p v-if="addMsg" class="mt-2 text-sm text-txt-muted">{{ addMsg }}</p>
    </div>
  </div>
</template>
