package servers

import (
	"strings"
	"time"

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

// Handler groups server/member endpoints.
type Handler struct{ cfg *config.Config }

// New builds a servers handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts server routes (all protected).
func (h *Handler) Register(r fiber.Router) {
	g := r.Group("/servers", middleware.Protected(h.cfg))
	g.Get("/", h.list)
	g.Post("/", h.create)
	g.Post("/join", h.join)
	g.Get("/:id", h.get)
	g.Put("/:id", h.update)
	g.Delete("/:id", h.delete)
	g.Post("/:id/invite", h.invite)
	g.Get("/:id/members", h.members)
}

func isMember(serverID, userID string) bool {
	var n int64
	database.DB.Model(&models.ServerMember{}).
		Where("server_id = ? AND user_id = ?", serverID, userID).Count(&n)
	return n > 0
}

func inviteCode() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
}

func (h *Handler) list(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var servers []models.Server
	database.DB.
		Joins("JOIN server_members sm ON sm.server_id = servers.id").
		Where("sm.user_id = ?", uid).
		Find(&servers)
	return utils.OK(c, servers)
}

type createReq struct {
	Name        string `json:"name" validate:"required,min=3,max=100"`
	Description string `json:"description" validate:"max=1024"`
	Icon        string `json:"icon" validate:"max=512"`
}

func (h *Handler) create(c *fiber.Ctx) error {
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	srv := models.Server{
		ID: uuid.NewString(), Name: req.Name, OwnerID: uid,
		Description: req.Description, Icon: req.Icon, InviteCode: inviteCode(),
	}
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&srv).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.ServerMember{
			ID: uuid.NewString(), ServerID: srv.ID, UserID: uid,
			Role: "owner", JoinedAt: time.Now(),
		}).Error; err != nil {
			return err
		}
		return tx.Create(&models.Channel{
			ID: uuid.NewString(), Name: "general", Type: "text", ServerID: srv.ID,
		}).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create server")
	}
	database.DB.Preload("Channels").First(&srv, "id = ?", srv.ID)
	return c.Status(fiber.StatusCreated).JSON(srv)
}

func (h *Handler) get(c *fiber.Ctx) error {
	id := c.Params("id")
	if !isMember(id, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var srv models.Server
	if err := database.DB.Preload("Channels").First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	return utils.OK(c, srv)
}

func (h *Handler) update(c *fiber.Ctx) error {
	id := c.Params("id")
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	if srv.OwnerID != middleware.UserID(c) {
		return utils.Error(c, fiber.StatusForbidden, "owner only")
	}
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	database.DB.Model(&srv).Updates(map[string]any{
		"name": req.Name, "description": req.Description, "icon": req.Icon,
	})
	return utils.OK(c, srv)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	id := c.Params("id")
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	if srv.OwnerID != middleware.UserID(c) {
		return utils.Error(c, fiber.StatusForbidden, "owner only")
	}
	database.DB.Transaction(func(tx *gorm.DB) error {
		var chans []models.Channel
		tx.Where("server_id = ?", id).Find(&chans)
		for _, ch := range chans {
			tx.Where("channel_id = ?", ch.ID).Delete(&models.Message{})
		}
		tx.Where("server_id = ?", id).Delete(&models.Channel{})
		tx.Where("server_id = ?", id).Delete(&models.ServerMember{})
		return tx.Delete(&srv).Error
	})
	return utils.OK(c, fiber.Map{"message": "server deleted"})
}

func (h *Handler) invite(c *fiber.Ctx) error {
	id := c.Params("id")
	if !isMember(id, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	if srv.InviteCode == "" {
		srv.InviteCode = inviteCode()
		database.DB.Model(&srv).Update("invite_code", srv.InviteCode)
	}
	return utils.OK(c, fiber.Map{"invite_code": srv.InviteCode})
}

type joinReq struct {
	InviteCode string `json:"invite_code" validate:"required"`
}

func (h *Handler) join(c *fiber.Ctx) error {
	var req joinReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	var srv models.Server
	if err := database.DB.Where("invite_code = ?", req.InviteCode).First(&srv).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "invalid invite code")
	}
	uid := middleware.UserID(c)
	if isMember(srv.ID, uid) {
		return utils.OK(c, srv)
	}
	database.DB.Create(&models.ServerMember{
		ID: uuid.NewString(), ServerID: srv.ID, UserID: uid,
		Role: "member", JoinedAt: time.Now(),
	})
	database.DB.Preload("Channels").First(&srv, "id = ?", srv.ID)
	return utils.OK(c, srv)
}

func (h *Handler) members(c *fiber.Ctx) error {
	id := c.Params("id")
	if !isMember(id, middleware.UserID(c)) {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var members []models.ServerMember
	database.DB.Preload("User").Where("server_id = ?", id).Find(&members)
	return utils.OK(c, members)
}
