<script setup lang="ts">
import { computed } from 'vue'
import { useServersStore } from '@/stores/servers'
import { useFriendsStore } from '@/stores/friends'

const servers = useServersStore()
const friends = useFriendsStore()

const sorted = computed(() =>
  [...servers.members].sort((a, b) => {
    const ao = friends.online.has(a.user_id) ? 0 : 1
    const bo = friends.online.has(b.user_id) ? 0 : 1
    return ao - bo
  }),
)
</script>

<template>
  <aside class="hidden w-60 flex-col bg-bg-alt lg:flex">
    <div class="flex h-12 items-center border-b border-black/20 px-4 text-xs font-semibold uppercase text-txt-muted">
      Members — {{ servers.members.length }}
    </div>
    <div class="flex-1 overflow-y-auto p-2">
      <div
        v-for="m in sorted"
        :key="m.id"
        class="flex items-center gap-2 rounded px-2 py-1.5 hover:bg-white/5"
      >
        <div class="relative">
          <div class="flex h-8 w-8 items-center justify-center rounded-full bg-secondary text-sm font-semibold text-white">
            {{ m.user?.username?.slice(0, 1).toUpperCase() }}
          </div>
          <span
            class="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-bg-alt"
            :class="friends.online.has(m.user_id) ? 'bg-green-500' : 'bg-gray-500'"
          ></span>
        </div>
        <span class="truncate text-sm text-txt">{{ m.user?.username }}</span>
        <span v-if="m.role === 'owner'" class="ml-auto text-xs text-yellow-500" title="Owner">👑</span>
      </div>
    </div>
  </aside>
</template>
