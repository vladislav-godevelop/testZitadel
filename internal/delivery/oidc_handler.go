package delivery

import (
	"fmt"
	"log"
	"sms-service/internal/domain"
	"sms-service/internal/service"
	"time"

	"github.com/gofiber/fiber/v2"
)

// OIDCHandler управляет OIDC flow
type OIDCHandler struct {
	oidcService          *service.OIDCService
	zitadelService       *service.ZitadelService
	otpStore             *service.OTPStore
	otpVerificationStore *service.OTPVerificationStore
	stateStore           map[string]string // phone -> state mapping (в production используйте Redis)
}

// NewOIDCHandler создает новый OIDC handler
func NewOIDCHandler(oidcService *service.OIDCService, zitadelService *service.ZitadelService, otpStore *service.OTPStore, otpVerificationStore *service.OTPVerificationStore) *OIDCHandler {
	return &OIDCHandler{
		oidcService:          oidcService,
		zitadelService:       zitadelService,
		otpStore:             otpStore,
		otpVerificationStore: otpVerificationStore,
		stateStore:           make(map[string]string),
	}
}

// SendOTP - отправка OTP для входа (шаг 1)
func (h *OIDCHandler) SendOTP(c *fiber.Ctx) error {
	var req domain.LoginSendOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse SendOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain.ErrPhoneRequired.Error())
	}

	// Генерируем OTP код
	code, err := h.otpStore.GenerateOTP(req.Phone)
	if err != nil {
		log.Printf("Failed to generate OTP for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to generate OTP code", err.Error())
	}

	log.Printf("OTP generated for login %s: %s", req.Phone, code)

	// В production здесь будет отправка SMS
	response := domain.LoginSendOTPResponse{
		Success: true,
		Message: "OTP code sent successfully",
		Code:    code, // В production убрать!
	}

	return respondOK(c, response)
}

// VerifyOTPAndRedirect - проверка OTP и создание сессии (шаг 2)
func (h *OIDCHandler) VerifyOTPAndRedirect(c *fiber.Ctx) error {
	var req domain.LoginVerifyOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse VerifyOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" || req.Code == "" {
		return respondBadRequest(c, "Phone and code are required")
	}

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("OTP verification failed for %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	log.Printf("✅ OTP verified successfully for %s", req.Phone)

	// Помечаем телефон как верифицированный
	h.otpVerificationStore.MarkAsVerified(req.Phone)

	// Находим пользователя по номеру телефона
	userID, err := h.zitadelService.GetUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("Failed to find user by phone %s: %v", req.Phone, err)
		return respondInternalError(c, "User not found", err.Error())
	}

	// Создаем сессию для пользователя
	tokens, err := h.zitadelService.CreateSessionForUser(c.Context(), userID)
	if err != nil {
		log.Printf("Failed to create session for user %s: %v", userID, err)
		return respondInternalError(c, "Failed to create session", err.Error())
	}

	log.Printf("✅ Session created for user %s (phone: %s)", userID, req.Phone)

	// Устанавливаем cookies с токенами
	setSessionCookiesWithRefresh(c, tokens.SessionToken, tokens.RefreshToken, tokens.ExpiresIn, userID)

	// Возвращаем успешный ответ с токенами
	return respondOK(c, fiber.Map{
		"success":       true,
		"user_id":       userID,
		"session_token": tokens.SessionToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    "Bearer",
		"message":       "Login successful",
		"expires_in":    tokens.ExpiresIn,
	})
}

// OIDCCallback - обработка callback от Zitadel (шаг 3)
func (h *OIDCHandler) OIDCCallback(c *fiber.Ctx) error {
	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")

	// Проверяем ошибки от Zitadel
	if errorParam != "" {
		errorDesc := c.Query("error_description")
		log.Printf("❌ OIDC error: %s - %s", errorParam, errorDesc)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error":             errorParam,
			"error_description": errorDesc,
		})
	}

	if code == "" || state == "" {
		return respondBadRequest(c, "Missing code or state parameter")
	}

	// Проверяем state
	phone, exists := h.stateStore[state]
	if !exists {
		log.Printf("❌ Invalid state: %s", state)
		return respondBadRequest(c, "Invalid state parameter")
	}

	// Удаляем использованный state
	delete(h.stateStore, state)

	log.Printf("📩 OIDC callback received: code=%s..., phone=%s", code[:10], phone)

	// Обмениваем code на токены
	token, claims, err := h.oidcService.ExchangeCode(c.Context(), code)
	if err != nil {
		log.Printf("Failed to exchange code: %v", err)
		return respondInternalError(c, "Failed to complete login", err.Error())
	}

	log.Printf("✅ Login successful: user_id=%s, email=%s", claims.Subject, claims.Email)

	// Вычисляем expires_in
	expiresIn := 3600 // По умолчанию 1 час

	// Устанавливаем cookies с токенами
	setOIDCCookies(c, token.AccessToken, token.RefreshToken, token.IDToken, expiresIn, claims.Subject)

	// Редиректим на success страницу или возвращаем JSON
	return c.JSON(fiber.Map{
		"success":      true,
		"user_id":      claims.Subject,
		"email":        claims.Email,
		"access_token": token.AccessToken,
		"id_token":     token.IDToken,
		"token_type":   "Bearer",
		"expires_in":   expiresIn,
	})
}

// setOIDCCookies устанавливает cookies после успешной OIDC аутентификации
func setOIDCCookies(c *fiber.Ctx, accessToken, refreshToken, idToken string, expiresIn int, userID string) {

	// Access Token cookie
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:access_token",
		Value:    accessToken,
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false, // true в production с HTTPS
		HTTPOnly: true,
		SameSite: "Lax",
	})

	// Refresh Token cookie
	if refreshToken != "" {
		c.Cookie(&fiber.Cookie{
			Name:     "zitadel:refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   60 * 60 * 24 * 30, // 30 дней
			Secure:   false,
			HTTPOnly: true,
			SameSite: "Lax",
		})
	}

	// ID Token cookie
	if idToken != "" {
		c.Cookie(&fiber.Cookie{
			Name:     "zitadel:id_token",
			Value:    idToken,
			Path:     "/",
			MaxAge:   expiresIn,
			Secure:   false,
			HTTPOnly: true,
			SameSite: "Lax",
		})
	}

	// Expires At cookie
	expiresAt := time.Now().Unix() + int64(expiresIn)
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:expires_at",
		Value:    fmt.Sprintf("%d", expiresAt),
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false,
		HTTPOnly: false, // Может читаться JS
		SameSite: "Lax",
	})

	log.Printf("🍪 OIDC cookies set for user %s", userID)
}

// setSessionCookies устанавливает cookies с session token
func setSessionCookies(c *fiber.Ctx, sessionToken string, expiresIn int, userID string) {
	// Session Token cookie (главный токен для авторизации)
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:session_token",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false, // true в production с HTTPS
		HTTPOnly: true,
		SameSite: "Lax",
	})

	// Expires At cookie
	expiresAt := time.Now().Unix() + int64(expiresIn)
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:expires_at",
		Value:    fmt.Sprintf("%d", expiresAt),
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false,
		HTTPOnly: false, // Может читаться JS
		SameSite: "Lax",
	})

	log.Printf("🍪 Session cookies set for user %s", userID)
}

// setSessionCookiesWithRefresh устанавливает cookies с session и refresh токенами
func setSessionCookiesWithRefresh(c *fiber.Ctx, sessionToken, refreshToken string, expiresIn int, userID string) {
	// Session Token cookie (главный токен для авторизации)
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:session_token",
		Value:    sessionToken,
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false, // true в production с HTTPS
		HTTPOnly: true,
		SameSite: "Lax",
	})

	// Refresh Token cookie (для обновления session token)
	if refreshToken != "" {
		c.Cookie(&fiber.Cookie{
			Name:     "zitadel:refresh_token",
			Value:    refreshToken,
			Path:     "/",
			MaxAge:   60 * 60 * 24 * 30, // 30 дней
			Secure:   false,
			HTTPOnly: true,
			SameSite: "Lax",
		})
	}

	// Expires At cookie
	expiresAt := time.Now().Unix() + int64(expiresIn)
	c.Cookie(&fiber.Cookie{
		Name:     "zitadel:expires_at",
		Value:    fmt.Sprintf("%d", expiresAt),
		Path:     "/",
		MaxAge:   expiresIn,
		Secure:   false,
		HTTPOnly: false, // Может читаться JS
		SameSite: "Lax",
	})

	log.Printf("🍪 Session cookies set for user %s (with refresh token)", userID)
}
