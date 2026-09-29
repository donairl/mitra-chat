package servers_test

import (
	"testing"

	"mitrachat/server/internal/testutil"
	"mitrachat/server/internal/ws"
)

// wantRemoved checks that c is out of rooms and was told userID left serverID
// for reason.
func wantRemoved(t *testing.T, who string, c *ws.Client, rooms []string, serverID, userID, reason string) {
	t.Helper()
	for _, ch := range rooms {
		if c.InRoomForTest(ch) {
			t.Errorf("%s is still in room %s", who, ch)
		}
	}
	ev := c.FramesForTest()["member_removed"]
	if len(ev) != 1 || ev[0]["server_id"] != serverID || ev[0]["user_id"] != userID || ev[0]["reason"] != reason {
		t.Errorf("%s's member_removed events = %v, want one with user %s and reason %q", who, ev, userID, reason)
	}
}

func TestRemovalEvictsRoomsAndNotifies(t *testing.T) {
	cases := []struct {
		name, actor, method, reason string
		path                        func(g testutil.Guild) string
		body                        func(g testutil.Guild) any
		status                      int
	}{
		{"kick", "mod", "DELETE", "kick",
			func(g testutil.Guild) string { return "/members/" + g.Member }, func(testutil.Guild) any { return nil }, 200},
		{"ban", "mod", "POST", "ban",
			func(testutil.Guild) string { return "/bans" }, func(g testutil.Guild) any { return map[string]any{"user_id": g.Member} }, 201},
		{"leave", "member", "DELETE", "leave",
			func(testutil.Guild) string { return "/members/me" }, func(testutil.Guild) any { return nil }, 200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.SetupDB(t)
			g := testutil.SeedGuild(t)
			// The removed user has two connections; a bystander member and an
			// outsider are connected too.
			tabA := clientIn(t, g.Member, g.Open, g.Announce)
			tabB := clientIn(t, g.Member, g.Open)
			bystander := clientIn(t, g.Mod, g.Open)
			outsider := clientIn(t, g.Stranger)

			status, body := testutil.Do(t, newApp(), c.method, "/api/servers/"+g.ServerID+c.path(g), testutil.Token(t, g.User(c.actor)), c.body(g))

			if status != c.status {
				t.Fatalf("status %d (%s), want %d", status, body, c.status)
			}
			wantRemoved(t, "first connection", tabA, []string{g.Open, g.Announce}, g.ServerID, g.Member, c.reason)
			wantRemoved(t, "second connection", tabB, []string{g.Open}, g.ServerID, g.Member, c.reason)
			if !bystander.InRoomForTest(g.Open) {
				t.Error("an unrelated member was evicted")
			}
			if ev := bystander.FramesForTest()["member_removed"]; len(ev) != 1 || ev[0]["reason"] != c.reason {
				t.Errorf("bystander's member_removed events = %v", ev)
			}
			if n := len(outsider.FramesForTest()["member_removed"]); n != 0 {
				t.Errorf("non-member got %d member_removed events", n)
			}
		})
	}
}

func TestSetRoleDemotionEvictsFromRoomsAboveNewTier(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	admin := clientIn(t, g.Admin, g.Open, g.Staff, g.AdminOnly)
	mod := clientIn(t, g.Mod, g.Staff)
	path := "/api/servers/" + g.ServerID + "/members/" + g.Admin + "/role"

	if status, body := testutil.Do(t, newApp(), "PUT", path, testutil.Token(t, g.Owner), map[string]any{"role": "moderator"}); status != 200 {
		t.Fatalf("status %d (%s)", status, body)
	}

	if admin.InRoomForTest(g.AdminOnly) {
		t.Error("demoted admin is still in the admin-only room")
	}
	if !admin.InRoomForTest(g.Open) || !admin.InRoomForTest(g.Staff) {
		t.Error("demoted admin lost rooms a moderator may view")
	}
	for name, c := range map[string]*ws.Client{"admin": admin, "mod": mod} {
		ev := c.FramesForTest()["member_role_updated"]
		if len(ev) != 1 || ev[0]["user_id"] != g.Admin || ev[0]["role"] != "moderator" {
			t.Errorf("%s's member_role_updated events = %v", name, ev)
		}
	}
}

func TestSetRolePromotionKeepsRooms(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	member := clientIn(t, g.Member, g.Open, g.Announce)
	path := "/api/servers/" + g.ServerID + "/members/" + g.Member + "/role"

	if status, body := testutil.Do(t, newApp(), "PUT", path, testutil.Token(t, g.Admin), map[string]any{"role": "moderator"}); status != 200 {
		t.Fatalf("status %d (%s)", status, body)
	}

	if !member.InRoomForTest(g.Open) || !member.InRoomForTest(g.Announce) {
		t.Fatal("promotion evicted the member from rooms they can still view")
	}
}

func TestPeersCannotActOnEachOther(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	mod2, admin2 := testutil.SeedUser(t, "mod2"), testutil.SeedUser(t, "admin2")
	testutil.AddMember(t, g.ServerID, mod2, "moderator")
	testutil.AddMember(t, g.ServerID, admin2, "admin")
	app := newApp()
	base := "/api/servers/" + g.ServerID

	// mod on mod
	if status, _ := testutil.Do(t, app, "DELETE", base+"/members/"+mod2, testutil.Token(t, g.Mod), nil); status != 403 {
		t.Errorf("mod kicks mod: %d, want 403", status)
	}
	if status, _ := testutil.Do(t, app, "POST", base+"/bans", testutil.Token(t, g.Mod), map[string]any{"user_id": mod2}); status != 403 {
		t.Errorf("mod bans mod: %d, want 403", status)
	}
	// admin on admin
	if status, _ := testutil.Do(t, app, "DELETE", base+"/members/"+admin2, testutil.Token(t, g.Admin), nil); status != 403 {
		t.Errorf("admin kicks admin: %d, want 403", status)
	}
	if status, _ := testutil.Do(t, app, "POST", base+"/bans", testutil.Token(t, g.Admin), map[string]any{"user_id": admin2}); status != 403 {
		t.Errorf("admin bans admin: %d, want 403", status)
	}
	if status, _ := testutil.Do(t, app, "PUT", base+"/members/"+admin2+"/role", testutil.Token(t, g.Admin), map[string]any{"role": "member"}); status != 403 {
		t.Errorf("admin demotes admin: %d, want 403", status)
	}
	for _, id := range []string{mod2, admin2} {
		if testutil.RoleOf(t, g.ServerID, id) == "" {
			t.Errorf("peer %s was removed", id)
		}
	}
}

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
		{"stranger cannot kick", "stranger", "member", 403},
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
		{"cannot set own role", "admin", "admin", "member", 400},
		{"stranger cannot set roles", "stranger", "member", "moderator", 403},
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
