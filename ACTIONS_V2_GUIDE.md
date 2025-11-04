# Полное руководство по Actions V2 в Zitadel

## 📚 Содержание

1. [Что такое Actions V2](#что-такое-actions-v2)
2. [Компоненты системы](#компоненты-системы)
3. [Типы условий](#типы-условий)
4. [Практические сценарии](#практические-сценарии)
5. [Управление через API](#управление-через-api)
6. [Управление через UI](#управление-через-ui)

---

## Что такое Actions V2

Actions V2 — это **event-driven middleware** для Zitadel, который позволяет:

✅ Перехватывать API запросы **ДО** обработки
✅ Модифицировать ответы **ПОСЛЕ** обработки
✅ Реагировать на события **ASYNC**
✅ Вызывать внешние сервисы (ваши webhook'и)
✅ Блокировать операции по бизнес-правилам
✅ Обогащать данные из внешних источников

---

## Компоненты системы

### 1. Target (Цель)

**Target** = конфигурация вашего endpoint

```json
{
  "name": "my-webhook",
  "endpoint": "http://localhost:2222/webhook",
  "timeout": "10s",
  "restWebhook": {          // Тип: Webhook
    "interruptOnError": true
  }
}
```

**Типы Target:**

| Тип | Когда использовать | Блокирует ли операцию при ошибке |
|-----|-------------------|----------------------------------|
| `restWebhook` | Валидация, блокировка | Да (если `interruptOnError: true`) |
| `restCall` | Обогащение данных | Опционально |
| `restAsync` | Уведомления, аналитика | Нет (fire-and-forget) |

### 2. Execution (Выполнение)

**Execution** = правило, КОГДА вызывать Target

```json
{
  "targets": ["target-id-1", "target-id-2"],
  "condition": {
    "request": {
      "method": "/zitadel.user.v2.UserService/CreateUser"
    }
  }
}
```

---

## Типы условий

### A) Request (Перехват запросов)

**Когда:** ДО выполнения операции
**Что можно:** Валидировать, блокировать, модифицировать

```json
{
  "condition": {
    "request": {
      "method": "/zitadel.user.v2.UserService/CreateUser"
    }
  }
}
```

**Полезные методы для кастомной регистрации:**
```
/zitadel.user.v2.UserService/CreateUser       - создание пользователя
/zitadel.user.v2.UserService/SetPhone         - установка телефона
/zitadel.user.v2.UserService/VerifyPhone      - верификация телефона
/zitadel.session.v2.SessionService/CreateSession - создание сессии
```

### B) Response (Обработка ответов)

**Когда:** ПОСЛЕ выполнения операции
**Что можно:** Добавить данные, логировать

```json
{
  "condition": {
    "response": {
      "method": "/zitadel.user.v2.UserService/CreateUser"
    }
  }
}
```

### C) Event (События)

**Когда:** Асинхронно при событиях
**Что можно:** Уведомления, синхронизация с внешними системами

```json
{
  "condition": {
    "event": {
      "event": "user.added"
    }
  }
}
```

**События для регистрации:**
- `user.added` - пользователь создан
- `user.phone.changed` - телефон изменен
- `user.phone.verified` - телефон верифицирован
- `session.added` - пользователь залогинился

### D) Function (Функции)

Для совместимости с Actions V1.

---

## Практические сценарии

### Сценарий 1: Кастомная регистрация только с телефоном

**Задача:** Разрешить регистрацию только по номеру телефона из России.

#### Решение через Actions V2:

**1. Создаем Target:**
```bash
curl -X POST http://localhost:8080/v2beta/actions/targets \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "validate-russian-phone",
    "restWebhook": {
      "interruptOnError": true
    },
    "endpoint": "http://localhost:2222/api/zitadel/pre-registration",
    "timeout": "5s"
  }'
```

**2. Создаем Execution:**
```bash
curl -X POST http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["TARGET_ID"],
    "condition": {
      "request": {
        "method": "/zitadel.user.v2.UserService/CreateUser"
      }
    }
  }'
```

**3. В вашем сервисе (webhook):**
```go
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    // Извлекаем телефон
    phone := extractPhoneFromRequest(req.Request)

    // Проверяем код страны
    if !strings.HasPrefix(phone, "+7") {
        return c.Status(403).JSON(fiber.Map{
            "error": "Only Russian phone numbers allowed"
        })
    }

    // Проверяем черный список
    if isBlacklisted(phone) {
        return c.Status(403).JSON(fiber.Map{
            "error": "This phone number is blocked"
        })
    }

    // Лимит регистраций (3 в час)
    if checkRateLimit(phone) {
        return c.Status(429).JSON(fiber.Map{
            "error": "Too many attempts"
        })
    }

    return c.Status(200).JSON(fiber.Map{"success": true})
}
```

**Результат:**
✅ Пользователь с +7 → создается
❌ Пользователь с +1 → блокируется
❌ Номер в черном списке → блокируется

---

### Сценарий 2: Автоматическое создание профиля после регистрации

**Задача:** После регистрации в Zitadel создать профиль в вашей БД и отправить Welcome SMS.

#### Решение:

**1. Создаем Async Target:**
```bash
curl -X POST http://localhost:8080/v2beta/actions/targets \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "post-registration-processing",
    "restAsync": {},
    "endpoint": "http://localhost:2222/api/zitadel/post-registration",
    "timeout": "30s"
  }'
```

**2. Создаем Event-based Execution:**
```bash
curl -X POST http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["TARGET_ID"],
    "condition": {
      "event": {
        "event": "user.added"
      }
    }
  }'
```

**3. В вашем сервисе:**
```go
func (h *Handler) PostRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    userID := req.Event.Payload["userId"]
    phone := req.Event.Payload["phone"]

    // 1. Создаем профиль в БД
    h.db.CreateUser(&User{
        ZitadelID: userID,
        Phone:     phone,
        CreatedAt: time.Now(),
    })

    // 2. Отправляем Welcome SMS
    h.smsService.Send(phone, "Welcome to our service!")

    // 3. Добавляем в CRM
    h.crmService.AddContact(phone, userID)

    // 4. Отправляем событие в analytics
    h.analytics.Track("user_registered", map[string]interface{}{
        "user_id": userID,
        "phone":   phone,
    })

    return c.Status(200).JSON(fiber.Map{"success": true})
}
```

---

### Сценарий 3: Добавление metadata при регистрации

**Задача:** Сохранить IP адрес и User-Agent при регистрации.

**Webhook (Request-based):**
```go
func (h *Handler) PreRegistrationWebhook(c *fiber.Ctx) error {
    var req ZitadelWebhookRequest
    c.BodyParser(&req)

    // Получаем IP и User-Agent из контекста Zitadel
    ip := req.Context["ip"]
    userAgent := req.Context["userAgent"]

    // Модифицируем request
    if userType, ok := req.Request["userType"].(map[string]interface{}); ok {
        if human, ok := userType["human"].(map[string]interface{}); ok {
            // Добавляем metadata
            human["metadata"] = []map[string]interface{}{
                {
                    "key": "registration_ip",
                    "value": base64.StdEncoding.EncodeToString([]byte(ip)),
                },
                {
                    "key": "registration_ua",
                    "value": base64.StdEncoding.EncodeToString([]byte(userAgent)),
                },
            }
        }
    }

    // Возвращаем модифицированный request
    return c.Status(200).JSON(fiber.Map{
        "request": req.Request,
    })
}
```

---

## Управление через API

### Создание Target

```bash
curl -X POST http://localhost:8080/v2beta/actions/targets \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-webhook",
    "restWebhook": {
      "interruptOnError": true
    },
    "endpoint": "http://localhost:2222/webhook",
    "timeout": "10s"
  }'
```

### Список всех Targets

```bash
curl http://localhost:8080/v2beta/actions/targets \
  -H "Authorization: Bearer YOUR_PAT"
```

### Удаление Target

```bash
curl -X DELETE http://localhost:8080/v2beta/actions/targets/TARGET_ID \
  -H "Authorization: Bearer YOUR_PAT"
```

### Создание Execution

```bash
curl -X POST http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT" \
  -H "Content-Type: application/json" \
  -d '{
    "targets": ["TARGET_ID"],
    "condition": {
      "request": {
        "method": "/zitadel.user.v2.UserService/CreateUser"
      }
    }
  }'
```

### Список всех Executions

```bash
curl http://localhost:8080/v2beta/actions/executions \
  -H "Authorization: Bearer YOUR_PAT"
```

---

## Управление через UI

### Создание Target через Console

1. Откройте `http://localhost:8080/ui/console`
2. Перейдите в **Actions** → **Targets**
3. Нажмите **New**
4. Заполните:
   - **Name:** `phone-validation`
   - **Type:** `Webhook`
   - **Endpoint:** `http://localhost:2222/api/zitadel/pre-registration`
   - **Timeout:** `10s`
   - **Interrupt on Error:** ✅ (да)
5. Сохраните

### Создание Execution через Console

1. Перейдите в **Actions** → **Executions**
2. Нажмите **New**
3. Выберите **Type:** `Request`
4. **Method:** `/zitadel.user.v2.UserService/CreateUser`
5. **Targets:** выберите созданный Target
6. Сохраните

---

## Автоматическая настройка

Используйте скрипт `setup_actions.sh`:

```bash
chmod +x setup_actions.sh
./setup_actions.sh
```

Скрипт автоматически:
1. ✅ Создаст Target для pre-registration
2. ✅ Создаст Target для post-registration
3. ✅ Настроит Execution для CreateUser
4. ✅ Настроит Execution для события user.added

---

## Проверка работы

### 1. Запустите ваш сервис:
```bash
./zitadel-service
```

### 2. Попробуйте зарегистрироваться:
```bash
curl -X POST http://localhost:2222/api/auth/send-otp \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567"}'

curl -X POST http://localhost:2222/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567", "code": "CODE_FROM_STEP_1"}'
```

### 3. Проверьте логи:

В логах вашего сервиса должны быть:
```
Received webhook from Zitadel: /zitadel.user.v2.UserService/CreateUser
Phone validation passed: +79991234567
User created in Zitadel: map[userId:344960150798336003 ...]
```

---

## Best Practices

### 1. Используйте Async для долгих операций

❌ **Плохо (Request-based):**
```go
func PreRegistration(c *fiber.Ctx) error {
    // Долгий запрос к внешнему API (5 секунд)
    result := externalAPI.Validate(phone) // БЛОКИРУЕТ!
    return c.JSON(result)
}
```

✅ **Хорошо (Event-based + Async):**
```go
func PostRegistration(c *fiber.Ctx) error {
    // Запускаем в горутине
    go func() {
        externalAPI.Validate(phone)
        h.db.UpdateValidation(userID, result)
    }()
    return c.JSON(fiber.Map{"success": true})
}
```

### 2. Всегда проверяйте HMAC подпись

```go
app.Use("/api/zitadel/*", validateHMACMiddleware)
```

### 3. Логируйте все webhook вызовы

```go
func LogWebhook(c *fiber.Ctx) error {
    body := c.Body()
    log.Printf("Webhook received: %s", string(body))
    return c.Next()
}
```

### 4. Используйте таймауты

```json
{
  "timeout": "5s"  // Для валидации
  "timeout": "30s" // Для post-processing
}
```

---

## Резюме

**Что вы теперь можете:**

✅ Перехватывать регистрацию через Actions V2
✅ Валидировать телефоны по своим правилам
✅ Блокировать нежелательных пользователей
✅ Автоматически создавать профили в своей БД
✅ Отправлять welcome сообщения
✅ Добавлять metadata к пользователям
✅ Интегрировать с внешними системами

**Actions V2 + ваш webhook сервис = полный контроль над регистрацией!** 🚀
