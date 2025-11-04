# Быстрый старт

## Шаг 1: Настройка Zitadel

1. Откройте вашу Zitadel консоль (например, `https://your-instance.zitadel.cloud`)

2. Создайте Service Account:
   - Перейдите в **Organization** → **Service Users**
   - Нажмите **New**
   - Введите имя: `user-registration-service`
   - Нажмите **Create**

3. Выдайте права:
   - Откройте созданный Service Account
   - Перейдите в **Authorizations**
   - Добавьте роль **Org User Manager**

4. Создайте ключ:
   - Откройте Service Account
   - Перейдите в **Keys**
   - Нажмите **New**
   - Выберите тип **JSON**
   - Скачайте файл (например, `zitadel-key.json`)

## Шаг 2: Настройка проекта

1. Скопируйте пример конфигурации:
```bash
cp .env.example .env
```

2. Отредактируйте `.env`:
```env
ZITADEL_DOMAIN=your-instance.zitadel.cloud
ZITADEL_KEY_PATH=/path/to/zitadel-key.json
```

3. Убедитесь, что зависимости установлены:
```bash
go mod download
```

## Шаг 3: Запуск

```bash
# Экспортируем переменные окружения
export $(cat .env | xargs)

# Запускаем сервер
go run .
```

Или используйте скомпилированный бинарник:
```bash
./zitadel-service
```

Сервер запустится на порту `2222`.

## Шаг 4: Тестирование

### Регистрация пользователя

```bash
curl -X POST http://localhost:2222/api/users/register \
  -H "Content-Type: application/json" \
  -d '{"phone": "+79991234567"}'
```

Ответ:
```json
{
  "success": true,
  "user_id": "123456789",
  "phone_code": "654321",
  "message": "User created successfully"
}
```

### Верификация телефона

```bash
curl -X POST http://localhost:2222/api/users/verify-phone \
  -H "Content-Type: application/json" \
  -d '{
    "user_id": "123456789",
    "code": "654321"
  }'
```

Ответ:
```json
{
  "success": true,
  "message": "Phone verified successfully"
}
```

### Health Check

```bash
curl http://localhost:2222/health
```

Ответ:
```json
{
  "status": "ok"
}
```

## Как это работает

1. **Регистрация**: Вы отправляете только номер телефона
2. **Автозаполнение**: Сервис автоматически заполняет обязательные поля:
   - `username` = номер телефона
   - `profile.given_name` = номер телефона
   - `profile.family_name` = номер телефона
   - `email` = `{phone_without_plus}@phone.local` (автоверифицирован)
   - `phone` = номер телефона
3. **Код верификации**: В ответе возвращается `phone_code` для верификации
4. **Верификация**: Пользователь может подтвердить телефон с помощью кода

## Следующие шаги

- Интегрируйте SMS-провайдера для автоматической отправки кодов
- Настройте ActionsV2 для кастомной логики (см. README.md)
- Добавьте rate limiting для защиты от злоупотреблений
- Настройте логирование и мониторинг
