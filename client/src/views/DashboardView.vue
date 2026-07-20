<script setup lang="ts">
import { onMounted, ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { useMessagesStore } from '@/stores/messages'
import { useFriendsStore } from '@/stores/friends'
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
const notifications = useNotificationsStore()

const mode = ref<'home' | 'server'>('home')

const currentChannel = computed(() =>
  channels.channels.find((c) => c.id === channels.currentChannelId),
)

onMounted(async () => {
  messages.wire()
  friends.wire()
  notifications.wire()
  if (!auth.user) await auth.fetchMe()
  await Promise.all([servers.fetch(), friends.fetch(), notifications.fetch()])
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
}

function openChannel(id: string) {
  channels.select(id)
  messages.open(id)
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
      <div class="p-3 text-sm text-txt-muted">Your friends &amp; requests appear on the right.</div>
    </aside>

    <main class="flex min-w-0 flex-1 flex-col bg-bg">
      <header class="flex h-12 items-center justify-between border-b border-black/20 px-4">
        <div class="truncate font-semibold text-white">
          <span v-if="mode === 'server' && currentChannel">#{{ currentChannel.name }}</span>
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
      <FriendsPanel v-else />
    </main>

    <MemberList v-if="mode === 'server'" />
  </div>
</template>
