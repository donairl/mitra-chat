<script setup lang="ts">
import { ref } from 'vue'
import { useNotificationsStore } from '@/stores/notifications'

const notifications = useNotificationsStore()
const open = ref(false)
</script>

<template>
  <div class="relative">
    <button @click="open = !open" class="relative text-lg text-txt-muted hover:text-white" title="Notifications">
      🔔
      <span
        v-if="notifications.unread"
        class="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-1 text-[10px] font-bold text-white"
      >
        {{ notifications.unread }}
      </span>
    </button>

    <div
      v-if="open"
      class="absolute right-0 top-8 z-20 w-72 rounded-lg bg-bg-alt shadow-2xl ring-1 ring-black/30"
    >
      <div class="flex items-center justify-between border-b border-white/10 px-3 py-2">
        <span class="text-sm font-semibold text-white">Notifications</span>
        <button class="text-xs text-blurple hover:underline" @click="notifications.markAll()">
          Mark all read
        </button>
      </div>
      <div class="max-h-80 overflow-y-auto">
        <p v-if="!notifications.items.length" class="px-3 py-4 text-sm text-txt-muted">
          Nothing here yet.
        </p>
        <button
          v-for="n in notifications.items"
          :key="n.id"
          @click="notifications.markRead(n.id)"
          class="block w-full border-b border-white/5 px-3 py-2 text-left text-sm hover:bg-white/5"
          :class="n.read ? 'text-txt-muted' : 'text-txt'"
        >
          <span v-if="!n.read" class="mr-1 text-blurple">●</span>{{ n.content }}
        </button>
      </div>
    </div>
  </div>
</template>
