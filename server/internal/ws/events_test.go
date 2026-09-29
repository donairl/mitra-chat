package ws

import (
	"strings"
	"testing"

	"gorm.io/gorm"

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

func TestJoinRoomRevokedAfterCheckIsEvicted(t *testing.T) {
	g := setupWS(t)
	listener, joiner := newTestClient(g.Owner), newTestClient(g.Admin)
	H.joinRoom(listener, g.AdminOnly)

	// Demote the joiner right after the first access check has read their
	// role, i.e. between the check and the join. The revoke's RecheckRooms
	// would have missed a client that was not in the room yet.
	fired := false
	database.DB.Callback().Query().After("gorm:query").Register("test:demote_after_check", func(tx *gorm.DB) {
		if fired || tx.Statement.Table != "server_members" {
			return
		}
		fired = true
		tx.Session(&gorm.Session{NewDB: true}).Model(&models.ServerMember{}).
			Where("server_id = ? AND user_id = ?", g.ServerID, g.Admin).Update("role", "member")
	})

	dispatch(joiner, "join_room", map[string]string{"channel_id": g.AdminOnly})

	if !fired {
		t.Fatal("test hook never ran")
	}
	if joiner.rooms[g.AdminOnly] || H.rooms[g.AdminOnly][joiner] {
		t.Fatal("demoted user stayed in the admin-only room")
	}
	frames := drain(joiner)
	if p := firstOf(frames, "error"); p == nil || p["code"] != "not_found" || p["channel_id"] != g.AdminOnly {
		t.Fatalf("error frame = %v", p)
	}
	if firstOf(drain(listener), "user_joined") != nil {
		t.Fatal("user_joined was broadcast for a user who lost access")
	}
}

func TestJoinRoomBroadcastsUserJoined(t *testing.T) {
	g := setupWS(t)
	listener, joiner := newTestClient(g.Mod), newTestClient(g.Member)
	H.joinRoom(listener, g.Open)

	dispatch(joiner, "join_room", map[string]string{"channel_id": g.Open})

	p := firstOf(drain(listener), "user_joined")
	if p == nil || p["user_id"] != g.Member {
		t.Fatalf("user_joined = %v", p)
	}
}

func TestLeaveRoomBroadcastsOnlyWhenInRoom(t *testing.T) {
	g := setupWS(t)
	listener, leaver := newTestClient(g.Mod), newTestClient(g.Member)
	H.joinRoom(listener, g.Open)

	dispatch(leaver, "leave_room", map[string]string{"channel_id": g.Open})
	if firstOf(drain(listener), "user_left") != nil {
		t.Fatal("user_left was broadcast for a client that was never in the room")
	}

	H.joinRoom(leaver, g.Open)
	dispatch(leaver, "leave_room", map[string]string{"channel_id": g.Open})
	if p := firstOf(drain(listener), "user_left"); p == nil || p["user_id"] != g.Member {
		t.Fatalf("user_left = %v", p)
	}
	if leaver.rooms[g.Open] {
		t.Fatal("leaver is still in the room")
	}
}

func TestSendMessageLengthLimit(t *testing.T) {
	g := setupWS(t)
	c := newTestClient(g.Member)

	// The limit counts characters, like the HTTP validator: 4000 two-byte
	// runes are accepted, 4001 are not.
	dispatch(c, "send_message", map[string]any{"channel_id": g.Open, "content": strings.Repeat("é", 4000)})
	if n := countMessages(g.Open); n != 1 {
		t.Fatalf("messages after a 4000-character send = %d, want 1", n)
	}
	drain(c)

	dispatch(c, "send_message", map[string]any{"channel_id": g.Open, "content": strings.Repeat("é", 4001)})
	if n := countMessages(g.Open); n != 1 {
		t.Fatalf("messages after a 4001-character send = %d, want 1", n)
	}
	expectError(t, c, "bad_request")
}

func TestEditMessageValidatesContent(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Open, g.Member, "old")
	c := newTestClient(g.Member)
	cases := map[string]string{"empty": "", "too long": strings.Repeat("x", 4001)}
	for name, content := range cases {
		dispatch(c, "edit_message", map[string]string{"message_id": msg, "content": content})
		var m models.Message
		database.DB.First(&m, "id = ?", msg)
		if m.Content != "old" {
			t.Fatalf("%s edit changed the message to %d characters", name, len(m.Content))
		}
		expectError(t, c, "bad_request")
	}

	dispatch(c, "edit_message", map[string]string{"message_id": msg, "content": strings.Repeat("x", 4000)})
	var m models.Message
	database.DB.First(&m, "id = ?", msg)
	if len(m.Content) != 4000 {
		t.Fatalf("a 4000-character edit was refused (content length %d)", len(m.Content))
	}
}

func TestEditMessageDBErrorSkipsBroadcast(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Open, g.Member, "old")
	author, listener := newTestClient(g.Member), newTestClient(g.Mod)
	H.joinRoom(listener, g.Open)
	testutil.FailOn("update", "messages", nil)

	dispatch(author, "edit_message", map[string]string{"message_id": msg, "content": "new"})

	if firstOf(drain(listener), "message_edited") != nil {
		t.Fatal("message_edited was broadcast although the update failed")
	}
	expectError(t, author, "internal_error")
}

func TestDeleteMessageDBErrorSkipsBroadcast(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Open, g.Member, "keep")
	author, listener := newTestClient(g.Member), newTestClient(g.Mod)
	H.joinRoom(listener, g.Open)
	testutil.FailOn("delete", "messages", nil)

	dispatch(author, "delete_message", map[string]string{"message_id": msg})

	if firstOf(drain(listener), "message_deleted") != nil {
		t.Fatal("message_deleted was broadcast although the delete failed")
	}
	if n := countMessages(g.Open); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
	expectError(t, author, "internal_error")
}

func TestEditAndDeleteHiddenChannelMessageIsNotFound(t *testing.T) {
	g := setupWS(t)
	msg := testutil.AddMessage(t, g.Staff, g.Mod, "staff only")
	c := newTestClient(g.Member)

	dispatch(c, "edit_message", map[string]string{"message_id": msg, "content": "hijack"})
	expectError(t, c, "not_found")
	dispatch(c, "delete_message", map[string]string{"message_id": msg})
	expectError(t, c, "not_found")

	var m models.Message
	database.DB.First(&m, "id = ?", msg)
	if m.Content != "staff only" {
		t.Fatal("a member changed a message in a channel they cannot see")
	}
}
