package servers

import (
	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

// action is a resolved moderation or role change: who acts on whom, and their tiers.
type action struct {
	serverID, actorID, targetID string
	actor, target               perms.Tier
}

// resolveAction checks that the caller holds need in the :id server and
// strictly outranks targetID. It returns a non-zero HTTP status and message
// when the action is refused.
func resolveAction(c *fiber.Ctx, need perms.Capability, targetID string) (action, int, string) {
	a := action{serverID: c.Params("id"), actorID: middleware.UserID(c), targetID: targetID}
	var status int
	var msg string
	if a.actor, status, msg = require(a.serverID, a.actorID, need); status != 0 {
		return a, status, msg
	}
	if targetID == a.actorID {
		return a, fiber.StatusBadRequest, "cannot target yourself"
	}
	var ok bool
	if a.target, ok = perms.MemberTier(a.serverID, targetID); !ok {
		return a, fiber.StatusNotFound, "member not found"
	}
	if !perms.CanActOn(a.actor, a.target) {
		return a, fiber.StatusForbidden, "cannot act on equal or higher role"
	}
	return a, 0, ""
}

func deleteMembership(serverID, userID string) error {
	return database.DB.Where("server_id = ? AND user_id = ?", serverID, userID).
		Delete(&models.ServerMember{}).Error
}

// notifyRemoved drops a removed user from the server's rooms and tells the
// remaining members and the removed user (who is no longer a member).
func notifyRemoved(serverID, userID, reason string) {
	ws.H.RecheckRooms(userID)
	ev := ws.Event("member_removed", fiber.Map{"server_id": serverID, "user_id": userID, "reason": reason})
	ws.SendToServerMembers(serverID, ev)
	ws.H.SendToUser(userID, ev)
}

// leave removes the caller from the server. The owner must delete it instead.
func (h *Handler) leave(c *fiber.Ctx) error {
	id, uid := c.Params("id"), middleware.UserID(c)
	t, ok := perms.MemberTier(id, uid)
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	if t == perms.Owner {
		return utils.Error(c, fiber.StatusBadRequest, "owner must delete the server instead")
	}
	if err := deleteMembership(id, uid); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not leave server")
	}
	notifyRemoved(id, uid, "leave")
	return utils.OK(c, fiber.Map{"message": "left server"})
}

func (h *Handler) kick(c *fiber.Ctx) error {
	a, status, msg := resolveAction(c, perms.CapKick, c.Params("userId"))
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	if err := deleteMembership(a.serverID, a.targetID); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not kick member")
	}
	notifyRemoved(a.serverID, a.targetID, "kick")
	return utils.OK(c, fiber.Map{"message": "member kicked"})
}

type roleReq struct {
	Role string `json:"role"`
}

// setRole changes a lower-ranked member's role to one below the caller's own.
func (h *Handler) setRole(c *fiber.Ctx) error {
	a, status, msg := resolveAction(c, perms.CapManageRoles, c.Params("userId"))
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	var req roleReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	role, ok := perms.ParseTier(req.Role)
	if !ok {
		return utils.Error(c, fiber.StatusBadRequest, "invalid role")
	}
	if !perms.CanAssign(a.actor, role) {
		return utils.Error(c, fiber.StatusForbidden, "cannot act on equal or higher role")
	}
	if err := database.DB.Model(&models.ServerMember{}).
		Where("server_id = ? AND user_id = ?", a.serverID, a.targetID).
		Update("role", role.String()).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not update role")
	}
	ws.H.RecheckRooms(a.targetID)
	payload := fiber.Map{"server_id": a.serverID, "user_id": a.targetID, "role": role.String()}
	ws.SendToServerMembers(a.serverID, ws.Event("member_role_updated", payload))
	return utils.OK(c, payload)
}
