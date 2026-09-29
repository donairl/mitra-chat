// Client-side mirror of server/internal/perms. It only decides what UI to show;
// the server enforces every rule again.
import type { Channel, Role } from '@/types'

export type Cap =
  | 'kick'
  | 'ban'
  | 'deleteAnyMessage'
  | 'manageChannels'
  | 'manageServer'
  | 'manageRoles'
  | 'deleteServer'

const ROLES: Role[] = ['member', 'moderator', 'admin', 'owner']

// Lowest role holding each capability. Keep in sync with perms.minTier.
const MIN_ROLE: Record<Cap, Role> = {
  kick: 'moderator',
  ban: 'moderator',
  deleteAnyMessage: 'moderator',
  manageChannels: 'admin',
  manageServer: 'admin',
  manageRoles: 'admin',
  deleteServer: 'owner',
}

// Roles a channel's view/post tier may be set to (owner is never needed:
// admins and owners always see everything).
export const CHANNEL_ROLES: Role[] = ['member', 'moderator', 'admin']

// Position in the hierarchy; -1 for unknown/undefined (not a member yet).
export function rank(role: Role | undefined): number {
  return role ? ROLES.indexOf(role) : -1
}

export function can(role: Role | undefined, cap: Cap): boolean {
  return rank(role) >= rank(MIN_ROLE[cap])
}

// Kick, ban and role changes only work on strictly lower roles.
export function canActOn(actor: Role | undefined, target: Role): boolean {
  return rank(actor) > rank(target)
}

// Roles the actor may hand out: below their own rank, never owner.
export function assignableRoles(actor: Role | undefined): Role[] {
  return ROLES.filter((r) => r !== 'owner' && rank(r) < rank(actor))
}

// DM channels (no server) are always open to their participants, and we only
// ever list our own DMs.
export function canView(role: Role | undefined, ch: Channel): boolean {
  if (!ch.server_id) return true
  return rank(role) >= rank(ch.min_view_role ?? 'member')
}

export function canPost(role: Role | undefined, ch: Channel): boolean {
  if (!ch.server_id) return true
  return canView(role, ch) && rank(role) >= rank(ch.min_post_role ?? 'member')
}
