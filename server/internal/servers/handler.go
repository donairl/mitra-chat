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
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
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
	g.Post("/:id/invite/regenerate", h.regenerateInvite)
	g.Get("/:id/members", h.members)
}

// require checks that userID is a member of serverID holding need. It returns
// the caller's tier, or a non-zero HTTP status and message to send.
func require(serverID, userID string, need perms.Capability) (perms.Tier, int, string) {
	t, ok := perms.MemberTier(serverID, userID)
	if !ok {
		return t, fiber.StatusForbidden, "not a member"
	}
	if !perms.Can(t, need) {
		return t, fiber.StatusForbidden, "insufficient permissions"
	}
	return t, 0, ""
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

// get returns the server with only the channels the caller's tier may view.
func (h *Handler) get(c *fiber.Ctx) error {
	id := c.Params("id")
	t, ok := perms.MemberTier(id, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var srv models.Server
	if err := database.DB.Preload("Channels", perms.VisibleChannels(t)).First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	return utils.OK(c, srv)
}

func (h *Handler) update(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapManageServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	var req createReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	srv.Name, srv.Description, srv.Icon = req.Name, req.Description, req.Icon
	database.DB.Model(&srv).Updates(map[string]any{
		"name": req.Name, "description": req.Description, "icon": req.Icon,
	})
	ws.SendToServerMembers(id, ws.Event("server_updated", srv))
	return utils.OK(c, srv)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapDeleteServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	var srv models.Server
	if err := database.DB.First(&srv, "id = ?", id).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "server not found")
	}
	// Collect event recipients and rooms before the rows disappear.
	var memberIDs, channelIDs []string
	database.DB.Model(&models.ServerMember{}).Where("server_id = ?", id).Pluck("user_id", &memberIDs)
	database.DB.Model(&models.Channel{}).Where("server_id = ?", id).Pluck("id", &channelIDs)
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if len(channelIDs) > 0 {
			if err := tx.Where("channel_id IN ?", channelIDs).Delete(&models.Message{}).Error; err != nil {
				return err
			}
		}
		for _, table := range []any{&models.Channel{}, &models.ServerMember{}, &models.ServerBan{}} {
			if err := tx.Where("server_id = ?", id).Delete(table).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&srv).Error
	})
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not delete server")
	}
	for _, ch := range channelIDs {
		ws.H.CloseRoom(ch)
	}
	ev := ws.Event("server_deleted", fiber.Map{"server_id": id})
	for _, uid := range memberIDs {
		ws.H.SendToUser(uid, ev)
	}
	return utils.OK(c, fiber.Map{"message": "server deleted"})
}

func (h *Handler) invite(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, ok := perms.MemberTier(id, middleware.UserID(c)); !ok {
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

// regenerateInvite replaces the invite code so old codes stop working.
func (h *Handler) regenerateInvite(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, status, msg := require(id, middleware.UserID(c), perms.CapManageServer); status != 0 {
		return utils.Error(c, status, msg)
	}
	code := inviteCode()
	if err := database.DB.Model(&models.Server{}).Where("id = ?", id).Update("invite_code", code).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not regenerate invite")
	}
	return utils.OK(c, fiber.Map{"invite_code": code})
}

type joinReq struct {
	InviteCode string `json:"invite_code" validate:"required"`
}

// join adds the caller as a member (unless banned) and returns the server with
// the channels a member may view.
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
	t, isMember := perms.MemberTier(srv.ID, uid)
	if !isMember {
		var bans int64
		database.DB.Model(&models.ServerBan{}).Where("server_id = ? AND user_id = ?", srv.ID, uid).Count(&bans)
		if bans > 0 {
			return utils.Error(c, fiber.StatusForbidden, "banned from server")
		}
		member := models.ServerMember{
			ID: uuid.NewString(), ServerID: srv.ID, UserID: uid,
			Role: "member", JoinedAt: time.Now(),
		}
		if err := database.DB.Create(&member).Error; err != nil {
			return utils.Error(c, fiber.StatusInternalServerError, "could not join server")
		}
		t = perms.Member
		database.DB.Preload("User").First(&member, "id = ?", member.ID)
		ws.SendToServerMembers(srv.ID, ws.Event("member_joined", fiber.Map{"server_id": srv.ID, "member": member}))
	}
	database.DB.Preload("Channels", perms.VisibleChannels(t)).First(&srv, "id = ?", srv.ID)
	return utils.OK(c, srv)
}

func (h *Handler) members(c *fiber.Ctx) error {
	id := c.Params("id")
	if _, ok := perms.MemberTier(id, middleware.UserID(c)); !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var members []models.ServerMember
	database.DB.Preload("User").Where("server_id = ?", id).Find(&members)
	return utils.OK(c, members)
}
