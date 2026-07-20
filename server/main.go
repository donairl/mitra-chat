package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"

	"mitrachat/server/internal/attachments"
	"mitrachat/server/internal/auth"
	"mitrachat/server/internal/channels"
	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/friends"
	"mitrachat/server/internal/messages"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/notifications"
	"mitrachat/server/internal/servers"
	"mitrachat/server/internal/utils"
	"mitrachat/server/internal/ws"

	"strings"
)

func main() {
	cfg := config.Load()
	if err := database.Connect(cfg); err != nil {
		log.Fatalf("database: %v", err)
	}

	app := fiber.New(fiber.Config{
		BodyLimit:             12 * 1024 * 1024, // headroom above 10MB attachments
		DisableStartupMessage: true,
	})

	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(cfg.CORSOrigins, ","),
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowCredentials: false,
	}))

	// Static file serving for uploaded attachments.
	app.Static("/uploads", cfg.UploadDir)

	// REST API, rate-limited per PRD NFR-08 (100 req/min).
	api := app.Group("/api", limiter.New(limiter.Config{
		Max:        100,
		Expiration: time.Minute,
	}))
	auth.New(cfg).Register(api.Group("/auth"))
	servers.New(cfg).Register(api)
	channels.New(cfg).Register(api)
	messages.New(cfg).Register(api)
	attachments.New(cfg).Register(api)
	friends.New(cfg).Register(api)
	notifications.New(cfg).Register(api)

	// WebSocket: authenticate via ?token= during the upgrade.
	app.Use("/ws", func(c *fiber.Ctx) error {
		if !websocket.IsWebSocketUpgrade(c) {
			return fiber.ErrUpgradeRequired
		}
		userID, err := utils.ParseToken(cfg.JWTSecret, c.Query("token"))
		if err != nil {
			return fiber.ErrUnauthorized
		}
		var user models.User
		if database.DB.First(&user, "id = ?", userID).Error != nil {
			return fiber.ErrUnauthorized
		}
		c.Locals("userID", user.ID)
		c.Locals("username", user.Username)
		return c.Next()
	})
	app.Get("/ws", websocket.New(ws.Serve))

	go func() {
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Printf("listen: %v", err)
		}
	}()
	log.Printf("MitraChat server listening on :%s", cfg.Port)

	// Graceful shutdown (PRD NFR-20).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("shutting down...")
	_ = app.ShutdownWithTimeout(5 * time.Second)
}
