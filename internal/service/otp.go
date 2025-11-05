package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sync"
	"time"
)

type OTPStore struct {
	mu    sync.RWMutex
	codes map[string]*OTPData // ключ - номер телефона
}

type OTPData struct {
	Code      string
	ExpiresAt time.Time
	Attempts  int
}

func NewOTPStore() *OTPStore {
	store := &OTPStore{
		codes: make(map[string]*OTPData),
	}

	go store.cleanupExpired()

	return store
}

func (s *OTPStore) GenerateOTP(phone string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	code := generateRandomCode(6)

	s.codes[phone] = &OTPData{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Attempts:  0,
	}

	return code, nil
}

func (s *OTPStore) VerifyOTP(phone, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	otpData, exists := s.codes[phone]
	if !exists {
		return fmt.Errorf("OTP code not found for this phone number")
	}

	if time.Now().After(otpData.ExpiresAt) {
		delete(s.codes, phone)
		return fmt.Errorf("OTP code has expired")
	}

	if otpData.Attempts >= 3 {
		delete(s.codes, phone)
		return fmt.Errorf("too many failed attempts")
	}

	if otpData.Code != code {
		otpData.Attempts++
		return fmt.Errorf("invalid OTP code")
	}

	delete(s.codes, phone)
	return nil
}

func (s *OTPStore) DeleteOTP(phone string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.codes, phone)
}

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

func generateRandomCode(length int) string {
	const digits = "0123456789"
	code := make([]byte, length)

	for i := range code {
		num, _ := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		code[i] = digits[num.Int64()]
	}

	return string(code)
}
