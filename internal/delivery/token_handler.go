package delivery

import (
	"log"
	"strings"

	"sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

// TokenHandler обрабатывает проверку токенов
type TokenHandler struct {
	oidcService *service.OIDCService
}

// NewTokenHandler создает новый token handler
func NewTokenHandler(oidcService *service.OIDCService) *TokenHandler {
	return &TokenHandler{
		oidcService: oidcService,
	}
}

// VerifyToken проверяет валидность токена
// POST /api/auth/verify-token
func (h *TokenHandler) VerifyToken(c *fiber.Ctx) error {
	// Извлекаем токен из заголовка Authorization
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		log.Printf("❌ Missing Authorization header")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"valid": false,
			"error": "Missing authorization token",
		})
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		log.Printf("❌ Invalid Authorization header format")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"valid": false,
			"error": "Invalid authorization header format",
		})
	}

	token := parts[1]
	log.Printf("Verifying token: %s...", token[:20])

	// Проверяем токен через introspection
	introspection, err := h.oidcService.IntrospectToken(c.Context(), token)
	if err != nil {
		log.Printf("❌ Token introspection failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"valid": false,
			"error": "Token validation failed",
		})
	}

	if !introspection.Active {
		log.Printf("Token is not active")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"valid": false,
			"error": "Token is expired or invalid",
		})
	}

	// ✅ Токен валиден
	log.Printf("Token is valid for user: %s", introspection.Subject)

	return c.JSON(fiber.Map{
		"valid":    true,
		"user_id":  introspection.Subject,
		"username": introspection.Username,
	})
}
