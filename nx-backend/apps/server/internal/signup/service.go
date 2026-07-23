package signup

import (
	"context"
	"fmt"
	"net/http"

	"nine-xing/nx-backend/apps/server/internal/businessmessage"
	"nine-xing/nx-backend/apps/server/internal/dbtx"
	"nine-xing/nx-backend/apps/server/internal/privacy"
)

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
	tx, err := s.beginner.BeginTx(ctx, nil)
	if err != nil {
		return Lead{}, fmt.Errorf("begin website signup transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	lead, err := s.leads.CreateWithDBTX(ctx, tx, input, r, "website")
	if err != nil {
		return Lead{}, fmt.Errorf("create website signup: %w", err)
	}
	event := businessmessage.WebsiteSignupCreated(
		lead.ID,
		lead.Name,
		contactTypeLabel(lead.ContactType),
		privacy.MaskPhone(lead.Contact),
	)
	if _, err := s.messages.Create(ctx, tx, event); err != nil {
		return Lead{}, fmt.Errorf("create website signup message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Lead{}, fmt.Errorf("commit website signup transaction: %w", err)
	}
	return lead, nil
}
