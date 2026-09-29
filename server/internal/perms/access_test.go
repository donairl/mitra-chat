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

func TestChannelAccessUnknownTierFailsClosed(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	cases := []struct {
		name, column string
		user         string
		view, post   bool
	}{
		{"unknown view tier hides from member", "min_view_role", g.Member, false, false},
		{"unknown view tier hides from mod", "min_view_role", g.Mod, false, false},
		{"unknown view tier admits admin", "min_view_role", g.Admin, true, true},
		{"unknown view tier admits owner", "min_view_role", g.Owner, true, true},
		{"unknown post tier is read-only for member", "min_post_role", g.Member, true, false},
		{"unknown post tier is read-only for mod", "min_post_role", g.Mod, true, false},
		{"unknown post tier lets admin post", "min_post_role", g.Admin, true, true},
	}
	for _, c := range cases {
		ch := testutil.AddChannel(t, g.ServerID, "member", "member")
		if err := database.DB.Model(&models.Channel{}).Where("id = ?", ch).Update(c.column, "vip").Error; err != nil {
			t.Fatal(err)
		}
		a := ChannelAccess(ch, c.user)
		if !a.Exists || a.CanView != c.view || a.CanPost != c.post {
			t.Errorf("%s: got %+v, want view=%v post=%v", c.name, a, c.view, c.post)
		}
	}
}
