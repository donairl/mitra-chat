<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { useMessagesStore } from '@/stores/messages'
import { useFriendsStore } from '@/stores/friends'
import { useDmStore } from '@/stores/dm'
import { useNotificationsStore } from '@/stores/notifications'
import ServerRail from '@/components/ServerRail.vue'
import ChannelSidebar from '@/components/ChannelSidebar.vue'
import ChatArea from '@/components/ChatArea.vue'
import MemberList from '@/components/MemberList.vue'
import FriendsPanel from '@/components/FriendsPanel.vue'
import NotificationBell from '@/components/NotificationBell.vue'

const auth = useAuthStore()
const servers = useServersStore()
const channels = useChannelsStore()
const messages = useMessagesStore()
const friends = useFriendsStore()
const dm = useDmStore()
const notifications = useNotificationsStore()

const mode = ref<'home' | 'server'>('home')
// In home mode: 'friends' shows the friends panel; any other value is a DM channel id.
const homeView = ref<'friends' | string>('friends')

const currentChannel = computed(() =>
  channels.channels.find((c) => c.id === channels.currentChannelId),
)
const currentDm = computed(() => dm.channels.find((c) => c.id === homeView.value))

onMounted(async () => {
  messages.wire()
  friends.wire()
  notifications.wire()
  if (!auth.user) await auth.fetchMe()
  await Promise.all([servers.fetch(), friends.fetch(), dm.fetch(), notifications.fetch()])
})

async function selectServer(id: string) {
  mode.value = 'server'
  await Promise.all([servers.selectServer(id), channels.fetch(id)])
  const first = channels.channels.find((c) => c.type === 'text')
  if (first) openChannel(first.id)
}

function goHome() {
  mode.value = 'home'
  channels.currentChannelId = ''
  homeView.value = 'friends'
}

function openChannel(id: string) {
  channels.select(id)
  messages.open(id)
}

function showFriends() {
  homeView.value = 'friends'
}

function openDmChannel(id: string) {
  homeView.value = id
  messages.open(id)
}

async function openDm(userId: string) {
  const ch = await dm.open(userId)
  openDmChannel(ch.id)
}
</script>

<template>
  <div class="flex h-full w-full overflow-hidden">
    <ServerRail
      :active="mode === 'server' ? servers.currentServerId : 'home'"
      @home="goHome"
      @select="selectServer"
    />

    <ChannelSidebar
      v-if="mode === 'server'"
      :active-channel="channels.currentChannelId"
      @open="openChannel"
    />
    <aside v-else class="flex w-60 flex-col bg-bg-alt">
      <div class="flex h-12 items-center border-b border-black/20 px-4 font-semibold text-white">
        Direct Messages
      </div>
      <div class="flex-1 overflow-y-auto p-2">
        <button
          @click="showFriends"
          :class="[
            'mb-1 flex w-full items-center gap-2 rounded px-2 py-2 text-left hover:bg-white/5',
            homeView === 'friends' ? 'bg-white/10 text-white' : 'text-txt-muted',
          ]"
        >
          <span>👥</span> Friends
        </button>
        <button
          v-for="c in dm.channels"
          :key="c.id"
          @click="openDmChannel(c.id)"
          :class="[
            'flex w-full items-center gap-2 rounded px-2 py-2 text-left hover:bg-white/5',
            homeView === c.id ? 'bg-white/10 text-white' : 'text-txt-muted',
          ]"
        >
          <div class="flex h-7 w-7 items-center justify-center rounded-full bg-secondary text-xs font-semibold text-white">
            {{ (c.dm_user?.username || '?').slice(0, 1).toUpperCase() }}
          </div>
          <span class="truncate">{{ c.dm_user?.username || 'Direct message' }}</span>
        </button>
      </div>
    </aside>

    <main class="flex min-w-0 flex-1 flex-col bg-bg">
      <header class="flex h-12 items-center justify-between border-b border-black/20 px-4">
        <div class="truncate font-semibold text-white">
          <span v-if="mode === 'server' && currentChannel">#{{ currentChannel.name }}</span>
          <span v-else-if="mode === 'home' && currentDm">@{{ currentDm.dm_user?.username }}</span>
          <span v-else-if="mode === 'home'">Friends</span>
          <span v-else>Home</span>
          <span v-if="currentChannel?.topic" class="ml-3 text-sm font-normal text-txt-muted">
            {{ currentChannel.topic }}
          </span>
        </div>
        <div class="flex items-center gap-3">
          <NotificationBell />
          <button
            @click="auth.logout()"
            class="rounded px-2 py-1 text-sm text-txt-muted hover:bg-white/5 hover:text-white"
          >
            Logout
          </button>
        </div>
      </header>

      <ChatArea v-if="mode === 'server' && currentChannel" :channel="currentChannel" />
      <ChatArea v-else-if="mode === 'home' && currentDm" :channel="currentDm" />
      <FriendsPanel v-else @open-dm="openDm" />
    </main>

    <MemberList v-if="mode === 'server'" />
  </div>
</template>
