package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type UserInfo struct {
	Avatar    string   `json:"avatar"`
	Email     string   `json:"email,omitempty"`
	HomePath  string   `json:"homePath"`
	ID        int64    `json:"id"`
	Phone     string   `json:"phone,omitempty"`
	RealName  string   `json:"realName"`
	Remark    string   `json:"remark,omitempty"`
	Roles     []string `json:"roles"`
	TokenKind string   `json:"tokenKind,omitempty"`
	UserID    string   `json:"userId"`
	Username  string   `json:"username"`
}

const (
	TokenKindBackend = "backend"
	TokenKindApp     = "app"
	TokenKindMiniapp = "miniapp"
)

type tokenPayload struct {
	UserInfo
	ExpiresAt int64 `json:"exp"`
}

func Sign(user UserInfo, secret string) (string, error) {
	payload := tokenPayload{
		UserInfo:  user,
		ExpiresAt: time.Now().Add(7 * 24 * time.Hour).Unix(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	bodyEncoded := base64.RawURLEncoding.EncodeToString(body)
	signature := sign(bodyEncoded, secret)
	return bodyEncoded + "." + signature, nil
}

func Verify(token string, secret string) (UserInfo, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return UserInfo{}, false
	}

	expected := sign(parts[0], secret)
	if !hmac.Equal([]byte(expected), []byte(parts[1])) {
		return UserInfo{}, false
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return UserInfo{}, false
	}

	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return UserInfo{}, false
	}
	if payload.ExpiresAt < time.Now().Unix() {
		return UserInfo{}, false
	}
	return payload.UserInfo, true
}

func BearerUser(authorization string, secret string) (UserInfo, error) {
	if !strings.HasPrefix(authorization, "Bearer ") {
		return UserInfo{}, errors.New("missing bearer token")
	}
	user, ok := Verify(strings.TrimPrefix(authorization, "Bearer "), secret)
	if !ok {
		return UserInfo{}, errors.New("invalid bearer token")
	}
	return user, nil
}

func BearerUserWithKind(authorization string, secret string, allowed ...string) (UserInfo, error) {
	user, err := BearerUser(authorization, secret)
	if err != nil {
		return UserInfo{}, err
	}
	if len(allowed) == 0 {
		return user, nil
	}
	for _, kind := range allowed {
		if user.TokenKind == kind {
			return user, nil
		}
		// Tokens issued before tokenKind existed were backend-admin tokens.
		if user.TokenKind == "" && kind == TokenKindBackend {
			for _, role := range user.Roles {
				if role == TokenKindMiniapp || role == TokenKindApp {
					return UserInfo{}, errors.New("invalid token kind")
				}
			}
			return user, nil
		}
	}
	return UserInfo{}, errors.New("invalid token kind")
}

func SignWithExpiry(user UserInfo, secret string, dur time.Duration) (string, error) {
	payload := tokenPayload{
		UserInfo:  user,
		ExpiresAt: time.Now().Add(dur).Unix(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	bodyEncoded := base64.RawURLEncoding.EncodeToString(body)
	signature := sign(bodyEncoded, secret)
	return bodyEncoded + "." + signature, nil
}

func sign(body string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
