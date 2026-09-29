package database_test

import (
	"testing"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/testutil"
)

func TestMigrateDefaultsChannelTiersToMember(t *testing.T) {
	testutil.SetupDB(t)
	ch := models.Channel{ID: "c1", Name: "general", ServerID: "s1"}
	if err := database.DB.Create(&ch).Error; err != nil {
		t.Fatal(err)
	}
	var got models.Channel
	database.DB.First(&got, "id = ?", "c1")
	if got.MinViewRole != "member" || got.MinPostRole != "member" {
		t.Fatalf("tiers = %q/%q, want member/member", got.MinViewRole, got.MinPostRole)
	}
}

func TestMigrateCreatesUniqueServerBans(t *testing.T) {
	testutil.SetupDB(t)
	first := models.ServerBan{ID: "b1", ServerID: "s1", UserID: "u1", BannedBy: "u2"}
	if err := database.DB.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	dup := models.ServerBan{ID: "b2", ServerID: "s1", UserID: "u1", BannedBy: "u2"}
	if err := database.DB.Create(&dup).Error; err == nil {
		t.Fatal("duplicate ban for the same server and user was accepted")
	}
}

func TestSeedGuild(t *testing.T) {
	testutil.SetupDB(t)
	g := testutil.SeedGuild(t)
	for name, want := range map[string]string{"owner": "owner", "admin": "admin", "mod": "moderator", "member": "member", "stranger": ""} {
		if got := testutil.RoleOf(t, g.ServerID, g.User(name)); got != want {
			t.Errorf("%s role = %q, want %q", name, got, want)
		}
	}
}
