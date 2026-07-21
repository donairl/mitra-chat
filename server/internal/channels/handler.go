package channels

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"

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
	r.Get("/channels/dm", p, h.dmList)
	r.Post("/channels/dm", p, h.dmOpen)
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

// dmChannelResp is a DM channel plus the other participant, for client display.
type dmChannelResp struct {
	models.Channel
	DMUser *models.User `json:"dm_user,omitempty"`
}

type dmReq struct {
	UserID string `json:"user_id" validate:"required"`
}

// dmOpen returns the existing 1:1 DM channel between the caller and the target
// user, creating it (and both memberships) if it does not exist yet.
func (h *Handler) dmOpen(c *fiber.Ctx) error {
	var req dmReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	if req.UserID == uid {
		return utils.Error(c, fiber.StatusBadRequest, "cannot DM yourself")
	}
	var other models.User
	if err := database.DB.First(&other, "id = ?", req.UserID).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "user not found")
	}

	var existing models.Channel
	err := database.DB.
		Joins("JOIN channel_members cm1 ON cm1.channel_id = channels.id AND cm1.user_id = ?", uid).
		Joins("JOIN channel_members cm2 ON cm2.channel_id = channels.id AND cm2.user_id = ?", req.UserID).
		Where("channels.type = ?", "dm").
		First(&existing).Error
	if err == nil {
		return utils.OK(c, dmChannelResp{Channel: existing, DMUser: &other})
	}

	ch := models.Channel{ID: uuid.NewString(), Name: "dm", Type: "dm"}
	txErr := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&ch).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ChannelMember{ID: uuid.NewString(), ChannelID: ch.ID, UserID: uid}).Error; err != nil {
			return err
		}
		return tx.Create(&models.ChannelMember{ID: uuid.NewString(), ChannelID: ch.ID, UserID: req.UserID}).Error
	})
	if txErr != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create dm")
	}
	return c.Status(fiber.StatusCreated).JSON(dmChannelResp{Channel: ch, DMUser: &other})
}

// dmList returns the caller's DM channels, each with the other participant.
func (h *Handler) dmList(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var chans []models.Channel
	database.DB.
		Joins("JOIN channel_members cm ON cm.channel_id = channels.id AND cm.user_id = ?", uid).
		Where("channels.type = ?", "dm").
		Order("channels.updated_at desc").
		Find(&chans)

	resp := make([]dmChannelResp, 0, len(chans))
	for _, ch := range chans {
		var other models.User
		if err := database.DB.
			Joins("JOIN channel_members cm ON cm.user_id = users.id").
			Where("cm.channel_id = ? AND users.id <> ?", ch.ID, uid).
			First(&other).Error; err != nil {
			continue
		}
		o := other
		resp = append(resp, dmChannelResp{Channel: ch, DMUser: &o})
	}
	return utils.OK(c, resp)
}
