package domain

import "errors"

var (
	// Auth errors
	ErrInvalidPhone   = errors.New("invalid phone number")
	ErrInvalidOTP     = errors.New("invalid OTP code")
	ErrOTPExpired     = errors.New("OTP code has expired")
	ErrOTPMaxAttempts = errors.New("maximum OTP attempts exceeded")
	ErrOTPNotFound    = errors.New("OTP code not found for this phone number")

	// User errors
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user already exists")

	// Validation errors
	ErrPhoneRequired  = errors.New("phone number is required")
	ErrCodeRequired   = errors.New("verification code is required")
	ErrUserIDRequired = errors.New("user ID is required")

	// Webhook errors
	ErrPhoneBlacklisted = errors.New("this phone number is not allowed")
	ErrPhoneNotAllowed  = errors.New("only Russian phone numbers are allowed")
	ErrPhoneNotFound    = errors.New("phone number not found in request")
)
