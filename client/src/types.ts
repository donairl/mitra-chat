// Server role, lowest first. Mirrors server/internal/perms tiers.
export type Role = 'member' | 'moderator' | 'admin' | 'owner'

export interface User {
  id: string
  username: string
  email?: string
  avatar?: string
  status?: string
  created_at?: string
}

export interface Server {
  id: string
  name: string
  owner_id: string
  icon?: string
  description?: string
  invite_code?: string
  channels?: Channel[]
}

export interface Channel {
  id: string
  name: string
  type: string
  topic?: string
  server_id?: string
  min_view_role?: Role // lowest role that can read (server channels only)
  min_post_role?: Role // lowest role that can post (server channels only)
  dm_user?: User
}

export interface Attachment {
  id: string
  message_id: string
  file_name: string
  file_path: string
  file_type: string
  file_size: number
}

export interface Message {
  id: string
  content: string
  user_id: string
  channel_id: string
  is_edited: boolean
  edited_at?: string | null
  created_at: string
  user?: User
  attachments?: Attachment[]
}

export interface FriendRequest {
  id: string
  user_id: string
  friend_id: string
  status: string
  user?: User
}

export interface Notification {
  id: string
  user_id: string
  type: string
  content: string
  read: boolean
  created_at: string
}

export interface ServerMember {
  id: string
  server_id: string
  user_id: string
  role: Role
  user?: User
}

export interface ServerBan {
  id: string
  server_id: string
  user_id: string
  banned_by: string
  created_at: string
  user?: User
}
