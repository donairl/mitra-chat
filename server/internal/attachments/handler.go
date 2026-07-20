package attachments

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
)

// MaxFileSize is the per-file upload limit (PRD FR-24: 10MB).
const MaxFileSize = 10 * 1024 * 1024

// Handler groups attachment endpoints.
type Handler struct{ cfg *config.Config }

// New builds an attachments handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts the upload route (protected).
func (h *Handler) Register(r fiber.Router) {
	r.Post("/attachments", middleware.Protected(h.cfg), h.upload)
}

func (h *Handler) upload(c *fiber.Ctx) error {
	fh, err := c.FormFile("file")
	if err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "file field required")
	}
	if fh.Size > MaxFileSize {
		return utils.Error(c, fiber.StatusRequestEntityTooLarge, "file exceeds 10MB limit")
	}
	if err := os.MkdirAll(h.cfg.UploadDir, 0o755); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "storage error")
	}
	ext := filepath.Ext(fh.Filename)
	stored := fmt.Sprintf("%s%s", uuid.NewString(), ext)
	dest := filepath.Join(h.cfg.UploadDir, stored)
	if err := c.SaveFile(fh, dest); err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not save file")
	}

	att := models.Attachment{
		ID:        uuid.NewString(),
		MessageID: "", // linked when the message is sent
		FileName:  filepath.Base(fh.Filename),
		FilePath:  "/uploads/" + stored,
		FileType:  fileType(fh.Filename, fh.Header.Get("Content-Type")),
		FileSize:  fh.Size,
		CreatedAt: time.Now(),
	}
	if err := database.DB.Create(&att).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not record attachment")
	}
	return c.Status(fiber.StatusCreated).JSON(att)
}

func fileType(name, ctype string) string {
	if ctype != "" {
		return ctype
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	default:
		return "application/octet-stream"
	}
}
