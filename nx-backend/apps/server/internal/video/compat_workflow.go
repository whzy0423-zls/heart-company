package video

import (
	"context"
	"database/sql"
	"errors"
)

var (
	ErrInvalidSubmissionRequest = errors.New("invalid submission request")
	ErrSubmissionNotFound       = errors.New("submission not found")
)

type InvalidSubmissionTransitionError = SubmissionTransitionError

func (s *Store) GenerationMode() string {
	if s == nil {
		return "demo"
	}
	return s.generationMode
}
func (s *Store) SubmissionByRequestKey(ctx context.Context, key string) (Submission, error) {
	if s == nil || s.submissions == nil {
		return Submission{}, ErrSubmissionNotFound
	}
	item, err := s.submissions.GetByRequestKey(ctx, key)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Submission{}, ErrSubmissionNotFound
		}
		return Submission{}, err
	}
	return item, nil
}

func (s *Store) SubmissionByID(ctx context.Context, id string) (Submission, error) {
	if s == nil || s.submissions == nil || s.submissions.db == nil {
		return Submission{}, ErrSubmissionNotFound
	}
	item, err := scanSubmission(s.submissions.db.QueryRowContext(ctx, `SELECT `+submissionSelectColumns+` FROM video_generation_submissions WHERE id::text=$1`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrSubmissionNotFound
	}
	return item, err
}
