package servers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
)

type banReq struct {
	UserID string `json:"user_id" validate:"required"`
}

func (h *Handler) listBans(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapBan); status != 0 {
		return utils.Error(c, status, msg)
	}
	var bans []models.ServerBan
	database.DB.Preload("User").Where("server_id = ?", id).Order("created_at desc").Find(&bans)
	return utils.OK(c, bans)
}

// ban removes a lower-ranked member and blocks them from rejoining by invite.
func (h *Handler) ban(c *fiber.Ctx) error {
	var req banReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	a, status, msg := resolveAction(c, perms.CapBan, req.UserID)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	ban := models.ServerBan{
		ID: uuid.NewString(), ServerID: a.serverID, UserID: a.targetID,
		BannedBy: a.actorID, CreatedAt: time.Now(),
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("server_id = ? AND user_id = ?", a.serverID, a.targetID).
			Delete(&models.ServerMember{}).Error; err != nil {
			return err
		}
		return tx.Create(&ban).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not ban member")
	}
	notifyRemoved(a.serverID, a.targetID, "ban")
	return c.Status(fiber.StatusCreated).JSON(ban)
}

func (h *Handler) unban(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapBan); status != 0 {
		return utils.Error(c, status, msg)
	}
	res := database.DB.Where("server_id = ? AND user_id = ?", id, c.Params("userId")).Delete(&models.ServerBan{})
	if res.Error != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not unban")
	}
	if res.RowsAffected == 0 {
		return utils.Error(c, fiber.StatusNotFound, "ban not found")
	}
	return utils.OK(c, fiber.Map{"message": "unbanned"})
}
