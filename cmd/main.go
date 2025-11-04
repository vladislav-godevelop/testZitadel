package main

import (
	"log"
	"sms-service/internal/delivery"
	service2 "sms-service/internal/service"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
)

func main() {
	// Загружаем переменные окружения из .env файла
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	} else {
		log.Println("Environment variables loaded from .env file")
	}

	// Инициализируем Zitadel сервис
	zitadelService, err := service2.NewZitadelService()
	if err != nil {
		log.Fatalf("Failed to initialize Zitadel service: %v", err)
	}

	// Инициализируем OIDC сервис
	oidcService, err := service2.NewOIDCService()
	if err != nil {
		log.Fatalf("Failed to initialize OIDC service: %v", err)
	}

	// Инициализируем OTP store
	otpStore := service2.NewOTPStore()

	// Инициализируем OTP verification store (для OIDC flow)
	otpVerificationStore := service2.NewOTPVerificationStore()

	// Создаем handlers
	handler := delivery.NewHandler(zitadelService, otpStore)
	oidcHandler := delivery.NewOIDCHandler(oidcService, zitadelService, otpStore, otpVerificationStore)
	preAuthHandler := delivery.NewPreAuthWebhookHandler(otpVerificationStore)

	// Настраиваем Fiber приложение
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	// Middleware
	app.Use(logger.New())
	app.Use(recover.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "http://localhost:3000, http://localhost:8080",
		AllowCredentials: true,
	}))

	// ============================================
	// НОВЫЙ ПРАВИЛЬНЫЙ OIDC FLOW С OTP
	// ============================================

	// 1. Отправить OTP
	app.Post("/api/auth/otp/send", oidcHandler.SendOTP)

	// 2. Проверить OTP и получить redirect URL для OIDC
	app.Post("/api/auth/otp/verify", oidcHandler.VerifyOTPAndRedirect)

	// 3. OIDC callback (сюда редиректит Zitadel после успешного входа)
	app.Get("/api/auth/callback", oidcHandler.OIDCCallback)

	// ============================================
	// ZITADEL ACTIONS V2 WEBHOOKS
	// ============================================

	// PreAuth webhook - проверяет OTP verification перед входом
	app.Post("/api/webhooks/preauth", preAuthHandler.HandlePreAuth)

	// Pre-registration webhook - валидация перед регистрацией
	app.Post("/api/webhooks/pre-registration", handler.PreRegistrationWebhook)

	// Post-registration webhook - действия после регистрации
	app.Post("/api/webhooks/post-registration", handler.PostRegistrationWebhook)

	// ============================================
	// СТАРЫЕ ЭНДПОИНТЫ (для обратной совместимости)
	// ============================================

	// OTP регистрация (старый способ без OIDC)
	app.Post("/api/auth/register/send-otp", handler.SendOTP)
	app.Post("/api/auth/register/verify-otp", handler.RegisterWithOTP)

	// OTP вход (старый способ без OIDC)
	app.Post("/api/auth/login/send-otp", handler.LoginSendOTP)
	app.Post("/api/auth/login/verify-otp", handler.LoginVerifyOTP)
	app.Post("/api/auth/refresh-token", handler.RefreshAccessToken)

	// Прямая регистрация через API
	app.Post("/api/users/register", handler.RegisterUser)
	app.Post("/api/users/verify-phone", handler.VerifyUserPhone)
	app.Post("/api/users/resend-code", handler.ResendVerificationCode)

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
		})
	})

	// Protected endpoint - требует session token
	app.Get("/api/profile", handler.GetProfile)

	log.Println("🚀 Server listening on :2222")
	log.Println("📍 OIDC Callback URL: http://localhost:2222/api/auth/callback")
	log.Println("📍 PreAuth Webhook: http://192.168.0.112:2222/api/webhooks/preauth")
	log.Fatal(app.Listen(":2222"))
}
