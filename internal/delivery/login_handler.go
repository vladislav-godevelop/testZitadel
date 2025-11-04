package delivery

import (
	"log"
	"sms-service/internal/domain"

	"github.com/gofiber/fiber/v2"
)

// LoginSendOTP - отправка OTP для входа
func (h *Handler) LoginSendOTP(c *fiber.Ctx) error {
	var req domain.LoginSendOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse LoginSendOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain.ErrPhoneRequired.Error())
	}

	// Проверяем, существует ли пользователь с таким телефоном
	userID, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("User not found for phone %s: %v", req.Phone, err)
		return respondBadRequest(c, "User with this phone number not found. Please register first.")
	}

	log.Printf("User found for login: UserID=%s, Phone=%s", userID, req.Phone)

	// Генерируем OTP код для входа
	code, err := h.otpStore.GenerateOTP(req.Phone)
	if err != nil {
		log.Printf("Failed to generate OTP for login %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to generate OTP code", err.Error())
	}

	log.Printf("Login OTP generated for %s: %s", req.Phone, code)

	// В production здесь будет отправка SMS
	response := domain.LoginSendOTPResponse{
		Success: true,
		Message: "OTP code sent successfully",
		Code:    code, // В production убрать!
	}

	return respondOK(c, response)
}

// LoginVerifyOTP - вход с проверкой OTP и получение токенов
func (h *Handler) LoginVerifyOTP(c *fiber.Ctx) error {
	var req domain.LoginVerifyOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse LoginVerifyOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain.ErrPhoneRequired.Error())
	}

	if req.Code == "" {
		return respondBadRequest(c, domain.ErrCodeRequired.Error())
	}

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("OTP verification failed for login %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	log.Printf("OTP verified successfully for login: %s", req.Phone)

	// OTP верен - создаем сессию в Zitadel
	loginResp, err := h.zitadelService.LoginByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("Failed to login user %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to login", err.Error())
	}

	log.Printf("User logged in successfully: UserID=%s, SessionID=%s", loginResp.UserID, loginResp.SessionID)

	// Обмениваем session token на OAuth2 токены через Zitadel Token Exchange
	tokens, err := h.zitadelService.ExchangeSessionForTokens(c.Context(), loginResp.SessionToken, loginResp.SessionID)
	if err != nil {
		log.Printf("Failed to exchange session for tokens: %v", err)
		return respondInternalError(c, "Failed to get access tokens", err.Error())
	}

	// Устанавливаем cookies как в Zitadel (для web-приложений)
	// setAuthCookies(c, tokens.AccessToken, tokens.IDToken, loginResp.SessionID, tokens.ExpiresIn)

	log.Printf("✅ Login successful for user %s, cookies set", loginResp.UserID)

	response := domain.LoginVerifyOTPResponse{
		Success:      true,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		ExpiresIn:    tokens.ExpiresIn,
		TokenType:    tokens.TokenType,
		UserID:       loginResp.UserID,
	}

	return respondOK(c, response)
}

// RefreshAccessToken - обновление access token через refresh token
func (h *Handler) RefreshAccessToken(c *fiber.Ctx) error {
	var req domain.RefreshTokenRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RefreshToken request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.RefreshToken == "" {
		return respondBadRequest(c, "Refresh token is required")
	}

	// TODO: Реализовать обновление токена через OIDC
	// Пока что возвращаем заглушку
	log.Printf("Token refresh requested with refresh_token: %s...", req.RefreshToken[:10])

	response := domain.RefreshTokenResponse{
		Success:      true,
		AccessToken:  "new_access_token_placeholder",
		RefreshToken: req.RefreshToken,
		IDToken:      "new_id_token_placeholder",
		ExpiresIn:    3600,
		TokenType:    "Bearer",
	}

	return respondOK(c, response)
}

// Вспомогательные функции

func getStringOrEmpty(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

func getIntOrDefault(m map[string]interface{}, key string, defaultVal int) int {
	if val, ok := m[key].(int); ok {
		return val
	}
	return defaultVal
}
