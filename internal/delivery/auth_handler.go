package delivery

import (
	"log"
	"os"
	"sms-service/internal/domain"
	"sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

type AuthHandler struct {
	oidcService    *service.OIDCService
	zitadelService *service.ZitadelService
	otpStore       *service.OTPStore
}

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

	log.Printf("OTP request for phone: %s", req.Phone)

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

	log.Printf("OTP generated for %s: %s (user_exists=%v, user_id=%s)",
		req.Phone, code, userExists, userID)

	// TODO: В production отправить SMS через SMS-провайдера
	// smsService.Send(req.Phone, fmt.Sprintf("Your verification code: %s", code))

	response := domain.LoginSendOTPResponse{
		Success: true,
		Message: "OTP code sent successfully",
		Code:    code, // В production убрать
	}

	return respondOK(c, response)
}

// VerifyOTP проверяет OTP и возвращает OAuth токены
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

	log.Printf("OTP verification attempt for phone: %s", req.Phone)

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("OTP verification failed for %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	log.Printf("OTP verified successfully for %s", req.Phone)

	// Проверяем существует ли пользователь
	userID, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err != nil {
		return respondBadRequest(c, err.Error())
	}

	actorToken := os.Getenv("ACCES_TOKEN_SERVICE_ACCOUNT")
	if actorToken == "" {
		log.Printf("ACCES_TOKEN_SERVICE_ACCOUNT not set, cannot perform Token Exchange")
		// Fallback: создаем сессию и возвращаем session token
		sessionResp, err := h.zitadelService.CreateSessionForUser(c.Context(), userID)
		if err != nil {
			log.Printf("Failed to create session: %v", err)
			return respondInternalError(c, "Failed to create session", err.Error())
		}

		response := domain.LoginVerifyOTPResponse{
			Success:      true,
			AccessToken:  sessionResp.SessionToken,
			RefreshToken: sessionResp.SessionToken,
			IDToken:      "",
			ExpiresIn:    sessionResp.ExpiresIn,
			TokenType:    "Bearer",
			UserID:       userID,
		}
		return respondOK(c, response)
	}

	// Обмениваем user ID на OAuth токены через Token Exchange с impersonation
	// Требует:
	// 1. Token Exchange feature включен в Zitadel (v2.49+)
	// 2. Impersonation включен в security settings приложения
	// 3. Service account PAT с правами impersonation
	tokens, err := h.oidcService.ExchangeUserIDForTokens(c.Context(), userID, actorToken)
	if err != nil {
		log.Printf("Failed to exchange user ID for tokens: %v", err)
		log.Printf("Falling back to session token (Token Exchange/Impersonation may not be configured)")

		sessionResp, err := h.zitadelService.CreateSessionForUser(c.Context(), userID)
		if err != nil {
			log.Printf("Failed to create session: %v", err)
			return respondInternalError(c, "Failed to create session", err.Error())
		}

		response := domain.LoginVerifyOTPResponse{
			Success:      true,
			AccessToken:  sessionResp.SessionToken,
			RefreshToken: sessionResp.SessionToken,
			IDToken:      "",
			ExpiresIn:    sessionResp.ExpiresIn,
			TokenType:    "Bearer",
			UserID:       userID,
		}
		return respondOK(c, response)
	}

	log.Printf("OAuth tokens obtained successfully for user %s via impersonation", userID)

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

// POST /api/auth/register/send-otp
func (h *AuthHandler) RegisterSendOTP(c *fiber.Ctx) error {
	var req domain.LoginSendOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RegisterSendOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain.ErrPhoneRequired.Error())
	}

	log.Printf("Registration OTP request for phone: %s", req.Phone)

	_, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err == nil {
		log.Printf("User already exists with phone %s", req.Phone)
		return respondBadRequest(c, "User with this phone number already exists")
	}

	// Генерируем OTP код
	code, err := h.otpStore.GenerateOTP(req.Phone)
	if err != nil {
		log.Printf("Failed to generate OTP for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to generate OTP code", err.Error())
	}

	log.Printf("Registration OTP generated for %s: %s", req.Phone, code)

	// TODO: В production отправить SMS через SMS-провайдера
	// smsService.Send(req.Phone, fmt.Sprintf("Your registration code: %s", code))

	response := domain.LoginSendOTPResponse{
		Success: true,
		Message: "Registration OTP code sent successfully",
		Code:    code, // В production убрать
	}

	return respondOK(c, response)
}

// RegisterVerifyOTP проверяет OTP и создает нового пользователя
// POST /api/auth/register/verify-otp
func (h *AuthHandler) RegisterVerifyOTP(c *fiber.Ctx) error {
	var req domain.LoginVerifyOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RegisterVerifyOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" || req.Code == "" {
		return respondBadRequest(c, "Phone and code are required")
	}

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("Registration OTP verification failed for %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	log.Printf("Registration OTP verified successfully for %s", req.Phone)

	// Проверяем, не создан ли уже пользователь
	existingUserID, err := h.zitadelService.FindUserByPhone(c.Context(), req.Phone)
	if err == nil {
		log.Printf("User already exists with phone %s, userID=%s", req.Phone, existingUserID)
		return respondBadRequest(c, "User with this phone number already exists")
	}

	// Создаем нового пользователя
	createResp, err := h.zitadelService.CreateUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("Failed to create user for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to create user", err.Error())
	}

	userID := createResp.UserID
	log.Printf("User created successfully: UserID=%s, Phone=%s", userID, req.Phone)
	log.Printf("User should now login using /api/auth/login/send-otp")

	response := map[string]interface{}{
		"success": true,
		"message": "Registration successful. Please login to get access tokens.",
		"user_id": userID,
	}

	return respondOK(c, response)
}
