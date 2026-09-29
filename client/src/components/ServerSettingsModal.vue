<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useServersStore } from '@/stores/servers'
import { can } from '@/permissions'
import type { Server, ServerBan } from '@/types'

// Server settings, with one tab per capability the viewer holds.
const props = defineProps<{ server: Server }>()
const emit = defineEmits<{ close: [] }>()
const servers = useServersStore()

type Tab = 'overview' | 'bans' | 'danger'
const tabs = computed(() => {
  const list: { id: Tab; label: string }[] = []
  if (can(servers.myRole, 'manageServer')) list.push({ id: 'overview', label: 'Overview' })
  if (can(servers.myRole, 'ban')) list.push({ id: 'bans', label: 'Bans' })
  if (can(servers.myRole, 'deleteServer')) list.push({ id: 'danger', label: 'Delete server' })
  return list
})
const tab = ref<Tab>(tabs.value[0]?.id ?? 'bans')

const name = ref(props.server.name)
const description = ref(props.server.description ?? '')
const icon = ref(props.server.icon ?? '')
const inviteCode = ref(props.server.invite_code ?? '')
const bans = ref<ServerBan[]>([])
const error = ref('')
const saved = ref(false)

function fail(e: any, fallback: string) {
  error.value = e.response?.data?.error || fallback
}

onMounted(async () => {
  if (!can(servers.myRole, 'ban')) return
  try {
    bans.value = await servers.listBans()
  } catch (e) {
    fail(e, 'Could not load bans')
  }
})

async function save() {
  error.value = ''
  saved.value = false
  try {
    await servers.update(props.server.id, {
      name: name.value,
      description: description.value,
      icon: icon.value,
    })
    saved.value = true
  } catch (e) {
    fail(e, 'Could not save')
  }
}

async function regenerate() {
  if (!window.confirm('Regenerate the invite code? The old code stops working.')) return
  try {
    inviteCode.value = await servers.regenerateInvite(props.server.id)
  } catch (e) {
    fail(e, 'Could not regenerate invite')
  }
}

async function unban(userId: string) {
  try {
    await servers.unban(userId)
    bans.value = bans.value.filter((b) => b.user_id !== userId)
  } catch (e) {
    fail(e, 'Could not unban')
  }
}

async function removeServer() {
  if (!window.confirm(`Delete ${props.server.name}? This cannot be undone.`)) return
  try {
    await servers.remove(props.server.id)
    emit('close')
  } catch (e) {
    fail(e, 'Could not delete server')
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="emit('close')">
    <div class="w-full max-w-lg rounded-lg bg-bg-alt p-6">
      <h2 class="mb-4 text-lg font-bold text-white">Server Settings</h2>
      <div class="mb-4 flex gap-2 border-b border-black/30">
        <button
          v-for="t in tabs"
          :key="t.id"
          @click="tab = t.id"
          :class="[
            '-mb-px border-b-2 px-3 py-2 text-sm',
            tab === t.id ? 'border-blurple text-white' : 'border-transparent text-txt-muted hover:text-white',
          ]"
        >
          {{ t.label }}
        </button>
      </div>

      <div v-if="tab === 'overview'">
        <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Server name</label>
        <input
          v-model="name"
          class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
        />
        <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Description</label>
        <input
          v-model="description"
          class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
        />
        <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Icon URL</label>
        <input
          v-model="icon"
          class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
        />
        <div class="mb-4 flex items-center justify-end gap-3">
          <span v-if="saved" class="text-sm text-green-400">Saved</span>
          <button class="rounded bg-blurple px-4 py-2 text-sm font-medium text-white hover:bg-blurple-dark" @click="save">
            Save
          </button>
        </div>
        <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Invite code</label>
        <div class="flex gap-2">
          <input :value="inviteCode" readonly class="w-full rounded bg-bg-input px-3 py-2 font-mono text-txt outline-none" />
          <button class="shrink-0 rounded bg-bg-input px-3 py-2 text-sm text-txt hover:text-white" @click="regenerate">
            Regenerate
          </button>
        </div>
      </div>

      <div v-else-if="tab === 'bans'">
        <p v-if="bans.length === 0" class="text-sm text-txt-muted">No banned users.</p>
        <div
          v-for="b in bans"
          :key="b.id"
          class="flex items-center justify-between rounded px-2 py-1.5 hover:bg-white/5"
        >
          <span class="truncate text-sm text-txt">{{ b.user?.username || b.user_id }}</span>
          <button class="text-sm text-blurple hover:underline" @click="unban(b.user_id)">Unban</button>
        </div>
      </div>

      <div v-else-if="tab === 'danger'">
        <p class="mb-3 text-sm text-txt-muted">
          Deleting the server removes every channel and message. This cannot be undone.
        </p>
        <button class="rounded bg-red-600 px-4 py-2 text-sm font-medium text-white hover:bg-red-700" @click="removeServer">
          Delete server
        </button>
      </div>

      <p v-if="error" class="mt-3 text-sm text-red-400">{{ error }}</p>
      <div class="mt-4 flex justify-end">
        <button class="px-3 py-2 text-sm text-txt-muted hover:text-white" @click="emit('close')">Close</button>
      </div>
    </div>
  </div>
</template>
