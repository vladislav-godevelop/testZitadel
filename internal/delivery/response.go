package delivery

import (
	"github.com/gofiber/fiber/v2"
)

// ErrorResponse - стандартный формат ошибки
type ErrorResponse struct {
	Error   string `json:"error"`
	Details string `json:"details,omitempty"`
}

// respondWithError - вспомогательная функция для отправки ошибок
func respondWithError(c *fiber.Ctx, status int, message string, details ...string) error {
	resp := ErrorResponse{
		Error: message,
	}
	if len(details) > 0 {
		resp.Details = details[0]
	}
	return c.Status(status).JSON(resp)
}

// respondBadRequest - ошибка валидации (400)
func respondBadRequest(c *fiber.Ctx, message string) error {
	return respondWithError(c, fiber.StatusBadRequest, message)
}

// respondUnauthorized - ошибка авторизации (401)
func respondUnauthorized(c *fiber.Ctx, message string) error {
	return respondWithError(c, fiber.StatusUnauthorized, message)
}

// respondForbidden - доступ запрещен (403)
func respondForbidden(c *fiber.Ctx, message string) error {
	return respondWithError(c, fiber.StatusForbidden, message)
}

// respondInternalError - внутренняя ошибка (500)
func respondInternalError(c *fiber.Ctx, message string, details string) error {
	return respondWithError(c, fiber.StatusInternalServerError, message, details)
}

// respondSuccess - успешный ответ с данными
func respondSuccess(c *fiber.Ctx, status int, data interface{}) error {
	return c.Status(status).JSON(data)
}

// respondCreated - успешное создание (201)
func respondCreated(c *fiber.Ctx, data interface{}) error {
	return respondSuccess(c, fiber.StatusCreated, data)
}

// respondOK - успешный ответ (200)
func respondOK(c *fiber.Ctx, data interface{}) error {
	return respondSuccess(c, fiber.StatusOK, data)
}
