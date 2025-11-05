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
)

type OIDCService struct {
	clientID                  string
	clientSecret              string
	introspectionClientID     string // Client ID для API application (introspection)
	introspectionClientSecret string // Client Secret для API application (introspection)
	issuer                    string
	tokenURL                  string
	httpClient                *http.Client
}

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
}

func NewOIDCService() (*OIDCService, error) {
	zitadelDomain := os.Getenv("ZITADEL_DOMAIN")
	clientID := os.Getenv("ZITADEL_CLIENT_ID")
	clientSecret := os.Getenv("ZITADEL_CLIENT_SECRET")

	// Credentials для introspection (API application)
	introspectionClientID := os.Getenv("ZITADEL_INTROSPECTION_CLIENT_ID")
	introspectionClientSecret := os.Getenv("ZITADEL_INTROSPECTION_CLIENT_SECRET")

	if clientID == "" {
		return nil, fmt.Errorf("ZITADEL_CLIENT_ID is required")
	}

	// Формируем issuer URL
	issuer := fmt.Sprintf("http://%s:8080", zitadelDomain)
	tokenURL := fmt.Sprintf("%s/oauth/v2/token", issuer)

	return &OIDCService{
		clientID:                  clientID,
		clientSecret:              clientSecret,
		introspectionClientID:     introspectionClientID,
		introspectionClientSecret: introspectionClientSecret,
		issuer:                    issuer,
		tokenURL:                  tokenURL,
		httpClient:                &http.Client{},
	}, nil
}

// ExchangeUserIDForTokens использует Token Exchange с impersonation для получения OAuth токенов
// Требует:
// 1. Token Exchange feature включен в Zitadel (v2.49+)
// 2. Impersonation включен в security settings приложения
// 3. Service account token (PAT или Client Credentials) как actor_token
// https://zitadel.com/docs/guides/integrate/token-exchange
func (s *OIDCService) ExchangeUserIDForTokens(ctx context.Context, userID, actorToken string) (*TokenResponse, error) {
	log.Printf("Exchanging user ID for OAuth tokens via Token Exchange (impersonation)")

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

// IntrospectToken проверяет валидность токена через introspection endpoint
func (s *OIDCService) IntrospectToken(ctx context.Context, token string) (*IntrospectionResponse, error) {
	introspectURL := fmt.Sprintf("%s/oauth/v2/introspect", s.issuer)

	data := url.Values{}
	data.Set("token", token)

	req, err := http.NewRequestWithContext(ctx, "POST", introspectURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create introspect request: %w", err)
	}

	// credentials для API application (introspection)
	req.SetBasicAuth(s.introspectionClientID, s.introspectionClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Printf("Request headers: Authorization=Basic %s:***", s.introspectionClientID)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to introspect token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		log.Printf("Token introspection failed: status=%d, body=%s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("introspection failed with status %d: %s", resp.StatusCode, string(body))
	}

	var introspectResp IntrospectionResponse
	if err := json.Unmarshal(body, &introspectResp); err != nil {
		return nil, fmt.Errorf("failed to parse introspection response: %w", err)
	}

	log.Printf("Token introspection successful: active=%v, sub=%s",
		introspectResp.Active, introspectResp.Subject)

	return &introspectResp, nil
}
