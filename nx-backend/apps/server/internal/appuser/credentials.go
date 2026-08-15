package appuser

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidAccount         = errors.New("invalid account")
	ErrInvalidPassword        = errors.New("invalid password")
	ErrInvalidNickname        = errors.New("invalid nickname")
	ErrInvalidSMSCode         = errors.New("invalid sms code")
	ErrAccountTaken           = errors.New("account already exists")
	ErrPhoneAlreadyRegistered = errors.New("phone already registered")
	ErrInvalidCredentials     = errors.New("invalid credentials")
	ErrUserDisabled           = errors.New("user disabled")
)

var accountPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{3,31}$`)

func NormalizeAccount(raw string) string {
	normalized := []byte(strings.TrimSpace(raw))
	for i, char := range normalized {
		if char >= 'A' && char <= 'Z' {
			normalized[i] = char + ('a' - 'A')
		}
	}
	return string(normalized)
}

func ValidateAccount(raw string) error {
	if !accountPattern.MatchString(strings.TrimSpace(raw)) {
		return ErrInvalidAccount
	}
	return nil
}

func ValidatePassword(raw string) error {
	if len(raw) < 6 || len(raw) > 72 {
		return ErrInvalidPassword
	}
	return nil
}

func NormalizeNickname(raw string) string {
	return strings.TrimSpace(raw)
}

func ValidateNickname(raw string) error {
	if !utf8.ValidString(raw) {
		return ErrInvalidNickname
	}
	length := utf8.RuneCountInString(NormalizeNickname(raw))
	if length < 1 || length > 32 {
		return ErrInvalidNickname
	}
	return nil
}
