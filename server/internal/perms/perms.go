// Package perms is the single place that decides who may do what in a server.
// HTTP handlers and websocket events both call it so the two paths stay in sync.
package perms

// Tier is a member's rank inside a server. A higher tier holds every
// capability of the tiers below it.
type Tier int

const (
	Member Tier = iota
	Moderator
	Admin
	Owner
)

// tierNames are the role strings stored in ServerMember.Role, indexed by Tier.
var tierNames = [...]string{"member", "moderator", "admin", "owner"}

// String returns the stored role name for t.
func (t Tier) String() string {
	if t < Member || t > Owner {
		return "unknown"
	}
	return tierNames[t]
}

// ParseTier maps a stored role string to its tier. ok is false for unknown roles.
func ParseTier(role string) (Tier, bool) {
	for i, name := range tierNames {
		if name == role {
			return Tier(i), true
		}
	}
	return Member, false
}

// Capability is a server-wide action gated by tier.
type Capability int

const (
	CapKick Capability = iota
	CapBan
	CapDeleteAnyMessage
	CapManageChannels
	CapManageServer
	CapManageRoles
	CapDeleteServer
)

// minTier is the lowest tier holding each capability. Keep in sync with
// client/src/permissions.ts.
var minTier = map[Capability]Tier{
	CapKick:             Moderator,
	CapBan:              Moderator,
	CapDeleteAnyMessage: Moderator,
	CapManageChannels:   Admin,
	CapManageServer:     Admin,
	CapManageRoles:      Admin,
	CapDeleteServer:     Owner,
}

// Can reports whether tier t holds capability c.
func Can(t Tier, c Capability) bool {
	need, ok := minTier[c]
	return ok && t >= need
}

// CanActOn reports whether an actor may kick, ban, or re-role a target:
// only members of strictly lower rank.
func CanActOn(actor, target Tier) bool { return actor > target }

// CanAssign reports whether actor may give someone role r: roles ranked below
// the actor only, and never owner.
func CanAssign(actor, r Tier) bool { return r < actor && r != Owner }

// ValidChannelTier reports whether t may be a channel's view or post tier.
// Admin is the highest so owners and admins always see every channel.
func ValidChannelTier(t Tier) bool { return t >= Member && t <= Admin }

// TiersUpTo returns the role names ranked at or below t, lowest first, for
// SQL IN filters.
func TiersUpTo(t Tier) []string {
	var names []string
	for i := Member; i <= t && i <= Owner; i++ {
		names = append(names, i.String())
	}
	return names
}
