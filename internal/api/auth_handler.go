package api

import (
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"hhwtrade.com/internal/config"
	"hhwtrade.com/internal/model"
)

type AuthHandler struct {
	db        *gorm.DB
	jwtSecret []byte
}

func NewAuthHandler(db *gorm.DB, cfg *config.Config) *AuthHandler {
	// Fallback secret if not configured
	secret := "super-secret-key"
	if cfg.Server.AppName != "" { 
		// Ideally, JWT Secret should be in config, for now using AppName or hardcoded
		// In production, MUST use a strong secret from config/env
		secret = "hhwtrade-secret-key-2025" 
	}
	
	return &AuthHandler{
		db:        db,
		jwtSecret: []byte(secret),
	}
}

type LoginRequest struct {
	Username string `json:"Username"`
	Email    string `json:"Email"`
	Password string `json:"Password"`
}

type RegisterRequest struct {
	Username string `json:"Username"`
	Email    string `json:"Email"`
	Password string `json:"Password"`
}

type AuthResponse struct {
	Token    string `json:"Token"`
	ID       uint   `json:"ID"`
	Username string `json:"Username"`
	Email    string `json:"Email"`
	Role     string `json:"Role"`
}

// Register creates a new user (default role: user)
func (h *AuthHandler) Register(c *fiber.Ctx) error {
	var req RegisterRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request"})
	}

	if req.Email == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Email is required"})
	}
	// Fallback: Use Email as Username if Username is empty (since Username is secondary)
	if req.Username == "" {
		req.Username = req.Email
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Crypto error"})
	}

	user := model.User{
		Username: req.Username,
		Email:    req.Email,
		Password: string(hashedPassword),
		Role:     "user", // Default role
		IsActive: true,
	}

	if err := h.db.Create(&user).Error; err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Username or Email already exists"})
	}

	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"Message": "User registered successfully"})
}

// Login authenticates user and returns JWT
func (h *AuthHandler) Login(c *fiber.Ctx) error {
	var req LoginRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request"})
	}

	// Determine login identifier (prioritize Email, fallback to Username)
	loginID := req.Email
	if loginID == "" {
		loginID = req.Username
	}

	if loginID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Email or Username is required"})
	}

	var user model.User
	// Support login by Username OR Email
	if err := h.db.Where("email = ? OR username = ?", loginID, loginID).First(&user).Error; err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Error": "Invalid credentials"})
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Error": "Invalid credentials"})
	}

	// Generate JWT
	// Claims adapted for Angular: use 'id' and 'email'
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":       user.ID,
		"email":    user.Email,
		"username": user.Username, // Optional: keep username just in case
		"role":     user.Role,
		"exp":      time.Now().Add(time.Hour * 72).Unix(), // 3 days expiration
	})

	t, err := token.SignedString(h.jwtSecret)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to sign token"})
	}

	return c.JSON(AuthResponse{
		Token:    t,
		ID:   user.ID,
		Email:    user.Email,
		Username: user.Username,
		Role:     user.Role,
	})
}

// EnsureAdminUser checks if any user exists, if not creates a default admin
func (h *AuthHandler) EnsureAdminUser() {
	var count int64
	h.db.Model(&model.User{}).Count(&count)
	if count == 0 {
		log.Println("Auth: No users found. Creating default 'admin' user...")
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		admin := model.User{
			Username: "admin",
			Email:    "admin@admin.com", // Mandatory Email
			Password: string(hashedPassword),
			Role:     "admin",
			IsActive: true,
		}
		if err := h.db.Create(&admin).Error; err != nil {
			log.Printf("Failed to create admin user: %v", err)
		} else {
			log.Println("Auth: Created default user: admin / admin123")
		}
	}
}

// GetMe implements the getCurrentUser API
func (h *AuthHandler) GetMe(c *fiber.Ctx) error {
	// The middleware injects "id" into Locals
	userID := c.Locals("id")
	if userID == nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Error": "Unauthorized"})
	}

	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Error": "User not found"})
	}

	return c.JSON(fiber.Map{
		"ID":         user.ID,
		"Username":   user.Username,
		"Email":      user.Email,
		"Role":       user.Role,
		"IsActive":   user.IsActive,
		"CreatedAt":  user.CreatedAt,
	})
}

// Logout is currently a placeholder for client-side token removal
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// In a stateless JWT system, the server doesn't "delete" the token unless we use a blacklist in Redis.
	// For now, we just return success.
	return c.JSON(fiber.Map{
		"Message": "Logged out successfully",
	})
}
