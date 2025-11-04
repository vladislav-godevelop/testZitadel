package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// OTPStore хранилище OTP кодов в памяти
type OTPStore struct {
	mu    sync.RWMutex
	codes map[string]*OTPData // ключ - номер телефона
}

// OTPData информация об OTP коде
type OTPData struct {
	Code      string
	ExpiresAt time.Time
	Attempts  int
}

// NewOTPStore создает новое хранилище OTP
func NewOTPStore() *OTPStore {
	store := &OTPStore{
		codes: make(map[string]*OTPData),
	}

	// Запускаем очистку истекших кодов каждые 5 минут
	go store.cleanupExpired()

	return store
}

// GenerateOTP генерирует 6-значный OTP код
func (s *OTPStore) GenerateOTP(phone string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Генерируем 6-значный код
	code := generateRandomCode(6)

	// Сохраняем код с временем истечения 5 минут
	s.codes[phone] = &OTPData{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Attempts:  0,
	}

	return code, nil
}

// VerifyOTP проверяет OTP код
func (s *OTPStore) VerifyOTP(phone, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	otpData, exists := s.codes[phone]
	if !exists {
		return fmt.Errorf("OTP code not found for this phone number")
	}

	// Проверяем истечение срока
	if time.Now().After(otpData.ExpiresAt) {
		delete(s.codes, phone)
		return fmt.Errorf("OTP code has expired")
	}

	// Проверяем количество попыток
	if otpData.Attempts >= 3 {
		delete(s.codes, phone)
		return fmt.Errorf("too many failed attempts")
	}

	// Проверяем код
	if otpData.Code != code {
		otpData.Attempts++
		return fmt.Errorf("invalid OTP code")
	}

	// Успешная верификация - удаляем код
	delete(s.codes, phone)
	return nil
}

// DeleteOTP удаляет OTP код для номера
func (s *OTPStore) DeleteOTP(phone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.codes, phone)
}

// cleanupExpired очищает истекшие коды
func (s *OTPStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for phone, data := range s.codes {
			if now.After(data.ExpiresAt) {
				delete(s.codes, phone)
			}
		}
		s.mu.Unlock()
	}
}

// generateRandomCode генерирует случайный числовой код заданной длины
func generateRandomCode(length int) string {
	const digits = "0123456789"
	code := make([]byte, length)

	for i := range code {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		code[i] = digits[num.Int64()]
	}

	return string(code)
}
