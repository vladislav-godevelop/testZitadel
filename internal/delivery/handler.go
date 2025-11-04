package delivery

import (
	"log"
	domain2 "sms-service/internal/domain"
	service2 "sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	zitadelService *service2.ZitadelService
	otpStore       *service2.OTPStore
}

func NewHandler(zitadelService *service2.ZitadelService, otpStore *service2.OTPStore) *Handler {
	return &Handler{
		zitadelService: zitadelService,
		otpStore:       otpStore,
	}
}

// SendOTP - отправка OTP кода (шаг 1 регистрации)
func (h *Handler) SendOTP(c *fiber.Ctx) error {
	var req domain2.SendOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse SendOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain2.ErrPhoneRequired.Error())
	}

	// Генерируем OTP код
	code, err := h.otpStore.GenerateOTP(req.Phone)
	if err != nil {
		log.Printf("Failed to generate OTP for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to generate OTP code", err.Error())
	}

	log.Printf("OTP generated for %s: %s", req.Phone, code)

	// В production здесь будет отправка SMS
	response := domain2.SendOTPResponse{
		Success: true,
		Message: "OTP code sent successfully",
		Code:    code, // В production убрать!
	}

	return respondOK(c, response)
}

// RegisterWithOTP - регистрация с проверкой OTP (шаг 2)
func (h *Handler) RegisterWithOTP(c *fiber.Ctx) error {
	var req domain2.RegisterWithOTPRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RegisterWithOTP request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain2.ErrPhoneRequired.Error())
	}

	if req.Code == "" {
		return respondBadRequest(c, domain2.ErrCodeRequired.Error())
	}

	// Проверяем OTP код
	if err := h.otpStore.VerifyOTP(req.Phone, req.Code); err != nil {
		log.Printf("OTP verification failed for %s: %v", req.Phone, err)
		return respondBadRequest(c, err.Error())
	}

	// OTP верен - создаем пользователя в Zitadel с верифицированным телефоном
	resp, err := h.zitadelService.CreateUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("Failed to create user for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to create user", err.Error())
	}

	log.Printf("User registered with verified phone: UserID=%s, Phone=%s", resp.UserID, req.Phone)

	response := domain2.RegisterWithOTPResponse{
		Success: true,
		UserID:  resp.UserID,
		Message: "User created successfully with verified phone",
	}

	return respondCreated(c, response)
}

// RegisterUser - handler для регистрации пользователя по номеру телефона (без OTP)
func (h *Handler) RegisterUser(c *fiber.Ctx) error {
	var req domain2.CreateUserRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse RegisterUser request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.Phone == "" {
		return respondBadRequest(c, domain2.ErrPhoneRequired.Error())
	}

	// Создаем пользователя в Zitadel
	resp, err := h.zitadelService.CreateUserByPhone(c.Context(), req.Phone)
	if err != nil {
		log.Printf("Failed to create user for %s: %v", req.Phone, err)
		return respondInternalError(c, "Failed to create user", err.Error())
	}

	log.Printf("User registered: UserID=%s, Phone=%s", resp.UserID, req.Phone)

	response := domain2.CreateUserResponse{
		Success:   true,
		UserID:    resp.UserID,
		PhoneCode: resp.PhoneCode,
		Message:   "User created successfully",
	}

	return respondCreated(c, response)
}

// VerifyUserPhone - handler для верификации номера телефона
func (h *Handler) VerifyUserPhone(c *fiber.Ctx) error {
	var req domain2.VerifyPhoneRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse VerifyUserPhone request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.UserID == "" {
		return respondBadRequest(c, domain2.ErrUserIDRequired.Error())
	}

	if req.Code == "" {
		return respondBadRequest(c, domain2.ErrCodeRequired.Error())
	}

	// Верифицируем телефон
	if err := h.zitadelService.VerifyPhone(c.Context(), req.UserID, req.Code); err != nil {
		log.Printf("Failed to verify phone for user %s: %v", req.UserID, err)
		return respondInternalError(c, "Failed to verify phone", err.Error())
	}

	log.Printf("Phone verified for user %s", req.UserID)

	response := domain2.VerifyPhoneResponse{
		Success: true,
		Message: "Phone verified successfully",
	}

	return respondOK(c, response)
}

// ResendVerificationCode - handler для повторной отправки кода верификации
func (h *Handler) ResendVerificationCode(c *fiber.Ctx) error {
	var req domain2.ResendCodeRequest

	if err := c.BodyParser(&req); err != nil {
		log.Printf("Failed to parse ResendVerificationCode request: %v", err)
		return respondBadRequest(c, "Invalid request body")
	}

	if req.UserID == "" {
		return respondBadRequest(c, domain2.ErrUserIDRequired.Error())
	}

	// Повторно отправляем код
	resp, err := h.zitadelService.ResendPhoneCode(c.Context(), req.UserID)
	if err != nil {
		log.Printf("Failed to resend verification code for user %s: %v", req.UserID, err)
		return respondInternalError(c, "Failed to resend verification code", err.Error())
	}

	log.Printf("Verification code resent for user %s", req.UserID)

	response := domain2.ResendCodeResponse{
		Success:   true,
		PhoneCode: resp.GetVerificationCode(),
		Message:   "Verification code sent successfully",
	}

	return respondOK(c, response)
}

// GetProfile - защищённый endpoint, требует session token
func (h *Handler) GetProfile(c *fiber.Ctx) error {
	// Получаем токен из cookie или Authorization header
	sessionToken := c.Cookies("zitadel:session_token")
	if sessionToken == "" {
		// Пробуем получить из Authorization header
		authHeader := c.Get("Authorization")
		if authHeader != "" && len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			sessionToken = authHeader[7:]
		}
	}

	if sessionToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized - session token required",
		})
	}

	// Проверяем токен через Zitadel Introspection
	introspectResp, err := h.zitadelService.IntrospectToken(c.Context(), sessionToken)
	if err != nil {
		log.Printf("Token introspection failed: %v", err)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error":   "Invalid token",
			"details": err.Error(),
		})
	}

	// Проверяем, что токен активен
	if !introspectResp.Active {
		log.Printf("Token is not active for subject: %s", introspectResp.Subject)
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Token is expired or revoked",
		})
	}

	log.Printf("✅ Authorized access: user_id=%s, username=%s", introspectResp.Subject, introspectResp.Username)

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
		},
	})
}
