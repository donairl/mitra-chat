<script setup lang="ts">
import { ref } from 'vue'
import { useServersStore } from '@/stores/servers'
import CreateServerModal from '@/components/CreateServerModal.vue'
import JoinServerModal from '@/components/JoinServerModal.vue'

defineProps<{ active: string }>()
defineEmits<{ home: []; select: [id: string] }>()

const servers = useServersStore()
const showCreate = ref(false)
const showJoin = ref(false)

function initials(name: string) {
  return name.slice(0, 2).toUpperCase()
}
</script>

<template>
  <nav class="flex w-[72px] flex-col items-center gap-2 bg-bg-dark py-3">
    <button
      @click="$emit('home')"
      :class="[
        'flex h-12 w-12 items-center justify-center rounded-3xl bg-bg-alt text-white transition-all hover:rounded-2xl hover:bg-blurple',
        active === 'home' ? 'rounded-2xl bg-blurple' : '',
      ]"
      title="Home"
    >
      🏠
    </button>
    <div class="h-px w-8 bg-white/10"></div>

    <button
      v-for="s in servers.servers"
      :key="s.id"
      @click="$emit('select', s.id)"
      :class="[
        'flex h-12 w-12 items-center justify-center rounded-3xl bg-bg-alt text-sm font-semibold text-white transition-all hover:rounded-2xl hover:bg-blurple',
        active === s.id ? 'rounded-2xl bg-blurple' : '',
      ]"
      :title="s.name"
    >
      <img v-if="s.icon" :src="s.icon" class="h-full w-full rounded-[inherit] object-cover" />
      <span v-else>{{ initials(s.name) }}</span>
    </button>

    <button
      @click="showCreate = true"
      class="flex h-12 w-12 items-center justify-center rounded-3xl bg-bg-alt text-2xl text-green-400 transition-all hover:rounded-2xl hover:bg-green-600 hover:text-white"
      title="Create server"
    >
      +
    </button>
    <button
      @click="showJoin = true"
      class="flex h-10 w-12 items-center justify-center rounded-3xl bg-bg-alt text-xs text-txt-muted transition-all hover:rounded-2xl hover:bg-blurple hover:text-white"
      title="Join server"
    >
      Join
    </button>

    <CreateServerModal v-if="showCreate" @close="showCreate = false" @created="$emit('select', $event)" />
    <JoinServerModal v-if="showJoin" @close="showJoin = false" @joined="$emit('select', $event)" />
  </nav>
</template>
