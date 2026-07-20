package auth

import (
	"errors"

	"github.com/go-playground/validator/v10"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"mitrachat/server/internal/config"
	"mitrachat/server/internal/database"
	"mitrachat/server/internal/middleware"
	"mitrachat/server/internal/models"
	"mitrachat/server/internal/utils"
)

var validate = validator.New()

// Handler groups auth endpoints and their dependencies.
type Handler struct {
	cfg *config.Config
}

// New builds an auth handler.
func New(cfg *config.Config) *Handler { return &Handler{cfg: cfg} }

// Register mounts the auth routes.
func (h *Handler) Register(r fiber.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
	r.Get("/me", middleware.Protected(h.cfg), h.me)
	r.Post("/logout", middleware.Protected(h.cfg), h.logout)
	r.Post("/refresh", middleware.Protected(h.cfg), h.refresh)
}

type registerReq struct {
	Username string `json:"username" validate:"required,min=3,max=32"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

type loginReq struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

func (h *Handler) register(c *fiber.Ctx) error {
	var req registerReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var count int64
	database.DB.Model(&models.User{}).Where("email = ? OR username = ?", req.Email, req.Username).Count(&count)
	if count > 0 {
		return utils.Error(c, fiber.StatusConflict, "email or username already in use")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not hash password")
	}
	user := models.User{
		ID:           uuid.NewString(),
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hash),
		Status:       "offline",
	}
	if err := database.DB.Create(&user).Error; err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not create user")
	}
	return h.issue(c, &user, fiber.StatusCreated)
}

func (h *Handler) login(c *fiber.Ctx) error {
	var req loginReq
	if err := c.BodyParser(&req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, "invalid body")
	}
	if err := validate.Struct(req); err != nil {
		return utils.Error(c, fiber.StatusBadRequest, err.Error())
	}

	var user models.User
	if err := database.DB.Where("email = ?", req.Email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.Error(c, fiber.StatusUnauthorized, "invalid credentials")
		}
		return utils.Error(c, fiber.StatusInternalServerError, "lookup failed")
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		return utils.Error(c, fiber.StatusUnauthorized, "invalid credentials")
	}
	return h.issue(c, &user, fiber.StatusOK)
}

func (h *Handler) me(c *fiber.Ctx) error {
	var user models.User
	if err := database.DB.First(&user, "id = ?", middleware.UserID(c)).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "user not found")
	}
	return utils.OK(c, user)
}

func (h *Handler) logout(c *fiber.Ctx) error {
	// Stateless JWT: client discards the token. Mark user offline.
	database.DB.Model(&models.User{}).Where("id = ?", middleware.UserID(c)).Update("status", "offline")
	return utils.OK(c, fiber.Map{"message": "logged out"})
}

func (h *Handler) refresh(c *fiber.Ctx) error {
	var user models.User
	if err := database.DB.First(&user, "id = ?", middleware.UserID(c)).Error; err != nil {
		return utils.Error(c, fiber.StatusNotFound, "user not found")
	}
	return h.issue(c, &user, fiber.StatusOK)
}

func (h *Handler) issue(c *fiber.Ctx, user *models.User, status int) error {
	token, err := utils.GenerateToken(h.cfg.JWTSecret, user.ID)
	if err != nil {
		return utils.Error(c, fiber.StatusInternalServerError, "could not sign token")
	}
	return c.Status(status).JSON(fiber.Map{"token": token, "user": user})
}
