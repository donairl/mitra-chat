package messages

import (
	"errors"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

var validate = validator.New()

// Handler groups message endpoints.
type Handler struct{ cfg *config.Config }

// New builds a messages handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts message routes (all protected).
func (h *Handler) Register(r fiber.Router) {
	p := middleware.Protected(h.cfg)
	r.Get("/channels/:channelId/messages", p, h.history)
	r.Post("/messages", p, h.send)
	r.Put("/messages/:id", p, h.edit)
	r.Delete("/messages/:id", p, h.delete)
}

// canAccessChannel checks the user is a member of the channel's server.
func canAccessChannel(channelID, userID string) bool {
	var n int64
	database.DB.Model(&models.Channel{}).
		Joins("JOIN server_members sm ON sm.server_id = channels.server_id").
		Where("channels.id = ? AND sm.user_id = ?", channelID, userID).Count(&n)
	return n > 0
}

func (h *Handler) history(c *fiber.Ctx) error {
	cid := c.Params("channelId")
	if !canAccessChannel(cid, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "no access to channel")
	}
	page := utils.ParsePagination(c)
	q := database.DB.Preload("User").Preload("Attachments").
		Where("channel_id = ?", cid)
	if page.Before != "" {
		var cursor models.Message
		if database.DB.Select("created_at").First(&cursor, "id = ?", page.Before).Error == nil {
			q = q.Where("created_at < ?", cursor.CreatedAt)
		}
	}
	var msgs []models.Message
	q.Order("created_at desc").Limit(page.Limit).Find(&msgs)
	// return ascending (oldest first) for display
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return utils.OK(c, msgs)
}

type sendReq struct {
	ChannelID     string   `json:"channel_id" validate:"required"`
	Content       string   `json:"content" validate:"max=4000"`
	AttachmentIDs []string `json:"attachment_ids"`
}

func (h *Handler) send(c *fiber.Ctx) error {
	var req sendReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	if !canAccessChannel(req.ChannelID, uid) {
		return utils.Error(c, fiber.StatusForbidden, "no access to channel")
	}
	if req.Content == "" && len(req.AttachmentIDs) == 0 {
		return utils.Error(c, fiber.StatusBadRequest, "empty message")
	}
	msg, err := ws.CreateAndBroadcast(uid, req.ChannelID, req.Content, req.AttachmentIDs)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not send message")
	}
	return c.Status(fiber.StatusCreated).JSON(msg)
}

type editReq struct {
	Content string `json:"content" validate:"required,max=4000"`
}

func (h *Handler) edit(c *fiber.Ctx) error {
	var req editReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	msg, err := ws.EditAndBroadcast(middleware.UserID(c), c.Params("id"), req.Content)
	if err != nil {
		if errors.Is(err, ws.ErrForbidden) {
			return utils.Error(c, fiber.StatusForbidden, "not your message")
		}
		return utils.Error(c, fiber.StatusNotFound, "message not found")
	}
	return utils.OK(c, msg)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	err := ws.DeleteAndBroadcast(middleware.UserID(c), c.Params("id"))
	if err != nil {
		if errors.Is(err, ws.ErrForbidden) {
			return utils.Error(c, fiber.StatusForbidden, "not your message")
		}
		return utils.Error(c, fiber.StatusNotFound, "message not found")
	}
	return utils.OK(c, fiber.Map{"message": "deleted"})
}
