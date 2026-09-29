<script setup lang="ts">
import { computed, ref } from 'vue'
import { apiError } from '@/api'
import { useServersStore } from '@/stores/servers'
import { useFriendsStore } from '@/stores/friends'
import { assignableRoles, can, canActOn } from '@/permissions'
import type { Role, ServerMember } from '@/types'

const servers = useServersStore()
const friends = useFriendsStore()
const menuFor = ref('') // user id whose action menu is open
const error = ref('')

const GROUPS: { role: Role; label: string }[] = [
  { role: 'owner', label: 'Owner' },
  { role: 'admin', label: 'Admins' },
  { role: 'moderator', label: 'Moderators' },
  { role: 'member', label: 'Members' },
]
const BADGE: Record<Role, string> = { owner: '👑', admin: '🛡️', moderator: '🔨', member: '' }

// Members bucketed by role (highest first), online members first in each bucket.
const groups = computed(() =>
  GROUPS.map((g) => ({
    ...g,
    members: servers.members
      .filter((m) => m.role === g.role)
      .sort(
        (a, b) => Number(!friends.online.has(a.user_id)) - Number(!friends.online.has(b.user_id)),
      ),
  })).filter((g) => g.members.length > 0),
)

// Roles the viewer may give m, excluding the one m already has.
function roleChoices(m: ServerMember): Role[] {
  return assignableRoles(servers.myRole).filter((r) => r !== m.role)
}

async function run(action: () => Promise<unknown>) {
  error.value = ''
  menuFor.value = ''
  try {
    await action()
  } catch (e: unknown) {
    error.value = apiError(e, 'Action failed')
  }
}

function kick(m: ServerMember) {
  if (window.confirm(`Kick ${m.user?.username}? They can rejoin with an invite.`)) {
    run(() => servers.kick(m.user_id))
  }
}

function ban(m: ServerMember) {
  if (window.confirm(`Ban ${m.user?.username}? They will not be able to rejoin.`)) {
    run(() => servers.ban(m.user_id))
  }
}

function setRole(m: ServerMember, role: Role) {
  run(() => servers.setRole(m.user_id, role))
}
</script>

<template>
  <aside class="hidden w-60 flex-col bg-bg-alt lg:flex">
    <div class="flex h-12 items-center border-b border-black/20 px-4 text-xs font-semibold uppercase text-txt-muted">
      Members — {{ servers.members.length }}
    </div>
    <div class="flex-1 overflow-y-auto p-2">
      <p v-if="error" class="mb-2 px-2 text-xs text-red-400">{{ error }}</p>
      <section v-for="g in groups" :key="g.role" class="mb-3">
        <h3 class="px-2 pb-1 text-xs font-semibold uppercase text-txt-muted">
          {{ g.label }} — {{ g.members.length }}
        </h3>
        <div v-for="m in g.members" :key="m.id">
          <div class="group flex items-center gap-2 rounded px-2 py-1.5 hover:bg-white/5">
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
            <span v-if="BADGE[m.role]" class="text-xs" :title="m.role">{{ BADGE[m.role] }}</span>
            <button
              v-if="canActOn(servers.myRole, m.role)"
              @click="menuFor = menuFor === m.user_id ? '' : m.user_id"
              class="ml-auto hidden px-1 text-txt-muted hover:text-white group-hover:block"
              title="Member actions"
            >
              ⋯
            </button>
          </div>
          <div v-if="menuFor === m.user_id" class="mb-1 ml-10 flex flex-col rounded bg-bg-dark py-1 text-sm">
            <template v-if="can(servers.myRole, 'manageRoles')">
              <button
                v-for="r in roleChoices(m)"
                :key="r"
                class="px-3 py-1 text-left text-txt hover:bg-white/5"
                @click="setRole(m, r)"
              >
                Make {{ r }}
              </button>
            </template>
            <button class="px-3 py-1 text-left text-red-400 hover:bg-white/5" @click="kick(m)">Kick</button>
            <button class="px-3 py-1 text-left text-red-400 hover:bg-white/5" @click="ban(m)">Ban</button>
          </div>
        </div>
      </section>
    </div>
  </aside>
</template>
