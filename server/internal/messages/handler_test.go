package messages_test

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/messages"
	"mitrachat/server/internal/models"
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

func TestHistoryBeforeCursorIsScopedToChannel(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	app := newApp()
	now := time.Now()
	at := func(id string, ago time.Duration) string {
		database.DB.Model(&models.Message{}).Where("id = ?", id).Update("created_at", now.Add(-ago))
		return id
	}
	oldest := at(testutil.AddMessage(t, g.Open, g.Member, "oldest"), 3*time.Hour)
	newest := at(testutil.AddMessage(t, g.Open, g.Member, "newest"), time.Hour)
	hidden := at(testutil.AddMessage(t, g.Staff, g.Mod, "staff only"), 2*time.Hour)
	history := func(before string) []string {
		status, body := testutil.Do(t, app, "GET", "/api/channels/"+g.Open+"/messages?before="+before, testutil.Token(t, g.Member), nil)
		if status != 200 {
			t.Fatalf("status %d (%s)", status, body)
		}
		var msgs []models.Message
		testutil.Decode(t, body, &msgs)
		ids := make([]string, len(msgs))
		for i, m := range msgs {
			ids[i] = m.ID
		}
		return ids
	}

	// A cursor from the same channel pages back from that message.
	testutil.SameIDs(t, history(newest), []string{oldest})
	// A cursor from another channel is not a valid cursor here, so it must not
	// leak that message's timestamp: it is ignored like an unknown id.
	testutil.SameIDs(t, history(hidden), []string{oldest, newest})
	testutil.SameIDs(t, history("no-such-message"), []string{oldest, newest})
}
