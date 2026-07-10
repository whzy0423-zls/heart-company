package video

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type SubmissionStatus string

const (
	SubmissionPrepared       SubmissionStatus = "prepared"
	SubmissionSubmitting     SubmissionStatus = "submitting"
	SubmissionAccepted       SubmissionStatus = "accepted"
	SubmissionUnknownOutcome SubmissionStatus = "unknown_outcome"
	SubmissionCompleted      SubmissionStatus = "completed"
	SubmissionFailed         SubmissionStatus = "failed"
	SubmissionCancelled      SubmissionStatus = "cancelled"
	SubmissionReconciled     SubmissionStatus = "reconciled"

	requestKeySubmissionConstraint = "uq_video_generation_submissions_request_key"
	activeShotSubmissionConstraint = "uq_video_generation_submissions_active_shot"
)

var submissionTransitions = map[SubmissionStatus]map[SubmissionStatus]bool{
	SubmissionPrepared: {
		SubmissionSubmitting: true,
		SubmissionCancelled:  true,
	},
	SubmissionSubmitting: {
		SubmissionAccepted:       true,
		SubmissionUnknownOutcome: true,
		SubmissionReconciled:     true,
	},
	SubmissionAccepted: {
		SubmissionCompleted: true,
		SubmissionFailed:    true,
	},
	SubmissionUnknownOutcome: {
		SubmissionReconciled: true,
	},
}

type Submission struct {
	ID                string           `json:"id"`
	RequestKey        string           `json:"requestKey"`
	ProjectID         string           `json:"projectId"`
	ShotID            string           `json:"shotId"`
	RequestHash       string           `json:"requestHash"`
	PromptHash        string           `json:"promptHash"`
	CapabilityVersion string           `json:"capabilityVersion"`
	RequestSnapshot   json.RawMessage  `json:"requestSnapshot"`
	Status            SubmissionStatus `json:"status"`
	UpstreamTaskID    string           `json:"upstreamTaskId"`
	GenerationID      string           `json:"generationId"`
	ErrorMessage      string           `json:"errorMessage"`
	CreateTime        string           `json:"createTime"`
	UpdateTime        string           `json:"updateTime"`
}

type PrepareSubmissionInput struct {
	RequestKey        string
	ProjectID         string
	ShotID            string
	RequestHash       string
	PromptHash        string
	CapabilityVersion string
	RequestSnapshot   json.RawMessage
}

type SubmissionTransition struct {
	UpstreamTaskID string
	GenerationID   string
	ErrorMessage   string
}

type SubmissionStore struct {
	db *sql.DB
}

type SubmissionValidationError struct {
	Code    string
	Field   string
	Message string
}

func (e *SubmissionValidationError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type SubmissionTransitionError struct {
	Code       string
	RequestKey string
	From       SubmissionStatus
	To         SubmissionStatus
	Current    SubmissionStatus
}

func (e *SubmissionTransitionError) Error() string {
	if e != nil && e.Code == "submission_state_conflict" {
		return "视频生成提交状态已变化，请刷新后查看最新状态。"
	}
	return "不允许执行当前视频生成提交状态转换。"
}

type ActiveSubmissionError struct {
	Code     string
	ShotID   string
	Existing Submission
}

func (e *ActiveSubmissionError) Error() string {
	return "当前分镜已有生成任务，请先查看现有任务状态。"
}

type RequestKeyConflictError struct {
	Code       string
	RequestKey string
	Existing   Submission
}

func (e *RequestKeyConflictError) Error() string {
	return "同一请求键不能用于不同的视频生成内容。"
}

type ReconciliationTaskConflictError struct {
	Code       string
	RequestKey string
	TaskID     string
}

func (e *ReconciliationTaskConflictError) Error() string {
	return "对账任务与已保存的视频生成任务不一致。"
}

func NewSubmissionStore(database *sql.DB) *SubmissionStore {
	return &SubmissionStore{db: database}
}

func validateSubmissionTransition(from, to SubmissionStatus) error {
	if submissionTransitions[from][to] {
		return nil
	}
	return &SubmissionTransitionError{
		Code: "submission_transition_invalid",
		From: from,
		To:   to,
	}
}

func isActiveSubmissionStatus(status SubmissionStatus) bool {
	switch status {
	case SubmissionPrepared, SubmissionSubmitting, SubmissionAccepted, SubmissionUnknownOutcome:
		return true
	default:
		return false
	}
}

func (s *SubmissionStore) Prepare(ctx context.Context, input PrepareSubmissionInput) (Submission, bool, error) {
	if s == nil || s.db == nil {
		return Submission{}, false, errors.New("video submission store is not configured")
	}
	normalized, projectID, shotID, err := normalizePrepareSubmissionInput(input)
	if err != nil {
		return Submission{}, false, err
	}

	item, err := scanSubmission(s.db.QueryRowContext(ctx, `
		INSERT INTO video_generation_submissions (
			request_key, project_id, shot_id, request_hash, prompt_hash,
			capability_version, request_snapshot, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'prepared')
		ON CONFLICT (request_key) DO NOTHING
		RETURNING `+submissionSelectColumns,
		normalized.RequestKey,
		projectID,
		shotID,
		normalized.RequestHash,
		normalized.PromptHash,
		normalized.CapabilityVersion,
		[]byte(normalized.RequestSnapshot),
	))
	if err == nil {
		return item, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) || isSubmissionConstraint(err, requestKeySubmissionConstraint) {
		existing, lookupErr := s.GetByRequestKey(ctx, normalized.RequestKey)
		if lookupErr != nil {
			return Submission{}, false, lookupErr
		}
		if !sameSubmissionIntent(existing, normalized) {
			return Submission{}, false, &RequestKeyConflictError{
				Code:       "request_key_payload_conflict",
				RequestKey: normalized.RequestKey,
				Existing:   existing,
			}
		}
		return existing, false, nil
	}
	if isSubmissionConstraint(err, activeShotSubmissionConstraint) && normalized.ShotID != "" {
		existing, lookupErr := s.FindActiveByShot(ctx, normalized.ShotID)
		if lookupErr != nil {
			return Submission{}, false, err
		}
		return Submission{}, false, &ActiveSubmissionError{
			Code:     "shot_submission_active",
			ShotID:   normalized.ShotID,
			Existing: existing,
		}
	}
	return Submission{}, false, err
}

func (s *SubmissionStore) GetByRequestKey(ctx context.Context, requestKey string) (Submission, error) {
	if s == nil || s.db == nil {
		return Submission{}, errors.New("video submission store is not configured")
	}
	requestKey, err := normalizeSubmissionRequestKey(requestKey)
	if err != nil {
		return Submission{}, err
	}
	return scanSubmission(s.db.QueryRowContext(ctx, `
		SELECT `+submissionSelectColumns+`
		  FROM video_generation_submissions
		 WHERE request_key=$1`, requestKey))
}

func (s *SubmissionStore) FindActiveByShot(ctx context.Context, shotID string) (Submission, error) {
	if s == nil || s.db == nil {
		return Submission{}, errors.New("video submission store is not configured")
	}
	parsedShotID, _, err := normalizeSubmissionID("shotId", shotID, false)
	if err != nil {
		return Submission{}, err
	}
	return scanSubmission(s.db.QueryRowContext(ctx, `
		SELECT `+submissionSelectColumns+`
		  FROM video_generation_submissions
		 WHERE shot_id=$1
		   AND status IN ('prepared','submitting','accepted','unknown_outcome')
		 ORDER BY create_time DESC, id DESC
		 LIMIT 1`, parsedShotID))
}

func (s *SubmissionStore) Transition(
	ctx context.Context,
	requestKey string,
	from SubmissionStatus,
	to SubmissionStatus,
	change SubmissionTransition,
) (Submission, error) {
	if s == nil || s.db == nil {
		return Submission{}, errors.New("video submission store is not configured")
	}
	requestKey, err := normalizeSubmissionRequestKey(requestKey)
	if err != nil {
		return Submission{}, err
	}
	if err := validateSubmissionTransition(from, to); err != nil {
		return Submission{}, err
	}
	change, generationID, err := normalizeSubmissionTransition(to, change)
	if err != nil {
		return Submission{}, err
	}

	item, err := scanSubmission(s.db.QueryRowContext(ctx, `
		UPDATE video_generation_submissions
		   SET status=$3,
		       upstream_task_id=CASE WHEN $4='' THEN upstream_task_id ELSE $4 END,
		       generation_id=COALESCE($5,generation_id),
		       error_message=$6,
		       update_time=now()
		 WHERE request_key=$1
		   AND status=$2
		RETURNING `+submissionSelectColumns,
		requestKey,
		string(from),
		string(to),
		change.UpstreamTaskID,
		generationID,
		change.ErrorMessage,
	))
	if err == nil {
		return item, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Submission{}, err
	}
	current, lookupErr := s.GetByRequestKey(ctx, requestKey)
	if lookupErr != nil {
		return Submission{}, lookupErr
	}
	if current.Status == to {
		if transitionLinkConflicts(current, change) {
			return Submission{}, &ReconciliationTaskConflictError{
				Code:       "reconciliation_task_conflict",
				RequestKey: requestKey,
				TaskID:     change.UpstreamTaskID,
			}
		}
		return current, nil
	}
	return Submission{}, &SubmissionTransitionError{
		Code:       "submission_state_conflict",
		RequestKey: requestKey,
		From:       from,
		To:         to,
		Current:    current.Status,
	}
}

func (s *SubmissionStore) Reconcile(ctx context.Context, requestKey, taskID, generationID string) (Submission, error) {
	requestKey, err := normalizeSubmissionRequestKey(requestKey)
	if err != nil {
		return Submission{}, err
	}
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Submission{}, submissionValidationError("upstream_task_id_required", "upstreamTaskId", "对账需要有效的中转站任务 ID。")
	}
	_, generationID, err = normalizeSubmissionID("generationId", generationID, false)
	if err != nil {
		return Submission{}, err
	}

	current, err := s.GetByRequestKey(ctx, requestKey)
	if err != nil {
		return Submission{}, err
	}
	if (current.UpstreamTaskID != "" && current.UpstreamTaskID != taskID) ||
		(current.GenerationID != "" && current.GenerationID != generationID) {
		return Submission{}, &ReconciliationTaskConflictError{
			Code:       "reconciliation_task_conflict",
			RequestKey: requestKey,
			TaskID:     taskID,
		}
	}
	if current.Status == SubmissionReconciled {
		return current, nil
	}
	if current.Status != SubmissionSubmitting && current.Status != SubmissionUnknownOutcome {
		return Submission{}, &SubmissionTransitionError{
			Code:       "submission_transition_invalid",
			RequestKey: requestKey,
			From:       current.Status,
			To:         SubmissionReconciled,
			Current:    current.Status,
		}
	}
	return s.Transition(ctx, requestKey, current.Status, SubmissionReconciled, SubmissionTransition{
		UpstreamTaskID: taskID,
		GenerationID:   generationID,
	})
}

const submissionSelectColumns = `
	id::text,
	request_key::text,
	COALESCE(project_id::text,''),
	COALESCE(shot_id::text,''),
	request_hash,
	prompt_hash,
	capability_version,
	request_snapshot,
	status,
	upstream_task_id,
	COALESCE(generation_id::text,''),
	error_message,
	create_time,
	update_time`

type submissionScanner interface {
	Scan(dest ...any) error
}

func scanSubmission(scanner submissionScanner) (Submission, error) {
	var item Submission
	var snapshot []byte
	var status string
	var createTime time.Time
	var updateTime time.Time
	if err := scanner.Scan(
		&item.ID,
		&item.RequestKey,
		&item.ProjectID,
		&item.ShotID,
		&item.RequestHash,
		&item.PromptHash,
		&item.CapabilityVersion,
		&snapshot,
		&status,
		&item.UpstreamTaskID,
		&item.GenerationID,
		&item.ErrorMessage,
		&createTime,
		&updateTime,
	); err != nil {
		return Submission{}, err
	}
	item.RequestSnapshot = append(json.RawMessage(nil), snapshot...)
	item.Status = SubmissionStatus(status)
	item.CreateTime = formatTime(createTime)
	item.UpdateTime = formatTime(updateTime)
	return item, nil
}

func normalizePrepareSubmissionInput(input PrepareSubmissionInput) (PrepareSubmissionInput, any, any, error) {
	requestKey, err := normalizeSubmissionRequestKey(input.RequestKey)
	if err != nil {
		return PrepareSubmissionInput{}, nil, nil, err
	}
	projectID, normalizedProjectID, err := normalizeSubmissionID("projectId", input.ProjectID, true)
	if err != nil {
		return PrepareSubmissionInput{}, nil, nil, err
	}
	shotID, normalizedShotID, err := normalizeSubmissionID("shotId", input.ShotID, true)
	if err != nil {
		return PrepareSubmissionInput{}, nil, nil, err
	}
	input.RequestHash = strings.TrimSpace(input.RequestHash)
	input.PromptHash = strings.TrimSpace(input.PromptHash)
	input.CapabilityVersion = strings.TrimSpace(input.CapabilityVersion)
	if input.RequestHash == "" {
		return PrepareSubmissionInput{}, nil, nil, submissionValidationError("request_hash_required", "requestHash", "视频生成提交缺少请求摘要。")
	}
	if input.PromptHash == "" {
		return PrepareSubmissionInput{}, nil, nil, submissionValidationError("prompt_hash_required", "promptHash", "视频生成提交缺少提示词摘要。")
	}
	if input.CapabilityVersion == "" {
		return PrepareSubmissionInput{}, nil, nil, submissionValidationError("capability_version_required", "capabilityVersion", "视频生成提交缺少能力版本。")
	}
	snapshot := json.RawMessage(strings.TrimSpace(string(input.RequestSnapshot)))
	if len(snapshot) == 0 || !json.Valid(snapshot) {
		return PrepareSubmissionInput{}, nil, nil, submissionValidationError("request_snapshot_invalid", "requestSnapshot", "视频生成提交快照必须是有效 JSON。")
	}
	input.RequestKey = requestKey
	input.ProjectID = normalizedProjectID
	input.ShotID = normalizedShotID
	input.RequestSnapshot = append(json.RawMessage(nil), snapshot...)
	return input, projectID, shotID, nil
}

func normalizeSubmissionTransition(to SubmissionStatus, change SubmissionTransition) (SubmissionTransition, any, error) {
	change.UpstreamTaskID = strings.TrimSpace(change.UpstreamTaskID)
	change.GenerationID = strings.TrimSpace(change.GenerationID)
	change.ErrorMessage = strings.TrimSpace(change.ErrorMessage)
	if to == SubmissionAccepted || to == SubmissionReconciled {
		if change.UpstreamTaskID == "" {
			return SubmissionTransition{}, nil, submissionValidationError("upstream_task_id_required", "upstreamTaskId", "当前提交状态需要有效的中转站任务 ID。")
		}
		if change.GenerationID == "" {
			return SubmissionTransition{}, nil, submissionValidationError("generation_id_required", "generationId", "当前提交状态需要关联本地视频记录。")
		}
	}
	generationID, normalizedGenerationID, err := normalizeSubmissionID("generationId", change.GenerationID, true)
	if err != nil {
		return SubmissionTransition{}, nil, err
	}
	change.GenerationID = normalizedGenerationID
	return change, generationID, nil
}

func normalizeSubmissionRequestKey(requestKey string) (string, error) {
	trimmed := strings.TrimSpace(requestKey)
	if requestKey != trimmed || !isNonZeroUUID(trimmed) {
		return "", submissionValidationError("request_key_invalid", "requestKey", "视频生成请求键必须是非零 UUID。")
	}
	return trimmed, nil
}

func normalizeSubmissionID(field, raw string, optional bool) (any, string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" && optional {
		return nil, "", nil
	}
	value, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || value <= 0 {
		return nil, "", submissionValidationError("submission_id_invalid", field, fmt.Sprintf("%s 必须是正整数。", field))
	}
	return value, strconv.FormatInt(value, 10), nil
}

func sameSubmissionIntent(existing Submission, input PrepareSubmissionInput) bool {
	return existing.ProjectID == input.ProjectID &&
		existing.ShotID == input.ShotID &&
		existing.RequestHash == input.RequestHash &&
		existing.PromptHash == input.PromptHash &&
		existing.CapabilityVersion == input.CapabilityVersion
}

func transitionLinkConflicts(current Submission, change SubmissionTransition) bool {
	return (change.UpstreamTaskID != "" && current.UpstreamTaskID != "" && current.UpstreamTaskID != change.UpstreamTaskID) ||
		(change.GenerationID != "" && current.GenerationID != "" && current.GenerationID != change.GenerationID)
}

func isSubmissionConstraint(err error, constraint string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == constraint
}

func submissionValidationError(code, field, message string) *SubmissionValidationError {
	return &SubmissionValidationError{Code: code, Field: field, Message: message}
}
