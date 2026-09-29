# Roles & Permissions — Design

Date: 2026-09-29
Status: Approved (brainstorming)

## Goal

Give servers a fixed role hierarchy that controls moderation, channel management,
server management, and private/read-only channels. Enforce every rule identically
on the HTTP and WebSocket paths.

## Current state (problems this fixes)

- `ServerMember.Role` (`owner|admin|member`) is written but never read. All checks
  compare against `Server.OwnerID`.
- `isMember` is duplicated in `servers` and `channels`; `isOwner` lives only in
  `channels`; `canAccessChannel` lives in `messages`. No central authorization.
- Any member can create a channel, but only the owner can edit or delete one.
- **Security gap:** WebSocket `join_room`, `send_message`, and typing events in
  `internal/ws/events.go` perform no access check. Any authenticated user who knows
  a channel ID can receive its live traffic and post to it, including DM channels.
- No kick, ban, or leave. Nobody can delete another user's message.
- The client gates admin UI on `server.owner_id === me`.

## Decisions

| Topic | Decision |
|---|---|
| Role model | Fixed tiers: owner > admin > moderator > member |
| Channel access | Per-channel `min_view_role` and `min_post_role` |
| Moderation | Kick, ban/unban, delete others' messages |
| Lifecycle | Leave server (non-owners). No ownership transfer in v1 |
| Realtime | Push WS events; server evicts users from rooms they lose access to |
| Architecture | Central `perms` package called explicitly from HTTP and WS handlers |

## 1. Tiers, capabilities, data model

### Tiers

Tiers stay stored as the `ServerMember.Role` string. Add the value `moderator`.
Go maps role strings to ranks.

| Role | Rank | Capabilities (cumulative) |
|---|---|---|
| member | 0 | View/post channels their rank allows; fetch invite code (unchanged); leave server |
| moderator | 1 | Kick and ban lower-ranked members; unban; list bans; delete any message in channels they can view |
| admin | 2 | Create/edit/delete channels, including channel tiers; edit server settings; regenerate invite code; set roles of lower-ranked members to at most `moderator` |
| owner | 3 | Everything; delete server; assign or revoke `admin` |

### Hierarchy rules

- An actor may kick, ban, or change the role of a target only when
  `rank(actor) > rank(target)`.
- An actor may assign only roles with `rank(role) < rank(actor)`.
- `owner` can never be assigned through the API.
- The owner cannot leave the server (400, "owner must delete the server instead").
- Nobody can kick, ban, or change the role of themselves (400).
- `ServerMember.Role` is the single source of truth for authorization.
  `Server.OwnerID` remains for display and is never changed after creation.

### Channel access

Add two columns to `models.Channel`:

```go
MinViewRole string `gorm:"type:varchar(16);default:member" json:"min_view_role"`
MinPostRole string `gorm:"type:varchar(16);default:member" json:"min_post_role"`
```

- Allowed values: `member`, `moderator`, `admin`. The maximum is `admin`, so owners
  and admins always see every channel and cannot lock themselves out.
- Invariant: `rank(MinPostRole) >= rank(MinViewRole)`. Violations return 400.
- Examples: staff channel = view `moderator`, post `moderator`;
  announcements = view `member`, post `moderator`.
- DM channels (`Type == "dm"`, empty `ServerID`) ignore tiers. Participants listed in
  `ChannelMember` can view and post; nobody else can.

### Bans

New model `models.ServerBan`, registered in `database.AutoMigrate`:

```go
type ServerBan struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ServerID  string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"server_id"`
	UserID    string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"user_id"`
	BannedBy  string    `gorm:"type:varchar(36);not null" json:"banned_by"`
	CreatedAt time.Time `json:"created_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
```

- Banning deletes the target's `ServerMember` row and inserts a `ServerBan` row in
  one transaction.
- Only current members can be banned (the hierarchy check needs their rank).
- `POST /servers/join` returns 403 "banned from server" when a ban row exists.
- Server deletion also deletes the server's ban rows.

### Migration

GORM AutoMigrate adds the new columns with defaults and creates `server_bans`.
Existing `owner` and `member` rows remain valid. No backfill is needed.

### Behavior change

Channel creation moves from "any member" to "admin and above".

## 2. `perms` package and enforcement

### Package `server/internal/perms`

Depends only on `database` and `models`.

```go
type Tier int

const (
	Member Tier = iota
	Moderator
	Admin
	Owner
)

type Capability int

const (
	CapKick Capability = iota        // moderator+
	CapBan                           // moderator+
	CapDeleteAnyMessage              // moderator+
	CapManageChannels                // admin+
	CapManageServer                  // admin+
	CapManageRoles                   // admin+
	CapDeleteServer                  // owner
)

type Access struct {
	Exists   bool   // channel exists
	CanView  bool
	CanPost  bool
	ServerID string // empty for DM channels
	Tier     Tier   // caller's tier in ServerID; meaningless for DMs
}

func ParseTier(role string) (Tier, bool)
func (t Tier) String() string
func MemberTier(serverID, userID string) (Tier, bool)   // false = not a member
func ChannelAccess(channelID, userID string) Access
func VisibleChannels(serverID string, t Tier) *gorm.DB  // query scoped to channels with rank(min_view_role) <= t
func Can(t Tier, c Capability) bool
func CanActOn(actor, target Tier) bool                  // actor > target
```

`VisibleChannels` filters with `min_view_role IN ?`, passing the role strings whose
rank is at most `t` (for example `["member","moderator"]` for a moderator). Ranks are
never computed in SQL.

Remove the duplicated `isMember`/`isOwner` helpers in `servers` and `channels` and
`canAccessChannel` in `messages`; call `perms` instead.

### Existing HTTP endpoints

| Endpoint | Rule |
|---|---|
| `GET /servers/:id` | Member. Preloaded `Channels` filtered through `VisibleChannels` |
| `POST /servers/join` | Reject banned users (403). Returned `Channels` filtered by the joiner's tier |
| `PUT /servers/:id` | `CapManageServer` |
| `DELETE /servers/:id` | `CapDeleteServer` |
| `GET /servers/:serverId/channels` | Member. Filtered through `VisibleChannels` |
| `POST /servers/:serverId/channels` | `CapManageChannels`. Body accepts `min_view_role`, `min_post_role` |
| `PUT /channels/:id` | `CanView` and `CapManageChannels`. Body accepts the tier fields |
| `DELETE /channels/:id` | `CanView` and `CapManageChannels` |
| `GET /channels/:channelId/messages` | `CanView` |
| `POST /messages` | `CanPost` |
| `PUT /messages/:id` | Author and `CanPost` |
| `DELETE /messages/:id` | (Author and `CanView`) or (`CapDeleteAnyMessage` and `CanView`) |

`PUT /channels/:id` and `DELETE /channels/:id` reject DM channels with 403
`insufficient permissions`; DM channels have no tier to check.

### New HTTP endpoints

All live in `internal/servers`, registered under the protected `/servers` group.
Register `/:id/members/me` before `/:id/members/:userId`.

| Route | Rule | Effect |
|---|---|---|
| `DELETE /servers/:id/members/me` | Member, not owner | Leave |
| `DELETE /servers/:id/members/:userId` | `CapKick`, `CanActOn` | Kick |
| `PUT /servers/:id/members/:userId/role` body `{role}` | `CapManageRoles`, `CanActOn`, `rank(role) < rank(actor)`, `role != owner` | Change role |
| `GET /servers/:id/bans` | `CapBan` | List bans with `User` preloaded |
| `POST /servers/:id/bans` body `{user_id}` | `CapBan`, target is a member, `CanActOn` | Ban |
| `DELETE /servers/:id/bans/:userId` | `CapBan` | Unban |
| `POST /servers/:id/invite/regenerate` | `CapManageServer` | New invite code |

### WebSocket enforcement (`internal/ws/events.go`)

| Event | Rule | On denial |
|---|---|---|
| `join_room` | `CanView` | Send `error {code:"not_found", channel_id}` to that client only; do not join |
| `send_message` | `CanPost`; content or attachments non-empty | Send `error`; no DB row created |
| `typing_start` / `typing_stop` | `CanPost` | Drop silently |
| `edit_message` | Enforced inside `EditAndBroadcast` (author and `CanPost`) | Send `error` |
| `delete_message` | Enforced inside `DeleteAndBroadcast` (author or moderator override, both need `CanView`) | Send `error` |

`CreateAndBroadcast` does not check permissions itself; both callers (HTTP `send`
and WS `send_message`) check `CanPost` before calling it. `EditAndBroadcast` and
`DeleteAndBroadcast` check internally because they already load the message.

## 3. Realtime events and room eviction

### Room eviction

New hub method in `internal/ws/hub.go`:

```go
// RecheckRooms removes each of the user's connections from every room the user can
// no longer view.
func (h *Hub) RecheckRooms(userID string)
```

1. Under the read lock, snapshot the user's clients and each client's rooms.
2. Release the lock. For each distinct room, call `perms.ChannelAccess`.
3. For each denied room, call `leaveRoom` for each affected client.

No database call runs while the hub lock is held.

Callers:

- Kick, ban, leave, role change: `RecheckRooms(targetUserID)`.
- Channel tier change: `RecheckRooms` for every user currently in that room.
- Channel delete: remove all clients from the room directly (new
  `h.closeRoom(channelID)`).

A banned user's socket stays open, because the JWT is only checked at connect time.
They lose all rooms in that server, and every later `join_room` fails the check.

### Push events

New helper `SendToServerMembers(serverID string, event any)`: loads member user IDs
and calls the existing `SendToUser` for each. For removals, the handler calls
`SendToUser(target, …)` explicitly, because the target is no longer a member.

| Event | Recipients | Payload | Client reaction |
|---|---|---|---|
| `member_joined` | Server members | `{server_id, member}` (member with `User`) | Append to member list |
| `member_removed` | Server members + target | `{server_id, user_id, reason}` where reason is `kick`, `ban`, or `leave` | Others remove the member. If me: remove server from rail, navigate home, show banner for kick/ban |
| `member_role_updated` | Server members | `{server_id, user_id, role}` | Update member list. If me: update `myRole`, refetch channels |
| `channels_changed` | Server members | `{server_id}` | Refetch channel list (server-filtered). If the active channel vanished, open the first visible channel |
| `server_updated` | Server members | Server object (no channels) | Update rail and header |
| `server_deleted` | Server members (IDs loaded before delete) | `{server_id}` | Remove server from rail; navigate home if active |
| `error` | Offending client | `{code, message, channel_id?}` | On `not_found`/`forbidden` for the active channel: refetch channels and leave that view |

`channels_changed` fires on channel create, update, and delete. Clients refetch
instead of receiving diffs, so private channel names never reach users who cannot
view them, and the server never computes per-user diffs.

## 4. Client

### `client/src/permissions.ts`

Mirrors the Go matrix for UI gating only; the server stays authoritative.

```ts
export type Role = 'member' | 'moderator' | 'admin' | 'owner'
export type Cap =
  | 'kick' | 'ban' | 'deleteAnyMessage'
  | 'manageChannels' | 'manageServer' | 'manageRoles' | 'deleteServer'
export function rank(role: Role): number
export function can(role: Role | undefined, cap: Cap): boolean
export function canActOn(actor: Role | undefined, target: Role): boolean
export function assignableRoles(actor: Role | undefined): Role[] // roles ranked below actor, never owner
export function canPost(role: Role | undefined, ch: Channel): boolean
```

### Types (`types.ts`)

- `ServerMember.role: Role`
- `Channel.min_view_role?: Role` and `Channel.min_post_role?: Role`
- New `ServerBan { id, server_id, user_id, banned_by, created_at, user?: User }`

### API (`api/index.ts`)

Add to `serverApi`: `leave`, `kick`, `setRole`, `bans`, `ban`, `unban`,
`regenerateInvite`. Extend `channelApi.create` and `channelApi.update` bodies with
the tier fields.

### Stores

- `servers`: computed `myRole` (the current user's row in `members`); actions
  `update`, `leave`, `kick`, `ban`, `unban`, `listBans`, `setRole`,
  `regenerateInvite`; handlers for `member_joined`, `member_removed`,
  `member_role_updated`, `server_updated`, `server_deleted`. On `channels_changed`
  it calls the channels store's `load(serverId)` when the server is current.
- `channels`: `update(id, body)`.
- `ws/socket.ts`: surface `error` events on the existing event bus.

### Components

| Component | Change |
|---|---|
| `ChannelSidebar.vue` | Replace `isOwner` with `can(myRole, 'manageChannels')` for the "+" button. Gear icon on channel hover (manage-channels only) opens the edit modal. 🔒 when `min_view_role !== 'member'`. Header menu: Invite, Server settings (visible with `manageServer` or `ban`), Leave server (non-owners, with `window.confirm`) |
| `CreateChannelModal.vue` | Add mode `edit` with an optional `channel` prop. Add "Who can view" and "Who can post" selects; post options never rank below view. Edit mode adds Delete |
| New `ServerSettingsModal.vue` | Tabs shown by capability: Overview (name, description, icon, regenerate invite; `manageServer`), Bans (list with Unban; `ban`), Danger (delete server; `deleteServer`) |
| `MemberList.vue` | Group by tier with headers (Owner, Admins, Moderators, Members). Badges: 👑 owner, 🛡️ admin, 🔨 moderator. A ⋯ menu on members where `canActOn` is true: Set role (from `assignableRoles`), Kick, Ban, each with `window.confirm` |
| `MessageItem.vue` | Show Delete on others' messages when `can(myRole, 'deleteAnyMessage')`. Edit stays author-only |
| `MessageInput.vue` | Disabled with placeholder "You don't have permission to post in this channel" when `!canPost` |
| `DashboardView.vue` | Handle `member_removed`/`server_deleted` for the current user and a vanished active channel. Show a dismissible inline banner (no toast system exists) |

## 5. Errors, testing, scope

### Errors

All errors use the existing `utils.Error(c, status, message)` shape.

| Case | Status | Message |
|---|---|---|
| Missing capability | 403 | `insufficient permissions` |
| Hierarchy violation | 403 | `cannot act on equal or higher role` |
| Banned user joining | 403 | `banned from server` |
| Invalid role or tier value | 400 | `invalid role` |
| `min_post_role` ranks below `min_view_role` | 400 | `post role must be at least view role` |
| Owner leaving | 400 | `owner must delete the server instead` |
| Acting on self (kick, ban, role) | 400 | `cannot target yourself` |
| Unknown member or ban | 404 | `member not found` / `ban not found` |
| Channel the caller cannot view | 404 | `channel not found` |

Channels the caller cannot view return **404, not 403**, on every path (history,
send, edit, delete, channel update/delete, WS `join_room`), so channel IDs cannot be
probed. Non-members of a server still get 403 `not a member` on server routes, as
today.

On a 403 or 404 for the active channel, the client refetches the channel list.

### Testing

The server has no Go tests yet. Add minimal infrastructure:

- Export the migration: rename `database.migrate()` to `database.Migrate()` so
  tests can reuse the model list.
- `internal/testutil`: open an in-memory SQLite database (one per test, via a unique
  `file:<name>?mode=memory&cache=shared` DSN), assign `database.DB`, call
  `database.Migrate()`, and provide seed helpers:
  `SeedUser(name) string`, `SeedServer(ownerID) string`,
  `AddMember(serverID, userID, role)`, `AddChannel(serverID, view, post) string`,
  `AddDM(userA, userB) string`.
- `perms` table-driven unit tests: tier parsing and ranks, the full `Can` matrix,
  `CanActOn`, `ChannelAccess` for each tier × channel tier combination plus DM
  participant, DM outsider, and non-member, and `VisibleChannels` filtering.
- Handler tests with `fiber.App.Test` and signed test JWTs:
  - moderator cannot kick admin; admin can kick moderator
  - ban, then rejoin returns 403; unban, then rejoin succeeds
  - admin cannot assign `admin`; owner can
  - private channel absent from `GET /servers/:id` and `GET /servers/:sid/channels`
    for a member; history returns 404
  - member cannot create a channel (403)
  - owner cannot leave (400)
  - read-only channel: member history 200, send 403
  - moderator deletes another user's message
- WS tests: call `handleMessage` on a test `Client` with a buffered send channel and
  a registered hub entry:
  - `join_room` on a hidden channel yields an `error` event and no room entry
  - `send_message` without post rights creates no message row
  - `RecheckRooms` after demotion removes the room
- Client: vitest unit tests for `permissions.ts`.
- Manual smoke test with two browsers: kick updates the kicked user's rail live;
  a tier change hides the channel for a member; a read-only channel disables input.

Verification commands:

```bash
cd server && go build ./... && go test ./...
cd client && npm run type-check && npm run build && npx vitest run
```

### Out of scope (v1)

- Ownership transfer, timeouts/mutes, audit log, custom roles, per-user channel
  allowlists.
- Revalidating the WS JWT mid-session.
- **Known limitation:** `/uploads` is served publicly by `app.Static`. An attachment
  URL from a private channel works for anyone who has the link. File names are
  UUIDs, so they are hard to guess, but access is not controlled. Track as a
  follow-up.
