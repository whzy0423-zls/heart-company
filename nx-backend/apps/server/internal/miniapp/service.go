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

type Service struct {
	beginner dbtx.Beginner
	users    userWriter
	messages messageWriter
}

func NewService(beginner dbtx.Beginner, users userWriter, messages messageWriter) *Service {
	return &Service{beginner: beginner, users: users, messages: messages}
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
