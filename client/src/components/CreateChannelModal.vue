<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useChannelsStore } from '@/stores/channels'
import { CHANNEL_ROLES, rank } from '@/permissions'
import type { Channel, Role } from '@/types'

// Creates a channel, or edits/deletes `channel` when one is passed.
const props = defineProps<{ serverId: string; channel?: Channel }>()
const emit = defineEmits<{ close: [] }>()
const channels = useChannelsStore()

const TIER_LABELS: Record<Role, string> = {
  member: 'Everyone',
  moderator: 'Moderators and above',
  admin: 'Admins and above',
  owner: 'Owner',
}

const name = ref(props.channel?.name ?? '')
const topic = ref(props.channel?.topic ?? '')
const viewRole = ref<Role>(props.channel?.min_view_role ?? 'member')
const postRole = ref<Role>(props.channel?.min_post_role ?? 'member')
const error = ref('')

// Posting can never be open to more people than viewing.
const postOptions = computed(() => CHANNEL_ROLES.filter((r) => rank(r) >= rank(viewRole.value)))
watch(viewRole, (v) => {
  if (rank(postRole.value) < rank(v)) postRole.value = v
})

async function submit() {
  error.value = ''
  const body = {
    name: name.value,
    topic: topic.value,
    min_view_role: viewRole.value,
    min_post_role: postRole.value,
  }
  try {
    if (props.channel) await channels.update(props.channel.id, body)
    else await channels.create(props.serverId, body)
    emit('close')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed'
  }
}

async function remove() {
  if (!props.channel) return
  if (!window.confirm(`Delete #${props.channel.name}? All of its messages are deleted too.`)) return
  try {
    await channels.remove(props.channel.id)
    emit('close')
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Failed'
  }
}
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60" @click.self="emit('close')">
    <div class="w-full max-w-sm rounded-lg bg-bg-alt p-6">
      <h2 class="mb-4 text-lg font-bold text-white">{{ channel ? 'Edit Channel' : 'Create Channel' }}</h2>
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Channel name</label>
      <input
        v-model="name"
        placeholder="new-channel"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Topic (optional)</label>
      <input
        v-model="topic"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      />
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Who can view</label>
      <select
        v-model="viewRole"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      >
        <option v-for="r in CHANNEL_ROLES" :key="r" :value="r">{{ TIER_LABELS[r] }}</option>
      </select>
      <label class="mb-1 block text-xs font-semibold uppercase text-txt-muted">Who can post</label>
      <select
        v-model="postRole"
        class="mb-3 w-full rounded bg-bg-input px-3 py-2 text-txt outline-none focus:ring-2 focus:ring-blurple"
      >
        <option v-for="r in postOptions" :key="r" :value="r">{{ TIER_LABELS[r] }}</option>
      </select>
      <p v-if="error" class="mb-2 text-sm text-red-400">{{ error }}</p>
      <div class="flex items-center gap-2">
        <button v-if="channel" class="px-3 py-2 text-sm text-red-400 hover:text-red-300" @click="remove">
          Delete
        </button>
        <div class="flex-1"></div>
        <button class="px-3 py-2 text-sm text-txt-muted hover:text-white" @click="emit('close')">Cancel</button>
        <button class="rounded bg-blurple px-4 py-2 text-sm font-medium text-white hover:bg-blurple-dark" @click="submit">
          {{ channel ? 'Save' : 'Create' }}
        </button>
      </div>
    </div>
  </div>
</template>
