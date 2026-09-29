package servers_test

import (
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

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

func TestJoinBanLookupErrorIsNotAJoin(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	database.DB.Create(&models.ServerBan{ID: uuid.NewString(), ServerID: g.ServerID, UserID: g.Stranger, BannedBy: g.Owner})
	testutil.FailOn("query", "server_bans", nil)
	body := map[string]any{"invite_code": inviteCodeOf(t, g.ServerID)}

	if status, resp := testutil.Do(t, newApp(), "POST", "/api/servers/join", testutil.Token(t, g.Stranger), body); status != 500 {
		t.Fatalf("status %d (%s), want 500", status, resp)
	}
	if role := testutil.RoleOf(t, g.ServerID, g.Stranger); role != "" {
		t.Fatalf("banned user became %q when the ban lookup failed", role)
	}
}

func TestUpdateServerWriteErrorIs500(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	testutil.FailOn("update", "servers", nil)

	status, resp := testutil.Do(t, newApp(), "PUT", "/api/servers/"+g.ServerID, testutil.Token(t, g.Admin), map[string]any{"name": "renamed"})

	if status != 500 {
		t.Fatalf("status %d (%s), want 500", status, resp)
	}
	var s models.Server
	database.DB.First(&s, "id = ?", g.ServerID)
	if s.Name == "renamed" {
		t.Fatal("name changed although the write failed")
	}
}

func TestInviteWriteErrorIs500(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	database.DB.Model(&models.Server{}).Where("id = ?", g.ServerID).Update("invite_code", "")
	testutil.FailOn("update", "servers", nil)

	status, resp := testutil.Do(t, newApp(), "POST", "/api/servers/"+g.ServerID+"/invite", testutil.Token(t, g.Member), nil)

	if status != 500 {
		t.Fatalf("status %d (%s), want 500", status, resp)
	}
}

func TestDeleteServerFailureLeavesEverythingIntact(t *testing.T) {
	cases := []struct {
		name string
		fail func()
	}{
		{"member lookup fails", func() {
			testutil.FailOn("query", "server_members", func(tx *gorm.DB) bool {
				_, isPluck := tx.Statement.Dest.(*[]string)
				return isPluck
			})
		}},
		{"channel lookup fails", func() {
			testutil.FailOn("query", "channels", func(tx *gorm.DB) bool {
				_, isPluck := tx.Statement.Dest.(*[]string)
				return isPluck
			})
		}},
		{"server row delete fails", func() { testutil.FailOn("delete", "servers", nil) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.SetupDB(t)
			g := testutil.SeedGuild(t)
			testutil.AddMessage(t, g.Open, g.Member, "hi")
			c.fail()

			status, resp := testutil.Do(t, newApp(), "DELETE", "/api/servers/"+g.ServerID, testutil.Token(t, g.Owner), nil)

			if status != 500 {
				t.Fatalf("status %d (%s), want 500", status, resp)
			}
			var servers, chans, members, msgs int64
			database.DB.Model(&models.Server{}).Where("id = ?", g.ServerID).Count(&servers)
			database.DB.Model(&models.Channel{}).Where("server_id = ?", g.ServerID).Count(&chans)
			database.DB.Model(&models.ServerMember{}).Where("server_id = ?", g.ServerID).Count(&members)
			database.DB.Model(&models.Message{}).Where("channel_id = ?", g.Open).Count(&msgs)
			if servers != 1 || chans != 4 || members != 4 || msgs != 1 {
				t.Fatalf("after failed delete: %d servers, %d channels, %d members, %d messages", servers, chans, members, msgs)
			}
		})
	}
}
