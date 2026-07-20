package channels

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
)

var validate = validator.New()

// Handler groups channel endpoints.
type Handler struct{ cfg *config.Config }

// New builds a channels handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts channel routes under both server-scoped and flat paths.
func (h *Handler) Register(r fiber.Router) {
	p := middleware.Protected(h.cfg)
	r.Get("/servers/:serverId/channels", p, h.list)
	r.Post("/servers/:serverId/channels", p, h.create)
	r.Put("/channels/:id", p, h.update)
	r.Delete("/channels/:id", p, h.delete)
}

func isMember(serverID, userID string) bool {
	var n int64
	database.DB.Model(&models.ServerMember{}).
		Where("server_id = ? AND user_id = ?", serverID, userID).Count(&n)
	return n > 0
}

func isOwner(serverID, userID string) bool {
	var n int64
	database.DB.Model(&models.Server{}).
		Where("id = ? AND owner_id = ?", serverID, userID).Count(&n)
	return n > 0
}

func (h *Handler) list(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	if !isMember(sid, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var chans []models.Channel
	database.DB.Where("server_id = ?", sid).Order("created_at asc").Find(&chans)
	return utils.OK(c, chans)
}

type channelReq struct {
	Name  string `json:"name" validate:"required,min=1,max=100"`
	Type  string `json:"type" validate:"omitempty,oneof=text voice"`
	Topic string `json:"topic" validate:"max=1024"`
}

func (h *Handler) create(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	if !isMember(sid, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var req channelReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	if req.Type == "" {
		req.Type = "text"
	}
	ch := models.Channel{
		ID: uuid.NewString(), Name: req.Name, Type: req.Type,
		Topic: req.Topic, ServerID: sid,
	}
	if err := database.DB.Create(&ch).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create channel")
	}
	return c.Status(fiber.StatusCreated).JSON(ch)
}

func (h *Handler) update(c *fiber.Ctx) error {
	var ch models.Channel
	if err := database.DB.First(&ch, "id = ?", c.Params("id")).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "channel not found")
	}
	if !isOwner(ch.ServerID, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "owner only")
	}
	var req channelReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	database.DB.Model(&ch).Updates(map[string]any{"name": req.Name, "topic": req.Topic})
	return utils.OK(c, ch)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	var ch models.Channel
	if err := database.DB.First(&ch, "id = ?", c.Params("id")).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "channel not found")
	}
	if !isOwner(ch.ServerID, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "owner only")
	}
	database.DB.Where("channel_id = ?", ch.ID).Delete(&models.Message{})
	database.DB.Delete(&ch)
	return utils.OK(c, fiber.Map{"message": "channel deleted"})
}
