# Руководство по регистрации с OTP

## Процесс регистрации в два шага

### Шаг 1: Отправка OTP кода

**Endpoint:** `POST /api/auth/send-otp`

**Request:**
```json
{
  "phone": "+79991234567"
}
```

**Response:**
```json
{
  "success": true,
  "message": "OTP code sent successfully",
  "code": "123456"
}
```

**Примечание:** Поле `code` в ответе присутствует только для тестирования. В production оно будет удалено, и код будет отправляться только через SMS.

### Шаг 2: Регистрация с подтверждением OTP

**Endpoint:** `POST /api/auth/register`

**Request:**
```json
{
  "phone": "+79991234567",
  "code": "123456"
}
```

**Response (Success):**
```json
{
  "success": true,
  "user_id": "344960150798336003",
  "message": "User created successfully with verified phone"
}
```

**Response (Invalid OTP):**
```json
{
  "error": "invalid OTP code"
}
```

**Response (Expired OTP):**
```json
{
  "error": "OTP code has expired"
}
```

## Особенности OTP системы

### Хранение кодов
- OTP коды хранятся в памяти приложения (map)
- При перезапуске сервиса все коды теряются
- Для production рекомендуется Redis

### Время жизни
- Код действителен **5 минут**
- Автоматическая очистка истекших кодов каждые 5 минут

### Безопасность
- Максимум **3 попытки** ввода кода
- После 3 неудачных попыток код удаляется
- Один активный код на номер телефона
- При повторной генерации старый код заменяется

### Формат кода
- 6-значный числовой код
- Генерируется криптографически безопасным генератором

## Примеры использования

### Полный цикл регистрации

```bash
# 1. Запросить OTP код
curl -X POST http://localhost:2222/api/auth/send-otp \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567"}'

# Ответ: {"success":true,"message":"OTP code sent successfully","code":"123456"}

# 2. Зарегистрироваться с OTP
curl -X POST http://localhost:2222/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567", "code": "123456"}'

# Ответ: {"success":true,"user_id":"344960150798336003","message":"User created successfully with verified phone"}
```

### Ошибка: неверный код

```bash
curl -X POST http://localhost:2222/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567", "code": "999999"}'

# Ответ: {"error":"invalid OTP code"}
```

### Ошибка: истекший код

```bash
# Подождать 5 минут после генерации кода, затем:
curl -X POST http://localhost:2222/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567", "code": "123456"}'

# Ответ: {"error":"OTP code has expired"}
```

## Интеграция с SMS провайдером

Для production необходимо:

1. Выбрать SMS провайдера (Twilio, Vonage, SMS.ru и т.д.)
2. Обновить метод `SendOTP` в `handler.go`:

```go
// Пример с Twilio
func (h *Handler) SendOTP(c *fiber.Ctx) error {
    // ... validation ...

    code, err := h.otpStore.GenerateOTP(req.Phone)
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Failed to generate OTP"})
    }

    // Отправка SMS через провайдера
    err = h.smsProvider.Send(req.Phone, fmt.Sprintf("Your verification code: %s", code))
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Failed to send SMS"})
    }

    // НЕ возвращаем код в ответе!
    return c.JSON(fiber.Map{
        "success": true,
        "message": "OTP code sent to your phone",
    })
}
```

## Что создается в Zitadel

После успешной регистрации с OTP в Zitadel создается пользователь:
- **Username:** номер телефона
- **Email:** `{phone_without_plus}@phone.local` (verified)
- **Phone:** номер телефона (**verified**)
- **Profile:** имя и фамилия = номер телефона

Пользователь готов к использованию без дополнительной верификации!
