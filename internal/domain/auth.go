package domain

// SendOTPRequest - запрос на отправку OTP кода
type SendOTPRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
}

// RegisterWithOTPRequest - запрос на регистрацию с OTP
type RegisterWithOTPRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
	Code  string `json:"code" validate:"required,len=6"`
}

// SendOTPResponse - ответ на отправку OTP
type SendOTPResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"` // Только для dev/test
}

// RegisterWithOTPResponse - ответ на регистрацию
type RegisterWithOTPResponse struct {
	Success bool   `json:"success"`
	UserID  string `json:"user_id"`
	Message string `json:"message"`
}
