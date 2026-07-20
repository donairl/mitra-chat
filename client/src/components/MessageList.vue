<script setup lang="ts">
import { ref, watch, nextTick, onMounted } from 'vue'
import { useMessagesStore } from '@/stores/messages'
import MessageItem from '@/components/MessageItem.vue'

const messages = useMessagesStore()
const container = ref<HTMLElement | null>(null)

function atBottom() {
  const el = container.value
  if (!el) return true
  return el.scrollHeight - el.scrollTop - el.clientHeight < 120
}

function scrollBottom() {
  nextTick(() => {
    const el = container.value
    if (el) el.scrollTop = el.scrollHeight
  })
}

async function onScroll() {
  const el = container.value
  if (!el || el.scrollTop > 40 || !messages.hasMore || messages.loading) return
  const prevHeight = el.scrollHeight
  await messages.load()
  await nextTick()
  el.scrollTop = el.scrollHeight - prevHeight
}

let lastLen = 0
watch(
  () => messages.messages.length,
  (len) => {
    if (len > lastLen && atBottom()) scrollBottom()
    lastLen = len
  },
)
watch(
  () => messages.channelId,
  () => {
    lastLen = 0
    scrollBottom()
  },
)

onMounted(scrollBottom)
</script>

<template>
  <div ref="container" @scroll="onScroll" class="flex-1 overflow-y-auto px-4 py-4">
    <div v-if="messages.loading && messages.messages.length === 0" class="text-sm text-txt-muted">
      Loading messages…
    </div>
    <p v-if="!messages.hasMore" class="mb-4 text-center text-xs text-txt-muted">
      — Beginning of this channel —
    </p>
    <MessageItem
      v-for="(m, i) in messages.messages"
      :key="m.id"
      :message="m"
      :prev="messages.messages[i - 1]"
    />
  </div>
</template>
