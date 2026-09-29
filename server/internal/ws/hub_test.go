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
