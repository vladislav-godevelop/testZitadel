package service

import (
	"sync"
	"time"
)

// OTPVerificationStore хранит статус подтверждения OTP для OIDC flow
type OTPVerificationStore struct {
	mu            sync.RWMutex
	verifications map[string]*OTPVerification // key: phone number
}

// OTPVerification статус верификации OTP
type OTPVerification struct {
	Phone      string
	Verified   bool
	VerifiedAt time.Time
	ExpiresAt  time.Time
}

// NewOTPVerificationStore создает новое хранилище верификаций
func NewOTPVerificationStore() *OTPVerificationStore {
	store := &OTPVerificationStore{
		verifications: make(map[string]*OTPVerification),
	}

	// Запускаем фоновую очистку истекших верификаций
	go store.cleanupExpired()

	return store
}

// MarkAsVerified помечает телефон как верифицированный через OTP
func (s *OTPVerificationStore) MarkAsVerified(phone string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.verifications[phone] = &OTPVerification{
		Phone:      phone,
		Verified:   true,
		VerifiedAt: time.Now(),
		ExpiresAt:  time.Now().Add(10 * time.Minute), // Верификация действительна 10 минут
	}
}

// IsVerified проверяет, был ли телефон верифицирован через OTP
func (s *OTPVerificationStore) IsVerified(phone string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	verification, exists := s.verifications[phone]
	if !exists {
		return false
	}

	// Проверяем, не истекла ли верификация
	if time.Now().After(verification.ExpiresAt) {
		return false
	}

	return verification.Verified
}

// ConsumeVerification использует верификацию (удаляет после использования)
func (s *OTPVerificationStore) ConsumeVerification(phone string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	verification, exists := s.verifications[phone]
	if !exists {
		return false
	}

	// Проверяем валидность
	if time.Now().After(verification.ExpiresAt) || !verification.Verified {
		delete(s.verifications, phone)
		return false
	}

	// Удаляем верификацию после использования
	delete(s.verifications, phone)
	return true
}

// cleanupExpired периодически очищает истекшие верификации
func (s *OTPVerificationStore) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for phone, verification := range s.verifications {
			if now.After(verification.ExpiresAt) {
				delete(s.verifications, phone)
			}
		}
		s.mu.Unlock()
	}
}
