package businessmessage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/privacy"
)

const (
	maxTitleRunes        = 100
	maxContentRunes      = 1000
	maxTargetRunes       = 512
	maxTypeRunes         = 32
	maxEventKeyRunes     = 128
	maxBusinessTypeRunes = 64
	maxBusinessIDRunes   = 128
)

var ErrNilDBTX = errors.New("businessmessage: query target is nil")

type Event struct {
	Type         string
	Title        string
	Content      string
	Platform     string
	EventKey     string
	BusinessID   string
	BusinessType string
	TargetPath   string
}

type Store struct{}

func Validate(event Event) error {
	event = normalizeEvent(event)
	return validateNormalized(event)
}

func validateNormalized(event Event) error {
	if strings.TrimSpace(event.EventKey) == "" {
		return errors.New("businessmessage: event key is required")
	}
	if strings.TrimSpace(event.BusinessType) == "" {
		return errors.New("businessmessage: business type is required")
	}
	if strings.TrimSpace(event.BusinessID) == "" {
		return errors.New("businessmessage: business id is required")
	}
	switch event.Platform {
	case "website", "miniapp", "system":
	default:
		return errors.New("businessmessage: invalid platform")
	}
	if strings.TrimSpace(event.Type) == "" {
		return errors.New("businessmessage: type is required")
	}
	if utf8.RuneCountInString(event.Type) > maxTypeRunes {
		return errors.New("businessmessage: type is too long")
	}
	if utf8.RuneCountInString(event.EventKey) > maxEventKeyRunes {
		return errors.New("businessmessage: event key is too long")
	}
	if utf8.RuneCountInString(event.BusinessType) > maxBusinessTypeRunes {
		return errors.New("businessmessage: business type is too long")
	}
	if utf8.RuneCountInString(event.BusinessID) > maxBusinessIDRunes {
		return errors.New("businessmessage: business id is too long")
	}
	for _, field := range []struct {
		name  string
		value string
	}{
		{name: "type", value: event.Type},
		{name: "event key", value: event.EventKey},
		{name: "business type", value: event.BusinessType},
		{name: "business id", value: event.BusinessID},
	} {
		if containsControlRune(field.value) {
			return fmt.Errorf("businessmessage: %s contains control characters", field.name)
		}
	}
	if strings.TrimSpace(event.Title) == "" {
		return errors.New("businessmessage: title is required")
	}
	if utf8.RuneCountInString(event.Title) > maxTitleRunes {
		return errors.New("businessmessage: title is too long")
	}
	if utf8.RuneCountInString(event.Content) > maxContentRunes {
		return errors.New("businessmessage: content is too long")
	}
	if !strings.HasPrefix(event.TargetPath, "/") {
		return errors.New("businessmessage: target path must be absolute")
	}
	if utf8.RuneCountInString(event.TargetPath) > maxTargetRunes {
		return errors.New("businessmessage: target path is too long")
	}
	return nil
}

func (Store) Create(ctx context.Context, q dbtx.DBTX, event Event) (bool, error) {
	event = normalizeEvent(event)
	if err := validateNormalized(event); err != nil {
		return false, err
	}
	if q == nil {
		return false, ErrNilDBTX
	}

	var id int64
	err := q.QueryRowContext(ctx,
		`INSERT INTO messages
		 (type,title,content,platform,event_key,business_id,business_type,target_path)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		 ON CONFLICT (event_key,business_type,business_id) DO NOTHING
		 RETURNING id`,
		event.Type,
		event.Title,
		privacy.MaskPhonesInText(event.Content),
		event.Platform,
		event.EventKey,
		event.BusinessID,
		event.BusinessType,
		event.TargetPath,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create business message: %w", err)
	}
	return true, nil
}

func WebsiteSignupCreated(id, name, contactLabel, maskedContact string) Event {
	name = displayName(name, "官网访客")
	contactLabel = displayName(contactLabel, "联系方式")
	return Event{
		Type:         "signup",
		Title:        "新的官网报名",
		Content:      fmt.Sprintf("%s提交了官网报名，%s：%s", name, contactLabel, strings.TrimSpace(maskedContact)),
		Platform:     "website",
		EventKey:     "signup.created",
		BusinessID:   strings.TrimSpace(id),
		BusinessType: "signup",
		TargetPath:   "/customer/signups?leadId=" + url.QueryEscape(strings.TrimSpace(id)) + "&open=detail",
	}
}

func MiniappUserCreated(id, name string) Event {
	name = displayName(name, "小程序用户")
	return Event{
		Type:         "miniapp",
		Title:        "新的小程序用户",
		Content:      name + "首次进入小程序",
		Platform:     "miniapp",
		EventKey:     "miniapp.user.created",
		BusinessID:   strings.TrimSpace(id),
		BusinessType: "miniapp-user",
		TargetPath:   "/customer/miniapp-users?userId=" + url.QueryEscape(strings.TrimSpace(id)) + "&open=detail",
	}
}

func MiniappQuizSubmitted(recordID, userID, name string, resultType int) Event {
	name = displayName(name, "小程序用户")
	return Event{
		Type:         "miniapp",
		Title:        "新的小程序测评",
		Content:      fmt.Sprintf("%s提交了 %d 型测评", name, resultType),
		Platform:     "miniapp",
		EventKey:     "miniapp.quiz.submitted",
		BusinessID:   strings.TrimSpace(recordID),
		BusinessType: "miniapp-test-record",
		TargetPath: "/customer/miniapp-users?userId=" + url.QueryEscape(strings.TrimSpace(userID)) +
			"&testRecordId=" + url.QueryEscape(strings.TrimSpace(recordID)) + "&open=test",
	}
}

// MiniappBookingCreated relies on the current invariant that every booking
// creates an independent signup: signupID is the idempotent target and
// bookingID remains in the message content for operational traceability.
func MiniappBookingCreated(bookingID, signupID, name, maskedPhone string) Event {
	name = displayName(name, "小程序用户")
	bookingID = safeTraceID(bookingID)
	return Event{
		Type:         "miniapp",
		Title:        "新的小程序预约",
		Content:      fmt.Sprintf("%s提交了预约咨询（预约编号：%s），手机号：%s", name, bookingID, strings.TrimSpace(maskedPhone)),
		Platform:     "miniapp",
		EventKey:     "miniapp.booking.created",
		BusinessID:   strings.TrimSpace(signupID),
		BusinessType: "signup",
		TargetPath:   "/customer/signups?leadId=" + url.QueryEscape(strings.TrimSpace(signupID)) + "&open=detail",
	}
}

func safeTraceID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > 64 || containsControlRune(value) {
		return "待回填"
	}
	return value
}

func normalizeEvent(event Event) Event {
	event.Type = strings.TrimSpace(event.Type)
	event.Platform = strings.TrimSpace(event.Platform)
	event.EventKey = strings.TrimSpace(event.EventKey)
	event.BusinessType = strings.TrimSpace(event.BusinessType)
	event.BusinessID = strings.TrimSpace(event.BusinessID)
	event.TargetPath = strings.TrimSpace(event.TargetPath)
	return event
}

func containsControlRune(value string) bool {
	return strings.ContainsFunc(value, unicode.IsControl)
}

func displayName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
