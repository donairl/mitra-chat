import client from './client'
import type {
  Channel,
  FriendRequest,
  Message,
  Notification,
  Server,
  ServerMember,
  User,
  Attachment,
} from '@/types'

export const authApi = {
  register: (b: { username: string; email: string; password: string }) =>
    client.post<{ token: string; user: User }>('/auth/register', b),
  login: (b: { email: string; password: string }) =>
    client.post<{ token: string; user: User }>('/auth/login', b),
  me: () => client.get<User>('/auth/me'),
  logout: () => client.post('/auth/logout'),
}

export const serverApi = {
  list: () => client.get<Server[]>('/servers'),
  create: (b: { name: string; description?: string; icon?: string }) =>
    client.post<Server>('/servers', b),
  get: (id: string) => client.get<Server>(`/servers/${id}`),
  update: (id: string, b: object) => client.put<Server>(`/servers/${id}`, b),
  remove: (id: string) => client.delete(`/servers/${id}`),
  invite: (id: string) => client.post<{ invite_code: string }>(`/servers/${id}/invite`),
  join: (invite_code: string) => client.post<Server>('/servers/join', { invite_code }),
  members: (id: string) => client.get<ServerMember[]>(`/servers/${id}/members`),
}

export const channelApi = {
  list: (serverId: string) => client.get<Channel[]>(`/servers/${serverId}/channels`),
  create: (serverId: string, b: { name: string; type?: string; topic?: string }) =>
    client.post<Channel>(`/servers/${serverId}/channels`, b),
  update: (id: string, b: object) => client.put<Channel>(`/channels/${id}`, b),
  remove: (id: string) => client.delete(`/channels/${id}`),
  dmList: () => client.get<Channel[]>('/channels/dm'),
  dmOpen: (user_id: string) => client.post<Channel>('/channels/dm', { user_id }),
}

export const messageApi = {
  history: (channelId: string, before?: string) =>
    client.get<Message[]>(`/channels/${channelId}/messages`, { params: { before } }),
  send: (b: { channel_id: string; content: string; attachment_ids?: string[] }) =>
    client.post<Message>('/messages', b),
  edit: (id: string, content: string) => client.put<Message>(`/messages/${id}`, { content }),
  remove: (id: string) => client.delete(`/messages/${id}`),
}

export const attachmentApi = {
  upload: (file: File) => {
    const fd = new FormData()
    fd.append('file', file)
    return client.post<Attachment>('/attachments', fd)
  },
}

export const friendApi = {
  list: () => client.get<User[]>('/friends'),
  requests: () => client.get<FriendRequest[]>('/friends/requests'),
  request: (username: string) => client.post('/friends/request', { username }),
  accept: (id: string) => client.put(`/friends/${id}/accept`),
  reject: (id: string) => client.put(`/friends/${id}/reject`),
  remove: (id: string) => client.delete(`/friends/${id}`),
  search: (q: string) => client.get<User[]>('/users/search', { params: { q } }),
}

export const notificationApi = {
  list: () => client.get<Notification[]>('/notifications'),
  markRead: (id: string) => client.put(`/notifications/${id}/read`),
  markAll: () => client.put('/notifications/read-all'),
}
