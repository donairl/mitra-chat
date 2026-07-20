package friends

import (
	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/notifications"
	"mitrachat/server/internal/utils"
)

var validate = validator.New()

// Handler groups friend + user-search endpoints.
type Handler struct{ cfg *config.Config }

// New builds a friends handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts friend routes (protected).
func (h *Handler) Register(r fiber.Router) {
	p := middleware.Protected(h.cfg)
	r.Get("/users/search", p, h.search)
	g := r.Group("/friends", p)
	g.Get("/", h.list)
	g.Get("/requests", h.requests)
	g.Post("/request", h.request)
	g.Put("/:id/accept", h.accept)
	g.Put("/:id/reject", h.reject)
	g.Delete("/:id", h.remove)
}

func (h *Handler) search(c *fiber.Ctx) error {
	q := c.Query("q")
	if len(q) < 2 {
		return utils.OK(c, []models.User{})
	}
	uid := middleware.UserID(c)
	var users []models.User
	database.DB.Where("username LIKE ? AND id <> ?", "%"+q+"%", uid).Limit(20).Find(&users)
	return utils.OK(c, users)
}

func (h *Handler) list(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var rels []models.Friend
	database.DB.Preload("User").Preload("Friend").
		Where("(user_id = ? OR friend_id = ?) AND status = ?", uid, uid, "accepted").
		Find(&rels)
	// project to the "other" user
	out := make([]models.User, 0, len(rels))
	for _, r := range rels {
		if r.UserID == uid && r.Friend != nil {
			out = append(out, *r.Friend)
		} else if r.User != nil {
			out = append(out, *r.User)
		}
	}
	return utils.OK(c, out)
}

func (h *Handler) requests(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var incoming []models.Friend
	database.DB.Preload("User").
		Where("friend_id = ? AND status = ?", uid, "pending").Find(&incoming)
	return utils.OK(c, incoming)
}

type requestReq struct {
	Username string `json:"username" validate:"required"`
}

func (h *Handler) request(c *fiber.Ctx) error {
	var req requestReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}
	uid := middleware.UserID(c)
	var me models.User
	database.DB.First(&me, "id = ?", uid)

	var target models.User
	if err := database.DB.Where("username = ?", req.Username).First(&target).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "user not found")
	}
	if target.ID == uid {
		return utils.Error(c, fiber.StatusBadRequest, "cannot friend yourself")
	}
	var existing int64
	database.DB.Model(&models.Friend{}).
		Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)",
			uid, target.ID, target.ID, uid).Count(&existing)
	if existing > 0 {
		return utils.Error(c, fiber.StatusConflict, "request already exists")
	}
	fr := models.Friend{
		ID: uuid.NewString(), UserID: uid, FriendID: target.ID, Status: "pending",
	}
	database.DB.Create(&fr)
	notifications.Create(target.ID, "friend_request",
		me.Username+" sent you a friend request")
	return c.Status(fiber.StatusCreated).JSON(fr)
}

func (h *Handler) accept(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var fr models.Friend
	if err := database.DB.First(&fr, "id = ?", c.Params("id")).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "request not found")
	}
	if fr.FriendID != uid {
		return utils.Error(c, fiber.StatusForbidden, "not your request")
	}
	database.DB.Model(&fr).Update("status", "accepted")
	notifications.Create(fr.UserID, "friend_accept", "your friend request was accepted")
	return utils.OK(c, fr)
}

func (h *Handler) reject(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var fr models.Friend
	if err := database.DB.First(&fr, "id = ?", c.Params("id")).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "request not found")
	}
	if fr.FriendID != uid {
		return utils.Error(c, fiber.StatusForbidden, "not your request")
	}
	database.DB.Delete(&fr)
	return utils.OK(c, fiber.Map{"message": "rejected"})
}

func (h *Handler) remove(c *fiber.Ctx) error {
	uid := middleware.UserID(c)
	var fr models.Friend
	if err := database.DB.First(&fr, "id = ?", c.Params("id")).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "friend not found")
	}
	if fr.UserID != uid && fr.FriendID != uid {
		return utils.Error(c, fiber.StatusForbidden, "not your friendship")
	}
	database.DB.Delete(&fr)
	return utils.OK(c, fiber.Map{"message": "removed"})
}
