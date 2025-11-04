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
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using system environment variables")
	} else {
		log.Println("Environment variables loaded from .env file")
	}

	zitadelService, err := service2.NewZitadelService()
	if err != nil {
		log.Fatalf("Failed to initialize Zitadel service: %v", err)
	}

	oidcService, err := service2.NewOIDCService()
	if err != nil {
		log.Fatalf("Failed to initialize OIDC service: %v", err)
	}

	otpStore := service2.NewOTPStore()

	otpVerificationStore := service2.NewOTPVerificationStore()

	oidcHandler := delivery.NewOIDCHandler(oidcService, zitadelService, otpStore, otpVerificationStore)
	authHandler := delivery.NewAuthHandler(oidcService, zitadelService, otpStore)

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

	// 🔐 АУТЕНТИФИКАЦИЯ ПО НОМЕРУ ТЕЛЕФОНА С OTP
	//
	// Flow:
	// 1. POST /api/auth/login/send-otp    - отправить OTP на телефон
	// 2. POST /api/auth/login/verify-otp  - проверить OTP и получить токены

	// Шаг 1: Отправить OTP код
	app.Post("/api/auth/login/send-otp", authHandler.SendOTP)

	// Шаг 2: Проверить OTP и получить authorization URL
	app.Post("/api/auth/login/verify-otp", authHandler.VerifyOTP)

	log.Fatal(app.Listen(":2222"))
}
