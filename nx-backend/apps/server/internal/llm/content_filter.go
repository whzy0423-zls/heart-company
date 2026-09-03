package llm

import (
	"errors"
	"fmt"
	"strings"
)

// ErrContentFiltered is the provider-neutral signal for structured moderation
// decisions. Provider response text is deliberately excluded from this error.
var ErrContentFiltered = errors.New("llm content filtered")

type ContentFilterError struct {
	Provider string
	Code     string
}

func (e *ContentFilterError) Error() string {
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "llm"
	}
	code := normalizeContentFilterCode(e.Code)
	if code == "" {
		code = "content_filtered"
	}
	return fmt.Sprintf("%s content filtered (%s)", provider, code)
}

func (e *ContentFilterError) Unwrap() error { return ErrContentFiltered }

func newContentFilterError(provider, code string) error {
	return &ContentFilterError{Provider: provider, Code: normalizeContentFilterCode(code)}
}

func isContentFilterCode(value any) bool {
	var code string
	switch typed := value.(type) {
	case string:
		code = typed
	case fmt.Stringer:
		code = typed.String()
	default:
		return false
	}
	switch normalizeContentFilterCode(code) {
	case "content_filter", "content_filtered", "content_policy_violation", "policy_violation", "safety_blocked", "refusal":
		return true
	default:
		return false
	}
}

func normalizeContentFilterCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.NewReplacer("-", "_", " ", "_").Replace(code)
	return code
}
