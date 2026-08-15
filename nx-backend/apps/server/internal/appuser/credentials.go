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
	return strings.ToLower(strings.TrimSpace(raw))
}

func ValidateAccount(raw string) error {
	if !accountPattern.MatchString(NormalizeAccount(raw)) {
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

func ValidateNickname(raw string) error {
	length := utf8.RuneCountInString(strings.TrimSpace(raw))
	if length < 1 || length > 32 {
		return ErrInvalidNickname
	}
	return nil
}
