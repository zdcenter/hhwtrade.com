package middleware

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
)

// CasbinMiddleware checks permissions for the request using JWT claims
func CasbinMiddleware(enforcer *casbin.Enforcer, jwtSecret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Extract Token
		authHeader := c.Get("Authorization")
		if authHeader == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Missing Authorization header"})
		}
		
		tokenString := strings.Replace(authHeader, "Bearer ", "", 1)
		
		// 2. Parse Token
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid or expired token"})
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Invalid token claims"})
		}

		// 3. User Identity for Casbin
		// We use 'role' as the Casbin subject for simplified RBAC
		// This means policies are defined for roles (e.g. p, admin, ...) not specific users
		role, _ := claims["role"].(string)
		sub := role // Subject is the Role
		
		username, _ := claims["username"].(string)
		email, _ := claims["email"].(string)

		// Store user info in context for downstream handlers
		// Adapted for Angular: using 'id' and 'email'
		c.Locals("id", claims["id"])
		c.Locals("user_id", claims["id"]) // Keep user_id for backward compatibility if backend code uses it
		c.Locals("email", email)
		c.Locals("username", username)
		c.Locals("role", role)

		// 4. Check Permission
		obj := c.Path()
		act := c.Method()

		permit, err := enforcer.Enforce(sub, obj, act)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Permission check failed"})
		}

		if permit {
			return c.Next()
		}

		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "Permission denied",
			"detail": fmt.Sprintf("User %s is not allowed to %s %s", sub, act, obj),
		})
	}
}
