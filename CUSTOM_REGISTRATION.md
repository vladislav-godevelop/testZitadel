# Кастомная регистрация с Zitadel

## 📋 Оглавление

1. [Варианты реализации](#варианты-реализации)
2. [Вариант 1: Полностью кастомный UI](#вариант-1-полностью-кастомный-ui)
3. [Вариант 2: Zitadel UI + Actions V2](#вариант-2-zitadel-ui--actions-v2)
4. [Вариант 3: Гибридный подход](#вариант-3-гибридный-подход)

---

## Варианты реализации

### Вариант 1: Полностью кастомный UI

**Архитектура:**
```
[Ваш Frontend] → [Ваш Backend API] → [Zitadel API]
```

**Когда использовать:**
- ✅ Нужен полный контроль над UX
- ✅ Мультитенантность
- ✅ Специфичная бизнес-логика
- ✅ Интеграция с внутренними системами

**Процесс:**

1. **Фронтенд отправляет запрос на OTP:**
```javascript
// 1. Запрос OTP кода
const response = await fetch('http://localhost:2222/api/auth/send-otp', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ phone: '+79991234567' })
});

// Response: { success: true, code: "123456" }
```

2. **Пользователь вводит OTP код**

3. **Фронтенд отправляет регистрацию:**
```javascript
// 2. Регистрация с OTP
const response = await fetch('http://localhost:2222/api/auth/register', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    phone: '+79991234567',
    code: '123456'
  })
});

// Response: { success: true, user_id: "344960150798336003" }
```

4. **Получение токенов через OAuth/OIDC:**
```javascript
// 3. Авторизация через Zitadel
// Используйте стандартный OIDC flow
window.location.href = `http://localhost:8080/oauth/v2/authorize?
  client_id=YOUR_CLIENT_ID&
  redirect_uri=http://localhost:3000/callback&
  response_type=code&
  scope=openid profile email phone&
  login_hint=+79991234567`; // Подсказка для логина
```

**Расширения:**

Можно добавить дополнительные поля при регистрации через **metadata**:

```go
// В service.go, метод CreateUserByPhone
func (s *ZitadelService) CreateUserByPhone(ctx context.Context, phone string, metadata map[string]string) (*CreateUserResponse, error) {
    // ... existing code ...

    // Добавляем metadata
    var metadataList []*v2.Metadata
    for key, value := range metadata {
        metadataList = append(metadataList, &v2.Metadata{
            Key:   key,
            Value: []byte(value), // base64 encode в production
        })
    }

    resp, err := s.client.UserServiceV2().CreateUser(ctx, &v2.CreateUserRequest{
        // ... existing fields ...
        UserType: &v2.CreateUserRequest_Human_{
            Human: &v2.CreateUserRequest_Human{
                // ... existing fields ...
                Metadata: metadataList, // Добавляем metadata
            },
        },
    })
}
```

---

### Вариант 2: Zitadel UI + Actions V2

**Архитектура:**
```
[Zitadel Login UI] → [Zitadel Actions V2] → [Ваш Backend Webhook] → [Ваша логика]
```

**Когда использовать:**
- ✅ Хотите использовать готовый UI Zitadel
- ✅ Нужна валидация/обогащение данных
- ✅ Интеграция с внешними системами после регистрации
- ✅ Модификация flow регистрации

**Настройка:**

#### Шаг 1: Создать Target (Webhook)

Через UI Console:
1. Перейдите в **Actions** → **Targets**
2. Нажмите **New**
3. Заполните:
   - Name: `phone-registration-webhook`
   - Type: `Webhook`
   - Endpoint: `http://your-service:2222/api/zitadel/pre-registration`
   - Timeout: `10s`
   - Interrupt on Error: `Yes`

Или через API:
```bash
curl -X POST http://localhost:8080/v2beta/actions/targets \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "phone-registration-webhook",
    "restWebhook": {
      "interruptOnError": true
    },
    "endpoint": "http://localhost:2222/api/zitadel/pre-registration",
    "timeout": "10s"
  }'
```

#### Шаг 2: Создать Execution

Определяет, когда вызывать webhook:

Через UI Console:
1. Перейдите в **Actions** → **Executions**
2. Нажмите **New**
3. Выберите тип: **Request**
4. Method: `/zitadel.user.v2.UserService/CreateUser`
5. Добавьте Target из Шага 1

Или через API:
```bash
curl -X POST http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["TARGET_ID_FROM_STEP_1"],
    "condition": {
      "request": {
        "method": "/zitadel.user.v2.UserService/CreateUser"
      }
    }
  }'
```

#### Шаг 3: Обработка в вашем сервисе

Webhook `PreRegistrationWebhook` уже реализован в `zitadel_webhook.go`:

```go
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    // 1. Получаем данные от Zitadel
    // 2. Проверяем телефон на черном списке
    // 3. Валидируем дополнительные поля

    // Если вернуть ошибку - регистрация будет отклонена
    if isBlacklisted(phone) {
        return c.Status(403).JSON(fiber.Map{
            "error": "Phone not allowed"
        })
    }

    // Иначе регистрация продолжится
    return c.Status(200).JSON(fiber.Map{"success": true})
}
```

**Примеры использования:**

1. **Валидация по региону:**
```go
if !strings.HasPrefix(phone, "+7") {
    return c.Status(403).JSON(fiber.Map{
        "error": "Only Russian phone numbers allowed"
    })
}
```

2. **Лимит регистраций:**
```go
count := getRegistrationCountToday(phone)
if count >= 3 {
    return c.Status(429).JSON(fiber.Map{
        "error": "Too many registration attempts"
    })
}
```

3. **Проверка с внешним API:**
```go
isValid := checkPhoneWithExternalAPI(phone)
if !isValid {
    return c.Status(403).JSON(fiber.Map{
        "error": "Invalid phone number"
    })
}
```

---

### Вариант 3: Гибридный подход

**Лучшее из двух миров:**

1. **Используйте свой API** для регистрации (OTP flow)
2. **Добавьте Actions V2** для пост-обработки

```
[Ваш UI] → [OTP API] → [Создание в Zitadel]
                           ↓
                    [Actions V2 Webhook]
                           ↓
                [Post-registration логика]
```

**Пример:**

После успешной регистрации через `/api/auth/register`, настройте Event-based Action:

```bash
curl -X POST http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["POST_REGISTRATION_TARGET"],
    "condition": {
      "event": {
        "event": "user.added"
      }
    }
  }'
```

В webhook `PostRegistrationWebhook`:
```go
func (h *Handler) PostRegistrationWebhook(c *fiber.Ctx) error {
    // 1. Создать профиль в вашей БД
    // 2. Отправить welcome SMS
    // 3. Добавить в CRM
    // 4. Отправить в analytics

    return c.Status(200).JSON(fiber.Map{"success": true})
}
```

---

## 🔐 Безопасность Webhooks

### Проверка подписи от Zitadel

Zitadel отправляет заголовок `ZITADEL-Signature` с HMAC подписью:

```go
import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
)

func validateZitadelSignature(c *fiber.Ctx, body []byte, secret string) bool {
    signature := c.Get("ZITADEL-Signature")

    h := hmac.New(sha256.New, []byte(secret))
    h.Write(body)
    expected := hex.EncodeToString(h.Sum(nil))

    return hmac.Equal([]byte(signature), []byte(expected))
}
```

Добавьте в middleware:
```go
app.Use("/api/zitadel/*", func(c *fiber.Ctx) error {
    body := c.Body()
    secret := os.Getenv("ZITADEL_WEBHOOK_SECRET")

    if !validateZitadelSignature(c, body, secret) {
        return c.Status(401).JSON(fiber.Map{
            "error": "Invalid signature"
        })
    }

    return c.Next()
})
```

---

## 📊 Сравнение подходов

| Критерий | Кастомный UI | Zitadel UI + Actions | Гибридный |
|----------|--------------|---------------------|-----------|
| Контроль UX | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ |
| Скорость разработки | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ |
| Гибкость логики | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Готовые фичи Zitadel | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Сложность | ⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |

---

## 🚀 Рекомендации

**Для MVP/Стартапа:**
→ Используйте **Вариант 1** (Кастомный UI + OTP API)

**Для Enterprise:**
→ Используйте **Вариант 3** (Гибридный)

**Для быстрого прототипа:**
→ Используйте **Вариант 2** (Zitadel UI + Actions)

---

## 📚 Дополнительные материалы

- [Zitadel Actions V2 Docs](https://zitadel.com/docs/concepts/features/actions_v2)
- [OIDC Integration](https://zitadel.com/docs/guides/integrate/login/oidc)
- [User Metadata API](https://zitadel.com/docs/apis/resources/user_service_v2/user-service-set-user-metadata)
