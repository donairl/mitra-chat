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
