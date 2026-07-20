package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/utils"
)

// Protected returns middleware that verifies the Bearer JWT and stores userID.
func Protected(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		auth := c.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			return utils.Error(c, fiber.StatusUnauthorized, "missing or malformed token")
		}
		userID, err := utils.ParseToken(cfg.JWTSecret, strings.TrimPrefix(auth, "Bearer "))
		if err != nil {
			return utils.Error(c, fiber.StatusUnauthorized, "invalid or expired token")
		}
		c.Locals("userID", userID)
		return c.Next()
	}
}

// UserID reads the authenticated user id set by Protected.
func UserID(c *fiber.Ctx) string {
	if v, ok := c.Locals("userID").(string); ok {
		return v
	}
	return ""
}
