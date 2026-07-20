package notifications

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"
)

// Handler groups notification endpoints.
type Handler struct{ cfg *config.Config }

// New builds a notifications handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts notification routes (protected).
func (h *Handler) Register(r fiber.Router) {
	g := r.Group("/notifications", middleware.Protected(h.cfg))
	g.Get("/", h.list)
	g.Put("/:id/read", h.markRead)
	g.Put("/read-all", h.markAll)
}

// Create persists a notification and pushes it to the user over the socket.
func Create(userID, ntype, content string) {
	n := models.Notification{
		ID: uuid.NewString(), UserID: userID, Type: ntype,
		Content: content, Read: false, CreatedAt: time.Now(),
	}
	if err := database.DB.Create(&n).Error; err != nil {
		return
	}
	ws.H.SendToUser(userID, map[string]any{"type": "notification", "payload": n})
}

func (h *Handler) list(c *fiber.Ctx) error {
	var ns []models.Notification
	database.DB.Where("user_id = ?", middleware.UserID(c)).
		Order("created_at desc").Limit(100).Find(&ns)
	return utils.OK(c, ns)
}

func (h *Handler) markRead(c *fiber.Ctx) error {
	database.DB.Model(&models.Notification{}).
		Where("id = ? AND user_id = ?", c.Params("id"), middleware.UserID(c)).
		Update("read", true)
	return utils.OK(c, fiber.Map{"message": "read"})
}

func (h *Handler) markAll(c *fiber.Ctx) error {
	database.DB.Model(&models.Notification{}).
		Where("user_id = ?", middleware.UserID(c)).Update("read", true)
	return utils.OK(c, fiber.Map{"message": "all read"})
}
