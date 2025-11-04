# Примеры данных, которые Zitadel отправляет в Webhook

## 1. Request-based Action (CreateUser)

### Что приходит от Zitadel:

```json
{
  "fullMethod": "/zitadel.user.v2.UserService/CreateUser",
  "instanceID": "344954705467736067",
  "orgID": "344954705467736067",
  "projectID": "",
  "userID": "",
  "request": {
    "organizationId": "344954705467736067",
    "username": "+79991234567",
    "userType": {
      "human": {
        "profile": {
          "givenName": "+79991234567",
          "familyName": "+79991234567"
        },
        "email": {
          "email": "79991234567@phone.local",
          "verification": {
            "isVerified": true
          }
        },
        "phone": {
          "phone": "+79991234567",
          "verification": {
            "isVerified": true
          }
        }
      }
    }
  },
  "context": {
    "headers": {
      "authorization": "Bearer ...",
      "content-type": "application/json"
    }
  }
}
```

### Что нужно вернуть:

**Успех (200 OK):**
```json
{
  "success": true
}
```

**Или можно модифицировать request:**
```json
{
  "request": {
    "organizationId": "344954705467736067",
    "username": "+79991234567",
    "userType": {
      "human": {
        "profile": {
          "givenName": "Пользователь",
          "familyName": "Телефон"
        },
        "metadata": [
          {
            "key": "source",
            "value": "mobile_app"
          }
        ]
      }
    }
  }
}
```

**Блокировка (403 Forbidden):**
```json
{
  "error": "Phone number is blacklisted"
}
```

---

## 2. Response-based Action (CreateUser)

### Что приходит от Zitadel:

```json
{
  "fullMethod": "/zitadel.user.v2.UserService/CreateUser",
  "instanceID": "344954705467736067",
  "orgID": "344954705467736067",
  "userID": "344960150798336003",
  "request": {
    // Оригинальный request
  },
  "response": {
    "id": "344960150798336003",
    "creationDate": "2025-11-02T22:10:30.123Z"
  }
}
```

### Что нужно вернуть:

**Просто OK:**
```json
{
  "success": true
}
```

**Или добавить данные в ответ:**
```json
{
  "response": {
    "id": "344960150798336003",
    "creationDate": "2025-11-02T22:10:30.123Z",
    "customField": "some_value"
  }
}
```

---

## 3. Event-based Action (user.added)

### Что приходит от Zitadel:

```json
{
  "event": {
    "id": "event-123",
    "type": "user.added",
    "sequence": "12345",
    "creationDate": "2025-11-02T22:10:30.123Z",
    "payload": {
      "userId": "344960150798336003",
      "username": "+79991234567",
      "human": {
        "profile": {
          "givenName": "+79991234567",
          "familyName": "+79991234567"
        },
        "email": "79991234567@phone.local",
        "phone": "+79991234567"
      }
    }
  },
  "instanceID": "344954705467736067",
  "orgID": "344954705467736067"
}
```

### Что нужно вернуть:

```json
{
  "success": true
}
```

Event-based actions **асинхронные**, ошибки не блокируют операцию.

---

## 4. Практические примеры обработки

### Пример 1: Блокировка регистрации по региону

```go
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    // Получаем телефон из request
    phone := extractPhone(req.Request)

    // Проверяем код страны
    if !strings.HasPrefix(phone, "+7") {
        log.Printf("Blocked registration from non-Russian number: %s", phone)
        return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
            "error": "Only Russian phone numbers are allowed"
        })
    }

    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "success": true
    })
}
```

### Пример 2: Добавление metadata при регистрации

```go
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    // Получаем IP адрес из контекста
    ip := c.IP()
    userAgent := c.Get("User-Agent")

    // Модифицируем request - добавляем metadata
    if userType, ok := req.Request["userType"].(map[string]interface{}); ok {
        if human, ok := userType["human"].(map[string]interface{}); ok {
            human["metadata"] = []map[string]interface{}{
                {
                    "key": "registration_ip",
                    "value": base64.StdEncoding.EncodeToString([]byte(ip)),
                },
                {
                    "key": "user_agent",
                    "value": base64.StdEncoding.EncodeToString([]byte(userAgent)),
                },
            }
        }
    }

    // Возвращаем модифицированный request
    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "request": req.Request,
    })
}
```

### Пример 3: Создание профиля в вашей БД после регистрации

```go
func (h *Handler) PostRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    // Извлекаем данные пользователя
    userID := req.UserID
    phone := extractPhone(req.Request)

    // Создаем профиль в вашей БД
    err := h.db.CreateUserProfile(UserProfile{
        ZitadelID: userID,
        Phone:     phone,
        CreatedAt: time.Now(),
    })

    if err != nil {
        log.Printf("Failed to create user profile: %v", err)
        // Можем вернуть ошибку, но это не заблокирует регистрацию
    }

    // Отправляем welcome SMS
    h.smsService.SendWelcome(phone)

    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "success": true
    })
}
```

### Пример 4: Лимит регистраций

```go
type RegistrationLimiter struct {
    attempts map[string][]time.Time
    mu       sync.RWMutex
}

func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    phone := extractPhone(req.Request)

    // Проверяем лимит (не более 3 регистраций в час)
    h.limiter.mu.RLock()
    attempts := h.limiter.attempts[phone]
    h.limiter.mu.RUnlock()

    // Считаем попытки за последний час
    oneHourAgo := time.Now().Add(-time.Hour)
    recentAttempts := 0
    for _, t := range attempts {
        if t.After(oneHourAgo) {
            recentAttempts++
        }
    }

    if recentAttempts >= 3 {
        return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
            "error": "Too many registration attempts. Please try again later."
        })
    }

    // Добавляем попытку
    h.limiter.mu.Lock()
    h.limiter.attempts[phone] = append(attempts, time.Now())
    h.limiter.mu.Unlock()

    return c.Status(fiber.StatusOK).JSON(fiber.Map{
        "success": true
    })
}
```

---

## 5. Заголовки, которые отправляет Zitadel

```
Content-Type: application/json
ZITADEL-Signature: sha256=<hmac_signature>
User-Agent: Zitadel-Webhook/2.0
```

### Проверка подписи HMAC

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func validateSignature(body []byte, signature string, secret string) bool {
    // Извлекаем хэш из заголовка (формат: sha256=<hash>)
    parts := strings.Split(signature, "=")
    if len(parts) != 2 {
        return false
    }

    h := hmac.New(sha256.New, []byte(secret))
    h.Write(body)
    expected := hex.EncodeToString(h.Sum(nil))

    return hmac.Equal([]byte(parts[1]), []byte(expected))
}

// Middleware для проверки
app.Use("/api/zitadel/*", func(c *fiber.Ctx) error {
    signature := c.Get("ZITADEL-Signature")
    secret := os.Getenv("ZITADEL_WEBHOOK_SECRET")

    if !validateSignature(c.Body(), signature, secret) {
        return c.Status(401).JSON(fiber.Map{
            "error": "Invalid signature"
        })
    }

    return c.Next()
})
```
