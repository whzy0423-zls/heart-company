package miniapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/privacy"
	"nine-xing/nx-backend/apps/server/internal/signup"
)

const miniappUserTimeout = 10 * time.Second

var ErrServiceNotConfigured = errors.New("miniapp: service is not configured")

type userWriter interface {
	UpsertByOpenIDWithDBTX(context.Context, dbtx.DBTX, string, string, string, string) (int64, bool, error)
	GetUserWithDBTX(context.Context, dbtx.DBTX, int64) (User, error)
}

type messageWriter interface {
	Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error)
}

type testRecordWriter interface {
	InsertTestRecord(context.Context, dbtx.DBTX, int64, TestRecordInput) (TestRecord, error)
	UpdateMainType(context.Context, dbtx.DBTX, int64, int) error
}

type bookingWriter interface {
	InsertBooking(context.Context, dbtx.DBTX, int64, BookingInput, int64) (Booking, error)
}

type signupWriter interface {
	CreateWithDBTX(context.Context, dbtx.DBTX, signup.LeadInput, *http.Request, string) (signup.Lead, error)
}

type Service struct {
	beginner dbtx.Beginner
	users    userWriter
	tests    testRecordWriter
	bookings bookingWriter
	signups  signupWriter
	messages messageWriter
}

type ServiceOption func(*Service)

func WithTestRecordWriter(writer testRecordWriter) ServiceOption {
	return func(service *Service) {
		service.tests = writer
	}
}

func WithBookingWriter(writer bookingWriter) ServiceOption {
	return func(service *Service) {
		service.bookings = writer
	}
}

func WithSignupWriter(writer signupWriter) ServiceOption {
	return func(service *Service) {
		service.signups = writer
	}
}

func NewService(beginner dbtx.Beginner, users userWriter, messages messageWriter, options ...ServiceOption) *Service {
	service := &Service{beginner: beginner, users: users, messages: messages}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) UpsertUser(ctx context.Context, openid, unionid, channel, scene string) (int64, error) {
	if s == nil || s.beginner == nil || s.users == nil || s.messages == nil {
		return 0, ErrServiceNotConfigured
	}
	var err error
	openid, unionid, channel, scene, err = normalizeUserSource(openid, unionid, channel, scene)
	if err != nil {
		return 0, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithTimeout(ctx, miniappUserTimeout)
	defer cancel()

	tx, err := s.beginner.BeginTx(opCtx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin miniapp user transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, created, err := s.users.UpsertByOpenIDWithDBTX(opCtx, tx, openid, unionid, channel, scene)
	if err != nil {
		return 0, fmt.Errorf("upsert miniapp user: %w", err)
	}
	if created {
		user, err := s.users.GetUserWithDBTX(opCtx, tx, id)
		if err != nil {
			return 0, fmt.Errorf("get created miniapp user: %w", err)
		}
		displayName := miniappUserDisplayName(user.Nickname, openid, unionid, id)
		if _, err := s.messages.Create(opCtx, tx, businessmessage.MiniappUserCreated(strconv.FormatInt(id, 10), displayName)); err != nil {
			return 0, fmt.Errorf("create miniapp user message: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit miniapp user transaction: %w", err)
	}
	return id, nil
}

func (s *Service) SaveTestRecord(ctx context.Context, userID int64, in TestRecordInput) (TestRecord, error) {
	if s == nil || s.beginner == nil || s.users == nil || s.tests == nil || s.messages == nil {
		return TestRecord{}, ErrServiceNotConfigured
	}
	in, err := normalizeTestRecordInput(userID, in)
	if err != nil {
		return TestRecord{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithTimeout(ctx, miniappUserTimeout)
	defer cancel()

	tx, err := s.beginner.BeginTx(opCtx, nil)
	if err != nil {
		return TestRecord{}, fmt.Errorf("begin miniapp test transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	record, err := s.tests.InsertTestRecord(opCtx, tx, userID, in)
	if err != nil {
		return TestRecord{}, fmt.Errorf("insert miniapp test record: %w", err)
	}
	if err := s.tests.UpdateMainType(opCtx, tx, userID, record.ResultType); err != nil {
		return TestRecord{}, fmt.Errorf("update miniapp main type: %w", err)
	}
	user, err := s.users.GetUserWithDBTX(opCtx, tx, userID)
	if err != nil {
		return TestRecord{}, fmt.Errorf("get miniapp test user: %w", err)
	}
	displayName := miniappTestUserDisplayName(user, userID)
	event := businessmessage.MiniappQuizSubmitted(record.ID, strconv.FormatInt(userID, 10), displayName, record.ResultType)
	event.Content += "，提交时间：" + miniappTestSubmittedAt(record.CreateTime)
	if _, err := s.messages.Create(opCtx, tx, event); err != nil {
		return TestRecord{}, fmt.Errorf("create miniapp test message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return TestRecord{}, fmt.Errorf("commit miniapp test transaction: %w", err)
	}
	return record, nil
}

type BookingResult struct {
	Booking Booking
	Lead    signup.Lead
}

func (s *Service) CreateBooking(ctx context.Context, userID int64, in BookingInput, r *http.Request) (BookingResult, error) {
	if s == nil || s.beginner == nil || s.users == nil || s.bookings == nil || s.signups == nil || s.messages == nil {
		return BookingResult{}, ErrServiceNotConfigured
	}
	if userID <= 0 || r == nil {
		return BookingResult{}, ErrInvalidBooking
	}
	in, err := normalizeBookingInput(in)
	if err != nil {
		return BookingResult{}, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	opCtx, cancel := context.WithTimeout(ctx, miniappUserTimeout)
	defer cancel()

	tx, err := s.beginner.BeginTx(opCtx, nil)
	if err != nil {
		return BookingResult{}, fmt.Errorf("begin miniapp booking transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	user, err := s.users.GetUserWithDBTX(opCtx, tx, userID)
	if err != nil {
		return BookingResult{}, fmt.Errorf("get miniapp booking user: %w", err)
	}
	lead, err := s.signups.CreateWithDBTX(opCtx, tx, signup.LeadInput{
		Name:        in.ContactName,
		Contact:     in.Phone,
		ContactType: signup.ContactTypePhone,
		Interest:    BookingInterest(in),
		Message:     in.Message,
	}, r, "miniapp")
	if err != nil {
		return BookingResult{}, fmt.Errorf("create miniapp booking signup: %w", err)
	}
	signupID, err := strconv.ParseInt(strings.TrimSpace(lead.ID), 10, 64)
	if err != nil || signupID <= 0 {
		return BookingResult{}, fmt.Errorf("create miniapp booking signup: invalid lead id")
	}
	booking, err := s.bookings.InsertBooking(opCtx, tx, userID, in, signupID)
	if err != nil {
		return BookingResult{}, fmt.Errorf("insert miniapp booking: %w", err)
	}
	displayName := miniappTestUserDisplayName(user, userID)
	if _, err := s.messages.Create(opCtx, tx, businessmessage.MiniappBookingCreated(booking.ID, lead.ID, displayName, privacy.MaskPhone(in.Phone))); err != nil {
		return BookingResult{}, fmt.Errorf("create miniapp booking message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return BookingResult{}, fmt.Errorf("commit miniapp booking transaction: %w", err)
	}
	return BookingResult{Booking: booking, Lead: lead}, nil
}

func BookingInterest(in BookingInput) string {
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = "consult"
	}
	label := map[string]string{
		"consult":    "1v1 咨询预约",
		"course":     "课程报名",
		"enterprise": "企业课程咨询",
	}[kind]
	if label == "" {
		label = "小程序预约"
	}
	if intent := strings.TrimSpace(in.Intent); intent != "" {
		return label + " · " + intent
	}
	return label
}

func miniappTestUserDisplayName(user User, userID int64) string {
	displayID := strconv.FormatInt(userID, 10)
	if parsed, err := strconv.ParseInt(strings.TrimSpace(user.ID), 10, 64); err == nil && parsed == userID {
		displayID = strconv.FormatInt(parsed, 10)
	}
	return "微信用户" + displayID
}

func miniappTestSubmittedAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len([]rune(value)) > 32 || strings.ContainsFunc(value, unicode.IsControl) {
		return "待确认"
	}
	return value
}

func miniappUserDisplayName(nickname, openid, unionid string, id int64) string {
	name := strings.TrimSpace(nickname)
	openid = strings.TrimSpace(openid)
	unionid = strings.TrimSpace(unionid)
	containsIdentity := (openid != "" && strings.Contains(name, openid)) || (unionid != "" && strings.Contains(name, unionid))
	if name == "" || containsIdentity || strings.ContainsFunc(name, unicode.IsControl) {
		return "微信用户" + strconv.FormatInt(id, 10)
	}
	runes := []rune(name)
	if len(runes) > 40 {
		name = string(runes[:40])
	}
	return name
}
