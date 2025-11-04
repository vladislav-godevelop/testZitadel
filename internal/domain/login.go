package domain

// LoginSendOTPRequest - запрос на отправку OTP для входа
type LoginSendOTPRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
}

// LoginSendOTPResponse - ответ на отправку OTP для входа
type LoginSendOTPResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"` // Только для dev/test
}

// LoginVerifyOTPRequest - запрос на вход с OTP
type LoginVerifyOTPRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
	Code  string `json:"code" validate:"required,len=6"`
}

// LoginVerifyOTPResponse - ответ с токенами или authorization URL после успешного входа
type LoginVerifyOTPResponse struct {
	Success      bool   `json:"success"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	UserID       string `json:"user_id"`

	// Для OIDC flow
	AuthorizationURL string `json:"authorization_url,omitempty"`
	State            string `json:"state,omitempty"`
}

// RefreshTokenRequest - запрос на обновление токена
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// RefreshTokenResponse - ответ с новыми токенами
type RefreshTokenResponse struct {
	Success      bool   `json:"success"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}
