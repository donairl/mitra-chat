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
	var staff models.Channel
	database.DB.First(&staff, "id = ?", g.Staff)
	if staff.Name != "staff-renamed" || staff.MinViewRole != "moderator" {
		t.Errorf("after rename: name=%q view=%q", staff.Name, staff.MinViewRole)
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
