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
	"mitrachat/server/internal/perms"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
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

// list returns the server's channels the caller's tier may view.
func (h *Handler) list(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	t, ok := perms.MemberTier(sid, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	var chans []models.Channel
	database.DB.Scopes(perms.VisibleChannels(t)).Where("server_id = ?", sid).Find(&chans)
	return utils.OK(c, chans)
}

type channelReq struct {
	Name        string `json:"name" validate:"required,min=1,max=100"`
	Type        string `json:"type" validate:"omitempty,oneof=text voice"`
	Topic       string `json:"topic" validate:"max=1024"`
	MinViewRole string `json:"min_view_role"`
	MinPostRole string `json:"min_post_role"`
}

// resolveTiers fills empty view/post roles from the current values and checks
// the pair. It returns a client-facing message when the pair is invalid.
func resolveTiers(view, post, curView, curPost string) (string, string, string) {
	if view == "" {
		view = curView
	}
	if post == "" {
		post = curPost
	}
	v, okV := perms.ParseTier(view)
	p, okP := perms.ParseTier(post)
	if !okV || !okP || !perms.ValidChannelTier(v) || !perms.ValidChannelTier(p) {
		return "", "", "invalid role"
	}
	if p < v {
		return "", "", "post role must be at least view role"
	}
	return view, post, ""
}

// channelsChanged tells a server's members to refetch their channel list. The
// refetch is tier-filtered, so private channel names never reach users who
// cannot view them.
func channelsChanged(serverID string) {
	ws.SendToServerMembers(serverID, ws.Event("channels_changed", fiber.Map{"server_id": serverID}))
}

func (h *Handler) create(c *fiber.Ctx) error {
	sid := c.Params("serverId")
	t, ok := perms.MemberTier(sid, middleware.UserID(c))
	if !ok {
		return utils.Error(c, fiber.StatusForbidden, "not a member")
	}
	if !perms.Can(t, perms.CapManageChannels) {
		return utils.Error(c, fiber.StatusForbidden, "insufficient permissions")
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
	view, post, msg := resolveTiers(req.MinViewRole, req.MinPostRole, "member", "member")
	if msg != "" {
		return utils.Error(c, fiber.StatusBadRequest, msg)
	}
	ch := models.Channel{
		ID: uuid.NewString(), Name: req.Name, Type: req.Type,
		Topic: req.Topic, ServerID: sid, MinViewRole: view, MinPostRole: post,
	}
	if err := database.DB.Create(&ch).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create channel")
	}
	channelsChanged(sid)
	return c.Status(fiber.StatusCreated).JSON(ch)
}

// managedChannel loads the :id channel for an edit or delete. It returns a
// non-zero HTTP status and message when the caller may not manage it: 404 when
// they cannot see it, 403 for DM channels or without CapManageChannels.
func managedChannel(c *fiber.Ctx) (models.Channel, int, string) {
	var ch models.Channel
	if err := database.DB.First(&ch, "id = ?", c.Params("id")).Error; err != nil {
		return ch, fiber.StatusNotFound, "channel not found"
	}
	a := perms.ChannelAccess(ch.ID, middleware.UserID(c))
	if !a.CanView {
		return ch, fiber.StatusNotFound, "channel not found"
	}
	if ch.ServerID == "" || !perms.Can(a.Tier, perms.CapManageChannels) {
		return ch, fiber.StatusForbidden, "insufficient permissions"
	}
	return ch, 0, ""
}

func (h *Handler) update(c *fiber.Ctx) error {
	ch, status, msg := managedChannel(c)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	var req channelReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	view, post, msg := resolveTiers(req.MinViewRole, req.MinPostRole, ch.MinViewRole, ch.MinPostRole)
	if msg != "" {
		return utils.Error(c, fiber.StatusBadRequest, msg)
	}
	ch.Name, ch.Topic, ch.MinViewRole, ch.MinPostRole = req.Name, req.Topic, view, post
	database.DB.Model(&ch).Updates(map[string]any{
		"name": ch.Name, "topic": ch.Topic, "min_view_role": view, "min_post_role": post,
	})
	ws.H.RecheckRoom(ch.ID)
	channelsChanged(ch.ServerID)
	return utils.OK(c, ch)
}

func (h *Handler) delete(c *fiber.Ctx) error {
	ch, status, msg := managedChannel(c)
	if status != 0 {
		return utils.Error(c, status, msg)
	}
	database.DB.Where("channel_id = ?", ch.ID).Delete(&models.Message{})
	database.DB.Delete(&ch)
	ws.H.CloseRoom(ch.ID)
	channelsChanged(ch.ServerID)
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
