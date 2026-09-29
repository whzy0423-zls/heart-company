package signup

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/privacy"
)

const websiteSignupTimeout = 10 * time.Second

// ErrServiceNotConfigured indicates that a required transaction dependency is missing.
var ErrServiceNotConfigured = errors.New("signup: service is not configured")

type leadWriter interface {
	CreateWithDBTX(context.Context, dbtx.DBTX, LeadInput, *http.Request, string) (Lead, error)
}

type messageWriter interface {
	Create(context.Context, dbtx.DBTX, businessmessage.Event) (bool, error)
}

type Service struct {
	beginner dbtx.Beginner
	leads    leadWriter
	messages messageWriter
}

func NewService(beginner dbtx.Beginner, leads leadWriter, messages messageWriter) *Service {
	return &Service{beginner: beginner, leads: leads, messages: messages}
}

func (s *Service) CreateWebsiteSignup(ctx context.Context, input LeadInput, r *http.Request) (Lead, error) {
	return s.createSignup(ctx, input, r, func(lead Lead) businessmessage.Event {
		return businessmessage.WebsiteSignupCreated(
			lead.ID,
			lead.Name,
			contactTypeLabel(lead.ContactType),
			privacy.MaskPhone(lead.Contact),
		)
	})
}

// CreateTeacherSignup stores a teacher enrollment in the existing signups
// table while emitting a teacher-specific management notification. The
// source platform remains website so existing admin filters and analytics keep
// their historical meaning.
func (s *Service) CreateTeacherSignup(ctx context.Context, input LeadInput, teacherName, teacherKey, kind string, r *http.Request) (Lead, error) {
	if s == nil {
		return Lead{}, ErrServiceNotConfigured
	}
	teacherName = strings.TrimSpace(teacherName)
	teacherKey = strings.TrimSpace(teacherKey)
	kind = strings.TrimSpace(kind)
	if teacherName == "" || teacherKey == "" {
		return Lead{}, errors.New("teacher is required")
	}
	return s.createSignup(ctx, input, r, func(lead Lead) businessmessage.Event {
		return businessmessage.TeacherSignupCreated(
			lead.ID,
			teacherName,
			teacherKey,
			kind,
			lead.Name,
			privacy.MaskPhone(lead.Contact),
		)
	})
}

func (s *Service) createSignup(ctx context.Context, input LeadInput, r *http.Request, eventFactory func(Lead) businessmessage.Event) (Lead, error) {
	if s == nil || s.beginner == nil || s.leads == nil || s.messages == nil {
		return Lead{}, ErrServiceNotConfigured
	}
	opCtx, cancel := context.WithTimeout(ctx, websiteSignupTimeout)
	defer cancel()

	tx, err := s.beginner.BeginTx(opCtx, nil)
	if err != nil {
		return Lead{}, fmt.Errorf("begin website signup transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	lead, err := s.leads.CreateWithDBTX(opCtx, tx, input, r, "website")
	if err != nil {
		return Lead{}, fmt.Errorf("create website signup: %w", err)
	}
	if eventFactory == nil {
		return Lead{}, errors.New("signup: event factory is nil")
	}
	event := eventFactory(lead)
	if _, err := s.messages.Create(opCtx, tx, event); err != nil {
		return Lead{}, fmt.Errorf("create website signup message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lead{}, fmt.Errorf("commit website signup transaction: %w", err)
	}
	return lead, nil
}
