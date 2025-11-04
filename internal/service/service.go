package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/zitadel/zitadel-go/v3/pkg/client"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/session/v2"
	v2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
)

type ZitadelService struct {
	client        *client.Client
	zitadelDomain string
}

type CreateUserRequest struct {
	Phone string `json:"phone"`
}

type CreateUserResponse struct {
	UserID    string `json:"user_id"`
	PhoneCode string `json:"phone_code,omitempty"`
}

// SessionTokenResponse - ответ с токенами сессии
type SessionTokenResponse struct {
	SessionToken string `json:"session_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}

// IntrospectionResponse - ответ от Zitadel introspection endpoint
type IntrospectionResponse struct {
	Active    bool   `json:"active"`
	Subject   string `json:"sub"`
	Username  string `json:"username"`
	TokenType string `json:"token_type"`
	ExpiresAt int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	ClientID  string `json:"client_id"`
}

// NewZitadelService создает новый сервис для работы с Zitadel
func NewZitadelService() (*ZitadelService, error) {
	ctx := context.Background()

	// Получаем конфигурацию из переменных окружения
	zitadelDomain := os.Getenv("ZITADEL_DOMAIN")
	if zitadelDomain == "" {
		return nil, fmt.Errorf("ZITADEL_DOMAIN environment variable is not set")
	}

	// Проверяем наличие PAT (Personal Access Token)
	pat := os.Getenv("ACCES_TOKEN_SERVICE_ACCOUNT")
	keyPath := os.Getenv("ZITADEL_KEY_PATH")

	if pat == "" && keyPath == "" {
		return nil, fmt.Errorf("either ACCES_TOKEN_SERVICE_ACCOUNT or ZITADEL_KEY_PATH must be set")
	}

	// Инициализируем Zitadel instance
	// Для localhost используем insecure соединение
	var zitadelInstance *zitadel.Zitadel
	if zitadelDomain == "homelab.localhost" || zitadelDomain == "localhost" {
		zitadelInstance = zitadel.New(zitadelDomain, zitadel.WithInsecure("8080"))
		log.Printf("Using insecure connection for %s", zitadelDomain)
	} else {
		zitadelInstance = zitadel.New(zitadelDomain)
	}

	// Выбираем метод аутентификации
	var authOption client.Option
	if pat != "" {
		// Используем Personal Access Token
		authOption = client.WithAuth(client.PAT(pat))
		log.Printf("Using Personal Access Token authentication")
	} else {
		// Используем JWT key file
		authOption = client.WithAuth(client.DefaultServiceUserAuthentication(
			keyPath,
			client.ScopeZitadelAPI(),
		))
		log.Printf("Using JWT key file authentication")
	}

	// Создаем client
	zitadelClient, err := client.New(
		ctx,
		zitadelInstance,
		authOption,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create zitadel client: %w", err)
	}

	log.Printf("Zitadel client initialized for domain: %s", zitadelDomain)

	return &ZitadelService{
		client:        zitadelClient,
		zitadelDomain: zitadelDomain,
	}, nil
}

// CreateUserByPhone создает пользователя в Zitadel используя только номер телефона
func (s *ZitadelService) CreateUserByPhone(ctx context.Context, phone string) (*CreateUserResponse, error) {
	// Валидация номера телефона
	if phone == "" {
		return nil, fmt.Errorf("phone number is required")
	}

	// Генерируем email на основе телефона (обязательное поле в Zitadel)
	// Формат: +79991234567 -> 79991234567@phone.local
	sanitizedPhone := phone
	if phone[0] == '+' {
		sanitizedPhone = phone[1:]
	}
	email := fmt.Sprintf("%s@phone.local", sanitizedPhone)

	// Получаем Organization ID из переменных окружения
	orgID := os.Getenv("ZITADEL_ORG_ID")
	if orgID == "" {
		return nil, fmt.Errorf("ZITADEL_ORG_ID environment variable is required")
	}

	// Создаем пользователя через UserServiceV2 (GA) используя CreateUser
	username := phone
	resp, err := s.client.UserServiceV2().CreateUser(ctx, &v2.CreateUserRequest{
		OrganizationId: orgID,     // ID организации
		Username:       &username, // Username = номер телефона
		UserType: &v2.CreateUserRequest_Human_{
			Human: &v2.CreateUserRequest_Human{
				Profile: &v2.SetHumanProfile{
					GivenName:  phone, // Используем телефон как имя
					FamilyName: phone, // Используем телефон как фамилию
				},
				Email: &v2.SetHumanEmail{
					Email: email,
					Verification: &v2.SetHumanEmail_IsVerified{
						IsVerified: true, // Email верифицирован (fake email)
					},
				},
				Phone: &v2.SetHumanPhone{
					Phone: phone,
					Verification: &v2.SetHumanPhone_IsVerified{
						IsVerified: true, // Телефон уже верифицирован через OTP
					},
				},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create user in zitadel: %w", err)
	}

	log.Printf("User created successfully: UserID=%s, Phone=%s", resp.Id, phone)

	return &CreateUserResponse{
		UserID:    resp.Id,
		PhoneCode: resp.GetPhoneCode(), // Код верификации для телефона
	}, nil
}

// VerifyPhone верифицирует номер телефона пользователя
func (s *ZitadelService) VerifyPhone(ctx context.Context, userID, verificationCode string) error {
	_, err := s.client.UserServiceV2().VerifyPhone(ctx, &v2.VerifyPhoneRequest{
		UserId:           userID,
		VerificationCode: verificationCode,
	})

	if err != nil {
		return fmt.Errorf("failed to verify phone: %w", err)
	}

	log.Printf("Phone verified successfully for user: %s", userID)
	return nil
}

// ResendPhoneCode повторно отправляет код верификации
func (s *ZitadelService) ResendPhoneCode(ctx context.Context, userID string) (*v2.ResendPhoneCodeResponse, error) {
	resp, err := s.client.UserServiceV2().ResendPhoneCode(ctx, &v2.ResendPhoneCodeRequest{
		UserId: userID,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to resend phone code: %w", err)
	}

	log.Printf("Phone code resent for user: %s", userID)
	return resp, nil
}

// GetUserByPhone ищет пользователя по номеру телефона
func (s *ZitadelService) GetUserByPhone(ctx context.Context, phone string) (string, error) {
	// Username = phone number в нашем случае
	username := phone

	resp, err := s.client.UserServiceV2().ListUsers(ctx, &v2.ListUsersRequest{
		Queries: []*v2.SearchQuery{
			{
				Query: &v2.SearchQuery_UserNameQuery{
					UserNameQuery: &v2.UserNameQuery{
						UserName: username,
					},
				},
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to find user by phone: %w", err)
	}

	if len(resp.Result) == 0 {
		return "", fmt.Errorf("user not found with phone: %s", phone)
	}

	userID := resp.Result[0].UserId
	log.Printf("Found user by phone %s: UserID=%s", phone, userID)
	return userID, nil
}

// CreateSessionForUser создает сессию для пользователя после OTP верификации
func (s *ZitadelService) CreateSessionForUser(ctx context.Context, userID string) (*SessionTokenResponse, error) {
	// Создаем сессию с проверенным телефоном
	resp, err := s.client.SessionServiceV2().CreateSession(ctx, &session.CreateSessionRequest{
		Checks: &session.Checks{
			User: &session.CheckUser{
				Search: &session.CheckUser_UserId{
					UserId: userID,
				},
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	sessionToken := resp.SessionToken
	sessionID := resp.SessionId

	expiresIn := 3600 // 1 час по умолчанию

	log.Printf("Session created for user %s: session_id=%s, session_token=%s",
		userID, sessionID, sessionToken[:20]+"...")

	// Session token от Zitadel можно использовать как refresh token
	// т.к. он долгоживущий и может быть обновлен
	return &SessionTokenResponse{
		SessionToken: sessionToken,
		RefreshToken: sessionToken, // Используем session token как refresh token
		ExpiresIn:    expiresIn,
	}, nil
}

// IntrospectToken проверяет токен через Zitadel introspection endpoint
func (s *ZitadelService) IntrospectToken(ctx context.Context, token string) (*IntrospectionResponse, error) {
	// Формируем URL introspection endpoint
	introspectURL := fmt.Sprintf("http://%s:8080/oauth/v2/introspect", s.zitadelDomain)

	// Получаем client credentials из env
	clientID := os.Getenv("ZITADEL_CLIENT_ID")
	clientSecret := os.Getenv("ZITADEL_CLIENT_SECRET")

	if clientID == "" || clientSecret == "" {
		return nil, fmt.Errorf("ZITADEL_CLIENT_ID and ZITADEL_CLIENT_SECRET are required for token introspection")
	}

	// Подготавливаем данные для запроса
	data := url.Values{}
	data.Set("token", token)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	// Создаем HTTP запрос
	req, err := http.NewRequestWithContext(ctx, "POST", introspectURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspect request: %w", err)
	}

	// Устанавливаем заголовки
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Выполняем запрос
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect token: %w", err)
	}
	defer resp.Body.Close()

	// Читаем ответ
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read introspect response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("introspect failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Парсим JSON ответ
	var introspectResp IntrospectionResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspect response: %w", err)
	}

	log.Printf("Token introspection: active=%v, subject=%s", introspectResp.Active, introspectResp.Subject)

	return &introspectResp, nil
}
