# Roles & Permissions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add fixed server roles (owner > admin > moderator > member), per-channel view/post tiers, kick/ban/leave, and live membership events, enforced identically on HTTP and WebSocket paths.

**Architecture:** A new `server/internal/perms` package is the single authorization choke point (tier ranks, capability matrix, DB loaders). HTTP handlers and WS event handlers call it; the WS hub gains room re-validation so users who lose access stop receiving traffic. The Vue client mirrors the matrix in `permissions.ts` for UI gating only and reacts to new socket events.

**Tech Stack:** Go 1.26 · Fiber v2 · GORM (SQLite via mattn/go-sqlite3, CGO) · gofiber/contrib/websocket · Vue 3 + TypeScript · Pinia · Tailwind v4 · Vitest

**Spec:** `docs/superpowers/specs/2026-09-29-roles-permissions-design.md`

---

## Conventions for the engineer

- Server commands run from `server/`, client commands from `client/`.
- The first `go test` compiles go-sqlite3 with CGO and can take ~1 minute. The C warning `assignment discards 'const' qualifier` is normal.
- Tests share the global `database.DB`. **Never call `t.Parallel()`** in these tests.
- Error responses always use `utils.Error(c, status, msg)`, which writes `{"error": msg}`. `utils.Error` returns the write error (normally `nil`), so never use its return value as a "denied" signal. Helpers below return `(status int, msg string)` instead, and callers write the response.
- Channels a caller cannot view return **404 `channel not found`**, never 403, so IDs can't be probed.
- Work on branch `feat/roles-permissions` (already exists and is checked out).

## File structure

**Server — create**
| File | Responsibility |
|---|---|
| `server/internal/models/server_ban.go` | `ServerBan` model |
| `server/internal/testutil/testutil.go` | Test DB, seed data (`Guild`), JWT, HTTP request helpers. Imports no handler packages |
| `server/internal/perms/perms.go` | Pure tier/capability logic |
| `server/internal/perms/access.go` | DB loaders: `MemberTier`, `ChannelAccess`, `VisibleChannels` |
| `server/internal/ws/notify.go` | `Event`, `SendToServerMembers` |
| `server/internal/servers/members.go` | Leave, kick, set role, shared action resolution |
| `server/internal/servers/bans.go` | List bans, ban, unban |
| Tests | `database/database_test.go`, `perms/perms_test.go`, `perms/access_test.go`, `ws/testhelpers_test.go`, `ws/hub_test.go`, `ws/events_test.go`, `messages/handler_test.go`, `channels/handler_test.go`, `servers/handler_test.go`, `servers/members_test.go`, `servers/bans_test.go` |

**Server — modify**
| File | Change |
|---|---|
| `server/internal/database/database.go` | Export `Migrate()`, register `ServerBan` |
| `server/internal/models/channel.go` | `MinViewRole`, `MinPostRole` |
| `server/internal/ws/errors.go` | Add `ErrNotFound` |
| `server/internal/ws/hub.go` | `removeFromRoom`, `RecheckRooms`, `RecheckRoom`, `CloseRoom` |
| `server/internal/ws/events.go` | Enforce perms on every event; error frames |
| `server/internal/messages/handler.go` | Use perms; 404 for hidden channels |
| `server/internal/channels/handler.go` | Use perms; tiers on create/update; events |
| `server/internal/servers/handler.go` | Use perms; ban check on join; filtered channels; events; regenerate invite |

**Client — create:** `client/src/permissions.ts`, `client/src/__tests__/permissions.spec.ts`, `client/src/components/ServerSettingsModal.vue`

**Client — modify:** `types.ts`, `api/index.ts`, `stores/servers.ts`, `stores/channels.ts`, `stores/messages.ts`, `components/CreateChannelModal.vue`, `components/ChannelSidebar.vue`, `components/MemberList.vue`, `components/MessageItem.vue`, `components/MessageInput.vue`, `views/DashboardView.vue`, `README.md`

`client/src/ws/socket.ts` needs **no change**: it already emits every incoming frame by its `type`, so `socket.on('error', …)` receives the new error frames.

---

### Task 1: Models, exported migration, and test utilities

**Files:**
- Create: `server/internal/models/server_ban.go`
- Create: `server/internal/testutil/testutil.go`
- Modify: `server/internal/models/channel.go`
- Modify: `server/internal/database/database.go`
- Test: `server/internal/database/database_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/database/database_test.go`:

```go
package database_test

import (
	"testing"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func TestMigrateDefaultsChannelTiersToMember(t *testing.T) {
	testutil.SetupDB(t)
	ch := models.Channel{ID: "c1", Name: "general", ServerID: "s1"}
	if err := database.DB.Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	var got models.Channel
	database.DB.First(&got, "id = ?", "c1")
	if got.MinViewRole != "member" || got.MinPostRole != "member" {
		t.Fatalf("tiers = %q/%q, want member/member", got.MinViewRole, got.MinPostRole)
	}
}

func TestMigrateCreatesUniqueServerBans(t *testing.T) {
	testutil.SetupDB(t)
	first := models.ServerBan{ID: "b1", ServerID: "s1", UserID: "u1", BannedBy: "u2"}
	if err := database.DB.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	dup := models.ServerBan{ID: "b2", ServerID: "s1", UserID: "u1", BannedBy: "u2"}
	if err := database.DB.Create(&dup).Error; err == nil {
		t.Fatal("duplicate ban for the same server and user was accepted")
	}
}

func TestSeedGuild(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	for name, want := range map[string]string{"owner": "owner", "admin": "admin", "mod": "moderator", "member": "member", "stranger": ""} {
		if got := testutil.RoleOf(t, g.ServerID, g.User(name)); got != want {
			t.Errorf("%s role = %q, want %q", name, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/database/ -v`
Expected: build failure: `package mitrachat/server/internal/testutil is not in std` (or `undefined: models.ServerBan`).

- [ ] **Step 3: Add channel tier columns**

Replace `server/internal/models/channel.go` with:

```go
package models

import "time"

// Channel is a conversation room. It belongs to a server, or has an empty
// ServerID when it is a direct-message channel between users.
//
// MinViewRole and MinPostRole are the lowest server roles that may read and
// post in the channel (member|moderator|admin). DM channels ignore them.
type Channel struct {
	ID          string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	Name        string    `gorm:"type:varchar(100);not null" json:"name"`
	Type        string    `gorm:"type:varchar(16);default:text" json:"type"` // text|voice|dm
	Topic       string    `gorm:"type:varchar(1024)" json:"topic"`
	ServerID    string    `gorm:"type:varchar(36);index" json:"server_id"`
	MinViewRole string    `gorm:"type:varchar(16);default:member" json:"min_view_role"`
	MinPostRole string    `gorm:"type:varchar(16);default:member" json:"min_post_role"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
```

- [ ] **Step 4: Add the ServerBan model**

Create `server/internal/models/server_ban.go`:

```go
package models

import "time"

// ServerBan blocks a user from rejoining a server through an invite code.
type ServerBan struct {
	ID        string    `gorm:"type:varchar(36);primaryKey" json:"id"`
	ServerID  string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"server_id"`
	UserID    string    `gorm:"type:varchar(36);index:idx_server_ban,unique;not null" json:"user_id"`
	BannedBy  string    `gorm:"type:varchar(36);not null" json:"banned_by"`
	CreatedAt time.Time `json:"created_at"`

	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
}
```

- [ ] **Step 5: Export the migration and register ServerBan**

In `server/internal/database/database.go`, replace the call `if err := migrate(); err != nil {` with `if err := Migrate(); err != nil {`, and replace the whole `migrate` function with:

```go
// Migrate creates or updates tables for every model. Exported so tests can
// migrate their own database.
func Migrate() error {
	return DB.AutoMigrate(
		&models.User{},
		&models.Server{},
		&models.ServerMember{},
		&models.ServerBan{},
		&models.Channel{},
		&models.ChannelMember{},
		&models.Message{},
		&models.Attachment{},
		&models.Friend{},
		&models.Notification{},
	)
}
```

- [ ] **Step 6: Create the test utilities**

Create `server/internal/testutil/testutil.go`:

```go
// Package testutil gives tests an isolated database, seed data, and HTTP helpers.
// It must not import handler packages (perms, ws, servers, channels, messages)
// so their internal tests can use it without import cycles.
package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
)

// Config is the configuration handlers under test are built with.
var Config = &config.Config{JWTSecret: "test-secret"}

// SetupDB points database.DB at a fresh SQLite file in t.TempDir and migrates
// it. Tests using it must not call t.Parallel: database.DB is a global.
func SetupDB(t *testing.T) {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	database.DB = db
	if err := database.Migrate(); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func shortID() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:10] }

// SeedUser creates a user whose username starts with name and returns its id.
func SeedUser(t *testing.T, name string) string {
	t.Helper()
	u := models.User{ID: uuid.NewString(), Username: name + "-" + shortID()[:6], PasswordHash: "x"}
	u.Email = u.Username + "@test.local"
	must(t, database.DB.Create(&u).Error)
	return u.ID
}

// SeedServer creates a server owned by ownerID, including the owner's
// membership, and returns its id. It creates no channels.
func SeedServer(t *testing.T, ownerID string) string {
	t.Helper()
	srv := models.Server{ID: uuid.NewString(), Name: "test-server", OwnerID: ownerID, InviteCode: shortID()}
	must(t, database.DB.Create(&srv).Error)
	AddMember(t, srv.ID, ownerID, "owner")
	return srv.ID
}

// AddMember adds userID to serverID with role.
func AddMember(t *testing.T, serverID, userID, role string) {
	t.Helper()
	must(t, database.DB.Create(&models.ServerMember{
		ID: uuid.NewString(), ServerID: serverID, UserID: userID, Role: role, JoinedAt: time.Now(),
	}).Error)
}

// AddChannel creates a text channel with the given view/post roles and returns
// its id. An empty role leaves the column default ("member").
func AddChannel(t *testing.T, serverID, view, post string) string {
	t.Helper()
	ch := models.Channel{
		ID: uuid.NewString(), Name: "chan", Type: "text", ServerID: serverID,
		MinViewRole: view, MinPostRole: post,
	}
	must(t, database.DB.Create(&ch).Error)
	return ch.ID
}

// AddDM creates a DM channel between two users and returns its id.
func AddDM(t *testing.T, userA, userB string) string {
	t.Helper()
	ch := models.Channel{ID: uuid.NewString(), Name: "dm", Type: "dm"}
	must(t, database.DB.Create(&ch).Error)
	for _, uid := range []string{userA, userB} {
		must(t, database.DB.Create(&models.ChannelMember{ID: uuid.NewString(), ChannelID: ch.ID, UserID: uid}).Error)
	}
	return ch.ID
}

// AddMessage creates a message and returns its id.
func AddMessage(t *testing.T, channelID, userID, content string) string {
	t.Helper()
	now := time.Now()
	m := models.Message{
		ID: uuid.NewString(), Content: content, UserID: userID, ChannelID: channelID,
		CreatedAt: now, UpdatedAt: now,
	}
	must(t, database.DB.Create(&m).Error)
	return m.ID
}

// RoleOf returns userID's role in serverID, or "" when they are not a member.
func RoleOf(t *testing.T, serverID, userID string) string {
	t.Helper()
	var m models.ServerMember
	database.DB.Where("server_id = ? AND user_id = ?", serverID, userID).Limit(1).Find(&m)
	return m.Role
}

// Guild is a seeded server with one user per role, an outsider, and channels
// covering the common view/post combinations.
type Guild struct {
	ServerID                            string
	Owner, Admin, Mod, Member, Stranger string // user ids; Stranger is not a member
	Open, Announce, Staff, AdminOnly    string // channel ids
}

// SeedGuild creates a Guild. Channel tiers (view/post): Open member/member,
// Announce member/moderator, Staff moderator/moderator, AdminOnly admin/admin.
func SeedGuild(t *testing.T) Guild {
	t.Helper()
	g := Guild{
		Owner: SeedUser(t, "owner"), Admin: SeedUser(t, "admin"), Mod: SeedUser(t, "mod"),
		Member: SeedUser(t, "member"), Stranger: SeedUser(t, "stranger"),
	}
	g.ServerID = SeedServer(t, g.Owner)
	AddMember(t, g.ServerID, g.Admin, "admin")
	AddMember(t, g.ServerID, g.Mod, "moderator")
	AddMember(t, g.ServerID, g.Member, "member")
	g.Open = AddChannel(t, g.ServerID, "member", "member")
	g.Announce = AddChannel(t, g.ServerID, "member", "moderator")
	g.Staff = AddChannel(t, g.ServerID, "moderator", "moderator")
	g.AdminOnly = AddChannel(t, g.ServerID, "admin", "admin")
	return g
}

// User returns the id of the guild user seeded as name: owner, admin, mod,
// member or stranger.
func (g Guild) User(name string) string {
	switch name {
	case "owner":
		return g.Owner
	case "admin":
		return g.Admin
	case "mod":
		return g.Mod
	case "member":
		return g.Member
	case "stranger":
		return g.Stranger
	}
	panic("testutil: unknown guild user " + name)
}

// Token returns a valid bearer token for userID.
func Token(t *testing.T, userID string) string {
	t.Helper()
	tok, err := utils.GenerateToken(Config.JWTSecret, userID)
	must(t, err)
	return tok
}

// Do sends a request through app, JSON-encoding body when non-nil, and returns
// the status code and raw response body.
func Do(t *testing.T, app *fiber.App, method, path, token string, body any) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		must(t, err)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

// Decode unmarshals a JSON response body into v.
func Decode(t *testing.T, body []byte, v any) {
	t.Helper()
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

// ChannelIDs returns the ids of chans.
func ChannelIDs(chans []models.Channel) []string {
	ids := make([]string, len(chans))
	for i, c := range chans {
		ids[i] = c.ID
	}
	return ids
}

// SameIDs fails the test unless got and want hold the same ids in any order.
func SameIDs(t *testing.T, got, want []string) {
	t.Helper()
	g, w := slices.Clone(got), slices.Clone(want)
	slices.Sort(g)
	slices.Sort(w)
	if !slices.Equal(g, w) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd server && go test ./internal/database/ -v && go build ./...`
Expected: `--- PASS` for all three tests, then `ok  mitrachat/server/internal/database`. Build succeeds.

- [ ] **Step 8: Commit**

```bash
git add server/internal/models/channel.go server/internal/models/server_ban.go server/internal/database/ server/internal/testutil/
git commit -m "feat(server): add channel tiers, server bans, and test utilities"
```

---

### Task 2: perms — tiers and capabilities (pure logic)

**Files:**
- Create: `server/internal/perms/perms.go`
- Test: `server/internal/perms/perms_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/perms/perms_test.go`:

```go
package perms

import (
	"slices"
	"testing"
)

var allTiers = []Tier{Member, Moderator, Admin, Owner}

func TestParseTier(t *testing.T) {
	cases := []struct {
		in   string
		want Tier
		ok   bool
	}{
		{"member", Member, true},
		{"moderator", Moderator, true},
		{"admin", Admin, true},
		{"owner", Owner, true},
		{"", Member, false},
		{"superuser", Member, false},
	}
	for _, c := range cases {
		got, ok := ParseTier(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseTier(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTierStringRoundTrips(t *testing.T) {
	for _, tier := range allTiers {
		if got, ok := ParseTier(tier.String()); !ok || got != tier {
			t.Errorf("ParseTier(%q) = %v, %v", tier.String(), got, ok)
		}
	}
}

func TestCanMatchesSpecMatrix(t *testing.T) {
	// Lowest tier holding each capability, from the design spec.
	minimum := map[Capability]Tier{
		CapKick:             Moderator,
		CapBan:              Moderator,
		CapDeleteAnyMessage: Moderator,
		CapManageChannels:   Admin,
		CapManageServer:     Admin,
		CapManageRoles:      Admin,
		CapDeleteServer:     Owner,
	}
	for c, lowest := range minimum {
		for _, tier := range allTiers {
			if got := Can(tier, c); got != (tier >= lowest) {
				t.Errorf("Can(%v, %d) = %v, want %v", tier, c, got, tier >= lowest)
			}
		}
	}
}

func TestCanActOn(t *testing.T) {
	cases := []struct {
		actor, target Tier
		want          bool
	}{
		{Moderator, Member, true},
		{Moderator, Moderator, false},
		{Moderator, Admin, false},
		{Admin, Moderator, true},
		{Admin, Owner, false},
		{Owner, Admin, true},
		{Member, Member, false},
	}
	for _, c := range cases {
		if got := CanActOn(c.actor, c.target); got != c.want {
			t.Errorf("CanActOn(%v, %v) = %v, want %v", c.actor, c.target, got, c.want)
		}
	}
}

func TestCanAssign(t *testing.T) {
	cases := []struct {
		actor, role Tier
		want        bool
	}{
		{Admin, Member, true},
		{Admin, Moderator, true},
		{Admin, Admin, false},
		{Owner, Admin, true},
		{Owner, Owner, false}, // owner is never assignable
		{Moderator, Member, true}, // rank rule only; handlers also require CapManageRoles
	}
	for _, c := range cases {
		if got := CanAssign(c.actor, c.role); got != c.want {
			t.Errorf("CanAssign(%v, %v) = %v, want %v", c.actor, c.role, got, c.want)
		}
	}
}

func TestValidChannelTier(t *testing.T) {
	for _, tier := range allTiers {
		if got, want := ValidChannelTier(tier), tier != Owner; got != want {
			t.Errorf("ValidChannelTier(%v) = %v, want %v", tier, got, want)
		}
	}
}

func TestTiersUpTo(t *testing.T) {
	if got := TiersUpTo(Moderator); !slices.Equal(got, []string{"member", "moderator"}) {
		t.Errorf("TiersUpTo(Moderator) = %v", got)
	}
	if got := TiersUpTo(Owner); !slices.Equal(got, []string{"member", "moderator", "admin", "owner"}) {
		t.Errorf("TiersUpTo(Owner) = %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/perms/ -v`
Expected: build failure: `undefined: Tier`, `undefined: ParseTier`, and so on.

- [ ] **Step 3: Write the implementation**

Create `server/internal/perms/perms.go`:

```go
// Package perms is the single place that decides who may do what in a server.
// HTTP handlers and websocket events both call it so the two paths stay in sync.
package perms

// Tier is a member's rank inside a server. A higher tier holds every
// capability of the tiers below it.
type Tier int

const (
	Member Tier = iota
	Moderator
	Admin
	Owner
)

// tierNames are the role strings stored in ServerMember.Role, indexed by Tier.
var tierNames = [...]string{"member", "moderator", "admin", "owner"}

// String returns the stored role name for t.
func (t Tier) String() string {
	if t < Member || t > Owner {
		return "unknown"
	}
	return tierNames[t]
}

// ParseTier maps a stored role string to its tier. ok is false for unknown roles.
func ParseTier(role string) (Tier, bool) {
	for i, name := range tierNames {
		if name == role {
			return Tier(i), true
		}
	}
	return Member, false
}

// Capability is a server-wide action gated by tier.
type Capability int

const (
	CapKick Capability = iota
	CapBan
	CapDeleteAnyMessage
	CapManageChannels
	CapManageServer
	CapManageRoles
	CapDeleteServer
)

// minTier is the lowest tier holding each capability. Keep in sync with
// client/src/permissions.ts.
var minTier = map[Capability]Tier{
	CapKick:             Moderator,
	CapBan:              Moderator,
	CapDeleteAnyMessage: Moderator,
	CapManageChannels:   Admin,
	CapManageServer:     Admin,
	CapManageRoles:      Admin,
	CapDeleteServer:     Owner,
}

// Can reports whether tier t holds capability c.
func Can(t Tier, c Capability) bool {
	need, ok := minTier[c]
	return ok && t >= need
}

// CanActOn reports whether an actor may kick, ban, or re-role a target:
// only members of strictly lower rank.
func CanActOn(actor, target Tier) bool { return actor > target }

// CanAssign reports whether actor may give someone role r: roles ranked below
// the actor only, and never owner.
func CanAssign(actor, r Tier) bool { return r < actor && r != Owner }

// ValidChannelTier reports whether t may be a channel's view or post tier.
// Admin is the highest so owners and admins always see every channel.
func ValidChannelTier(t Tier) bool { return t >= Member && t <= Admin }

// TiersUpTo returns the role names ranked at or below t, lowest first, for
// SQL IN filters.
func TiersUpTo(t Tier) []string {
	var names []string
	for i := Member; i <= t && i <= Owner; i++ {
		names = append(names, i.String())
	}
	return names
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/perms/ -v`
Expected: all tests PASS, `ok  mitrachat/server/internal/perms`.

- [ ] **Step 5: Commit**

```bash
git add server/internal/perms/
git commit -m "feat(perms): add role tiers and capability matrix"
```

---

### Task 3: perms — database loaders

**Files:**
- Create: `server/internal/perms/access.go`
- Test: `server/internal/perms/access_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/perms/access_test.go`:

```go
package perms

import (
	"testing"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func TestMemberTier(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	cases := []struct {
		user string
		want Tier
		ok   bool
	}{
		{g.Owner, Owner, true},
		{g.Admin, Admin, true},
		{g.Mod, Moderator, true},
		{g.Member, Member, true},
		{g.Stranger, Member, false},
	}
	for _, c := range cases {
		got, ok := MemberTier(g.ServerID, c.user)
		if got != c.want || ok != c.ok {
			t.Errorf("MemberTier(%s) = %v, %v; want %v, %v", c.user, got, ok, c.want, c.ok)
		}
	}
}

func TestChannelAccessServerChannels(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	cases := []struct {
		name, user, channel string
		view, post          bool
	}{
		{"member open", g.Member, g.Open, true, true},
		{"member announce", g.Member, g.Announce, true, false},
		{"member staff", g.Member, g.Staff, false, false},
		{"mod announce", g.Mod, g.Announce, true, true},
		{"mod staff", g.Mod, g.Staff, true, true},
		{"mod admin-only", g.Mod, g.AdminOnly, false, false},
		{"admin admin-only", g.Admin, g.AdminOnly, true, true},
		{"owner admin-only", g.Owner, g.AdminOnly, true, true},
		{"stranger open", g.Stranger, g.Open, false, false},
	}
	for _, c := range cases {
		a := ChannelAccess(c.channel, c.user)
		if !a.Exists || a.ServerID != g.ServerID || a.CanView != c.view || a.CanPost != c.post {
			t.Errorf("%s: got %+v, want view=%v post=%v", c.name, a, c.view, c.post)
		}
	}
}

func TestChannelAccessDM(t *testing.T) {
	testutil.SetupDB(t)
	a, b, outsider := testutil.SeedUser(t, "a"), testutil.SeedUser(t, "b"), testutil.SeedUser(t, "c")
	dm := testutil.AddDM(t, a, b)

	if got := ChannelAccess(dm, a); !got.Exists || !got.CanView || !got.CanPost || got.ServerID != "" {
		t.Errorf("participant: %+v", got)
	}
	if got := ChannelAccess(dm, outsider); !got.Exists || got.CanView || got.CanPost {
		t.Errorf("outsider: %+v", got)
	}
}

func TestChannelAccessMissingChannel(t *testing.T) {
	testutil.SetupDB(t)
	if got := ChannelAccess("no-such-channel", "someone"); got.Exists || got.CanView || got.CanPost {
		t.Errorf("missing channel: %+v", got)
	}
}

func TestVisibleChannels(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	visible := func(tier Tier) []string {
		var chans []models.Channel
		database.DB.Scopes(VisibleChannels(tier)).Where("server_id = ?", g.ServerID).Find(&chans)
		return testutil.ChannelIDs(chans)
	}
	testutil.SameIDs(t, visible(Member), []string{g.Open, g.Announce})
	testutil.SameIDs(t, visible(Moderator), []string{g.Open, g.Announce, g.Staff})
	testutil.SameIDs(t, visible(Admin), []string{g.Open, g.Announce, g.Staff, g.AdminOnly})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/perms/ -run 'MemberTier|ChannelAccess|VisibleChannels' -v`
Expected: build failure: `undefined: MemberTier`, `undefined: ChannelAccess`, `undefined: VisibleChannels`.

- [ ] **Step 3: Write the implementation**

Create `server/internal/perms/access.go`:

```go
package perms

import (
	"gorm.io/gorm"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
)

// MemberTier returns userID's tier in serverID. ok is false when the user is
// not a member. An unrecognised stored role counts as Member (least privilege).
func MemberTier(serverID, userID string) (Tier, bool) {
	var m models.ServerMember
	res := database.DB.Select("role").
		Where("server_id = ? AND user_id = ?", serverID, userID).
		Limit(1).Find(&m)
	if res.Error != nil || res.RowsAffected == 0 {
		return Member, false
	}
	t, _ := ParseTier(m.Role)
	return t, true
}

// Access is what one user may do in one channel.
type Access struct {
	Exists   bool   // the channel exists
	CanView  bool   // may read history and join the channel's room
	CanPost  bool   // may send messages and typing events
	ServerID string // empty for DM channels
	Tier     Tier   // caller's tier in ServerID; Member for DMs and non-members
}

// ChannelAccess resolves userID's rights in a channel. DM channels (no server)
// grant view and post to their participants only. Server channels compare the
// caller's tier with the channel's MinViewRole and MinPostRole.
func ChannelAccess(channelID, userID string) Access {
	var ch models.Channel
	res := database.DB.Limit(1).Find(&ch, "id = ?", channelID)
	if res.Error != nil || res.RowsAffected == 0 {
		return Access{}
	}
	a := Access{Exists: true, ServerID: ch.ServerID}

	if ch.ServerID == "" {
		var n int64
		database.DB.Model(&models.ChannelMember{}).
			Where("channel_id = ? AND user_id = ?", channelID, userID).Count(&n)
		a.CanView, a.CanPost = n > 0, n > 0
		return a
	}

	t, ok := MemberTier(ch.ServerID, userID)
	if !ok {
		return a
	}
	a.Tier = t
	view, _ := ParseTier(ch.MinViewRole)
	post, _ := ParseTier(ch.MinPostRole)
	a.CanView = t >= view
	a.CanPost = a.CanView && t >= post
	return a
}

// VisibleChannels is a GORM scope that keeps only channels tier t may view,
// oldest first. Callers add their own server_id condition. Works for Find and
// as a Preload condition.
func VisibleChannels(t Tier) func(*gorm.DB) *gorm.DB {
	roles := TiersUpTo(t)
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("min_view_role IN ?", roles).Order("created_at asc")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/perms/ -v`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/perms/
git commit -m "feat(perms): load member tiers and channel access from the database"
```

---

### Task 4: WS hub — room re-validation and server fan-out

**Files:**
- Create: `server/internal/ws/notify.go`
- Modify: `server/internal/ws/hub.go` (`leaveRoom` at lines 73-84; append new methods)
- Test: `server/internal/ws/testhelpers_test.go`, `server/internal/ws/hub_test.go`

- [ ] **Step 1: Write the test helpers**

Create `server/internal/ws/testhelpers_test.go`:

```go
package ws

import "encoding/json"

// resetHub swaps in an empty hub so tests do not see each other's clients.
func resetHub() {
	H = &Hub{
		clients: make(map[*Client]bool),
		rooms:   make(map[string]map[*Client]bool),
		users:   make(map[string]map[*Client]bool),
	}
}

// newTestClient registers a client with no network connection. Frames sent to
// it queue on c.send; read them with drain.
func newTestClient(userID string) *Client {
	c := &Client{
		userID:   userID,
		username: userID,
		rooms:    make(map[string]bool),
		send:     make(chan []byte, sendBuffer),
	}
	H.register(c)
	return c
}

// drain returns and removes every frame queued for c.
func drain(c *Client) []Envelope {
	var frames []Envelope
	for {
		select {
		case data := <-c.send:
			var env Envelope
			json.Unmarshal(data, &env)
			frames = append(frames, env)
		default:
			return frames
		}
	}
}

// firstOf returns the decoded payload of the first frame of type typ, or nil.
func firstOf(frames []Envelope, typ string) map[string]any {
	for _, f := range frames {
		if f.Type == typ {
			p := map[string]any{}
			json.Unmarshal(f.Payload, &p)
			return p
		}
	}
	return nil
}

// dispatch feeds one client frame through the event handler.
func dispatch(c *Client, typ string, payload any) {
	raw, _ := json.Marshal(map[string]any{"type": typ, "payload": payload})
	c.handleMessage(raw)
}
```

- [ ] **Step 2: Write the failing test**

Create `server/internal/ws/hub_test.go`:

```go
package ws

import (
	"testing"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func TestRecheckRoomsDropsRoomsAfterDemotion(t *testing.T) {
	testutil.SetupDB(t)
	resetHub()
	g := testutil.SeedGuild(t)
	c := newTestClient(g.Admin)
	H.joinRoom(c, g.AdminOnly)
	H.joinRoom(c, g.Open)

	database.DB.Model(&models.ServerMember{}).
		Where("server_id = ? AND user_id = ?", g.ServerID, g.Admin).Update("role", "member")
	H.RecheckRooms(g.Admin)

	if c.rooms[g.AdminOnly] || H.rooms[g.AdminOnly][c] {
		t.Fatal("demoted user is still in the admin-only room")
	}
	if !c.rooms[g.Open] {
		t.Fatal("demoted user was dropped from a room they can still view")
	}
}

func TestRecheckRoomDropsUsersWhoLostAccess(t *testing.T) {
	testutil.SetupDB(t)
	resetHub()
	g := testutil.SeedGuild(t)
	member, mod := newTestClient(g.Member), newTestClient(g.Mod)
	H.joinRoom(member, g.Open)
	H.joinRoom(mod, g.Open)

	database.DB.Model(&models.Channel{}).Where("id = ?", g.Open).
		Updates(map[string]any{"min_view_role": "moderator", "min_post_role": "moderator"})
	H.RecheckRoom(g.Open)

	if member.rooms[g.Open] {
		t.Fatal("member still in a channel raised to moderator")
	}
	if !mod.rooms[g.Open] {
		t.Fatal("moderator dropped from a channel they can view")
	}
}

func TestCloseRoomRemovesEveryone(t *testing.T) {
	resetHub()
	a, b := newTestClient("u1"), newTestClient("u2")
	H.joinRoom(a, "ch")
	H.joinRoom(b, "ch")

	H.CloseRoom("ch")

	if a.rooms["ch"] || b.rooms["ch"] || H.rooms["ch"] != nil {
		t.Fatal("room still has clients after CloseRoom")
	}
}

func TestSendToServerMembersSkipsOutsiders(t *testing.T) {
	testutil.SetupDB(t)
	resetHub()
	g := testutil.SeedGuild(t)
	owner, member, stranger := newTestClient(g.Owner), newTestClient(g.Member), newTestClient(g.Stranger)

	SendToServerMembers(g.ServerID, Event("ping", nil))

	if len(drain(owner)) != 1 || len(drain(member)) != 1 {
		t.Fatal("members did not receive exactly one event")
	}
	if len(drain(stranger)) != 0 {
		t.Fatal("non-member received a server event")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd server && go test ./internal/ws/ -v`
Expected: build failure: `H.RecheckRooms undefined`, `undefined: SendToServerMembers`, `undefined: Event`.

- [ ] **Step 4: Refactor leaveRoom and add the hub methods**

In `server/internal/ws/hub.go`, add `"mitrachat/server/internal/perms"` to the imports:

```go
import (
	"encoding/json"
	"sync"

	"mitrachat/server/internal/perms"
)
```

Replace the existing `leaveRoom` function with:

```go
func (h *Hub) leaveRoom(c *Client, channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.removeFromRoom(c, channelID)
}

// removeFromRoom drops c from a room. The caller must hold h.mu for writing.
func (h *Hub) removeFromRoom(c *Client, channelID string) {
	if h.rooms[channelID] != nil {
		delete(h.rooms[channelID], c)
		if len(h.rooms[channelID]) == 0 {
			delete(h.rooms, channelID)
		}
	}
	delete(c.rooms, channelID)
}

// RecheckRooms removes each of userID's connections from every room the user
// can no longer view. Access checks hit the database, so they run outside the lock.
func (h *Hub) RecheckRooms(userID string) {
	h.mu.RLock()
	rooms := make(map[string]bool)
	for c := range h.users[userID] {
		for ch := range c.rooms {
			rooms[ch] = true
		}
	}
	h.mu.RUnlock()

	for ch := range rooms {
		if perms.ChannelAccess(ch, userID).CanView {
			continue
		}
		h.mu.Lock()
		for c := range h.users[userID] {
			h.removeFromRoom(c, ch)
		}
		h.mu.Unlock()
	}
}

// RecheckRoom re-validates every user currently in a room, e.g. after the
// channel's tiers change.
func (h *Hub) RecheckRoom(channelID string) {
	for _, uid := range h.roomUsers(channelID) {
		h.RecheckRooms(uid)
	}
}

// roomUsers returns the ids of users with at least one connection in a room.
func (h *Hub) roomUsers(channelID string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	seen := make(map[string]bool)
	var ids []string
	for c := range h.rooms[channelID] {
		if !seen[c.userID] {
			seen[c.userID] = true
			ids = append(ids, c.userID)
		}
	}
	return ids
}

// CloseRoom removes every client from a room (used when a channel is deleted).
func (h *Hub) CloseRoom(channelID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.rooms[channelID] {
		delete(c.rooms, channelID)
	}
	delete(h.rooms, channelID)
}
```

- [ ] **Step 5: Add the server fan-out helpers**

Create `server/internal/ws/notify.go`:

```go
package ws

import (
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
)

// Event builds an outgoing envelope. Exported for handlers outside this package.
func Event(t string, payload any) map[string]any { return out(t, payload) }

// SendToServerMembers delivers an event to every connection of every member of
// serverID.
func SendToServerMembers(serverID string, event any) {
	var ids []string
	database.DB.Model(&models.ServerMember{}).Where("server_id = ?", serverID).Pluck("user_id", &ids)
	for _, id := range ids {
		H.SendToUser(id, event)
	}
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd server && go test ./internal/ws/ -v && go build ./...`
Expected: all four tests PASS. Build succeeds.

- [ ] **Step 7: Commit**

```bash
git add server/internal/ws/
git commit -m "feat(ws): re-validate rooms on access changes and fan out server events"
```

---

### Task 5: WS events — enforce permissions (closes the WS access gap)

**Files:**
- Modify: `server/internal/ws/errors.go`
- Modify: `server/internal/ws/events.go` (entire file)
- Test: `server/internal/ws/events_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/ws/events_test.go`:

```go
package ws

import (
	"testing"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func setupWS(t *testing.T) testutil.Guild {
	t.Helper()
	testutil.SetupDB(t)
	resetHub()
	return testutil.SeedGuild(t)
}

func countMessages(channelID string) int64 {
	var n int64
	database.DB.Model(&models.Message{}).Where("channel_id = ?", channelID).Count(&n)
	return n
}

func expectError(t *testing.T, c *Client, code string) {
	t.Helper()
	p := firstOf(drain(c), "error")
	if p == nil || p["code"] != code {
		t.Fatalf("error frame = %v, want code %q", p, code)
	}
}

func TestJoinRoomHiddenChannelIsRefused(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Member)

	dispatch(c, "join_room", map[string]string{"channel_id": g.Staff})

	if c.rooms[g.Staff] {
		t.Fatal("member joined a moderator-only room")
	}
	frames := drain(c)
	p := firstOf(frames, "error")
	if p == nil || p["code"] != "not_found" || p["channel_id"] != g.Staff {
		t.Fatalf("error frame = %v", p)
	}
}

func TestJoinRoomVisibleChannel(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Member)

	dispatch(c, "join_room", map[string]string{"channel_id": g.Open})

	if !c.rooms[g.Open] {
		t.Fatal("member could not join an open room")
	}
}

func TestJoinRoomStrangerIsRefused(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Stranger)

	dispatch(c, "join_room", map[string]string{"channel_id": g.Open})

	if c.rooms[g.Open] {
		t.Fatal("non-member joined a server room")
	}
	expectError(t, c, "not_found")
}

func TestJoinRoomDMParticipantsOnly(t *testing.T) {
	g := setupWS(t)
	dm := testutil.AddDM(t, g.Member, g.Mod)
	outsider, participant := newTestClient(g.Admin), newTestClient(g.Member)

	dispatch(outsider, "join_room", map[string]string{"channel_id": dm})
	dispatch(participant, "join_room", map[string]string{"channel_id": dm})

	if outsider.rooms[dm] {
		t.Fatal("outsider joined someone else's DM room")
	}
	if !participant.rooms[dm] {
		t.Fatal("participant could not join their DM room")
	}
}

func TestSendMessageReadOnlyChannelIsRefused(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Member)

	dispatch(c, "send_message", map[string]any{"channel_id": g.Announce, "content": "hi"})

	if n := countMessages(g.Announce); n != 0 {
		t.Fatalf("%d messages created in a read-only channel", n)
	}
	expectError(t, c, "forbidden")
}

func TestSendMessageAllowed(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Mod)

	dispatch(c, "send_message", map[string]any{"channel_id": g.Announce, "content": "news"})

	if n := countMessages(g.Announce); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestSendMessageStrangerIsRefused(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Stranger)

	dispatch(c, "send_message", map[string]any{"channel_id": g.Open, "content": "hi"})

	if n := countMessages(g.Open); n != 0 {
		t.Fatalf("%d messages created by a non-member", n)
	}
	expectError(t, c, "not_found")
}

func TestSendMessageEmptyIsRefused(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Member)

	dispatch(c, "send_message", map[string]any{"channel_id": g.Open, "content": ""})

	if n := countMessages(g.Open); n != 0 {
		t.Fatal("empty message was stored")
	}
	expectError(t, c, "bad_request")
}

func TestTypingNeedsPostRights(t *testing.T) {
	g := setupWS(t)
	listener, typist := newTestClient(g.Mod), newTestClient(g.Member)
	H.joinRoom(listener, g.Announce)
	H.joinRoom(listener, g.Open)

	dispatch(typist, "typing_start", map[string]string{"channel_id": g.Announce})
	if firstOf(drain(listener), "typing") != nil {
		t.Fatal("typing was broadcast in a channel the typist cannot post in")
	}

	dispatch(typist, "typing_start", map[string]string{"channel_id": g.Open})
	if firstOf(drain(listener), "typing") == nil {
		t.Fatal("typing in an open channel was not delivered")
	}
}

func TestDeleteMessageModeratorOverride(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Open, g.Member, "spam")
	c := newTestClient(g.Mod)

	dispatch(c, "delete_message", map[string]string{"message_id": msg})

	if n := countMessages(g.Open); n != 0 {
		t.Fatal("moderator could not delete a member's message")
	}
}

func TestDeleteMessageMemberCannotDeleteOthers(t *testing.T) {
	g := setupWS(t)
	testutil.AddMessage(t, g.Open, g.Mod, "rules")
	c := newTestClient(g.Member)

	dispatch(c, "delete_message", map[string]string{"message_id": firstMessageID(g.Open)})

	if n := countMessages(g.Open); n != 1 {
		t.Fatal("member deleted someone else's message")
	}
	expectError(t, c, "forbidden")
}

func TestDeleteMessageNoModeratorOverrideInDMs(t *testing.T) {
	g := setupWS(t)
	dm := testutil.AddDM(t, g.Mod, g.Member)
	msg := testutil.AddMessage(t, dm, g.Member, "private")
	c := newTestClient(g.Mod)

	dispatch(c, "delete_message", map[string]string{"message_id": msg})

	if n := countMessages(dm); n != 1 {
		t.Fatal("server moderator deleted a DM message they did not write")
	}
	expectError(t, c, "forbidden")
}

func TestEditMessageNeedsPostRights(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Announce, g.Member, "old")
	c := newTestClient(g.Member)

	dispatch(c, "edit_message", map[string]string{"message_id": msg, "content": "new"})

	var m models.Message
	database.DB.First(&m, "id = ?", msg)
	if m.Content != "old" {
		t.Fatal("member edited a message in a channel they cannot post in")
	}
	expectError(t, c, "forbidden")
}

func firstMessageID(channelID string) string {
	var m models.Message
	database.DB.First(&m, "channel_id = ?", channelID)
	return m.ID
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/ws/ -run 'Room|Message|Typing' -v`
Expected: FAIL. `TestJoinRoomHiddenChannelIsRefused` fails with "member joined a moderator-only room", and the send, typing, and delete tests fail the same way, because today no event checks access.

- [ ] **Step 3: Add ErrNotFound**

Replace `server/internal/ws/errors.go` with:

```go
package ws

import "errors"

// ErrForbidden is returned when a user may see a resource but not change it.
var ErrForbidden = errors.New("forbidden")

// ErrNotFound is returned when a resource is missing or hidden from the user.
var ErrNotFound = errors.New("not found")
```

- [ ] **Step 4: Enforce perms in every event**

Replace `server/internal/ws/events.go` with:

```go
package ws

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
)

// Envelope is the wire format for all websocket messages.
type Envelope struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// out builds an outgoing envelope with an arbitrary payload.
func out(t string, payload any) map[string]any {
	return map[string]any{"type": t, "payload": payload}
}

// handleMessage dispatches one client frame. Every event is checked with the
// perms package, the same way the HTTP handlers are.
func (c *Client) handleMessage(raw []byte) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return
	}
	switch env.Type {
	case "join_room":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			if !perms.ChannelAccess(p.ChannelID, c.userID).CanView {
				c.sendError("not_found", "channel not found", p.ChannelID)
				return
			}
			H.joinRoom(c, p.ChannelID)
			H.BroadcastToChannel(p.ChannelID, out("user_joined", map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	case "leave_room":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			H.leaveRoom(c, p.ChannelID)
			H.BroadcastToChannel(p.ChannelID, out("user_left", map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	case "send_message":
		var p struct {
			ChannelID     string   `json:"channel_id"`
			Content       string   `json:"content"`
			AttachmentIDs []string `json:"attachment_ids"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" {
			a := perms.ChannelAccess(p.ChannelID, c.userID)
			switch {
			case !a.CanView:
				c.sendError("not_found", "channel not found", p.ChannelID)
			case !a.CanPost:
				c.sendError("forbidden", "insufficient permissions", p.ChannelID)
			case p.Content == "" && len(p.AttachmentIDs) == 0:
				c.sendError("bad_request", "empty message", p.ChannelID)
			default:
				CreateAndBroadcast(c.userID, p.ChannelID, p.Content, p.AttachmentIDs)
			}
		}
	case "edit_message":
		var p struct {
			MessageID string `json:"message_id"`
			Content   string `json:"content"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if _, err := EditAndBroadcast(c.userID, p.MessageID, p.Content); err != nil {
				c.sendMessageError(err)
			}
		}
	case "delete_message":
		var p struct {
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil {
			if err := DeleteAndBroadcast(c.userID, p.MessageID); err != nil {
				c.sendMessageError(err)
			}
		}
	case "typing_start", "typing_stop":
		var p struct {
			ChannelID string `json:"channel_id"`
		}
		if json.Unmarshal(env.Payload, &p) == nil && p.ChannelID != "" &&
			perms.ChannelAccess(p.ChannelID, c.userID).CanPost {
			t := "typing"
			if env.Type == "typing_stop" {
				t = "typing_stop"
			}
			H.BroadcastToChannel(p.ChannelID, out(t, map[string]any{
				"user_id": c.userID, "username": c.username, "channel_id": p.ChannelID,
			}), c)
		}
	}
}

// sendError tells only this connection that its request was refused.
func (c *Client) sendError(code, message, channelID string) {
	p := map[string]any{"code": code, "message": message}
	if channelID != "" {
		p["channel_id"] = channelID
	}
	if data, err := json.Marshal(out("error", p)); err == nil {
		c.trySend(data)
	}
}

// sendMessageError maps an edit/delete failure to an error frame.
func (c *Client) sendMessageError(err error) {
	if errors.Is(err, ErrForbidden) {
		c.sendError("forbidden", "insufficient permissions", "")
		return
	}
	c.sendError("not_found", "message not found", "")
}

// CreateAndBroadcast persists a message (with optional attachments) and
// broadcasts it. Callers must check perms.ChannelAccess(...).CanPost first.
func CreateAndBroadcast(userID, channelID, content string, attachmentIDs []string) (*models.Message, error) {
	msg := models.Message{
		ID:        uuid.NewString(),
		Content:   content,
		UserID:    userID,
		ChannelID: channelID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := database.DB.Create(&msg).Error; err != nil {
		return nil, err
	}
	if len(attachmentIDs) > 0 {
		database.DB.Model(&models.Attachment{}).
			Where("id IN ? AND message_id = ?", attachmentIDs, "").
			Update("message_id", msg.ID)
	}
	database.DB.Preload("User").Preload("Attachments").First(&msg, "id = ?", msg.ID)
	H.BroadcastToChannel(channelID, out("message", msg), nil)
	return &msg, nil
}

// EditAndBroadcast updates the caller's own message and broadcasts the change.
// It returns ErrNotFound when the message is missing or its channel is hidden
// from the caller, and ErrForbidden when the caller is not the author or can
// no longer post in the channel.
func EditAndBroadcast(userID, messageID, content string) (*models.Message, error) {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return nil, ErrNotFound
	}
	a := perms.ChannelAccess(msg.ChannelID, userID)
	if !a.CanView {
		return nil, ErrNotFound
	}
	if msg.UserID != userID || !a.CanPost {
		return nil, ErrForbidden
	}
	now := time.Now()
	msg.Content = content
	msg.IsEdited = true
	msg.EditedAt = &now
	database.DB.Model(&msg).Updates(map[string]any{
		"content": content, "is_edited": true, "edited_at": now,
	})
	H.BroadcastToChannel(msg.ChannelID, out("message_edited", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
		"content": content, "is_edited": true, "edited_at": now,
	}), nil)
	return &msg, nil
}

// DeleteAndBroadcast removes a message and broadcasts the deletion. Authors may
// delete their own messages; moderators and above may delete any message in a
// server channel they can view. Errors match EditAndBroadcast.
func DeleteAndBroadcast(userID, messageID string) error {
	var msg models.Message
	if err := database.DB.First(&msg, "id = ?", messageID).Error; err != nil {
		return ErrNotFound
	}
	a := perms.ChannelAccess(msg.ChannelID, userID)
	if !a.CanView {
		return ErrNotFound
	}
	moderator := a.ServerID != "" && perms.Can(a.Tier, perms.CapDeleteAnyMessage)
	if msg.UserID != userID && !moderator {
		return ErrForbidden
	}
	database.DB.Delete(&msg)
	database.DB.Where("message_id = ?", msg.ID).Delete(&models.Attachment{})
	H.BroadcastToChannel(msg.ChannelID, out("message_deleted", map[string]any{
		"message_id": msg.ID, "channel_id": msg.ChannelID,
	}), nil)
	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && go test ./internal/ws/ -v && go build ./...`
Expected: all hub and event tests PASS. Build succeeds. The messages package still compiles because `ws.ErrForbidden` is unchanged.

- [ ] **Step 6: Commit**

```bash
git add server/internal/ws/
git commit -m "fix(ws): enforce channel access on join, send, typing, edit and delete

Previously any authenticated user could join any channel room and post
to it by id, including other users' DMs."
```

---

### Task 6: Messages HTTP handler — use perms

**Files:**
- Modify: `server/internal/messages/handler.go` (entire file)
- Test: `server/internal/messages/handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/messages/handler_test.go`:

```go
package messages_test

import (
	"testing"

	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/messages"
	"mitrachat/server/internal/testutil"
)

func newApp() *fiber.App {
	app := fiber.New()
	messages.New(testutil.Config).Register(app.Group("/api"))
	return app
}

func TestHistoryAccess(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	dm := testutil.AddDM(t, g.Member, g.Mod)
	app := newApp()
	cases := []struct {
		name, user, channel string
		status              int
	}{
		{"member open", g.Member, g.Open, 200},
		{"member read-only", g.Member, g.Announce, 200},
		{"member hidden", g.Member, g.Staff, 404},
		{"mod staff", g.Mod, g.Staff, 200},
		{"stranger open", g.Stranger, g.Open, 404},
		{"dm participant", g.Member, dm, 200},
		{"dm outsider", g.Admin, dm, 404},
		{"missing channel", g.Member, "no-such-channel", 404},
	}
	for _, c := range cases {
		status, body := testutil.Do(t, app, "GET", "/api/channels/"+c.channel+"/messages", testutil.Token(t, c.user), nil)
		if status != c.status {
			t.Errorf("%s: status %d (%s), want %d", c.name, status, body, c.status)
		}
	}
}

func TestSendAccess(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	cases := []struct {
		name, user, channel string
		status              int
	}{
		{"member open", g.Member, g.Open, 201},
		{"member read-only", g.Member, g.Announce, 403},
		{"member hidden", g.Member, g.Staff, 404},
		{"mod read-only", g.Mod, g.Announce, 201},
		{"stranger open", g.Stranger, g.Open, 404},
	}
	for _, c := range cases {
		body := map[string]any{"channel_id": c.channel, "content": "hi"}
		status, resp := testutil.Do(t, app, "POST", "/api/messages", testutil.Token(t, c.user), body)
		if status != c.status {
			t.Errorf("%s: status %d (%s), want %d", c.name, status, resp, c.status)
		}
	}
}

func TestDeleteOthersMessage(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	byMember := testutil.AddMessage(t, g.Open, g.Member, "spam")
	byMod := testutil.AddMessage(t, g.Open, g.Mod, "rules")

	if status, body := testutil.Do(t, app, "DELETE", "/api/messages/"+byMod, testutil.Token(t, g.Member), nil); status != 403 {
		t.Errorf("member deleting mod's message: %d (%s), want 403", status, body)
	}
	if status, body := testutil.Do(t, app, "DELETE", "/api/messages/"+byMember, testutil.Token(t, g.Mod), nil); status != 200 {
		t.Errorf("mod deleting member's message: %d (%s), want 200", status, body)
	}
}

func TestEditNeedsPostRights(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	locked := testutil.AddMessage(t, g.Announce, g.Member, "old")
	open := testutil.AddMessage(t, g.Open, g.Member, "old")
	body := map[string]any{"content": "new"}

	if status, resp := testutil.Do(t, app, "PUT", "/api/messages/"+locked, testutil.Token(t, g.Member), body); status != 403 {
		t.Errorf("edit in read-only channel: %d (%s), want 403", status, resp)
	}
	if status, resp := testutil.Do(t, app, "PUT", "/api/messages/"+open, testutil.Token(t, g.Member), body); status != 200 {
		t.Errorf("edit in open channel: %d (%s), want 200", status, resp)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/messages/ -v`
Expected: FAIL. For example, `member hidden: status 200 ... want 404` (the old `canAccessChannel` ignores tiers), `stranger open: status 403 ... want 404`, and `member read-only: status 201 ... want 403`.

- [ ] **Step 3: Write the implementation**

Replace `server/internal/messages/handler.go` with:

```go
package messages

import (
	"errors"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

var validate = validator.New()

// Handler groups message endpoints.
type Handler struct{ cfg *config.Config }

// New builds a messages handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts message routes (all protected).
func (h *Handler) Register(r fiber.Router) {
	p := middleware.Protected(h.cfg)
	r.Get("/channels/:channelId/messages", p, h.history)
	r.Post("/messages", p, h.send)
	r.Put("/messages/:id", p, h.edit)
	r.Delete("/messages/:id", p, h.delete)
}

func (h *Handler) history(c *fiber.Ctx) error {
	cid := c.Params("channelId")
	// Hidden and missing channels look the same, so ids cannot be probed.
	if !perms.ChannelAccess(cid, middleware.UserID(c)).CanView {
		return utils.Error(c, fiber.StatusNotFound, "channel not found")
	}
	page := utils.ParsePagination(c)
	q := database.DB.Preload("User").Preload("Attachments").
		Where("channel_id = ?", cid)
	if page.Before != "" {
		var cursor models.Message
		if database.DB.Select("created_at").First(&cursor, "id = ?", page.Before).Error == nil {
			q = q.Where("created_at < ?", cursor.CreatedAt)
		}
	}
	var msgs []models.Message
	q.Order("created_at desc").Limit(page.Limit).Find(&msgs)
	// return ascending (oldest first) for display
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return utils.OK(c, msgs)
}

type sendReq struct {
	ChannelID     string   `json:"channel_id" validate:"required"`
	Content       string   `json:"content" validate:"max=4000"`
	AttachmentIDs []string `json:"attachment_ids"`
}

func (h *Handler) send(c *fiber.Ctx) error {
	var req sendReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	a := perms.ChannelAccess(req.ChannelID, uid)
	if !a.CanView {
		return utils.Error(c, fiber.StatusNotFound, "channel not found")
	}
	if !a.CanPost {
		return utils.Error(c, fiber.StatusForbidden, "insufficient permissions")
	}
	if req.Content == "" && len(req.AttachmentIDs) == 0 {
		return utils.Error(c, fiber.StatusBadRequest, "empty message")
	}
	msg, err := ws.CreateAndBroadcast(uid, req.ChannelID, req.Content, req.AttachmentIDs)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not send message")
	}
	return c.Status(fiber.StatusCreated).JSON(msg)
}

type editReq struct {
	Content string `json:"content" validate:"required,max=4000"`
}

func (h *Handler) edit(c *fiber.Ctx) error {
	var req editReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	msg, err := ws.EditAndBroadcast(middleware.UserID(c), c.Params("id"), req.Content)
	if err != nil {
		return messageError(c, err)
	}
	return utils.OK(c, msg)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	if err := ws.DeleteAndBroadcast(middleware.UserID(c), c.Params("id")); err != nil {
		return messageError(c, err)
	}
	return utils.OK(c, fiber.Map{"message": "deleted"})
}

// messageError maps ws edit/delete errors to HTTP responses.
func messageError(c *fiber.Ctx, err error) error {
	if errors.Is(err, ws.ErrForbidden) {
		return utils.Error(c, fiber.StatusForbidden, "insufficient permissions")
	}
	return utils.Error(c, fiber.StatusNotFound, "message not found")
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/messages/ -v && go build ./...`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/messages/
git commit -m "feat(messages): enforce channel tiers on history, send, edit and delete"
```

---

### Task 7: Channels handler — tiers, admin-only management, events

**Files:**
- Modify: `server/internal/channels/handler.go` (everything above the `// dmChannelResp` comment; the DM functions stay as they are)
- Test: `server/internal/channels/handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/channels/handler_test.go`:

```go
package channels_test

import (
	"testing"

	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/channels"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func newApp() *fiber.App {
	app := fiber.New()
	channels.New(testutil.Config).Register(app.Group("/api"))
	return app
}

func TestListChannelsFiltersByTier(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	cases := []struct {
		user string
		want []string
	}{
		{g.Member, []string{g.Open, g.Announce}},
		{g.Mod, []string{g.Open, g.Announce, g.Staff}},
		{g.Admin, []string{g.Open, g.Announce, g.Staff, g.AdminOnly}},
	}
	for _, c := range cases {
		status, body := testutil.Do(t, app, "GET", "/api/servers/"+g.ServerID+"/channels", testutil.Token(t, c.user), nil)
		if status != 200 {
			t.Fatalf("status %d (%s)", status, body)
		}
		var chans []models.Channel
		testutil.Decode(t, body, &chans)
		testutil.SameIDs(t, testutil.ChannelIDs(chans), c.want)
	}
}

func TestListChannelsStrangerIsForbidden(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	if status, _ := testutil.Do(t, newApp(), "GET", "/api/servers/"+g.ServerID+"/channels", testutil.Token(t, g.Stranger), nil); status != 403 {
		t.Fatalf("status %d, want 403", status)
	}
}

func TestCreateChannel(t *testing.T) {
	cases := []struct {
		name, actor, view, post string
		status                  int
		wantView                string
	}{
		{"member cannot create", "member", "", "", 403, ""},
		{"mod cannot create", "mod", "", "", 403, ""},
		{"admin creates with defaults", "admin", "", "", 201, "member"},
		{"admin creates staff channel", "admin", "moderator", "moderator", 201, "moderator"},
		{"post below view", "admin", "moderator", "member", 400, ""},
		{"owner tier not allowed", "owner", "owner", "owner", 400, ""},
		{"unknown tier", "admin", "vip", "vip", 400, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.SetupDB(t)
			g := testutil.SeedGuild(t)
			body := map[string]any{"name": "new", "min_view_role": c.view, "min_post_role": c.post}
			status, resp := testutil.Do(t, newApp(), "POST", "/api/servers/"+g.ServerID+"/channels", testutil.Token(t, g.User(c.actor)), body)
			if status != c.status {
				t.Fatalf("status %d (%s), want %d", status, resp, c.status)
			}
			if status == 201 {
				var ch models.Channel
				testutil.Decode(t, resp, &ch)
				if ch.MinViewRole != c.wantView {
					t.Fatalf("min_view_role = %q, want %q", ch.MinViewRole, c.wantView)
				}
			}
		})
	}
}

func TestUpdateChannel(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()

	lock := map[string]any{"name": "open", "min_view_role": "moderator", "min_post_role": "moderator"}
	if status, body := testutil.Do(t, app, "PUT", "/api/channels/"+g.Open, testutil.Token(t, g.Member), lock); status != 403 {
		t.Errorf("member update: %d (%s), want 403", status, body)
	}
	if status, body := testutil.Do(t, app, "PUT", "/api/channels/"+g.Staff, testutil.Token(t, g.Member), lock); status != 404 {
		t.Errorf("member update of hidden channel: %d (%s), want 404", status, body)
	}
	if status, body := testutil.Do(t, app, "PUT", "/api/channels/"+g.Open, testutil.Token(t, g.Admin), lock); status != 200 {
		t.Errorf("admin update: %d (%s), want 200", status, body)
	}
	var ch models.Channel
	database.DB.First(&ch, "id = ?", g.Open)
	if ch.MinViewRole != "moderator" || ch.MinPostRole != "moderator" {
		t.Errorf("tiers after update = %q/%q", ch.MinViewRole, ch.MinPostRole)
	}

	// Omitted tiers keep their current values.
	rename := map[string]any{"name": "staff-renamed"}
	if status, body := testutil.Do(t, app, "PUT", "/api/channels/"+g.Staff, testutil.Token(t, g.Admin), rename); status != 200 {
		t.Fatalf("rename: %d (%s)", status, body)
	}
	database.DB.First(&ch, "id = ?", g.Staff)
	if ch.Name != "staff-renamed" || ch.MinViewRole != "moderator" {
		t.Errorf("after rename: name=%q view=%q", ch.Name, ch.MinViewRole)
	}
}

func TestUpdateDMChannelIsForbidden(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	dm := testutil.AddDM(t, g.Member, g.Mod)
	if status, _ := testutil.Do(t, newApp(), "PUT", "/api/channels/"+dm, testutil.Token(t, g.Member), map[string]any{"name": "x"}); status != 403 {
		t.Fatalf("status %d, want 403", status)
	}
}

func TestDeleteChannel(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	testutil.AddMessage(t, g.Open, g.Member, "hi")

	if status, _ := testutil.Do(t, app, "DELETE", "/api/channels/"+g.Open, testutil.Token(t, g.Member), nil); status != 403 {
		t.Errorf("member delete: %d, want 403", status)
	}
	if status, body := testutil.Do(t, app, "DELETE", "/api/channels/"+g.Open, testutil.Token(t, g.Admin), nil); status != 200 {
		t.Fatalf("admin delete: %d (%s)", status, body)
	}
	var chans, msgs int64
	database.DB.Model(&models.Channel{}).Where("id = ?", g.Open).Count(&chans)
	database.DB.Model(&models.Message{}).Where("channel_id = ?", g.Open).Count(&msgs)
	if chans != 0 || msgs != 0 {
		t.Fatalf("after delete: %d channels, %d messages remain", chans, msgs)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/channels/ -v`
Expected: FAIL. For example, `ids = [... staff ...], want [open announce]` (the list isn't filtered), `member cannot create: status 201, want 403`, and admin update returns `403` (owner-only today).

- [ ] **Step 3: Write the implementation**

In `server/internal/channels/handler.go`, replace everything from the top of the file down to (but not including) the line `// dmChannelResp is a DM channel plus the other participant, for client display.` with:

```go
package channels

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

var validate = validator.New()

// Handler groups channel endpoints.
type Handler struct{ cfg *config.Config }

// New builds a channels handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts channel routes under both server-scoped and flat paths.
func (h *Handler) Register(r fiber.Router) {
	p := middleware.Protected(h.cfg)
	r.Get("/servers/:serverId/channels", p, h.list)
	r.Post("/servers/:serverId/channels", p, h.create)
	r.Get("/channels/dm", p, h.dmList)
	r.Post("/channels/dm", p, h.dmOpen)
	r.Put("/channels/:id", p, h.update)
	r.Delete("/channels/:id", p, h.delete)
}

// list returns the server's channels the caller's tier may view.
func (h *Handler) list(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	t, ok := perms.MemberTier(sid, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var chans []models.Channel
	database.DB.Scopes(perms.VisibleChannels(t)).Where("server_id = ?", sid).Find(&chans)
	return utils.OK(c, chans)
}

type channelReq struct {
	Name        string `json:"name" validate:"required,min=1,max=100"`
	Type        string `json:"type" validate:"omitempty,oneof=text voice"`
	Topic       string `json:"topic" validate:"max=1024"`
	MinViewRole string `json:"min_view_role"`
	MinPostRole string `json:"min_post_role"`
}

// resolveTiers fills empty view/post roles from the current values and checks
// the pair. It returns a client-facing message when the pair is invalid.
func resolveTiers(view, post, curView, curPost string) (string, string, string) {
	if view == "" {
		view = curView
	}
	if post == "" {
		post = curPost
	}
	v, okV := perms.ParseTier(view)
	p, okP := perms.ParseTier(post)
	if !okV || !okP || !perms.ValidChannelTier(v) || !perms.ValidChannelTier(p) {
		return "", "", "invalid role"
	}
	if p < v {
		return "", "", "post role must be at least view role"
	}
	return view, post, ""
}

// channelsChanged tells a server's members to refetch their channel list. The
// refetch is tier-filtered, so private channel names never reach users who
// cannot view them.
func channelsChanged(serverID string) {
	ws.SendToServerMembers(serverID, ws.Event("channels_changed", fiber.Map{"server_id": serverID}))
}

func (h *Handler) create(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	t, ok := perms.MemberTier(sid, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	if !perms.Can(t, perms.CapManageChannels) {
		return utils.Error(c, fiber.StatusForbidden, "insufficient permissions")
	}
	var req channelReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	if req.Type == "" {
		req.Type = "text"
	}
	view, post, msg := resolveTiers(req.MinViewRole, req.MinPostRole, "member", "member")
	if msg != "" {
		return utils.Error(c, fiber.StatusBadRequest, msg)
	}
	ch := models.Channel{
		ID: uuid.NewString(), Name: req.Name, Type: req.Type,
		Topic: req.Topic, ServerID: sid, MinViewRole: view, MinPostRole: post,
	}
	if err := database.DB.Create(&ch).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create channel")
	}
	channelsChanged(sid)
	return c.Status(fiber.StatusCreated).JSON(ch)
}

// managedChannel loads the :id channel for an edit or delete. It returns a
// non-zero HTTP status and message when the caller may not manage it: 404 when
// they cannot see it, 403 for DM channels or without CapManageChannels.
func managedChannel(c *fiber.Ctx) (models.Channel, int, string) {
	var ch models.Channel
	if err := database.DB.First(&ch, "id = ?", c.Params("id")).Error; err != nil {
		return ch, fiber.StatusNotFound, "channel not found"
	}
	a := perms.ChannelAccess(ch.ID, middleware.UserID(c))
	if !a.CanView {
		return ch, fiber.StatusNotFound, "channel not found"
	}
	if ch.ServerID == "" || !perms.Can(a.Tier, perms.CapManageChannels) {
		return ch, fiber.StatusForbidden, "insufficient permissions"
	}
	return ch, 0, ""
}

func (h *Handler) update(c *fiber.Ctx) error {
	ch, status, msg := managedChannel(c)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	var req channelReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	view, post, msg := resolveTiers(req.MinViewRole, req.MinPostRole, ch.MinViewRole, ch.MinPostRole)
	if msg != "" {
		return utils.Error(c, fiber.StatusBadRequest, msg)
	}
	ch.Name, ch.Topic, ch.MinViewRole, ch.MinPostRole = req.Name, req.Topic, view, post
	database.DB.Model(&ch).Updates(map[string]any{
		"name": ch.Name, "topic": ch.Topic, "min_view_role": view, "min_post_role": post,
	})
	ws.H.RecheckRoom(ch.ID)
	channelsChanged(ch.ServerID)
	return utils.OK(c, ch)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	ch, status, msg := managedChannel(c)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	database.DB.Where("channel_id = ?", ch.ID).Delete(&models.Message{})
	database.DB.Delete(&ch)
	ws.H.CloseRoom(ch.ID)
	channelsChanged(ch.ServerID)
	return utils.OK(c, fiber.Map{"message": "channel deleted"})
}

```

(The old `isMember` and `isOwner` helpers are gone, and nothing else in the package used them.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/channels/ -v && go build ./...`
Expected: all tests PASS. If the build says `"gorm.io/gorm" imported and not used`, the DM functions below the marker were removed by mistake. Restore them from git: `git diff server/internal/channels/handler.go`.

- [ ] **Step 5: Commit**

```bash
git add server/internal/channels/
git commit -m "feat(channels): add view/post tiers and admin-only channel management"
```

---

### Task 8: Servers handler — perms, ban-aware join, filtered channels, events

**Files:**
- Modify: `server/internal/servers/handler.go` (entire file)
- Test: `server/internal/servers/handler_test.go`

- [ ] **Step 1: Write the failing test**

Create `server/internal/servers/handler_test.go`:

```go
package servers_test

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/servers"
	"mitrachat/server/internal/testutil"
)

func newApp() *fiber.App {
	app := fiber.New()
	servers.New(testutil.Config).Register(app.Group("/api"))
	return app
}

func inviteCodeOf(t *testing.T, serverID string) string {
	t.Helper()
	var s models.Server
	if err := database.DB.First(&s, "id = ?", serverID).Error; err != nil {
		t.Fatal(err)
	}
	return s.InviteCode
}

func TestGetServerFiltersChannelsByTier(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	cases := []struct {
		user string
		want []string
	}{
		{g.Member, []string{g.Open, g.Announce}},
		{g.Mod, []string{g.Open, g.Announce, g.Staff}},
		{g.Admin, []string{g.Open, g.Announce, g.Staff, g.AdminOnly}},
	}
	for _, c := range cases {
		status, body := testutil.Do(t, app, "GET", "/api/servers/"+g.ServerID, testutil.Token(t, c.user), nil)
		if status != 200 {
			t.Fatalf("status %d (%s)", status, body)
		}
		var srv models.Server
		testutil.Decode(t, body, &srv)
		testutil.SameIDs(t, testutil.ChannelIDs(srv.Channels), c.want)
	}
}

func TestJoinAddsMemberWithFilteredChannels(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	body := map[string]any{"invite_code": inviteCodeOf(t, g.ServerID)}

	status, resp := testutil.Do(t, newApp(), "POST", "/api/servers/join", testutil.Token(t, g.Stranger), body)

	if status != 200 {
		t.Fatalf("status %d (%s)", status, resp)
	}
	if role := testutil.RoleOf(t, g.ServerID, g.Stranger); role != "member" {
		t.Fatalf("role after join = %q, want member", role)
	}
	var srv models.Server
	testutil.Decode(t, resp, &srv)
	testutil.SameIDs(t, testutil.ChannelIDs(srv.Channels), []string{g.Open, g.Announce})
}

func TestJoinRejectsBannedUser(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	database.DB.Create(&models.ServerBan{ID: uuid.NewString(), ServerID: g.ServerID, UserID: g.Stranger, BannedBy: g.Owner})
	body := map[string]any{"invite_code": inviteCodeOf(t, g.ServerID)}

	if status, resp := testutil.Do(t, newApp(), "POST", "/api/servers/join", testutil.Token(t, g.Stranger), body); status != 403 {
		t.Fatalf("status %d (%s), want 403", status, resp)
	}
	if role := testutil.RoleOf(t, g.ServerID, g.Stranger); role != "" {
		t.Fatalf("banned user became %q", role)
	}
}

func TestUpdateServerNeedsAdmin(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	body := map[string]any{"name": "renamed"}
	for user, want := range map[string]int{g.Member: 403, g.Mod: 403, g.Admin: 200} {
		if status, resp := testutil.Do(t, app, "PUT", "/api/servers/"+g.ServerID, testutil.Token(t, user), body); status != want {
			t.Errorf("update: %d (%s), want %d", status, resp, want)
		}
	}
	var s models.Server
	database.DB.First(&s, "id = ?", g.ServerID)
	if s.Name != "renamed" {
		t.Fatalf("name = %q, want renamed", s.Name)
	}
}

func TestDeleteServerOwnerOnly(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	database.DB.Create(&models.ServerBan{ID: uuid.NewString(), ServerID: g.ServerID, UserID: g.Stranger, BannedBy: g.Owner})

	if status, _ := testutil.Do(t, app, "DELETE", "/api/servers/"+g.ServerID, testutil.Token(t, g.Admin), nil); status != 403 {
		t.Errorf("admin delete: %d, want 403", status)
	}
	if status, body := testutil.Do(t, app, "DELETE", "/api/servers/"+g.ServerID, testutil.Token(t, g.Owner), nil); status != 200 {
		t.Fatalf("owner delete: %d (%s)", status, body)
	}
	var nServers, nChans, nBans int64
	database.DB.Model(&models.Server{}).Where("id = ?", g.ServerID).Count(&nServers)
	database.DB.Model(&models.Channel{}).Where("server_id = ?", g.ServerID).Count(&nChans)
	database.DB.Model(&models.ServerBan{}).Where("server_id = ?", g.ServerID).Count(&nBans)
	if nServers+nChans+nBans != 0 {
		t.Fatalf("left behind: %d servers, %d channels, %d bans", nServers, nChans, nBans)
	}
}

func TestRegenerateInvite(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	before := inviteCodeOf(t, g.ServerID)
	path := "/api/servers/" + g.ServerID + "/invite/regenerate"

	if status, _ := testutil.Do(t, app, "POST", path, testutil.Token(t, g.Member), nil); status != 403 {
		t.Errorf("member regenerate: %d, want 403", status)
	}
	status, body := testutil.Do(t, app, "POST", path, testutil.Token(t, g.Admin), nil)
	if status != 200 {
		t.Fatalf("admin regenerate: %d (%s)", status, body)
	}
	var resp struct {
		InviteCode string `json:"invite_code"`
	}
	testutil.Decode(t, body, &resp)
	if resp.InviteCode == "" || resp.InviteCode == before || inviteCodeOf(t, g.ServerID) != resp.InviteCode {
		t.Fatalf("invite code %q -> %q not stored", before, resp.InviteCode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/servers/ -v`
Expected: FAIL. For example, `ids = [... staff admin-only ...]` for the member (channels aren't filtered), `TestJoinRejectsBannedUser` gets status 200, the admin update gets 403 (owner-only today), and regenerate gets 404 or 405 (no route yet).

- [ ] **Step 3: Write the implementation**

Replace `server/internal/servers/handler.go` with:

```go
package servers

import (
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

var validate = validator.New()

// Handler groups server/member endpoints.
type Handler struct{ cfg *config.Config }

// New builds a servers handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts server routes (all protected).
func (h *Handler) Register(r fiber.Router) {
	g := r.Group("/servers", middleware.Protected(h.cfg))
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Post("/join", h.join)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.delete)
	g.Post("/:id/invite", h.invite)
	g.Post("/:id/invite/regenerate", h.regenerateInvite)
	g.Get("/:id/members", h.members)
}

// require checks that userID is a member of serverID holding need. It returns
// the caller's tier, or a non-zero HTTP status and message to send.
func require(serverID, userID string, need perms.Capability) (perms.Tier, int, string) {
	t, ok := perms.MemberTier(serverID, userID)
	if !ok {
		return t, fiber.StatusForbidden, "not a member"
	}
	if !perms.Can(t, need) {
		return t, fiber.StatusForbidden, "insufficient permissions"
	}
	return t, 0, ""
}

func inviteCode() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

func (h *Handler) list(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var servers []models.Server
	database.DB.
		Joins("JOIN server_members sm ON sm.server_id = servers.id").
		Where("sm.user_id = ?", uid).
		Find(&servers)
	return utils.OK(c, servers)
}

type createReq struct {
	Name        string `json:"name" validate:"required,min=3,max=100"`
	Description string `json:"description" validate:"max=1024"`
	Icon        string `json:"icon" validate:"max=512"`
}

func (h *Handler) create(c *fiber.Ctx) error {
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	srv := models.Server{
		ID: uuid.NewString(), Name: req.Name, OwnerID: uid,
		Description: req.Description, Icon: req.Icon, InviteCode: inviteCode(),
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&srv).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ServerMember{
			ID: uuid.NewString(), ServerID: srv.ID, UserID: uid,
			Role: "owner", JoinedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Create(&models.Channel{
			ID: uuid.NewString(), Name: "general", Type: "text", ServerID: srv.ID,
		}).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create server")
	}
	database.DB.Preload("Channels").First(&srv, "id = ?", srv.ID)
	return c.Status(fiber.StatusCreated).JSON(srv)
}

// get returns the server with only the channels the caller's tier may view.
func (h *Handler) get(c *fiber.Ctx) error {
	id := c.Params("id")
	t, ok := perms.MemberTier(id, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var srv models.Server
	if err := database.DB.Preload("Channels", perms.VisibleChannels(t)).First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	return utils.OK(c, srv)
}

func (h *Handler) update(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapManageServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	srv.Name, srv.Description, srv.Icon = req.Name, req.Description, req.Icon
	database.DB.Model(&srv).Updates(map[string]any{
		"name": req.Name, "description": req.Description, "icon": req.Icon,
	})
	ws.SendToServerMembers(id, ws.Event("server_updated", srv))
	return utils.OK(c, srv)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapDeleteServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	// Collect event recipients and rooms before the rows disappear.
	var memberIDs, channelIDs []string
	database.DB.Model(&models.ServerMember{}).Where("server_id = ?", id).Pluck("user_id", &memberIDs)
	database.DB.Model(&models.Channel{}).Where("server_id = ?", id).Pluck("id", &channelIDs)
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if len(channelIDs) > 0 {
			if err := tx.Where("channel_id IN ?", channelIDs).Delete(&models.Message{}).Error; err != nil {
				return err
			}
		}
		for _, table := range []any{&models.Channel{}, &models.ServerMember{}, &models.ServerBan{}} {
			if err := tx.Where("server_id = ?", id).Delete(table).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&srv).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not delete server")
	}
	for _, ch := range channelIDs {
		ws.H.CloseRoom(ch)
	}
	ev := ws.Event("server_deleted", fiber.Map{"server_id": id})
	for _, uid := range memberIDs {
		ws.H.SendToUser(uid, ev)
	}
	return utils.OK(c, fiber.Map{"message": "server deleted"})
}

func (h *Handler) invite(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, ok := perms.MemberTier(id, middleware.UserID(c)); !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	if srv.InviteCode == "" {
		srv.InviteCode = inviteCode()
		database.DB.Model(&srv).Update("invite_code", srv.InviteCode)
	}
	return utils.OK(c, fiber.Map{"invite_code": srv.InviteCode})
}

// regenerateInvite replaces the invite code so old codes stop working.
func (h *Handler) regenerateInvite(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapManageServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	code := inviteCode()
	if err := database.DB.Model(&models.Server{}).Where("id = ?", id).Update("invite_code", code).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not regenerate invite")
	}
	return utils.OK(c, fiber.Map{"invite_code": code})
}

type joinReq struct {
	InviteCode string `json:"invite_code" validate:"required"`
}

// join adds the caller as a member (unless banned) and returns the server with
// the channels a member may view.
func (h *Handler) join(c *fiber.Ctx) error {
	var req joinReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var srv models.Server
	if err := database.DB.Where("invite_code = ?", req.InviteCode).First(&srv).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "invalid invite code")
	}
	uid := middleware.UserID(c)
	t, isMember := perms.MemberTier(srv.ID, uid)
	if !isMember {
		var bans int64
		database.DB.Model(&models.ServerBan{}).Where("server_id = ? AND user_id = ?", srv.ID, uid).Count(&bans)
		if bans > 0 {
			return utils.Error(c, fiber.StatusForbidden, "banned from server")
		}
		member := models.ServerMember{
			ID: uuid.NewString(), ServerID: srv.ID, UserID: uid,
			Role: "member", JoinedAt: time.Now(),
		}
		if err := database.DB.Create(&member).Error; err != nil {
			return utils.Error(c, fiber.StatusInternalServerError, "could not join server")
		}
		t = perms.Member
		database.DB.Preload("User").First(&member, "id = ?", member.ID)
		ws.SendToServerMembers(srv.ID, ws.Event("member_joined", fiber.Map{"server_id": srv.ID, "member": member}))
	}
	database.DB.Preload("Channels", perms.VisibleChannels(t)).First(&srv, "id = ?", srv.ID)
	return utils.OK(c, srv)
}

func (h *Handler) members(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, ok := perms.MemberTier(id, middleware.UserID(c)); !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var members []models.ServerMember
	database.DB.Preload("User").Where("server_id = ?", id).Find(&members)
	return utils.OK(c, members)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd server && go test ./internal/servers/ -v && go build ./...`
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/servers/
git commit -m "feat(servers): role-gated settings, ban-aware join, tier-filtered channels"
```

---

### Task 9: Member lifecycle — leave, kick, set role

**Files:**
- Create: `server/internal/servers/members.go`
- Modify: `server/internal/servers/handler.go` (`Register`)
- Test: `server/internal/servers/members_test.go` (uses `newApp` from `handler_test.go`, Task 8)

- [ ] **Step 1: Write the failing test**

Create `server/internal/servers/members_test.go`:

```go
package servers_test

import (
	"testing"

	"mitrachat/server/internal/testutil"
)

func TestLeave(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	path := "/api/servers/" + g.ServerID + "/members/me"

	if status, body := testutil.Do(t, app, "DELETE", path, testutil.Token(t, g.Member), nil); status != 200 {
		t.Fatalf("member leave: %d (%s)", status, body)
	}
	if role := testutil.RoleOf(t, g.ServerID, g.Member); role != "" {
		t.Fatalf("still a %q after leaving", role)
	}
	if status, _ := testutil.Do(t, app, "DELETE", path, testutil.Token(t, g.Owner), nil); status != 400 {
		t.Errorf("owner leave: %d, want 400", status)
	}
	if status, _ := testutil.Do(t, app, "DELETE", path, testutil.Token(t, g.Stranger), nil); status != 403 {
		t.Errorf("stranger leave: %d, want 403", status)
	}
}

func TestKick(t *testing.T) {
	cases := []struct {
		name, actor, target string
		status              int
	}{
		{"mod kicks member", "mod", "member", 200},
		{"admin kicks mod", "admin", "mod", 200},
		{"owner kicks admin", "owner", "admin", 200},
		{"mod cannot kick admin", "mod", "admin", 403},
		{"admin cannot kick owner", "admin", "owner", 403},
		{"member cannot kick", "member", "mod", 403},
		{"cannot kick self", "mod", "mod", 400},
		{"target not a member", "admin", "stranger", 404},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.SetupDB(t)
			g := testutil.SeedGuild(t)
			path := "/api/servers/" + g.ServerID + "/members/" + g.User(c.target)
			status, body := testutil.Do(t, newApp(), "DELETE", path, testutil.Token(t, g.User(c.actor)), nil)
			if status != c.status {
				t.Fatalf("status %d (%s), want %d", status, body, c.status)
			}
			if status == 200 && testutil.RoleOf(t, g.ServerID, g.User(c.target)) != "" {
				t.Fatal("kicked user is still a member")
			}
		})
	}
}

func TestSetRole(t *testing.T) {
	cases := []struct {
		name, actor, target, role string
		status                    int
	}{
		{"admin promotes member", "admin", "member", "moderator", 200},
		{"admin demotes mod", "admin", "mod", "member", 200},
		{"owner grants admin", "owner", "mod", "admin", 200},
		{"admin cannot grant admin", "admin", "member", "admin", 403},
		{"owner cannot grant owner", "owner", "admin", "owner", 403},
		{"mod cannot set roles", "mod", "member", "moderator", 403},
		{"admin cannot touch owner", "admin", "owner", "member", 403},
		{"unknown role", "owner", "member", "superuser", 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.SetupDB(t)
			g := testutil.SeedGuild(t)
			path := "/api/servers/" + g.ServerID + "/members/" + g.User(c.target) + "/role"
			status, body := testutil.Do(t, newApp(), "PUT", path, testutil.Token(t, g.User(c.actor)), map[string]any{"role": c.role})
			if status != c.status {
				t.Fatalf("status %d (%s), want %d", status, body, c.status)
			}
			if status == 200 {
				if got := testutil.RoleOf(t, g.ServerID, g.User(c.target)); got != c.role {
					t.Fatalf("role = %q, want %q", got, c.role)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/servers/ -run 'Leave|Kick|SetRole' -v`
Expected: FAIL. The routes don't exist yet, so the statuses come back as `405` or `404`, not the wanted values.

- [ ] **Step 3: Write the member handlers**

Create `server/internal/servers/members.go`:

```go
package servers

import (
	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

// action is a resolved moderation or role change: who acts on whom, and their tiers.
type action struct {
	serverID, actorID, targetID string
	actor, target               perms.Tier
}

// resolveAction checks that the caller holds need in the :id server and
// strictly outranks targetID. It returns a non-zero HTTP status and message
// when the action is refused.
func resolveAction(c *fiber.Ctx, need perms.Capability, targetID string) (action, int, string) {
	a := action{serverID: c.Params("id"), actorID: middleware.UserID(c), targetID: targetID}
	var status int
	var msg string
	if a.actor, status, msg = require(a.serverID, a.actorID, need); status != 0 {
		return a, status, msg
	}
	if targetID == a.actorID {
		return a, fiber.StatusBadRequest, "cannot target yourself"
	}
	var ok bool
	if a.target, ok = perms.MemberTier(a.serverID, targetID); !ok {
		return a, fiber.StatusNotFound, "member not found"
	}
	if !perms.CanActOn(a.actor, a.target) {
		return a, fiber.StatusForbidden, "cannot act on equal or higher role"
	}
	return a, 0, ""
}

func deleteMembership(serverID, userID string) error {
	return database.DB.Where("server_id = ? AND user_id = ?", serverID, userID).
		Delete(&models.ServerMember{}).Error
}

// notifyRemoved drops a removed user from the server's rooms and tells the
// remaining members and the removed user (who is no longer a member).
func notifyRemoved(serverID, userID, reason string) {
	ws.H.RecheckRooms(userID)
	ev := ws.Event("member_removed", fiber.Map{"server_id": serverID, "user_id": userID, "reason": reason})
	ws.SendToServerMembers(serverID, ev)
	ws.H.SendToUser(userID, ev)
}

// leave removes the caller from the server. The owner must delete it instead.
func (h *Handler) leave(c *fiber.Ctx) error {
	id, uid := c.Params("id"), middleware.UserID(c)
	t, ok := perms.MemberTier(id, uid)
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	if t == perms.Owner {
		return utils.Error(c, fiber.StatusBadRequest, "owner must delete the server instead")
	}
	if err := deleteMembership(id, uid); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not leave server")
	}
	notifyRemoved(id, uid, "leave")
	return utils.OK(c, fiber.Map{"message": "left server"})
}

func (h *Handler) kick(c *fiber.Ctx) error {
	a, status, msg := resolveAction(c, perms.CapKick, c.Params("userId"))
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	if err := deleteMembership(a.serverID, a.targetID); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not kick member")
	}
	notifyRemoved(a.serverID, a.targetID, "kick")
	return utils.OK(c, fiber.Map{"message": "member kicked"})
}

type roleReq struct {
	Role string `json:"role"`
}

// setRole changes a lower-ranked member's role to one below the caller's own.
func (h *Handler) setRole(c *fiber.Ctx) error {
	a, status, msg := resolveAction(c, perms.CapManageRoles, c.Params("userId"))
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	var req roleReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	role, ok := perms.ParseTier(req.Role)
	if !ok {
		return utils.Error(c, fiber.StatusBadRequest, "invalid role")
	}
	if !perms.CanAssign(a.actor, role) {
		return utils.Error(c, fiber.StatusForbidden, "cannot act on equal or higher role")
	}
	if err := database.DB.Model(&models.ServerMember{}).
		Where("server_id = ? AND user_id = ?", a.serverID, a.targetID).
		Update("role", role.String()).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not update role")
	}
	ws.H.RecheckRooms(a.targetID)
	payload := fiber.Map{"server_id": a.serverID, "user_id": a.targetID, "role": role.String()}
	ws.SendToServerMembers(a.serverID, ws.Event("member_role_updated", payload))
	return utils.OK(c, payload)
}
```

- [ ] **Step 4: Register the routes**

In `server/internal/servers/handler.go` `Register`, add these lines after `g.Get("/:id/members", h.members)`. The `me` route must come before `:userId`:

```go
	g.Delete("/:id/members/me", h.leave)
	g.Delete("/:id/members/:userId", h.kick)
	g.Put("/:id/members/:userId/role", h.setRole)
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd server && go test ./internal/servers/ -v`
Expected: all tests PASS, including the Task 8 tests.

- [ ] **Step 6: Commit**

```bash
git add server/internal/servers/
git commit -m "feat(servers): add leave, kick and role assignment with hierarchy rules"
```

---

### Task 10: Bans — list, ban, unban

**Files:**
- Create: `server/internal/servers/bans.go`
- Modify: `server/internal/servers/handler.go` (`Register`)
- Test: `server/internal/servers/bans_test.go` (uses `newApp` and `inviteCodeOf` from `handler_test.go`, Task 8)

- [ ] **Step 1: Write the failing test**

Create `server/internal/servers/bans_test.go`:

```go
package servers_test

import (
	"testing"

	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func TestBanBlocksRejoinUntilUnban(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	bans := "/api/servers/" + g.ServerID + "/bans"
	join := map[string]any{"invite_code": inviteCodeOf(t, g.ServerID)}

	if status, body := testutil.Do(t, app, "POST", bans, testutil.Token(t, g.Mod), map[string]any{"user_id": g.Member}); status != 201 {
		t.Fatalf("ban: %d (%s)", status, body)
	}
	if role := testutil.RoleOf(t, g.ServerID, g.Member); role != "" {
		t.Fatalf("banned user still a %q", role)
	}
	if status, _ := testutil.Do(t, app, "POST", "/api/servers/join", testutil.Token(t, g.Member), join); status != 403 {
		t.Fatalf("banned rejoin: %d, want 403", status)
	}
	if status, body := testutil.Do(t, app, "DELETE", bans+"/"+g.Member, testutil.Token(t, g.Mod), nil); status != 200 {
		t.Fatalf("unban: %d (%s)", status, body)
	}
	if status, _ := testutil.Do(t, app, "POST", "/api/servers/join", testutil.Token(t, g.Member), join); status != 200 {
		t.Fatalf("rejoin after unban: %d, want 200", status)
	}
}

func TestBanHierarchy(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	bans := "/api/servers/" + g.ServerID + "/bans"

	if status, _ := testutil.Do(t, app, "POST", bans, testutil.Token(t, g.Mod), map[string]any{"user_id": g.Admin}); status != 403 {
		t.Errorf("mod bans admin: %d, want 403", status)
	}
	if status, _ := testutil.Do(t, app, "POST", bans, testutil.Token(t, g.Member), map[string]any{"user_id": g.Mod}); status != 403 {
		t.Errorf("member bans mod: %d, want 403", status)
	}
	if status, _ := testutil.Do(t, app, "POST", bans, testutil.Token(t, g.Admin), map[string]any{"user_id": g.Stranger}); status != 404 {
		t.Errorf("ban non-member: %d, want 404", status)
	}
}

func TestListBans(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	bans := "/api/servers/" + g.ServerID + "/bans"
	testutil.Do(t, app, "POST", bans, testutil.Token(t, g.Mod), map[string]any{"user_id": g.Member})

	if status, _ := testutil.Do(t, app, "GET", bans, testutil.Token(t, g.Member), nil); status != 403 {
		t.Errorf("list as banned ex-member: %d, want 403", status)
	}
	status, body := testutil.Do(t, app, "GET", bans, testutil.Token(t, g.Mod), nil)
	if status != 200 {
		t.Fatalf("list as mod: %d (%s)", status, body)
	}
	var list []models.ServerBan
	testutil.Decode(t, body, &list)
	if len(list) != 1 || list[0].UserID != g.Member || list[0].User == nil {
		t.Fatalf("bans = %+v", list)
	}
}

func TestUnbanMissingIs404(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	path := "/api/servers/" + g.ServerID + "/bans/" + g.Stranger
	if status, _ := testutil.Do(t, newApp(), "DELETE", path, testutil.Token(t, g.Mod), nil); status != 404 {
		t.Fatalf("status %d, want 404", status)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd server && go test ./internal/servers/ -run 'Ban' -v`
Expected: FAIL. The routes are missing, so the statuses come back as `404` or `405`.

- [ ] **Step 3: Write the ban handlers**

Create `server/internal/servers/bans.go`:

```go
package servers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
)

type banReq struct {
	UserID string `json:"user_id" validate:"required"`
}

func (h *Handler) listBans(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapBan); status != 0 {
		return utils.Error(c, status, msg)
	}
	var bans []models.ServerBan
	database.DB.Preload("User").Where("server_id = ?", id).Order("created_at desc").Find(&bans)
	return utils.OK(c, bans)
}

// ban removes a lower-ranked member and blocks them from rejoining by invite.
func (h *Handler) ban(c *fiber.Ctx) error {
	var req banReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	a, status, msg := resolveAction(c, perms.CapBan, req.UserID)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	ban := models.ServerBan{
		ID: uuid.NewString(), ServerID: a.serverID, UserID: a.targetID,
		BannedBy: a.actorID, CreatedAt: time.Now(),
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("server_id = ? AND user_id = ?", a.serverID, a.targetID).
			Delete(&models.ServerMember{}).Error; err != nil {
			return err
		}
		return tx.Create(&ban).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not ban member")
	}
	notifyRemoved(a.serverID, a.targetID, "ban")
	return c.Status(fiber.StatusCreated).JSON(ban)
}

func (h *Handler) unban(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapBan); status != 0 {
		return utils.Error(c, status, msg)
	}
	res := database.DB.Where("server_id = ? AND user_id = ?", id, c.Params("userId")).Delete(&models.ServerBan{})
	if res.Error != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not unban")
	}
	if res.RowsAffected == 0 {
		return utils.Error(c, fiber.StatusNotFound, "ban not found")
	}
	return utils.OK(c, fiber.Map{"message": "unbanned"})
}
```

- [ ] **Step 4: Register the routes**

In `server/internal/servers/handler.go` `Register`, add after the `setRole` route from Task 9:

```go
	g.Get("/:id/bans", h.listBans)
	g.Post("/:id/bans", h.ban)
	g.Delete("/:id/bans/:userId", h.unban)
```

- [ ] **Step 5: Run the whole server suite**

Run: `cd server && go vet ./... && go test ./...`
Expected: vet prints nothing. Every package with tests reports `ok` (database, perms, ws, messages, channels, servers), and the rest report `[no test files]`.

- [ ] **Step 6: Commit**

```bash
git add server/internal/servers/
git commit -m "feat(servers): add ban, unban and ban list"
```

---

### Task 11: Client — types and permissions helper

**Files:**
- Create: `client/src/permissions.ts`
- Modify: `client/src/types.ts`
- Test: `client/src/__tests__/permissions.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `client/src/__tests__/permissions.spec.ts`:

```ts
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd client && npx vitest run src/__tests__/permissions.spec.ts`
Expected: FAIL with `Failed to resolve import "@/permissions"`.

- [ ] **Step 3: Update types**

In `client/src/types.ts`, add at the top of the file (above `export interface User`):

```ts
// Server role, lowest first. Mirrors server/internal/perms tiers.
export type Role = 'member' | 'moderator' | 'admin' | 'owner'

```

Replace `export interface Channel { … }` with:

```ts
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
```

Replace `export interface ServerMember { … }` with:

```ts
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
```

- [ ] **Step 4: Write the permissions helper**

Create `client/src/permissions.ts`:

```ts
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
```

- [ ] **Step 5: Run tests and type-check**

Run: `cd client && npx vitest run && npm run type-check`
Expected: vitest `Test Files  1 passed`, all tests pass. The type-check exits 0.

- [ ] **Step 6: Commit**

```bash
git add client/src/types.ts client/src/permissions.ts client/src/__tests__/
git commit -m "feat(client): add role types and permissions helper"
```

---

### Task 12: Client — API wrappers and stores

**Files:**
- Modify: `client/src/api/index.ts`
- Modify: `client/src/stores/servers.ts` (entire file)
- Modify: `client/src/stores/channels.ts` (entire file)
- Modify: `client/src/stores/messages.ts` (imports, `load()`, `wire()`)
- Modify: `client/src/views/DashboardView.vue` (`onMounted` only; the rest is in Task 16)

- [ ] **Step 1: Extend the API wrappers**

In `client/src/api/index.ts`, replace the `import type { … } from '@/types'` block with:

```ts
import type {
  Channel,
  FriendRequest,
  Message,
  Notification,
  Role,
  Server,
  ServerBan,
  ServerMember,
  User,
  Attachment,
} from '@/types'

// Body for creating or editing a channel. Omitted tiers keep their current
// value on edit and default to 'member' on create.
export type ChannelBody = {
  name: string
  type?: string
  topic?: string
  min_view_role?: Role
  min_post_role?: Role
}
```

Replace the `serverApi` object with:

```ts
export const serverApi = {
  list: () => client.get<Server[]>('/servers'),
  create: (b: { name: string; description?: string; icon?: string }) =>
    client.post<Server>('/servers', b),
  get: (id: string) => client.get<Server>(`/servers/${id}`),
  update: (id: string, b: { name: string; description?: string; icon?: string }) =>
    client.put<Server>(`/servers/${id}`, b),
  remove: (id: string) => client.delete(`/servers/${id}`),
  invite: (id: string) => client.post<{ invite_code: string }>(`/servers/${id}/invite`),
  regenerateInvite: (id: string) =>
    client.post<{ invite_code: string }>(`/servers/${id}/invite/regenerate`),
  join: (invite_code: string) => client.post<Server>('/servers/join', { invite_code }),
  members: (id: string) => client.get<ServerMember[]>(`/servers/${id}/members`),
  leave: (id: string) => client.delete(`/servers/${id}/members/me`),
  kick: (id: string, userId: string) => client.delete(`/servers/${id}/members/${userId}`),
  setRole: (id: string, userId: string, role: Role) =>
    client.put(`/servers/${id}/members/${userId}/role`, { role }),
  bans: (id: string) => client.get<ServerBan[]>(`/servers/${id}/bans`),
  ban: (id: string, user_id: string) => client.post<ServerBan>(`/servers/${id}/bans`, { user_id }),
  unban: (id: string, userId: string) => client.delete(`/servers/${id}/bans/${userId}`),
}
```

Replace the `channelApi` `create` and `update` lines with:

```ts
  create: (serverId: string, b: ChannelBody) =>
    client.post<Channel>(`/servers/${serverId}/channels`, b),
  update: (id: string, b: ChannelBody) => client.put<Channel>(`/channels/${id}`, b),
```

- [ ] **Step 2: Rewrite the channels store**

Replace `client/src/stores/channels.ts` with:

```ts
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { channelApi, type ChannelBody } from '@/api'
import type { Channel } from '@/types'

// Channels store: channels for the active server plus the currently selected channel.
// The list comes from the server already filtered to what the user may view.
export const useChannelsStore = defineStore('channels', () => {
  const channels = ref<Channel[]>([])
  const currentChannelId = ref<string>('')

  async function fetch(serverId: string) {
    const { data } = await channelApi.list(serverId)
    channels.value = data
  }

  async function create(serverId: string, b: ChannelBody) {
    const { data } = await channelApi.create(serverId, b)
    // A channels_changed refetch may have added it already.
    if (!channels.value.some((c) => c.id === data.id)) channels.value.push(data)
    return data
  }

  async function update(id: string, b: ChannelBody) {
    const { data } = await channelApi.update(id, b)
    const i = channels.value.findIndex((c) => c.id === id)
    if (i !== -1) channels.value[i] = data
    return data
  }

  async function remove(id: string) {
    await channelApi.remove(id)
    channels.value = channels.value.filter((c) => c.id !== id)
  }

  function select(id: string) {
    currentChannelId.value = id
  }

  return { channels, currentChannelId, fetch, create, update, remove, select }
})
```

- [ ] **Step 3: Rewrite the servers store**

Replace `client/src/stores/servers.ts` with:

```ts
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
```

- [ ] **Step 4: Handle lost channel access in the messages store**

In `client/src/stores/messages.ts`, add these imports below `import { socket } from '@/ws/socket'`:

```ts
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
```

Replace the `load` function with:

```ts
  async function load() {
    loading.value = true
    try {
      // Page backwards using the oldest loaded message as the `before` cursor.
      const before = messages.value[0]?.id
      const { data } = await messageApi.history(channelId.value, before)
      if (data.length < 50) hasMore.value = false // short page => no older history left
      // Prepend the older page ahead of what we already have (keeps ascending order).
      messages.value = [...data, ...messages.value]
    } catch (e: any) {
      const status = e.response?.status
      if (status !== 403 && status !== 404) throw e
      hasMore.value = false
      refreshChannels()
    } finally {
      loading.value = false
    }
  }

  // The server says we can no longer see the open channel. Refetch the channel
  // list so the dashboard can move us to one we can see.
  function refreshChannels() {
    const servers = useServersStore()
    if (servers.currentServerId) useChannelsStore().fetch(servers.currentServerId)
  }
```

In `wire()`, add this handler directly after the `socket.on('message_deleted', …)` block:

```ts
    socket.on('error', (p: { code: string; channel_id?: string }) => {
      if (p.code === 'not_found' && p.channel_id && p.channel_id === channelId.value) {
        refreshChannels()
      }
    })
```

- [ ] **Step 5: Wire the servers store on the dashboard**

In `client/src/views/DashboardView.vue`, in `onMounted`, add `servers.wire()` after `notifications.wire()`:

```ts
onMounted(async () => {
  messages.wire()
  friends.wire()
  notifications.wire()
  servers.wire()
  if (!auth.user) await auth.fetchMe()
  await Promise.all([servers.fetch(), friends.fetch(), dm.fetch(), notifications.fetch()])
})
```

- [ ] **Step 6: Type-check and test**

Run: `cd client && npm run type-check && npx vitest run`
Expected: type-check exits 0 and the vitest suite passes. If the type-check reports errors in `CreateChannelModal.vue` about the `create` argument, check that `ChannelBody` is exported (Step 1).

- [ ] **Step 7: Commit**

```bash
git add client/src/api/index.ts client/src/stores/ client/src/views/DashboardView.vue
git commit -m "feat(client): add moderation API, role-aware stores and live membership events"
```

---

### Task 13: Client — channel modal with tiers, and server settings modal

**Files:**
- Modify: `client/src/components/CreateChannelModal.vue` (entire file)
- Create: `client/src/components/ServerSettingsModal.vue`

- [ ] **Step 1: Rewrite the channel modal (create + edit)**

Replace `client/src/components/CreateChannelModal.vue` with:

```vue
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
```

- [ ] **Step 2: Create the server settings modal**

Create `client/src/components/ServerSettingsModal.vue`:

```vue
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
```

- [ ] **Step 3: Type-check**

Run: `cd client && npm run type-check`
Expected: exits 0.

- [ ] **Step 4: Commit**

```bash
git add client/src/components/CreateChannelModal.vue client/src/components/ServerSettingsModal.vue
git commit -m "feat(client): channel tier editing and server settings modal"
```

---

### Task 14: Client — channel sidebar gating and server menu

**Files:**
- Modify: `client/src/components/ChannelSidebar.vue` (entire file)

- [ ] **Step 1: Rewrite the sidebar**

Replace `client/src/components/ChannelSidebar.vue` with:

```vue
<script setup lang="ts">
import { ref, computed } from 'vue'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { useAuthStore } from '@/stores/auth'
import { can } from '@/permissions'
import type { Channel } from '@/types'
import CreateChannelModal from '@/components/CreateChannelModal.vue'
import InviteServerModal from '@/components/InviteServerModal.vue'
import ServerSettingsModal from '@/components/ServerSettingsModal.vue'

defineProps<{ activeChannel: string }>()
defineEmits<{ open: [id: string] }>()

const servers = useServersStore()
const channels = useChannelsStore()
const auth = useAuthStore()
const showCreate = ref(false)
const showInvite = ref(false)
const showSettings = ref(false)
const showMenu = ref(false)
const editing = ref<Channel | null>(null) // channel whose edit modal is open

const server = computed(() => servers.servers.find((s) => s.id === servers.currentServerId))
const canManageChannels = computed(() => can(servers.myRole, 'manageChannels'))
// Settings has an Overview tab for admins and a Bans tab for moderators.
const canOpenSettings = computed(
  () => can(servers.myRole, 'manageServer') || can(servers.myRole, 'ban'),
)
const canLeave = computed(() => !!servers.myRole && servers.myRole !== 'owner')

function openFromMenu(which: 'invite' | 'settings') {
  showMenu.value = false
  if (which === 'invite') showInvite.value = true
  else showSettings.value = true
}

async function leave() {
  showMenu.value = false
  if (!server.value || !window.confirm(`Leave ${server.value.name}?`)) return
  try {
    await servers.leave(server.value.id)
  } catch (e: any) {
    alert(e.response?.data?.error || 'Could not leave server')
  }
}
</script>

<template>
  <aside class="flex w-60 flex-col bg-bg-alt">
    <div class="relative border-b border-black/30 shadow-sm">
      <button
        v-if="server"
        @click="showMenu = !showMenu"
        class="flex h-12 w-full items-center justify-between px-4 font-semibold text-white hover:bg-white/5"
      >
        <span class="truncate">{{ server.name }}</span>
        <span class="ml-2 shrink-0 text-xs text-txt-muted">{{ showMenu ? '✕' : '▾' }}</span>
      </button>
      <div v-else class="h-12"></div>
      <div
        v-if="showMenu"
        class="absolute left-2 right-2 top-12 z-40 flex flex-col rounded bg-bg-dark py-1 text-sm shadow-lg"
      >
        <button class="px-3 py-2 text-left text-txt hover:bg-blurple hover:text-white" @click="openFromMenu('invite')">
          Invite people
        </button>
        <button
          v-if="canOpenSettings"
          class="px-3 py-2 text-left text-txt hover:bg-blurple hover:text-white"
          @click="openFromMenu('settings')"
        >
          Server settings
        </button>
        <button v-if="canLeave" class="px-3 py-2 text-left text-red-400 hover:bg-red-500 hover:text-white" @click="leave">
          Leave server
        </button>
      </div>
    </div>

    <div class="flex-1 overflow-y-auto px-2 py-3">
      <div class="mb-1 flex items-center justify-between px-2">
        <span class="text-xs font-semibold uppercase tracking-wide text-txt-muted">Text Channels</span>
        <button
          v-if="canManageChannels"
          @click="showCreate = true"
          class="text-lg leading-none text-txt-muted hover:text-white"
          title="Create channel"
        >
          +
        </button>
      </div>

      <div
        v-for="c in channels.channels"
        :key="c.id"
        :class="['group flex items-center rounded hover:bg-white/5', activeChannel === c.id ? 'bg-white/10' : '']"
      >
        <button
          @click="$emit('open', c.id)"
          :class="[
            'flex min-w-0 flex-1 items-center gap-1 px-2 py-1.5 text-left',
            activeChannel === c.id ? 'text-white' : 'text-txt-muted hover:text-txt',
          ]"
        >
          <span class="text-lg text-txt-muted">#</span>
          <span class="truncate">{{ c.name }}</span>
          <span v-if="c.min_view_role && c.min_view_role !== 'member'" class="text-xs" title="Private channel">🔒</span>
        </button>
        <button
          v-if="canManageChannels"
          @click="editing = c"
          class="mr-1 hidden px-1 text-sm text-txt-muted hover:text-white group-hover:block"
          title="Edit channel"
        >
          ⚙
        </button>
      </div>
    </div>

    <div class="flex items-center gap-2 bg-bg-dark/60 px-3 py-2">
      <div class="flex h-8 w-8 items-center justify-center rounded-full bg-blurple text-sm font-semibold text-white">
        {{ auth.user?.username?.slice(0, 1).toUpperCase() }}
      </div>
      <div class="min-w-0">
        <div class="truncate text-sm font-medium text-white">{{ auth.user?.username }}</div>
        <div class="text-xs text-green-400">online</div>
      </div>
    </div>

    <CreateChannelModal v-if="showCreate" :server-id="servers.currentServerId" @close="showCreate = false" />
    <CreateChannelModal
      v-if="editing"
      :server-id="servers.currentServerId"
      :channel="editing"
      @close="editing = null"
    />
    <InviteServerModal v-if="showInvite" :server-id="servers.currentServerId" @close="showInvite = false" />
    <ServerSettingsModal v-if="showSettings && server" :server="server" @close="showSettings = false" />
  </aside>
</template>
```

- [ ] **Step 2: Type-check**

Run: `cd client && npm run type-check`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add client/src/components/ChannelSidebar.vue
git commit -m "feat(client): role-gated channel management and server menu"
```

---

### Task 15: Client — member list roles and moderation menu

**Files:**
- Modify: `client/src/components/MemberList.vue` (entire file)

- [ ] **Step 1: Rewrite the member list**

Replace `client/src/components/MemberList.vue` with:

```vue
<script setup lang="ts">
import { computed, ref } from 'vue'
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
  } catch (e: any) {
    error.value = e.response?.data?.error || 'Action failed'
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
```

- [ ] **Step 2: Type-check**

Run: `cd client && npm run type-check`
Expected: exits 0.

- [ ] **Step 3: Commit**

```bash
git add client/src/components/MemberList.vue
git commit -m "feat(client): group members by role with kick, ban and role menu"
```

---

### Task 16: Client — message actions, read-only input, dashboard reactions

**Files:**
- Modify: `client/src/components/MessageItem.vue` (script, toolbar)
- Modify: `client/src/components/MessageInput.vue` (entire file)
- Modify: `client/src/views/DashboardView.vue` (`selectServer`, watchers, banner)

- [ ] **Step 1: Update MessageItem**

In `client/src/components/MessageItem.vue`, replace the imports and setup lines above `const grouped = computed(` with:

```ts
import { ref, computed } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useMessagesStore } from '@/stores/messages'
import { useServersStore } from '@/stores/servers'
import { useChannelsStore } from '@/stores/channels'
import { can, canPost } from '@/permissions'
import type { Message } from '@/types'

const props = defineProps<{ message: Message; prev?: Message }>()
const auth = useAuthStore()
const messages = useMessagesStore()
const servers = useServersStore()
const channels = useChannelsStore()

const isOwn = computed(() => props.message.user_id === auth.user?.id)
// The server channel this message is in; undefined for DMs.
const serverChannel = computed(() => channels.channels.find((c) => c.id === props.message.channel_id))
// Authors edit their own messages while they may still post in the channel.
const canEdit = computed(
  () => isOwn.value && (!serverChannel.value || canPost(servers.myRole, serverChannel.value)),
)
// Authors delete their own; moderators and up delete anyone's in server channels.
const canDelete = computed(
  () => isOwn.value || (!!serverChannel.value && can(servers.myRole, 'deleteAnyMessage')),
)
```

Add this function after `saveEdit()`:

```ts
function remove() {
  if (!isOwn.value && !window.confirm(`Delete this message by ${props.message.user?.username}?`)) return
  messages.remove(props.message.id)
}
```

Replace the toolbar block at the end of the template (the `<div v-if="isOwn && !editing" …>` element and its children) with:

```vue
    <div
      v-if="(canEdit || canDelete) && !editing"
      class="absolute right-2 top-0 hidden gap-2 rounded bg-bg-dark px-2 py-1 text-xs text-txt-muted group-hover:flex"
    >
      <button v-if="canEdit" @click="startEdit" class="hover:text-white">Edit</button>
      <button v-if="canDelete" @click="remove" class="hover:text-red-400">Delete</button>
    </div>
```

- [ ] **Step 2: Make MessageInput read-only when posting is not allowed**

Replace `client/src/components/MessageInput.vue` with:

```vue
<script setup lang="ts">
import { computed, ref } from 'vue'
import { useMessagesStore } from '@/stores/messages'
import { useServersStore } from '@/stores/servers'
import { attachmentApi } from '@/api'
import { canPost } from '@/permissions'
import type { Channel, Attachment } from '@/types'

const props = defineProps<{ channel: Channel }>()
const messages = useMessagesStore()
const servers = useServersStore()

// False in channels whose post tier is above the viewer's role (e.g. announcements).
const allowed = computed(() => canPost(servers.myRole, props.channel))
const placeholder = computed(() =>
  allowed.value
    ? `Message #${props.channel.name}`
    : "You don't have permission to post in this channel",
)

const text = ref('')
const pending = ref<Attachment[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
let typingTimer: number | undefined
let isTyping = false

function onInput() {
  if (!isTyping) {
    isTyping = true
    messages.sendTyping(true)
  }
  clearTimeout(typingTimer)
  typingTimer = window.setTimeout(() => {
    isTyping = false
    messages.sendTyping(false)
  }, 2000)
}

async function pickFile(e: Event) {
  const file = (e.target as HTMLInputElement).files?.[0]
  if (!file) return
  try {
    const { data } = await attachmentApi.upload(file)
    pending.value.push(data)
  } catch {
    alert('Upload failed (max 10MB).')
  }
  if (fileInput.value) fileInput.value.value = ''
}

async function send() {
  const content = text.value.trim()
  if (!allowed.value || (!content && pending.value.length === 0)) return
  await messages.send(content, pending.value.map((a) => a.id))
  text.value = ''
  pending.value = []
  isTyping = false
  messages.sendTyping(false)
}
</script>

<template>
  <div class="px-4 pb-4">
    <div v-if="pending.length" class="mb-2 flex gap-2">
      <div
        v-for="a in pending"
        :key="a.id"
        class="flex items-center gap-1 rounded bg-bg-input px-2 py-1 text-xs text-txt"
      >
        📎 {{ a.file_name }}
        <button class="text-red-400" @click="pending = pending.filter((p) => p.id !== a.id)">✕</button>
      </div>
    </div>

    <div :class="['flex items-end gap-2 rounded-lg bg-bg-input px-3 py-2', allowed ? '' : 'opacity-60']">
      <button
        v-if="allowed"
        @click="fileInput?.click()"
        class="text-xl text-txt-muted hover:text-white"
        title="Attach"
      >
        ＋
      </button>
      <input ref="fileInput" type="file" class="hidden" @change="pickFile" />
      <textarea
        v-model="text"
        @input="onInput"
        @keydown.enter.exact.prevent="send"
        :placeholder="placeholder"
        :disabled="!allowed"
        rows="1"
        class="max-h-40 flex-1 resize-none bg-transparent text-txt outline-none placeholder:text-txt-muted disabled:cursor-not-allowed"
      ></textarea>
      <button v-if="allowed" @click="send" class="font-medium text-blurple hover:text-white">Send</button>
    </div>
  </div>
</template>
```

- [ ] **Step 3: React to removals and hidden channels on the dashboard**

In `client/src/views/DashboardView.vue`:

Change the first import line to add `watch`:

```ts
import { onMounted, ref, computed, watch } from 'vue'
```

Replace `selectServer` with:

```ts
async function selectServer(id: string) {
  mode.value = 'server'
  // Clear first so the channel-list watcher ignores the previous server's channel.
  channels.currentChannelId = ''
  await Promise.all([servers.selectServer(id), channels.fetch(id)])
  const first = channels.channels.find((c) => c.type === 'text')
  if (first) openChannel(first.id)
}
```

Add after the `openDm` function (end of the script):

```ts
// The selected server went away (left, kicked, banned, deleted): go home.
watch(
  () => servers.currentServerId,
  (id) => {
    if (!id && mode.value === 'server') goHome()
  },
)

// The channel list was refetched (tier change, deletion, demotion). If the open
// channel is no longer visible, move to the first text channel we can see.
watch(
  () => channels.channels,
  (list) => {
    if (mode.value !== 'server' || !channels.currentChannelId) return
    if (list.some((c) => c.id === channels.currentChannelId)) return
    const first = list.find((c) => c.type === 'text')
    if (first) openChannel(first.id)
    else channels.currentChannelId = ''
  },
)
```

In the template, insert this banner as the first child of `<main class="flex min-w-0 flex-1 flex-col bg-bg">`, directly above `<header …>`:

```vue
      <div
        v-if="servers.notice"
        class="flex items-center justify-between bg-red-500/20 px-4 py-2 text-sm text-red-200"
      >
        <span>{{ servers.notice }}</span>
        <button class="ml-4 text-red-200 hover:text-white" @click="servers.notice = ''">✕</button>
      </div>
```

- [ ] **Step 4: Type-check, test, build**

Run: `cd client && npm run type-check && npx vitest run && npm run build`
Expected: all three succeed. The build prints `✓ built in …`.

- [ ] **Step 5: Commit**

```bash
git add client/src/components/MessageItem.vue client/src/components/MessageInput.vue client/src/views/DashboardView.vue
git commit -m "feat(client): moderator message delete, read-only input, removal handling"
```

---

### Task 17: Docs and end-to-end verification

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document the feature**

In `README.md`, add these lines to the end of the `## Features (MVP + v1.1)` list:

```markdown
- Roles: owner > admin > moderator > member. Moderators kick/ban and delete any
  message; admins manage channels, server settings, and member roles
- Private and read-only channels via per-channel "who can view" / "who can post"
- Leave server; banned users cannot rejoin until unbanned
```

In the `## Architecture` list, add after the `ws` bullet line group (after the "WebSocket owns message persistence…" bullet):

```markdown
- `server/internal/perms` is the single authorization point (role tiers,
  capability matrix, channel access). HTTP handlers and WebSocket events both
  call it; the hub drops users from rooms when they lose access.
```

In the `## Verify` list, replace `- \`cd server && go build ./...\`` with:

```markdown
- `cd server && go build ./... && go test ./...`
- `cd client && npx vitest run`
```

- [ ] **Step 2: Run the full verification**

Run:
```bash
cd server && go vet ./... && go build ./... && go test ./...
cd ../client && npm run type-check && npx vitest run && npm run build
```
Expected: every command exits 0. Go reports `ok` for database, perms, ws, messages, channels, servers. Vitest reports 1 file passed.

- [ ] **Step 3: Manual smoke test with two browsers**

Run `./dev.sh` from the repo root. Open `http://localhost:5173` in two browser profiles (A and B) and register a different user in each.

Check each of these, in order:
1. A creates a server, opens the ▾ menu, and chooses Invite people. B joins with the code. A's member list shows B under **Members** without a reload.
2. B does not see the "+" next to Text Channels. The ▾ menu shows only Invite people and Leave server.
3. A hovers #general, clicks ⚙, and sets "Who can post" to *Moderators and above*. B's input turns disabled with "You don't have permission to post in this channel", without a reload.
4. A creates `#staff` with view *Moderators and above*. B's sidebar never shows it. A opens ⋯ on B and clicks **Make moderator**. `#staff` appears for B within a second.
5. B posts in `#staff`. A opens ⋯ on B and clicks **Make member** while B has `#staff` open. B is moved to the first visible channel, and messages A posts in `#staff` no longer reach B.
6. A clicks ⋯ → **Ban** on B. B's server disappears from the rail and the red banner says "You were banned from …". B cannot rejoin with the code (error shown). A opens Server settings → Bans → Unban. B can now rejoin.
7. B clicks ▾ → Leave server. The server leaves B's rail with no banner, and A's member list drops B.

If any step fails, stop and debug with superpowers:systematic-debugging before continuing.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "docs: document roles, channel tiers, and moderation"
```

---

## Spec coverage map

| Spec requirement | Task |
|---|---|
| Tiers, capability matrix, hierarchy, `CanAssign` | 2 |
| Channel `MinViewRole`/`MinPostRole`, `ServerBan`, migration | 1 |
| `perms` loaders, DM rules, `VisibleChannels` | 3 |
| WS enforcement (join/send/typing/edit/delete), error frames, DM no-override | 5 |
| HTTP messages rules, 404 for hidden | 6 |
| Channel create/update/delete rules, tier validation, DM reject, `channels_changed` | 7 |
| Server get/join/update/delete/invite/regenerate, bans on join, `member_joined`, `server_updated`, `server_deleted` | 8 |
| Leave, kick, set role, `member_removed`, `member_role_updated`, `RecheckRooms` | 4, 9 |
| Ban list/ban/unban | 10 |
| Room eviction on tier change and channel delete | 4, 7 |
| Client permissions helper and types | 11 |
| Client API, stores, event handling, error-frame refetch | 12 |
| Channel tier modal, server settings modal | 13 |
| Sidebar gating, 🔒, server menu, leave | 14 |
| Member list groups, badges, moderation menu | 15 |
| Moderator delete, read-only input, banner, vanished channel | 16 |
| Tests (Go unit/handler/WS, vitest, manual smoke), README | 1–17 |
