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

	// ВАЖНО: Session token от Zitadel - это валидный токен для Zitadel API,
	// но он не является стандартным OAuth access token.
	// Для упрощения возвращаем session token как access token,
	// но проверяем его через GetSession API вместо OIDC introspection.

	log.Printf("✅ Returning session tokens for user %s", userID)

	// Возвращаем session tokens
	response := domain.LoginVerifyOTPResponse{
		Success:      true,
		AccessToken:  sessionResp.SessionToken,
		RefreshToken: sessionResp.SessionToken, // Session token можно переиспользовать
		IDToken:      "",                       // ID token недоступен без полного OIDC flow
		ExpiresIn:    sessionResp.ExpiresIn,
		TokenType:    "Bearer",
		UserID:       userID,
	}

	return respondOK(c, response)
}
