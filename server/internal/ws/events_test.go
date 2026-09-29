package ws

import (
	"testing"

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
