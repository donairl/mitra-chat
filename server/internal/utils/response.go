package utils

import "github.com/gofiber/fiber/v2"

// Error sends a JSON error with the given status.
func Error(c *fiber.Ctx, status int, msg string) error {
	return c.Status(status).JSON(fiber.Map{"error": msg})
}

// OK sends a 200 JSON payload.
func OK(c *fiber.Ctx, data any) error {
	return c.JSON(data)
}
