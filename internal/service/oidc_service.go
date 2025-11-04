package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

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
	tokenURL     string
	httpClient   *http.Client
}

// TokenResponse структура ответа с токенами
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
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
	tokenURL := fmt.Sprintf("%s/oauth/v2/token", issuer)

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
		[]string{oidc.ScopeOpenID, oidc.ScopeProfile, oidc.ScopeEmail, oidc.ScopePhone, oidc.ScopeOfflineAccess},
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
		tokenURL:     tokenURL,
		httpClient:   &http.Client{},
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
	authURL = authURL + "&login_hint=" + url.QueryEscape(phone)

	return authURL, state, nil
}

// ExchangeCode обменивает authorization code на токены
func (s *OIDCService) ExchangeCode(ctx context.Context, code string) (*oidc.Tokens[*oidc.IDTokenClaims], *oidc.IDTokenClaims, error) {
	tokens, err := rp.CodeExchange[*oidc.IDTokenClaims](ctx, code, s.relyingParty)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to exchange code: %w", err)
	}

	claims, err := rp.VerifyTokens[*oidc.IDTokenClaims](ctx, tokens.AccessToken, tokens.IDToken, s.relyingParty.IDTokenVerifier())
	if err != nil {
		return nil, nil, fmt.Errorf("failed to verify tokens: %w", err)
	}

	log.Printf("✅ Token exchange successful: user_id=%s", claims.Subject)

	return tokens, claims, nil
}

// ExchangeSessionToken обменивает session token на OAuth токены
// Используется для получения access_token после создания сессии
func (s *OIDCService) ExchangeSessionToken(ctx context.Context, sessionToken, sessionID string) (*TokenResponse, error) {
	// Token Exchange согласно RFC 8693
	// https://zitadel.com/docs/apis/openidoauth/grant-types#jwt-bearer-token-exchange

	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	data.Set("subject_token", sessionToken)
	data.Set("subject_token_type", "urn:ietf:params:oauth:token-type:access_token")
	data.Set("requested_token_type", "urn:ietf:params:oauth:token-type:refresh_token")
	data.Set("scope", "openid profile email phone offline_access")

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	// Basic Auth с client credentials
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("Token exchange failed: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("token exchange failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	log.Printf("✅ Session token exchanged successfully")
	return &tokenResp, nil
}

// RefreshAccessToken обновляет access token используя refresh token
func (s *OIDCService) RefreshAccessToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("scope", "openid profile email phone offline_access")

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create refresh request: %w", err)
	}

	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to refresh token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("Token refresh failed: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse refresh response: %w", err)
	}

	log.Printf("✅ Access token refreshed successfully")
	return &tokenResp, nil
}

// IntrospectToken проверяет валидность токена через introspection endpoint
func (s *OIDCService) IntrospectToken(ctx context.Context, token string) (*IntrospectionResponse, error) {
	introspectURL := fmt.Sprintf("%s/oauth/v2/introspect", s.issuer)

	data := url.Values{}
	data.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, "POST", introspectURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspect request: %w", err)
	}

	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("Token introspection failed: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("introspection failed with status %d", resp.StatusCode)
	}

	var introspectResp IntrospectionResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	return &introspectResp, nil
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
