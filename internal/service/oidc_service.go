package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/zitadel/oidc/v3/pkg/client/rp"
	"github.com/zitadel/oidc/v3/pkg/oidc"
)

// OIDCService управляет OIDC аутентификацией с Zitadel
type OIDCService struct {
	relyingParty rp.RelyingParty
	clientID     string
	clientSecret string
	redirectURI  string
	issuer       string
}

// NewOIDCService создает новый OIDC сервис
func NewOIDCService() (*OIDCService, error) {
	zitadelDomain := os.Getenv("ZITADEL_DOMAIN")
	clientID := os.Getenv("ZITADEL_CLIENT_ID")
	clientSecret := os.Getenv("ZITADEL_CLIENT_SECRET")
	redirectURI := os.Getenv("ZITADEL_REDIRECT_URI")

	if clientID == "" {
		return nil, fmt.Errorf("ZITADEL_CLIENT_ID is required")
	}

	if redirectURI == "" {
		redirectURI = "http://localhost:2222/api/auth/callback"
	}

	// Формируем issuer URL
	issuer := fmt.Sprintf("http://%s:8080", zitadelDomain)

	log.Printf("Initializing OIDC service: issuer=%s, client_id=%s, redirect_uri=%s",
		issuer, clientID, redirectURI)

	// Создаем Relying Party (клиент OIDC)
	ctx := context.Background()

	rp, err := rp.NewRelyingPartyOIDC(
		ctx,
		issuer,
		clientID,
		clientSecret,
		redirectURI,
		[]string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopePhone},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC relying party: %w", err)
	}

	log.Println("✅ OIDC service initialized successfully")

	return &OIDCService{
		relyingParty: rp,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		issuer:       issuer,
	}, nil
}

// GetAuthorizationURL возвращает URL для начала OIDC flow
func (s *OIDCService) GetAuthorizationURL(phone string) (string, string, error) {
	// Генерируем state для защиты от CSRF
	state, err := generateRandomString(32)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Формируем authorization URL с login_hint (номер телефона)
	authURL := rp.AuthURL(state, s.relyingParty)

	// Добавляем login_hint с URL encoding (+ должен быть закодирован как %2B)
	authURL = authURL + "&login_hint=" + url.QueryEscape(phone)

	return authURL, state, nil
}

// ExchangeCode обменивает authorization code на токены
func (s *OIDCService) ExchangeCode(ctx context.Context, code string) (*oidc.Tokens[*oidc.IDTokenClaims], *oidc.IDTokenClaims, error) {
	// Обмениваем code на токены
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, s.relyingParty)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	// Получаем ID token claims
	claims, err := rp.VerifyTokens[*oidc.IDTokenClaims](ctx, tokens.AccessToken, tokens.IDToken, s.relyingParty.IDTokenVerifier())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify tokens: %w", err)
	}

	log.Printf("✅ Token exchange successful: user_id=%s", claims.Subject)

	return tokens, claims, nil
}

// GetUserInfo получает информацию о пользователе по access token
func (s *OIDCService) GetUserInfo(ctx context.Context, accessToken, subject string) (*oidc.UserInfo, error) {
	userInfo, err := rp.Userinfo[*oidc.UserInfo](ctx, accessToken, oidc.BearerToken, subject, s.relyingParty)
	if err != nil {
		return nil, fmt.Errorf("failed to get user info: %w", err)
	}

	return userInfo, nil
}

// generateRandomString генерирует случайную строку заданной длины
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
