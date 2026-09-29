package perms

import (
	"slices"
	"testing"
)

var allTiers = []Tier{Member, Moderator, Admin, Owner}

func TestParseTier(t *testing.T) {
	cases := []struct {
		in   string
		want Tier
		ok   bool
	}{
		{"member", Member, true},
		{"moderator", Moderator, true},
		{"admin", Admin, true},
		{"owner", Owner, true},
		{"", Member, false},
		{"superuser", Member, false},
	}
	for _, c := range cases {
		got, ok := ParseTier(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseTier(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestTierStringRoundTrips(t *testing.T) {
	for _, tier := range allTiers {
		if got, ok := ParseTier(tier.String()); !ok || got != tier {
			t.Errorf("ParseTier(%q) = %v, %v", tier.String(), got, ok)
		}
	}
}

func TestCanMatchesSpecMatrix(t *testing.T) {
	// Lowest tier holding each capability, from the design spec.
	minimum := map[Capability]Tier{
		CapKick:             Moderator,
		CapBan:              Moderator,
		CapDeleteAnyMessage: Moderator,
		CapManageChannels:   Admin,
		CapManageServer:     Admin,
		CapManageRoles:      Admin,
		CapDeleteServer:     Owner,
	}
	for c, lowest := range minimum {
		for _, tier := range allTiers {
			if got := Can(tier, c); got != (tier >= lowest) {
				t.Errorf("Can(%v, %d) = %v, want %v", tier, c, got, tier >= lowest)
			}
		}
	}
}

func TestCanActOn(t *testing.T) {
	cases := []struct {
		actor, target Tier
		want          bool
	}{
		{Moderator, Member, true},
		{Moderator, Moderator, false},
		{Moderator, Admin, false},
		{Admin, Moderator, true},
		{Admin, Owner, false},
		{Owner, Admin, true},
		{Member, Member, false},
	}
	for _, c := range cases {
		if got := CanActOn(c.actor, c.target); got != c.want {
			t.Errorf("CanActOn(%v, %v) = %v, want %v", c.actor, c.target, got, c.want)
		}
	}
}

func TestCanAssign(t *testing.T) {
	cases := []struct {
		actor, role Tier
		want        bool
	}{
		{Admin, Member, true},
		{Admin, Moderator, true},
		{Admin, Admin, false},
		{Owner, Admin, true},
		{Owner, Owner, false}, // owner is never assignable
		{Moderator, Member, true}, // rank rule only; handlers also require CapManageRoles
	}
	for _, c := range cases {
		if got := CanAssign(c.actor, c.role); got != c.want {
			t.Errorf("CanAssign(%v, %v) = %v, want %v", c.actor, c.role, got, c.want)
		}
	}
}

func TestValidChannelTier(t *testing.T) {
	for _, tier := range allTiers {
		if got, want := ValidChannelTier(tier), tier != Owner; got != want {
			t.Errorf("ValidChannelTier(%v) = %v, want %v", tier, got, want)
		}
	}
}

func TestTiersUpTo(t *testing.T) {
	if got := TiersUpTo(Moderator); !slices.Equal(got, []string{"member", "moderator"}) {
		t.Errorf("TiersUpTo(Moderator) = %v", got)
	}
	if got := TiersUpTo(Owner); !slices.Equal(got, []string{"member", "moderator", "admin", "owner"}) {
		t.Errorf("TiersUpTo(Owner) = %v", got)
	}
}
