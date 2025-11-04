package delivery

import (
	"log"
	"sms-service/internal/domain"
	"sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

// AuthHandler обрабатывает аутентификацию через OTP + OIDC
type AuthHandler struct {
	oidcService    *service.OIDCService
	zitadelService *service.ZitadelService
	otpStore       *service.OTPStore
}

// NewAuthHandler создает новый auth handler
func NewAuthHandler(
	oidcService *service.OIDCService,
	zitadelService *service.ZitadelService,
	otpStore *service.OTPStore,
) *AuthHandler {
	return &AuthHandler{
		oidcService:    oidcService,
		zitadelService: zitadelService,
		otpStore:       otpStore,
	}
}

// SendOTP отправляет OTP код на номер телефона (шаг 1)
// POST /api/auth/login/send-otp
func (h *AuthHandler) SendOTP(c *fiber.Ctx) error {
	var req domain.LoginSendOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse SendOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain.ErrPhoneRequired.Error())
	}

	log.Printf("📱 OTP request for phone: %s", req.Phone)

	// Проверяем, существует ли пользователь
	userExists := true
	userID, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("User not found for phone %s, will create on verification", req.Phone)
		userExists = false
	}

	// Генерируем OTP код
	code, err := h.otpStore.GenerateOTP(req.Phone)
	if err != nil {
		log.Printf("Failed to generate OTP for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to generate OTP code", err.Error())
	}

	log.Printf("✅ OTP generated for %s: %s (user_exists=%v, user_id=%s)",
		req.Phone, code, userExists, userID)

	// TODO: В production отправить SMS через SMS-провайдера
	// smsService.Send(req.Phone, fmt.Sprintf("Your verification code: %s", code))

	response := domain.LoginSendOTPResponse{
		Success: true,
		Message: "OTP code sent successfully",
		Code:    code, // В production убрать!
	}

	return respondOK(c, response)
}

// VerifyOTP проверяет OTP и возвращает OAuth токены (шаг 2)
// POST /api/auth/login/verify-otp
func (h *AuthHandler) VerifyOTP(c *fiber.Ctx) error {
	var req domain.LoginVerifyOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse VerifyOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" || req.Code == "" {
		return respondBadRequest(c, "Phone and code are required")
	}

	log.Printf("🔐 OTP verification attempt for phone: %s", req.Phone)

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("❌ OTP verification failed for %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	log.Printf("✅ OTP verified successfully for %s", req.Phone)

	// Проверяем существует ли пользователь
	userID, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err != nil {
		// Пользователь не найден - создаем нового
		log.Printf("👤 Creating new user for phone %s", req.Phone)
		createResp, createErr := h.zitadelService.CreateUserByPhone(c.Context(), req.Phone)
		if createErr != nil {
			log.Printf("❌ Failed to create user: %v", createErr)
			return respondInternalError(c, "Failed to create user", createErr.Error())
		}
		userID = createResp.UserID
		log.Printf("✅ New user created: user_id=%s, phone=%s", userID, req.Phone)
	} else {
		log.Printf("👤 Existing user found: user_id=%s, phone=%s", userID, req.Phone)
	}

	// Создаем сессию для пользователя
	sessionResp, err := h.zitadelService.CreateSessionForUser(c.Context(), userID)
	if err != nil {
		log.Printf("❌ Failed to create session: %v", err)
		return respondInternalError(c, "Failed to create session", err.Error())
	}

	log.Printf("🎫 Session created: user_id=%s, session_token=%s...",
		userID, sessionResp.SessionToken[:20])

	// Обмениваем session token на OAuth токены
	tokens, err := h.oidcService.ExchangeSessionToken(
		c.Context(),
		sessionResp.SessionToken,
		"", // session_id не нужен для token exchange
	)
	if err != nil {
		log.Printf("⚠️  Token exchange failed: %v", err)
		// Если token exchange не работает, возвращаем session token как access token
		log.Printf("📋 Falling back to session token as access token")

		response := domain.LoginVerifyOTPResponse{
			Success:      true,
			AccessToken:  sessionResp.SessionToken,
			RefreshToken: sessionResp.RefreshToken,
			IDToken:      "",
			ExpiresIn:    sessionResp.ExpiresIn,
			TokenType:    "Bearer",
			UserID:       userID,
		}

		return respondOK(c, response)
	}

	log.Printf("✅ OAuth tokens obtained successfully for user %s", userID)

	// Возвращаем OAuth токены
	response := domain.LoginVerifyOTPResponse{
		Success:      true,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		ExpiresIn:    tokens.ExpiresIn,
		TokenType:    tokens.TokenType,
		UserID:       userID,
	}

	return respondOK(c, response)
}

// RefreshToken обновляет access token используя refresh token (шаг 3)
// POST /api/auth/refresh-token
func (h *AuthHandler) RefreshToken(c *fiber.Ctx) error {
	var req domain.RefreshTokenRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RefreshToken request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.RefreshToken == "" {
		return respondBadRequest(c, "Refresh token is required")
	}

	log.Printf("🔄 Token refresh requested")

	// Обновляем токены
	tokens, err := h.oidcService.RefreshAccessToken(c.Context(), req.RefreshToken)
	if err != nil {
		log.Printf("❌ Failed to refresh token: %v", err)
		return respondUnauthorized(c, "Invalid or expired refresh token")
	}

	log.Printf("✅ Tokens refreshed successfully")

	response := domain.RefreshTokenResponse{
		Success:      true,
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		ExpiresIn:    tokens.ExpiresIn,
		TokenType:    tokens.TokenType,
	}

	return respondOK(c, response)
}

// GetProfile защищённый endpoint - проверяет access token
// GET /api/profile
func (h *AuthHandler) GetProfile(c *fiber.Ctx) error {
	// Получаем токен из Authorization header
	authHeader := c.Get("Authorization")
	if authHeader == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Authorization header required",
		})
	}

	// Извлекаем токен из "Bearer <token>"
	token := ""
	if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
		token = authHeader[7:]
	} else {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Invalid authorization header format",
		})
	}

	log.Printf("🔍 Token introspection for profile access")

	// Проверяем токен через introspection
	introspectResp, err := h.oidcService.IntrospectToken(c.Context(), token)
	if err != nil {
		log.Printf("❌ Token introspection failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "Invalid token",
			"details": err.Error(),
		})
	}

	// Проверяем, что токен активен
	if !introspectResp.Active {
		log.Printf("❌ Token is not active")
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Token is expired or revoked",
		})
	}

	log.Printf("✅ Access granted: user_id=%s, username=%s",
		introspectResp.Subject, introspectResp.Username)

	// Возвращаем информацию о пользователе
	return c.JSON(fiber.Map{
		"success":  true,
		"message":  "Access granted",
		"user_id":  introspectResp.Subject,
		"username": introspectResp.Username,
		"token_info": fiber.Map{
			"active":     introspectResp.Active,
			"expires_at": introspectResp.ExpiresAt,
			"issued_at":  introspectResp.IssuedAt,
			"scope":      introspectResp.Scope,
		},
	})
}

// Logout отзывает токены (опционально)
// POST /api/auth/logout
func (h *AuthHandler) Logout(c *fiber.Ctx) error {
	// TODO: Реализовать revocation токенов через Zitadel
	// https://zitadel.com/docs/apis/openidoauth/endpoints#revocation_endpoint

	log.Printf("🚪 Logout requested")

	return respondOK(c, fiber.Map{
		"success": true,
		"message": "Logged out successfully",
	})
}

// HealthCheck проверяет работоспособность auth service
// GET /api/auth/health
func (h *AuthHandler) HealthCheck(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":  "ok",
		"service": "auth",
		"message": "Authentication service is running",
	})
}
