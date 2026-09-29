<script setup lang="ts">
import { ref, computed } from 'vue'
import { apiError } from '@/api'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { useAuthStore } from '@/stores/auth'
import { can } from '@/permissions'
import type { Channel } from '@/types'
import CreateChannelModal from '@/components/CreateChannelModal.vue'
import InviteServerModal from '@/components/InviteServerModal.vue'
import ServerSettingsModal from '@/components/ServerSettingsModal.vue'

defineProps<{ activeChannel: string }>()
defineEmits<{ open: [id: string] }>()

const servers = useServersStore()
const channels = useChannelsStore()
const auth = useAuthStore()
const showCreate = ref(false)
const showInvite = ref(false)
const showSettings = ref(false)
const showMenu = ref(false)
const editing = ref<Channel | null>(null) // channel whose edit modal is open

const server = computed(() => servers.servers.find((s) => s.id === servers.currentServerId))
const canManageChannels = computed(() => can(servers.myRole, 'manageChannels'))
// Settings has an Overview tab for admins and a Bans tab for moderators.
const canOpenSettings = computed(
  () => can(servers.myRole, 'manageServer') || can(servers.myRole, 'ban'),
)
const canLeave = computed(() => !!servers.myRole && servers.myRole !== 'owner')

function openFromMenu(which: 'invite' | 'settings') {
  showMenu.value = false
  if (which === 'invite') showInvite.value = true
  else showSettings.value = true
}

async function leave() {
  showMenu.value = false
  if (!server.value || !window.confirm(`Leave ${server.value.name}?`)) return
  try {
    await servers.leave(server.value.id)
  } catch (e: unknown) {
    alert(apiError(e, 'Could not leave server'))
  }
}
</script>

<template>
  <aside class="flex w-60 flex-col bg-bg-alt">
    <div class="relative border-b border-black/30 shadow-sm">
      <button
        v-if="server"
        @click="showMenu = !showMenu"
        class="flex h-12 w-full items-center justify-between px-4 font-semibold text-white hover:bg-white/5"
      >
        <span class="truncate">{{ server.name }}</span>
        <span class="ml-2 shrink-0 text-xs text-txt-muted">{{ showMenu ? '✕' : '▾' }}</span>
      </button>
      <div v-else class="h-12"></div>
      <div
        v-if="showMenu"
        class="absolute left-2 right-2 top-12 z-40 flex flex-col rounded bg-bg-dark py-1 text-sm shadow-lg"
      >
        <button class="px-3 py-2 text-left text-txt hover:bg-blurple hover:text-white" @click="openFromMenu('invite')">
          Invite people
        </button>
        <button
          v-if="canOpenSettings"
          class="px-3 py-2 text-left text-txt hover:bg-blurple hover:text-white"
          @click="openFromMenu('settings')"
        >
          Server settings
        </button>
        <button v-if="canLeave" class="px-3 py-2 text-left text-red-400 hover:bg-red-500 hover:text-white" @click="leave">
          Leave server
        </button>
      </div>
    </div>

    <div class="flex-1 overflow-y-auto px-2 py-3">
      <div class="mb-1 flex items-center justify-between px-2">
        <span class="text-xs font-semibold uppercase tracking-wide text-txt-muted">Text Channels</span>
        <button
          v-if="canManageChannels"
          @click="showCreate = true"
          class="text-lg leading-none text-txt-muted hover:text-white"
          title="Create channel"
        >
          +
        </button>
      </div>

      <div
        v-for="c in channels.channels"
        :key="c.id"
        :class="['group flex items-center rounded hover:bg-white/5', activeChannel === c.id ? 'bg-white/10' : '']"
      >
        <button
          @click="$emit('open', c.id)"
          :class="[
            'flex min-w-0 flex-1 items-center gap-1 px-2 py-1.5 text-left',
            activeChannel === c.id ? 'text-white' : 'text-txt-muted hover:text-txt',
          ]"
        >
          <span class="text-lg text-txt-muted">#</span>
          <span class="truncate">{{ c.name }}</span>
          <span v-if="c.min_view_role && c.min_view_role !== 'member'" class="text-xs" title="Private channel">🔒</span>
        </button>
        <button
          v-if="canManageChannels"
          @click="editing = c"
          class="mr-1 hidden px-1 text-sm text-txt-muted hover:text-white group-hover:block group-focus-within:block [@media(hover:none)]:block"
          :aria-label="`Edit #${c.name}`"
          title="Edit channel"
        >
          ⚙
        </button>
      </div>
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
    <CreateChannelModal
      v-if="editing"
      :server-id="servers.currentServerId"
      :channel="editing"
      @close="editing = null"
    />
    <InviteServerModal v-if="showInvite" :server-id="servers.currentServerId" @close="showInvite = false" />
    <ServerSettingsModal v-if="showSettings && server" :server="server" @close="showSettings = false" />
  </aside>
</template>
