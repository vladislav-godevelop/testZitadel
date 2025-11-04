package service

import (
	"context"
	"fmt"
	"log"
	"strings"

	session "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/session/v2beta"
	v2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
)

// LoginResponse содержит данные для успешного входа
type LoginResponse struct {
	SessionID    string `json:"session_id"`
	SessionToken string `json:"session_token"`
	UserID       string `json:"user_id"`
}

// LoginByPhone осуществляет вход по номеру телефона
// Возвращает session token, который можно использовать для OIDC flow
func (s *ZitadelService) LoginByPhone(ctx context.Context, phone string) (*LoginResponse, error) {
	// 1. Находим пользователя по номеру телефона
	userID, err := s.FindUserByPhone(ctx, phone)
	if err != nil {
		return nil, err
	}

	log.Printf("Found user by phone %s: UserID=%s", phone, userID)

	// 2. Создаем сессию для пользователя
	sessionResp, err := s.client.SessionService().CreateSession(ctx, &session.CreateSessionRequest{
		Checks: &session.Checks{
			User: &session.CheckUser{
				Search: &session.CheckUser_UserId{
					UserId: userID,
				},
			},
		},
		Metadata: map[string][]byte{
			"phone":        []byte(phone),
			"login_method": []byte("phone_otp"),
		},
	})

	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	log.Printf("Session created: SessionID=%s, UserID=%s", sessionResp.GetDetails().GetSequence(), userID)

	return &LoginResponse{
		SessionID:    sessionResp.GetSessionId(),
		SessionToken: sessionResp.GetSessionToken(),
		UserID:       userID,
	}, nil
}

// TokenExchangeResponse структура ответа от Zitadel Token Exchange
type TokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// ExchangeSessionForTokens возвращает session_token как access_token
// Session token от Zitadel уже является валидным Bearer токеном для API
func (s *ZitadelService) ExchangeSessionForTokens(ctx context.Context, sessionToken, sessionID string) (*TokenExchangeResponse, error) {
	// Session token от Zitadel.CreateSession() уже можно использовать как Bearer token
	// для аутентификации в Zitadel APIs
	// Документация: https://zitadel.com/docs/apis/resources/session_service_v2beta/session-service-create-session

	log.Printf("Using Zitadel session_token as access_token (session_id=%s)", sessionID)

	// Session token действителен и может использоваться для:
	// 1. OIDC authorization (передается в /oauth/v2/authorize)
	// 2. Прямых вызовов Zitadel API с заголовком Authorization: Bearer {session_token}

	return &TokenExchangeResponse{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: "",   // Refresh token нужно получать через OIDC flow
		ExpiresIn:    3600, // Session обычно живет 1 час
		IDToken:      "",   // ID token получается через OIDC flow
		Scope:        "openid profile email phone",
	}, nil
}

// FindUserByPhone находит пользователя по номеру телефона
func (s *ZitadelService) FindUserByPhone(ctx context.Context, phone string) (string, error) {
	// Нормализуем номер телефона
	normalizedPhone := strings.TrimSpace(phone)

	// Ищем пользователя по phone используя username (т.к. username = phone)
	listResp, err := s.client.UserServiceV2().ListUsers(ctx, &v2.ListUsersRequest{
		Queries: []*v2.SearchQuery{
			{
				Query: &v2.SearchQuery_UserNameQuery{
					UserNameQuery: &v2.UserNameQuery{
						UserName: normalizedPhone,
					},
				},
			},
		},
	})

	if err != nil {
		return "", fmt.Errorf("failed to search user by phone: %w", err)
	}

	if listResp.GetDetails() == nil || listResp.GetDetails().GetTotalResult() == 0 {
		return "", fmt.Errorf("user not found")
	}

	// Берем первого пользователя из результатов
	if len(listResp.GetResult()) == 0 {
		return "", fmt.Errorf("user not found")
	}

	userID := listResp.GetResult()[0].GetUserId()
	log.Printf("User found by phone %s: UserID=%s", normalizedPhone, userID)

	return userID, nil
}
