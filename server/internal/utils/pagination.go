package utils

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// DefaultPageSize matches PRD FR-20 (50 messages per load).
const DefaultPageSize = 50

// Pagination reads limit/before query params for cursor-style message loading.
type Pagination struct {
	Limit  int
	Before string // message id or ISO timestamp cursor
}

// ParsePagination extracts pagination params with sane bounds.
func ParsePagination(c *fiber.Ctx) Pagination {
	limit := DefaultPageSize
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	return Pagination{Limit: limit, Before: c.Query("before")}
}
