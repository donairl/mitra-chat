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
	res := database.DB.Where("server_id = ? AND user_id = ?", serverID, userID).Limit(1).Find(&m)
	if res.Error != nil {
		t.Fatalf("RoleOf: %v", res.Error)
	}
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
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	must(t, err)
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
