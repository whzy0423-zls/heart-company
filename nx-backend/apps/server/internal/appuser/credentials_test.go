package appuser

import (
	"errors"
	"strings"
	"testing"
)

func TestAccountValidation(t *testing.T) {
	t.Run("normalizes account", func(t *testing.T) {
		const raw = "  XinZhiLi_01  "
		if got, want := NormalizeAccount(raw), "xinzhili_01"; got != want {
			t.Fatalf("NormalizeAccount(%q) = %q, want %q", raw, got, want)
		}
	})

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "minimum length", raw: "a123"},
		{name: "maximum length", raw: "a" + strings.Repeat("b", 31)},
		{name: "mixed case and outer whitespace", raw: "  User_Name01  "},
		{name: "too short", raw: "abc", wantErr: true},
		{name: "too long", raw: "a" + strings.Repeat("b", 32), wantErr: true},
		{name: "numeric first", raw: "1abc", wantErr: true},
		{name: "embedded space", raw: "ab cd", wantErr: true},
		{name: "punctuation", raw: "abcd-", wantErr: true},
		{name: "non ASCII", raw: "\u7528\u6237name", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAccount(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidAccount) {
					t.Fatalf("ValidateAccount(%q) error = %v, want ErrInvalidAccount", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateAccount(%q) error = %v, want nil", tt.raw, err)
			}
		})
	}
}

func TestPasswordValidation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "minimum bytes", raw: strings.Repeat("a", 6)},
		{name: "maximum bytes", raw: strings.Repeat("a", 72)},
		{name: "whitespace is not trimmed", raw: strings.Repeat(" ", 6)},
		{name: "multibyte at maximum bytes", raw: strings.Repeat("\u754c", 24)},
		{name: "too short", raw: strings.Repeat("a", 5), wantErr: true},
		{name: "too long", raw: strings.Repeat("a", 73), wantErr: true},
		{name: "multibyte over maximum bytes", raw: strings.Repeat("\u754c", 25), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidPassword) {
					t.Fatalf("ValidatePassword() error = %v, want ErrInvalidPassword", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidatePassword() error = %v, want nil", err)
			}
		})
	}
}

func TestNicknameValidation(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "one rune", raw: "\u754c"},
		{name: "outer whitespace is trimmed", raw: "  Alice  "},
		{name: "maximum Unicode runes", raw: strings.Repeat("\u754c", 32)},
		{name: "maximum runes with outer whitespace", raw: "\t" + strings.Repeat("\u754c", 32) + "\n"},
		{name: "empty", raw: "", wantErr: true},
		{name: "whitespace only", raw: " \t\n ", wantErr: true},
		{name: "too many Unicode runes", raw: strings.Repeat("\u754c", 33), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNickname(tt.raw)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidNickname) {
					t.Fatalf("ValidateNickname(%q) error = %v, want ErrInvalidNickname", tt.raw, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateNickname(%q) error = %v, want nil", tt.raw, err)
			}
		})
	}
}

func TestCredentialDomainErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "invalid account", err: ErrInvalidAccount, want: "invalid account"},
		{name: "invalid password", err: ErrInvalidPassword, want: "invalid password"},
		{name: "invalid nickname", err: ErrInvalidNickname, want: "invalid nickname"},
		{name: "invalid SMS code", err: ErrInvalidSMSCode, want: "invalid sms code"},
		{name: "account taken", err: ErrAccountTaken, want: "account already exists"},
		{name: "phone registered", err: ErrPhoneAlreadyRegistered, want: "phone already registered"},
		{name: "invalid credentials", err: ErrInvalidCredentials, want: "invalid credentials"},
		{name: "user disabled", err: ErrUserDisabled, want: "user disabled"},
	}

	seen := make(map[error]string, len(tests))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err == nil {
				t.Fatal("sentinel error is nil")
			}
			if got := tt.err.Error(); got != tt.want {
				t.Fatalf("error text = %q, want %q", got, tt.want)
			}
			if previous, ok := seen[tt.err]; ok {
				t.Fatalf("sentinel error is shared with %s", previous)
			}
			seen[tt.err] = tt.name
		})
	}
}
