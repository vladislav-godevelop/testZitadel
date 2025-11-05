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
	relyingParty    rp.RelyingParty
	clientID        string
	clientSecret    string
	redirectURI     string
	issuer          string
	tokenURL        string
	authorizeURL    string
	httpClient      *http.Client
	codeVerifierMap map[string]string // state -> code_verifier для PKCE
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
	authorizeURL := fmt.Sprintf("%s/oauth/v2/authorize", issuer)

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
		relyingParty:    rp,
		clientID:        clientID,
		clientSecret:    clientSecret,
		redirectURI:     redirectURI,
		issuer:          issuer,
		tokenURL:        tokenURL,
		authorizeURL:    authorizeURL,
		httpClient:      &http.Client{},
		codeVerifierMap: make(map[string]string),
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

// ExchangeUserIDForTokens использует Token Exchange с impersonation для получения OAuth токенов
// Требует:
// 1. Token Exchange feature включен в Zitadel (v2.49+)
// 2. Impersonation включен в security settings приложения
// 3. Service account token (PAT или Client Credentials) как actor_token
// https://zitadel.com/docs/guides/integrate/token-exchange
func (s *OIDCService) ExchangeUserIDForTokens(ctx context.Context, userID, actorToken string) (*TokenResponse, error) {
	log.Printf("🔄 Exchanging user ID for OAuth tokens via Token Exchange (impersonation)")

	// Token Exchange с impersonation согласно RFC 8693
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:token-exchange")
	data.Set("subject_token", userID) // User ID напрямую
	data.Set("subject_token_type", "urn:zitadel:params:oauth:token-type:user_id")
	data.Set("actor_token", actorToken) // Токен service account (PAT)
	data.Set("actor_token_type", "urn:ietf:params:oauth:token-type:access_token")
	data.Set("scope", "openid profile email phone offline_access")
	// Запрашиваем JWT токен
	data.Set("requested_token_type", "urn:ietf:params:oauth:token-type:jwt")

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	// Basic Auth с client credentials
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Printf("Token exchange request: subject=%s (user_id), actor_token present", userID)

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

	log.Printf("User ID exchanged for OAuth tokens successfully")
	log.Printf("access_token: %s..., expires_in: %d", tokenResp.AccessToken[:20], tokenResp.ExpiresIn)

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

	log.Printf("Access token refreshed successfully")
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

// GetAuthorizationCodeWithSession получает authorization code через session token
// Это серверный метод, который выполняет Authorization Code Flow от имени пользователя
func (s *OIDCService) GetAuthorizationCodeWithSession(ctx context.Context, sessionToken string) (string, error) {
	// Генерируем PKCE параметры
	codeVerifier, err := generateRandomString(64)
	if err != nil {
		return "", fmt.Errorf("failed to generate code verifier: %w", err)
	}

	codeChallenge := base64.RawURLEncoding.EncodeToString([]byte(codeVerifier))

	// Генерируем state
	state, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	// Сохраняем code_verifier для последующего использования
	s.codeVerifierMap[state] = codeVerifier

	// Формируем параметры запроса
	params := url.Values{}
	params.Set("client_id", s.clientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", s.redirectURI)
	params.Set("scope", "openid profile email phone offline_access")
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "plain")
	params.Set("sessionToken", sessionToken) // Передаем session token

	authURL := fmt.Sprintf("%s?%s", s.authorizeURL, params.Encode())

	log.Printf("Requesting authorization with session token: url=%s", authURL)

	// Выполняем запрос к authorization endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", authURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create authorization request: %w", err)
	}

	// Настраиваем клиент, чтобы НЕ следовать редиректам автоматически
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform authorization request: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("Authorization response: status=%d, headers=%v", resp.StatusCode, resp.Header)

	// Ожидаем редирект (302/303)
	if resp.StatusCode != http.StatusFound && resp.StatusCode != http.StatusSeeOther {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("Unexpected authorization response: status=%d, body=%s", resp.StatusCode, string(body))
		return "", fmt.Errorf("authorization failed with status %d", resp.StatusCode)
	}

	// Извлекаем location из заголовка
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no location header in authorization response")
	}

	log.Printf("Authorization redirect location: %s", location)

	// Парсим URL и извлекаем code
	redirectURL, err := url.Parse(location)
	if err != nil {
		return "", fmt.Errorf("failed to parse redirect URL: %w", err)
	}

	// Детальное логирование для дебага
	log.Printf("🔍 Redirect URL query params: %v", redirectURL.Query())

	code := redirectURL.Query().Get("code")
	if code == "" {
		// Проверяем, есть ли ошибка в редиректе
		errorCode := redirectURL.Query().Get("error")
		errorDesc := redirectURL.Query().Get("error_description")
		if errorCode != "" {
			return "", fmt.Errorf("authorization error: %s - %s", errorCode, errorDesc)
		}
		return "", fmt.Errorf("no authorization code in redirect URL (location: %s)", location)
	}

	log.Printf("✅ Authorization code received: %s", code[:10]+"...")

	return code, nil
}

// ExchangeAuthorizationCode обменивает authorization code на OAuth токены с PKCE
func (s *OIDCService) ExchangeAuthorizationCode(ctx context.Context, code, state string) (*TokenResponse, error) {
	// Получаем code_verifier из map
	codeVerifier, exists := s.codeVerifierMap[state]
	if !exists {
		return nil, fmt.Errorf("code verifier not found for state")
	}

	// Удаляем использованный state
	delete(s.codeVerifierMap, state)

	// Формируем запрос token exchange
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", s.redirectURI)
	data.Set("client_id", s.clientID)
	data.Set("code_verifier", codeVerifier)

	req, err := http.NewRequestWithContext(ctx, "POST", s.tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	// Basic Auth
	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
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

	log.Printf("Authorization code exchanged for tokens successfully")

	return &tokenResp, nil
}

// GetTokensFromSessionToken - полный flow: session token -> authorization code -> OAuth tokens
// Это высокоуровневый метод, который объединяет GetAuthorizationCodeWithSession и ExchangeAuthorizationCode
func (s *OIDCService) GetTokensFromSessionToken(ctx context.Context, sessionToken, state string) (*TokenResponse, error) {
	log.Printf("Starting full OAuth flow from session token")

	// Получаем authorization code через session token
	code, err := s.GetAuthorizationCodeWithSession(ctx, sessionToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get authorization code: %w", err)
	}

	log.Printf("Step 1/2: Authorization code obtained")

	// Обмениваем code на OAuth токены
	tokens, err := s.ExchangeAuthorizationCode(ctx, code, state)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}

	log.Printf("Step 2/2: OAuth tokens obtained successfully")

	return tokens, nil
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
