package domain

// CreateUserRequest - запрос на создание пользователя
type CreateUserRequest struct {
	Phone string `json:"phone" validate:"required,e164"`
}

// CreateUserResponse - ответ на создание пользователя
type CreateUserResponse struct {
	Success   bool   `json:"success"`
	UserID    string `json:"user_id"`
	PhoneCode string `json:"phone_code,omitempty"`
	Message   string `json:"message"`
}

// VerifyPhoneRequest - запрос на верификацию телефона
type VerifyPhoneRequest struct {
	UserID string `json:"user_id" validate:"required"`
	Code   string `json:"code" validate:"required"`
}

// VerifyPhoneResponse - ответ на верификацию
type VerifyPhoneResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ResendCodeRequest - запрос на повторную отправку кода
type ResendCodeRequest struct {
	UserID string `json:"user_id" validate:"required"`
}

// ResendCodeResponse - ответ на повторную отправку
type ResendCodeResponse struct {
	Success   bool   `json:"success"`
	PhoneCode string `json:"phone_code,omitempty"`
	Message   string `json:"message"`
}
