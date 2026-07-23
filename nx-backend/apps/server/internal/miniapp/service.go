package miniapp

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
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

type Service struct {
	beginner dbtx.Beginner
	users    userWriter
	tests    testRecordWriter
	messages messageWriter
}

func NewService(beginner dbtx.Beginner, users userWriter, messages messageWriter) *Service {
	tests, _ := users.(testRecordWriter)
	return &Service{beginner: beginner, users: users, tests: tests, messages: messages}
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
