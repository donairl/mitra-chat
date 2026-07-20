<script setup lang="ts">
import { ref, computed } from 'vue'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { useAuthStore } from '@/stores/auth'
import CreateChannelModal from '@/components/CreateChannelModal.vue'

defineProps<{ activeChannel: string }>()
defineEmits<{ open: [id: string] }>()

const servers = useServersStore()
const channels = useChannelsStore()
const auth = useAuthStore()
const showCreate = ref(false)

const server = computed(() => servers.servers.find((s) => s.id === servers.currentServerId))
const isOwner = computed(() => server.value?.owner_id === auth.user?.id)
</script>

<template>
  <aside class="flex w-60 flex-col bg-bg-alt">
    <div class="flex h-12 items-center border-b border-black/30 px-4 font-semibold text-white shadow-sm">
      <span class="truncate">{{ server?.name }}</span>
    </div>

    <div class="flex-1 overflow-y-auto px-2 py-3">
      <div class="mb-1 flex items-center justify-between px-2">
        <span class="text-xs font-semibold uppercase tracking-wide text-txt-muted">Text Channels</span>
        <button
          v-if="isOwner"
          @click="showCreate = true"
          class="text-lg leading-none text-txt-muted hover:text-white"
          title="Create channel"
        >
          +
        </button>
      </div>

      <button
        v-for="c in channels.channels"
        :key="c.id"
        @click="$emit('open', c.id)"
        :class="[
          'group flex w-full items-center gap-1 rounded px-2 py-1.5 text-left text-txt-muted hover:bg-white/5 hover:text-txt',
          activeChannel === c.id ? 'bg-white/10 text-white' : '',
        ]"
      >
        <span class="text-lg text-txt-muted">#</span>
        <span class="truncate">{{ c.name }}</span>
      </button>
    </div>

    <div class="flex items-center gap-2 bg-bg-dark/60 px-3 py-2">
      <div class="flex h-8 w-8 items-center justify-center rounded-full bg-blurple text-sm font-semibold text-white">
        {{ auth.user?.username?.slice(0, 1).toUpperCase() }}
      </div>
      <div class="min-w-0">
        <div class="truncate text-sm font-medium text-white">{{ auth.user?.username }}</div>
        <div class="text-xs text-green-400">online</div>
      </div>
    </div>

    <CreateChannelModal v-if="showCreate" :server-id="servers.currentServerId" @close="showCreate = false" />
  </aside>
</template>
