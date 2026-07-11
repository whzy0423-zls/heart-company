package video

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubmissionStatus string

const (
	SubmissionPrepared       SubmissionStatus = "prepared"
	SubmissionSubmitting     SubmissionStatus = "submitting"
	SubmissionAccepted       SubmissionStatus = "accepted"
	SubmissionUnknownOutcome SubmissionStatus = "unknown_outcome"
	SubmissionReconciled     SubmissionStatus = "reconciled"
	SubmissionCompleted      SubmissionStatus = "completed"
	SubmissionFailed         SubmissionStatus = "failed"
	SubmissionCancelled      SubmissionStatus = "cancelled"
)

var allowedSubmissionTransitions = map[SubmissionStatus]map[SubmissionStatus]bool{
	SubmissionPrepared:       {SubmissionSubmitting: true, SubmissionCancelled: true},
	SubmissionSubmitting:     {SubmissionAccepted: true, SubmissionUnknownOutcome: true},
	SubmissionUnknownOutcome: {SubmissionReconciled: true},
	SubmissionAccepted:       {SubmissionCompleted: true, SubmissionFailed: true},
	SubmissionReconciled:     {SubmissionCompleted: true, SubmissionFailed: true},
}

func (s SubmissionStatus) Active() bool {
	switch s {
	case SubmissionPrepared, SubmissionSubmitting, SubmissionAccepted, SubmissionUnknownOutcome, SubmissionReconciled:
		return true
	default:
		return false
	}
}

func canTransitionSubmission(from, to SubmissionStatus) bool {
	return allowedSubmissionTransitions[from][to]
}

type Submission struct {
	ID              int64            `json:"submissionId"`
	RequestKey      string           `json:"requestKey"`
	ShotID          string           `json:"shotId"`
	GenerationID    string           `json:"generationId,omitempty"`
	TaskID          string           `json:"taskId,omitempty"`
	Status          SubmissionStatus `json:"status"`
	RequestSnapshot json.RawMessage  `json:"-"`
	ErrorMessage    string           `json:"error,omitempty"`
}

type PrepareSubmissionInput struct {
	RequestKey      string
	ShotID          string
	RequestSnapshot json.RawMessage
}

type SubmissionPatch struct {
	TaskID       *string
	GenerationID *string
	ErrorMessage *string
}

var (
	ErrSubmissionNotFound             = errors.New("generation submission not found")
	errSubmissionRequestKeyConstraint = errors.New("generation submission request key constraint")
	errSubmissionActiveShotConstraint = errors.New("generation submission active shot constraint")
	errSubmissionCompareAndSwap       = errors.New("generation submission compare-and-swap failed")
	ErrInvalidSubmissionRequest       = errors.New("invalid generation submission request")
)

type ActiveSubmissionError struct {
	ShotID     string
	RequestKey string
	Status     SubmissionStatus
}

func (e *ActiveSubmissionError) Error() string {
	return fmt.Sprintf("shot %s already has active submission %s (%s)", e.ShotID, e.RequestKey, e.Status)
}

type InvalidSubmissionTransitionError struct {
	RequestKey string
	From       SubmissionStatus
	To         SubmissionStatus
}

func (e *InvalidSubmissionTransitionError) Error() string {
	return fmt.Sprintf("submission %s cannot transition from %s to %s", e.RequestKey, e.From, e.To)
}

type RequestKeyReuseError struct {
	RequestKey string
}

func (e *RequestKeyReuseError) Error() string {
	return fmt.Sprintf("submission request key %s was reused for different input", e.RequestKey)
}

type ReconciliationTaskConflictError struct {
	RequestKey string
	Existing   string
	Received   string
}

func (e *ReconciliationTaskConflictError) Error() string {
	return fmt.Sprintf("submission %s is linked to task %s, not %s", e.RequestKey, e.Existing, e.Received)
}

type UnknownOutcomeError struct {
	RequestKey string
	TaskID     string
	Cause      error
}

func (e *UnknownOutcomeError) Error() string {
	if e.TaskID != "" {
		return fmt.Sprintf("generation outcome is unknown for request %s (task %s)", e.RequestKey, e.TaskID)
	}
	return fmt.Sprintf("generation outcome is unknown for request %s", e.RequestKey)
}

func (e *UnknownOutcomeError) Unwrap() error { return e.Cause }

type submissionRepository interface {
	Insert(context.Context, PrepareSubmissionInput) (Submission, error)
	GetByRequestKey(context.Context, string) (Submission, error)
	FindActiveByShot(context.Context, string) (Submission, error)
	CompareAndSwap(context.Context, string, SubmissionStatus, SubmissionStatus, SubmissionPatch) (Submission, error)
}

type SubmissionStore struct {
	repo submissionRepository
}

func NewSubmissionStore(database *sql.DB) *SubmissionStore {
	return newSubmissionStore(&sqlSubmissionRepository{db: database})
}

func newSubmissionStore(repo submissionRepository) *SubmissionStore {
	return &SubmissionStore{repo: repo}
}

func (s *SubmissionStore) Prepare(ctx context.Context, input PrepareSubmissionInput) (Submission, error) {
	input.RequestKey = strings.TrimSpace(input.RequestKey)
	input.ShotID = strings.TrimSpace(input.ShotID)
	if input.RequestKey == "" || input.ShotID == "" || !json.Valid(normalizeSnapshot(input.RequestSnapshot)) {
		return Submission{}, ErrInvalidSubmissionRequest
	}
	input.RequestSnapshot = normalizeSnapshot(input.RequestSnapshot)
	created, err := s.repo.Insert(ctx, input)
	if err == nil {
		return created, nil
	}
	if errors.Is(err, errSubmissionRequestKeyConstraint) {
		existing, getErr := s.repo.GetByRequestKey(ctx, input.RequestKey)
		if getErr != nil {
			return Submission{}, getErr
		}
		if existing.ShotID != input.ShotID || !bytes.Equal(normalizeSnapshot(existing.RequestSnapshot), input.RequestSnapshot) {
			return Submission{}, &RequestKeyReuseError{RequestKey: input.RequestKey}
		}
		return existing, nil
	}
	if errors.Is(err, errSubmissionActiveShotConstraint) {
		active, getErr := s.repo.FindActiveByShot(ctx, input.ShotID)
		if getErr != nil {
			return Submission{}, err
		}
		return Submission{}, &ActiveSubmissionError{
			ShotID:     active.ShotID,
			RequestKey: active.RequestKey,
			Status:     active.Status,
		}
	}
	return Submission{}, err
}

func (s *SubmissionStore) Transition(
	ctx context.Context,
	requestKey string,
	from SubmissionStatus,
	to SubmissionStatus,
) (Submission, error) {
	return s.transition(ctx, requestKey, from, to, SubmissionPatch{})
}

func (s *SubmissionStore) transition(
	ctx context.Context,
	requestKey string,
	from SubmissionStatus,
	to SubmissionStatus,
	patch SubmissionPatch,
) (Submission, error) {
	if !canTransitionSubmission(from, to) {
		return Submission{}, &InvalidSubmissionTransitionError{RequestKey: requestKey, From: from, To: to}
	}
	updated, err := s.repo.CompareAndSwap(ctx, requestKey, from, to, patch)
	if err == nil {
		return updated, nil
	}
	if !errors.Is(err, errSubmissionCompareAndSwap) {
		return Submission{}, err
	}
	current, getErr := s.repo.GetByRequestKey(ctx, requestKey)
	if getErr != nil {
		return Submission{}, getErr
	}
	return Submission{}, &InvalidSubmissionTransitionError{RequestKey: requestKey, From: current.Status, To: to}
}

func (s *SubmissionStore) GetByRequestKey(ctx context.Context, requestKey string) (Submission, error) {
	return s.repo.GetByRequestKey(ctx, strings.TrimSpace(requestKey))
}

func (s *SubmissionStore) FindActiveByShot(ctx context.Context, shotID string) (Submission, error) {
	return s.repo.FindActiveByShot(ctx, strings.TrimSpace(shotID))
}

func (s *SubmissionStore) AttachAccepted(
	ctx context.Context,
	requestKey string,
	taskID string,
	generationID string,
) (Submission, error) {
	taskID = strings.TrimSpace(taskID)
	generationID = strings.TrimSpace(generationID)
	if taskID == "" || generationID == "" {
		return Submission{}, ErrInvalidSubmissionRequest
	}
	return s.transition(ctx, requestKey, SubmissionSubmitting, SubmissionAccepted, SubmissionPatch{
		TaskID:       &taskID,
		GenerationID: &generationID,
	})
}

func (s *SubmissionStore) MarkUnknownOutcome(
	ctx context.Context,
	requestKey string,
	taskID string,
) (Submission, error) {
	taskID = strings.TrimSpace(taskID)
	message := "generation gateway outcome unknown"
	patch := SubmissionPatch{ErrorMessage: &message}
	if taskID != "" {
		patch.TaskID = &taskID
	}
	return s.transition(
		ctx,
		requestKey,
		SubmissionSubmitting,
		SubmissionUnknownOutcome,
		patch,
	)
}

func (s *SubmissionStore) Reconcile(
	ctx context.Context,
	requestKey string,
	taskID string,
	generationID string,
) (Submission, error) {
	taskID = strings.TrimSpace(taskID)
	generationID = strings.TrimSpace(generationID)
	if taskID == "" || generationID == "" {
		return Submission{}, ErrInvalidSubmissionRequest
	}
	current, err := s.repo.GetByRequestKey(ctx, requestKey)
	if err != nil {
		return Submission{}, err
	}
	if current.TaskID != "" && current.TaskID != taskID {
		return Submission{}, &ReconciliationTaskConflictError{
			RequestKey: requestKey,
			Existing:   current.TaskID,
			Received:   taskID,
		}
	}
	if current.Status == SubmissionReconciled {
		if current.GenerationID != "" && current.GenerationID != generationID {
			return Submission{}, &ReconciliationTaskConflictError{
				RequestKey: requestKey,
				Existing:   current.GenerationID,
				Received:   generationID,
			}
		}
		return current, nil
	}
	return s.transition(ctx, requestKey, SubmissionUnknownOutcome, SubmissionReconciled, SubmissionPatch{
		TaskID:       &taskID,
		GenerationID: &generationID,
	})
}

func normalizeSnapshot(raw json.RawMessage) json.RawMessage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return json.RawMessage(`{}`)
	}
	return bytes.TrimSpace(raw)
}

type sqlSubmissionRepository struct {
	db *sql.DB
}

const submissionColumns = `id, request_key::text, shot_id::text,
       COALESCE(generation_id::text,''), task_id, status,
       request_snapshot, error_message`

type submissionScanner interface {
	Scan(...any) error
}

func scanSubmission(scanner submissionScanner) (Submission, error) {
	var row Submission
	err := scanner.Scan(
		&row.ID,
		&row.RequestKey,
		&row.ShotID,
		&row.GenerationID,
		&row.TaskID,
		&row.Status,
		&row.RequestSnapshot,
		&row.ErrorMessage,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, ErrSubmissionNotFound
	}
	return row, err
}

func (r *sqlSubmissionRepository) Insert(ctx context.Context, input PrepareSubmissionInput) (Submission, error) {
	row, err := scanSubmission(r.db.QueryRowContext(ctx, `
		INSERT INTO video_generation_submissions (request_key, shot_id, request_snapshot)
		VALUES ($1::uuid, $2::bigint, $3::jsonb)
		RETURNING `+submissionColumns,
		input.RequestKey,
		input.ShotID,
		[]byte(input.RequestSnapshot),
	))
	if err == nil {
		return row, nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		switch pgErr.ConstraintName {
		case "video_generation_submissions_request_key_key":
			return Submission{}, errSubmissionRequestKeyConstraint
		case "idx_video_generation_submissions_active_shot":
			return Submission{}, errSubmissionActiveShotConstraint
		}
	}
	return Submission{}, err
}

func (r *sqlSubmissionRepository) GetByRequestKey(ctx context.Context, requestKey string) (Submission, error) {
	return scanSubmission(r.db.QueryRowContext(ctx, `
		SELECT `+submissionColumns+`
		FROM video_generation_submissions
		WHERE request_key=$1::uuid`, requestKey))
}

func (r *sqlSubmissionRepository) FindActiveByShot(ctx context.Context, shotID string) (Submission, error) {
	return scanSubmission(r.db.QueryRowContext(ctx, `
		SELECT `+submissionColumns+`
		FROM video_generation_submissions
		WHERE shot_id=$1::bigint
		  AND status IN ('prepared','submitting','accepted','unknown_outcome','reconciled')
		ORDER BY id DESC
		LIMIT 1`, shotID))
}

func (r *sqlSubmissionRepository) CompareAndSwap(
	ctx context.Context,
	requestKey string,
	from SubmissionStatus,
	to SubmissionStatus,
	patch SubmissionPatch,
) (Submission, error) {
	taskID, setTaskID := patchValue(patch.TaskID)
	generationID, setGenerationID := patchValue(patch.GenerationID)
	errorMessage, setError := patchValue(patch.ErrorMessage)
	row, err := scanSubmission(r.db.QueryRowContext(ctx, `
		UPDATE video_generation_submissions
		SET status=$3,
		    task_id=CASE WHEN $5 THEN $4 ELSE task_id END,
		    generation_id=CASE WHEN $7 THEN NULLIF($6,'')::bigint ELSE generation_id END,
		    error_message=CASE WHEN $9 THEN $8 ELSE error_message END,
		    update_time=now()
		WHERE request_key=$1::uuid AND status=$2
		RETURNING `+submissionColumns,
		requestKey,
		from,
		to,
		taskID,
		setTaskID,
		generationID,
		setGenerationID,
		errorMessage,
		setError,
	))
	if errors.Is(err, ErrSubmissionNotFound) {
		return Submission{}, errSubmissionCompareAndSwap
	}
	return row, err
}

func patchValue(value *string) (string, bool) {
	if value == nil {
		return "", false
	}
	return *value, true
}
