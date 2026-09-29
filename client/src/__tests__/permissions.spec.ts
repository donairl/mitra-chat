import { describe, expect, it } from 'vitest'
import { assignableRoles, can, canActOn, canPost, canView, rank } from '@/permissions'
import type { Channel, Role } from '@/types'

const serverChannel = (view: Role, post: Role): Channel => ({
  id: 'c',
  name: 'c',
  type: 'text',
  server_id: 's',
  min_view_role: view,
  min_post_role: post,
})

describe('rank', () => {
  it('orders roles and ranks unknown below member', () => {
    expect(rank('member')).toBeLessThan(rank('moderator'))
    expect(rank('moderator')).toBeLessThan(rank('admin'))
    expect(rank('admin')).toBeLessThan(rank('owner'))
    expect(rank(undefined)).toBe(-1)
  })
})

describe('can', () => {
  it('matches the server capability matrix', () => {
    expect(can('member', 'kick')).toBe(false)
    expect(can('moderator', 'kick')).toBe(true)
    expect(can('moderator', 'ban')).toBe(true)
    expect(can('moderator', 'deleteAnyMessage')).toBe(true)
    expect(can('moderator', 'manageChannels')).toBe(false)
    expect(can('admin', 'manageChannels')).toBe(true)
    expect(can('admin', 'manageServer')).toBe(true)
    expect(can('admin', 'manageRoles')).toBe(true)
    expect(can('admin', 'deleteServer')).toBe(false)
    expect(can('owner', 'deleteServer')).toBe(true)
    expect(can(undefined, 'kick')).toBe(false)
  })
})

describe('canActOn', () => {
  it('requires a strictly higher role', () => {
    expect(canActOn('moderator', 'member')).toBe(true)
    expect(canActOn('moderator', 'moderator')).toBe(false)
    expect(canActOn('admin', 'owner')).toBe(false)
    expect(canActOn(undefined, 'member')).toBe(false)
  })
})

describe('assignableRoles', () => {
  it('lists roles below the actor, never owner', () => {
    expect(assignableRoles('owner')).toEqual(['member', 'moderator', 'admin'])
    expect(assignableRoles('admin')).toEqual(['member', 'moderator'])
    expect(assignableRoles('member')).toEqual([])
    expect(assignableRoles(undefined)).toEqual([])
  })
})

describe('canView / canPost', () => {
  it('applies channel tiers', () => {
    const announce = serverChannel('member', 'moderator')
    expect(canView('member', announce)).toBe(true)
    expect(canPost('member', announce)).toBe(false)
    expect(canPost('moderator', announce)).toBe(true)
    expect(canView('member', serverChannel('moderator', 'moderator'))).toBe(false)
    expect(canPost(undefined, serverChannel('member', 'member'))).toBe(false)
  })

  it('always allows DM channels', () => {
    const dm: Channel = { id: 'd', name: 'dm', type: 'dm', server_id: '' }
    expect(canView(undefined, dm)).toBe(true)
    expect(canPost(undefined, dm)).toBe(true)
  })
})
