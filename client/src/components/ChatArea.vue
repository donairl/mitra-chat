<script setup lang="ts">
import { computed } from 'vue'
import { useMessagesStore } from '@/stores/messages'
import { useAuthStore } from '@/stores/auth'
import type { Channel } from '@/types'
import MessageList from '@/components/MessageList.vue'
import MessageInput from '@/components/MessageInput.vue'

defineProps<{ channel: Channel }>()

const messages = useMessagesStore()
const auth = useAuthStore()

const typers = computed(() =>
  Object.entries(messages.typing)
    .filter(([id]) => id !== auth.user?.id)
    .map(([, t]) => t.username),
)
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <MessageList />
    <div class="h-6 px-4 text-xs text-txt-muted">
      <span v-if="typers.length">
        {{ typers.join(', ') }} {{ typers.length > 1 ? 'are' : 'is' }} typing…
      </span>
    </div>
    <MessageInput :channel="channel" />
  </div>
</template>
