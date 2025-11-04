package delivery

import (
	"log"
	domain2 "sms-service/internal/domain"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// PreRegistrationWebhook - webhook для проверки перед регистрацией
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
	var req domain2.ZitadelWebhookRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse webhook request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	log.Printf("Received webhook from Zitadel: %s", req.FullMethod)
	log.Printf("Request data: %+v", req.Request)

	// Извлекаем телефон через domain метод
	phoneNumber, ok := req.ExtractPhoneNumber()
	if !ok || phoneNumber == "" {
		log.Printf("Phone number not found in webhook request")
		return respondBadRequest(c, domain2.ErrPhoneNotFound.Error())
	}

	log.Printf("Phone number extracted: %s", phoneNumber)

	// Проверяем черный список
	if isBlacklisted(phoneNumber) {
		log.Printf("Phone number is blacklisted: %s", phoneNumber)
		return respondForbidden(c, domain2.ErrPhoneBlacklisted.Error())
	}

	// Проверяем регион (только РФ)
	if !strings.HasPrefix(phoneNumber, "+7") {
		log.Printf("Only Russian numbers allowed: %s", phoneNumber)
		return respondForbidden(c, domain2.ErrPhoneNotAllowed.Error())
	}

	// Отправляем уведомление в CRM (мок)
	log.Printf("📨 Sending notification to CRM: new user registration with phone %s", phoneNumber)
	log.Printf("✅ Phone validation passed: %s", phoneNumber)

	// Возвращаем успех - Zitadel продолжит регистрацию
	response := domain2.ZitadelWebhookResponse{
		Success: true,
	}

	return respondOK(c, response)
}

// PostRegistrationWebhook - webhook после успешной регистрации
func (h *Handler) PostRegistrationWebhook(c *fiber.Ctx) error {
	var req domain2.ZitadelWebhookRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse post-registration webhook: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	log.Printf("User created in Zitadel: %+v", req.Request)

	// Извлекаем данные
	phoneNumber, _ := req.ExtractPhoneNumber()
	username, _ := req.ExtractUsername()
	orgID, _ := req.ExtractOrganizationID()

	log.Printf("Post-registration processing: phone=%s, username=%s, orgID=%s",
		phoneNumber, username, orgID)

	// Здесь можно:
	// 1. Создать профиль пользователя в вашей БД
	// 2. Отправить welcome SMS
	// 3. Добавить пользователя в CRM
	// 4. Отправить событие в analytics

	response := domain2.ZitadelWebhookResponse{
		Success: true,
	}

	return respondOK(c, response)
}

// isBlacklisted проверяет номер телефона в черном списке
// TODO: перенести в service layer с использованием БД/Redis
func isBlacklisted(phone string) bool {
	blacklist := []string{
		"+79999999999",
		"+71111111111",
	}

	for _, blocked := range blacklist {
		if phone == blocked {
			return true
		}
	}

	return false
}
