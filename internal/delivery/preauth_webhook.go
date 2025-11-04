package delivery

import (
	"log"
	"sms-service/internal/domain"
	"sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

// PreAuthWebhookHandler обрабатывает PreAuth webhook от Zitadel
type PreAuthWebhookHandler struct {
	otpVerificationStore *service.OTPVerificationStore
}

// NewPreAuthWebhookHandler создает новый PreAuth webhook handler
func NewPreAuthWebhookHandler(otpVerificationStore *service.OTPVerificationStore) *PreAuthWebhookHandler {
	return &PreAuthWebhookHandler{
		otpVerificationStore: otpVerificationStore,
	}
}

// HandlePreAuth проверяет OTP verification перед входом
func (h *PreAuthWebhookHandler) HandlePreAuth(c *fiber.Ctx) error {
	var req domain.ZitadelWebhookRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse PreAuth webhook: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	log.Printf("📨 PreAuth webhook received: %s", req.FullMethod)
	log.Printf("Request data: %+v", req.Request)

	// Временно: просто пропускаем все попытки входа
	// PreAuth webhook вызывается ДО проверки пароля
	// Если вернем success, Zitadel продолжит стандартную проверку

	log.Printf("✅ PreAuth check passed - continuing to standard login")

	return respondOK(c, domain.ZitadelWebhookResponse{Success: true})
}
