# Zitadel User Registration Service

Сервис для регистрации пользователей в Zitadel по номеру телефона с использованием Go SDK.

## Структура проекта

```
.
├── main.go         # Точка входа приложения, настройка маршрутов
├── service.go      # Бизнес-логика работы с Zitadel SDK
├── handler.go      # HTTP handlers для API endpoints
├── go.mod          # Зависимости Go
└── .env.example    # Пример конфигурации
```

## Возможности

- Регистрация пользователей только по номеру телефона
- Автоматическое заполнение обязательных полей (имя, email) на основе телефона
- Верификация номера телефона
- Повторная отправка кода верификации

## Установка

1. Клонируйте репозиторий или скопируйте файлы

2. Установите зависимости:
```bash
go mod download
```

3. Создайте `.env` файл на основе `.env.example`:
```bash
cp .env.example .env
```

4. Настройте переменные окружения в `.env`:
```env
ZITADEL_DOMAIN=your-instance.zitadel.cloud
ZITADEL_KEY_PATH=/path/to/service-account-key.json
```

Обратите внимание: `ZITADEL_DOMAIN` должен содержать только домен без `https://`

## Настройка Zitadel

### 1. Создание Service Account

1. Перейдите в консоль Zitadel: `https://your-instance.zitadel.cloud`
2. Откройте **Organization** → **Service Users**
3. Нажмите **New**
4. Введите имя сервисного аккаунта (например, `user-registration-service`)
5. Нажмите **Create**

### 2. Настройка прав доступа

1. Откройте созданный Service Account
2. Перейдите в **Authorizations**
3. Добавьте роль **Org User Manager** или **Org Owner**

### 3. Создание ключа

1. Откройте Service Account
2. Перейдите в **Keys**
3. Нажмите **New**
4. Выберите **JSON** тип
5. Скачайте JSON файл с ключом
6. Сохраните путь к файлу в переменную `ZITADEL_KEY_PATH`

## API Endpoints

### 1. Регистрация пользователя

**POST** `/api/users/register`

Request:
```json
{
  "phone": "+79991234567"
}
```

Response (201 Created):
```json
{
  "success": true,
  "user_id": "123456789",
  "phone_code": "123456",
  "message": "User created successfully"
}
```

### 2. Верификация телефона

**POST** `/api/users/verify-phone`

Request:
```json
{
  "user_id": "123456789",
  "code": "123456"
}
```

Response (200 OK):
```json
{
  "success": true,
  "message": "Phone verified successfully"
}
```

### 3. Повторная отправка кода

**POST** `/api/users/resend-code`

Request:
```json
{
  "user_id": "123456789"
}
```

Response (200 OK):
```json
{
  "success": true,
  "phone_code": "654321",
  "message": "Verification code sent successfully"
}
```

### 4. Health Check

**GET** `/health`

Response (200 OK):
```json
{
  "status": "ok"
}
```

## Запуск

```bash
# Загрузить переменные окружения
export $(cat .env | xargs)

# Запустить сервер
go run .
```

Сервер будет доступен на `http://localhost:2222`

## Примеры использования

### Регистрация пользователя

```bash
curl -X POST http://localhost:2222/api/users/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567"}'
```

### Верификация

```bash
curl -X POST http://localhost:2222/api/users/verify-phone \
  -H "Content-Type: application/json" \
  -d '{"user_id": "123456789", "code": "123456"}'
```

## Особенности реализации

### Использование Zitadel User Service V2 (GA)

Проект использует **User Service V2 (GA)** с методом `CreateUser` вместо deprecated `AddHumanUser`:
- `UserServiceV2().CreateUser()` - современный метод
- Поддерживает создание human и machine пользователей
- Стабильный API с долгосрочной поддержкой

### Регистрация только по телефону

Zitadel требует обязательные поля при создании пользователя:
- `profile.given_name` - заполняется номером телефона
- `profile.family_name` - заполняется номером телефона
- `email` - генерируется как `{phone_without_plus}@phone.local` и помечается как verified
- `username` - используется номер телефона
- `phone` - основное поле

### Верификация

- При создании пользователя можно получить код верификации в ответе (`phone_code`)
- Код можно отправить пользователю через SMS (требуется интеграция с SMS-провайдером)
- Или использовать код из ответа для тестирования

## Интеграция с ActionsV2

Для расширения функциональности можно использовать Zitadel ActionsV2:

1. Создать Target (webhook endpoint в вашем сервисе)
2. Настроить Execution для события создания пользователя
3. Добавить кастомную логику (отправка SMS, создание профиля и т.д.)

Документация: https://zitadel.com/docs/concepts/features/actions_v2

## Возможные улучшения

- [ ] Интеграция с SMS-провайдером для автоматической отправки кодов
- [ ] Добавление rate limiting для защиты от спама
- [ ] Поддержка нескольких организаций
- [ ] Логирование в structured формате
- [ ] Добавление метрик и мониторинга
- [ ] Поддержка миграции с других систем

## Troubleshooting

### Ошибка "ZITADEL_DOMAIN environment variable is not set"

Убедитесь, что вы экспортировали переменные окружения:
```bash
export $(cat .env | xargs)
```

### Ошибка "failed to create zitadel client"

- Проверьте корректность `ZITADEL_DOMAIN` (без https://)
- Убедитесь, что путь к ключу `ZITADEL_KEY_PATH` существует
- Проверьте, что JSON файл ключа валиден

### Ошибка "failed to create user in zitadel: permission denied"

- Убедитесь, что Service Account имеет права **Org User Manager** или выше
- Проверьте корректность JSON ключа сервисного аккаунта

## Лицензия

MIT
