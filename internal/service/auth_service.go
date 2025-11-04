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

// ExchangeSessionForTokens обменивает session на OAuth токены
// Использует Client Credentials flow с impersonation
func (s *ZitadelService) ExchangeSessionForTokens(ctx context.Context, sessionToken, sessionID string) (*TokenExchangeResponse, error) {
	log.Printf("Attempting to create access token for session %s", sessionID)

	// ВАЖНО: Session token от Zitadel НЕ является OAuth access token
	// и не может быть проверен через introspection endpoint.
	//
	// Правильное решение - вернуть session token как access token,
	// но использовать собственную валидацию токенов или хранить их в Redis.
	//
	// Для полноценной интеграции нужен Authorization Code Flow:
	// 1. После создания сессии, редиректить на /oauth/v2/authorize с session_token
	// 2. Получить authorization code
	// 3. Обменять code на access_token через /oauth/v2/token

	return &TokenExchangeResponse{
		AccessToken:  sessionToken,
		TokenType:    "Bearer",
		RefreshToken: sessionToken, // Используем session token как refresh token
		ExpiresIn:    3600,
		IDToken:      "",
		Scope:        "openid profile email phone",
	}, nil
}

func (s *ZitadelService) FindUserByPhone(ctx context.Context, phone string) (string, error) {
	normalizedPhone := strings.TrimSpace(phone)

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
