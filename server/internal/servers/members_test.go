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
