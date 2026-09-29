import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { serverApi } from '@/api'
import { socket } from '@/ws/socket'
import { useAuthStore } from '@/stores/auth'
import { useChannelsStore } from '@/stores/channels'
import type { Role, Server, ServerBan, ServerMember } from '@/types'

// Servers store: the user's server list, the selected server, its member list,
// the current user's role there, and moderation actions. Membership and server
// changes made by other people arrive over the socket (see wire()).
export const useServersStore = defineStore('servers', () => {
  const auth = useAuthStore()
  const channels = useChannelsStore()
  const servers = ref<Server[]>([])
  const currentServerId = ref<string>('')
  const members = ref<ServerMember[]>([])
  // One-line message for the dashboard banner, e.g. "You were kicked from X."
  const notice = ref('')
  let wired = false // one-time guard so socket handlers register only once

  // The current user's role in the selected server (undefined until members load).
  const myRole = computed<Role | undefined>(
    () => members.value.find((m) => m.user_id === auth.user?.id)?.role,
  )

  async function fetch() {
    const { data } = await serverApi.list()
    servers.value = data
  }

  async function create(b: { name: string; description?: string; icon?: string }) {
    const { data } = await serverApi.create(b)
    servers.value.push(data)
    return data
  }

  async function join(inviteCode: string) {
    const { data } = await serverApi.join(inviteCode)
    // Avoid duplicates when re-joining a server already in the list.
    if (!servers.value.find((s) => s.id === data.id)) servers.value.push(data)
    return data
  }

  async function invite(id: string) {
    const { data } = await serverApi.invite(id)
    const s = servers.value.find((x) => x.id === id)
    if (s) s.invite_code = data.invite_code
    return data.invite_code
  }

  async function regenerateInvite(id: string) {
    const { data } = await serverApi.regenerateInvite(id)
    const s = servers.value.find((x) => x.id === id)
    if (s) s.invite_code = data.invite_code
    return data.invite_code
  }

  async function update(id: string, b: { name: string; description?: string; icon?: string }) {
    const { data } = await serverApi.update(id, b)
    patchServer(data)
  }

  // Switch active server and load its members.
  async function selectServer(id: string) {
    currentServerId.value = id
    const { data } = await serverApi.members(id)
    members.value = data
  }

  async function remove(id: string) {
    await serverApi.remove(id)
    dropServer(id)
  }

  async function leave(id: string) {
    await serverApi.leave(id)
    dropServer(id)
  }

  // Moderation actions target the selected server. The socket echoes each change
  // to everyone; we also apply it locally so the UI updates without waiting.
  async function kick(userId: string) {
    await serverApi.kick(currentServerId.value, userId)
    dropMember(userId)
  }

  async function ban(userId: string) {
    await serverApi.ban(currentServerId.value, userId)
    dropMember(userId)
  }

  async function unban(userId: string) {
    await serverApi.unban(currentServerId.value, userId)
  }

  async function listBans(): Promise<ServerBan[]> {
    const { data } = await serverApi.bans(currentServerId.value)
    return data
  }

  async function setRole(userId: string, role: Role) {
    await serverApi.setRole(currentServerId.value, userId, role)
    const m = members.value.find((x) => x.user_id === userId)
    if (m) m.role = role
  }

  // Forget a server locally, clearing the selection if it was active.
  function dropServer(id: string) {
    servers.value = servers.value.filter((s) => s.id !== id)
    if (currentServerId.value === id) {
      currentServerId.value = ''
      members.value = []
    }
  }

  function dropMember(userId: string) {
    members.value = members.value.filter((m) => m.user_id !== userId)
  }

  function patchServer(p: Server) {
    const s = servers.value.find((x) => x.id === p.id)
    if (s) Object.assign(s, { name: p.name, description: p.description, icon: p.icon })
  }

  function wire() {
    if (wired) return
    wired = true
    socket.on('member_joined', (p: { server_id: string; member: ServerMember }) => {
      if (p.server_id !== currentServerId.value) return
      if (!members.value.some((m) => m.user_id === p.member.user_id)) members.value.push(p.member)
    })
    socket.on(
      'member_removed',
      (p: { server_id: string; user_id: string; reason: 'kick' | 'ban' | 'leave' }) => {
        if (p.user_id === auth.user?.id) {
          const name = servers.value.find((s) => s.id === p.server_id)?.name ?? 'a server'
          if (p.reason !== 'leave') {
            notice.value = `You were ${p.reason === 'ban' ? 'banned' : 'kicked'} from ${name}.`
          }
          dropServer(p.server_id)
        } else if (p.server_id === currentServerId.value) {
          dropMember(p.user_id)
        }
      },
    )
    socket.on('member_role_updated', (p: { server_id: string; user_id: string; role: Role }) => {
      if (p.server_id !== currentServerId.value) return
      const m = members.value.find((x) => x.user_id === p.user_id)
      if (m) m.role = p.role
      // My role changed: the set of channels I may see may have changed too.
      if (p.user_id === auth.user?.id) channels.fetch(p.server_id)
    })
    socket.on('channels_changed', (p: { server_id: string }) => {
      if (p.server_id === currentServerId.value) channels.fetch(p.server_id)
    })
    socket.on('server_updated', (p: Server) => patchServer(p))
    socket.on('server_deleted', (p: { server_id: string }) => {
      const s = servers.value.find((x) => x.id === p.server_id)
      if (s && s.owner_id !== auth.user?.id) notice.value = `${s.name} was deleted.`
      dropServer(p.server_id)
    })
  }

  return {
    servers,
    currentServerId,
    members,
    notice,
    myRole,
    fetch,
    create,
    join,
    invite,
    regenerateInvite,
    update,
    selectServer,
    remove,
    leave,
    kick,
    ban,
    unban,
    listBans,
    setRole,
    wire,
  }
})
