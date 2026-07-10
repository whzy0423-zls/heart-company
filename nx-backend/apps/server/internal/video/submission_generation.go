package video

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

type generationSubmissionSnapshot struct {
	Model             string               `json:"model"`
	Prompt            string               `json:"prompt"`
	Seconds           int                  `json:"seconds"`
	AspectRatio       string               `json:"aspectRatio"`
	Images            []string             `json:"images"`
	Videos            []string             `json:"videos"`
	Audios            []string             `json:"audios"`
	References        []CanonicalReference `json:"references"`
	CapabilityVersion string               `json:"capabilityVersion"`
	GatewayContract   string               `json:"gatewayContract"`
	GatewayVersion    string               `json:"gatewayVersion"`
	GatewayPayload    map[string]any       `json:"gatewayPayload"`
}

type preparedGenerationSubmission struct {
	request     GenerateRequest
	references  CanonicalReferences
	snapshot    json.RawMessage
	requestHash string
	promptHash  string
	imageURL    string
	images      []string
	videos      []string
	audios      []string
	projectID   string
	shotID      string
}

type ReconciliationResult struct {
	Submission Submission `json:"submission"`
	Generation Generation `json:"generation"`
}

type generationRecordInput struct {
	Model       string
	Prompt      string
	ImageURL    string
	Images      []string
	Videos      []string
	Audios      []string
	TaskID      string
	Seconds     int
	AspectRatio string
	Status      string
	ProjectID   string
	ShotID      string
}

const submissionPersistenceTimeout = 10 * time.Second

func submissionPersistenceContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, submissionPersistenceTimeout)
}

func (s *Store) prepareGenerationSubmission(input GenerateInput, model, prompt string, seconds int, aspectRatio string, images, videos, audios []string) (preparedGenerationSubmission, error) {
	requestKey := strings.TrimSpace(input.RequestKey)
	if requestKey == "" {
		var err error
		requestKey, err = newVideoRequestKey()
		if err != nil {
			return preparedGenerationSubmission{}, err
		}
	}
	if _, err := normalizeSubmissionRequestKey(requestKey); err != nil {
		return preparedGenerationSubmission{}, err
	}

	references, err := CanonicalizeReferences(legacyGatewayReferences(images, videos, audios))
	if err != nil {
		return preparedGenerationSubmission{}, err
	}
	capabilities := ResolveCapabilities(CapabilityConfig{
		Model:           model,
		ModelProfile:    s.modelProfile,
		GatewayContract: s.gatewayContract,
	})
	capabilityVersion := strings.TrimSpace(input.CapabilityVersion)
	if capabilityVersion == "" {
		capabilityVersion = capabilities.CapabilityVersion
	} else if capabilityVersion != capabilities.CapabilityVersion {
		return preparedGenerationSubmission{}, validationError(
			"capability_version_stale",
			"capabilityVersion",
			"视频模型能力已经变化，请重新确认生成参数。",
			"刷新模型能力后重新提交。",
			&capabilities,
		)
	}
	request := GenerateRequest{
		Model:             model,
		Prompt:            prompt,
		Duration:          seconds,
		AspectRatio:       aspectRatio,
		TaskMode:          "reference",
		References:        canonicalReferenceSnapshots(references),
		RequestKey:        requestKey,
		CapabilityVersion: capabilityVersion,
	}
	contract, err := validatedGatewayContract(s.gatewayContract)
	if err != nil {
		return preparedGenerationSubmission{}, err
	}
	payload, err := MapGatewayPayload(request, references, contract)
	if err != nil {
		return preparedGenerationSubmission{}, err
	}
	snapshot, err := json.Marshal(generationSubmissionSnapshot{
		Model:             model,
		Prompt:            prompt,
		Seconds:           seconds,
		AspectRatio:       aspectRatio,
		Images:            append([]string(nil), images...),
		Videos:            append([]string(nil), videos...),
		Audios:            append([]string(nil), audios...),
		References:        append([]CanonicalReference(nil), references.References...),
		CapabilityVersion: capabilityVersion,
		GatewayContract:   contract.Name,
		GatewayVersion:    contract.Version,
		GatewayPayload:    payload,
	})
	if err != nil {
		return preparedGenerationSubmission{}, err
	}
	imageURL := ""
	if len(images) > 0 {
		imageURL = images[0]
	}
	return preparedGenerationSubmission{
		request:     request,
		references:  references,
		snapshot:    snapshot,
		requestHash: hashGenerationValue(snapshot),
		promptHash:  hashGenerationValue([]byte(prompt)),
		imageURL:    imageURL,
		images:      append([]string(nil), images...),
		videos:      append([]string(nil), videos...),
		audios:      append([]string(nil), audios...),
		projectID:   strings.TrimSpace(input.ProjectID),
		shotID:      strings.TrimSpace(input.ShotID),
	}, nil
}

func (s *Store) existingGenerateIntent(
	ctx context.Context,
	input GenerateInput,
	model string,
	prompt string,
	seconds int,
	aspectRatio string,
	images []string,
	videos []string,
	audios []string,
) (Submission, bool, error) {
	if strings.TrimSpace(input.RequestKey) == "" {
		return Submission{}, false, nil
	}
	submission, err := s.submissions.GetByRequestKey(ctx, input.RequestKey)
	if errors.Is(err, sql.ErrNoRows) {
		return Submission{}, false, nil
	}
	if err != nil {
		return Submission{}, false, err
	}
	var snapshot generationSubmissionSnapshot
	if err := json.Unmarshal(submission.RequestSnapshot, &snapshot); err != nil {
		return Submission{}, false, submissionValidationError("request_snapshot_invalid", "requestSnapshot", "已保存的视频生成请求快照无效。")
	}
	_, projectID, err := normalizeSubmissionID("projectId", input.ProjectID, true)
	if err != nil {
		return Submission{}, false, err
	}
	_, shotID, err := normalizeSubmissionID("shotId", input.ShotID, true)
	if err != nil {
		return Submission{}, false, err
	}
	modelChanged := strings.TrimSpace(input.Model) != "" && snapshot.Model != model
	secondsChanged := input.Seconds != 0 && snapshot.Seconds != seconds
	aspectRatioChanged := strings.TrimSpace(input.AspectRatio) != "" && snapshot.AspectRatio != aspectRatio
	if modelChanged ||
		snapshot.Prompt != prompt ||
		secondsChanged ||
		aspectRatioChanged ||
		!slices.Equal(snapshot.Images, images) ||
		!slices.Equal(snapshot.Videos, videos) ||
		!slices.Equal(snapshot.Audios, audios) ||
		submission.ProjectID != projectID ||
		submission.ShotID != shotID {
		return Submission{}, false, &RequestKeyConflictError{
			Code:       "request_key_payload_conflict",
			RequestKey: submission.RequestKey,
			Existing:   submission,
		}
	}
	if submission.Status == SubmissionPrepared {
		capabilities := ResolveCapabilities(CapabilityConfig{
			Model:           model,
			ModelProfile:    s.modelProfile,
			GatewayContract: s.gatewayContract,
		})
		if snapshot.CapabilityVersion != capabilities.CapabilityVersion {
			return Submission{}, false, validationError(
				"capability_version_stale",
				"capabilityVersion",
				"视频模型能力已经变化，请重新确认生成参数。",
				"取消旧的待提交记录并创建新的生成版本。",
				&capabilities,
			)
		}
	}
	return submission, true, nil
}

func (s *Store) existingSubmissionGeneration(ctx context.Context, submission Submission) (Generation, error) {
	if submission.GenerationID != "" {
		generation, err := s.Generation(ctx, submission.GenerationID)
		if err != nil {
			return Generation{}, &LocalLinkageError{
				RequestKey:   submission.RequestKey,
				SubmissionID: submission.ID,
				TaskID:       submission.UpstreamTaskID,
			}
		}
		return attachSubmission(generation, submission), nil
	}
	switch submission.Status {
	case SubmissionPrepared, SubmissionSubmitting:
		return Generation{}, &SubmissionInProgressError{
			RequestKey:   submission.RequestKey,
			SubmissionID: submission.ID,
			Status:       submission.Status,
		}
	case SubmissionUnknownOutcome:
		return Generation{}, &UnknownOutcomeError{
			RequestKey:   submission.RequestKey,
			SubmissionID: submission.ID,
			Persisted:    true,
		}
	default:
		return Generation{}, &SubmissionTerminalError{
			RequestKey:   submission.RequestKey,
			SubmissionID: submission.ID,
			Status:       submission.Status,
		}
	}
}

func (s *Store) hydrateGenerationSubmission(ctx context.Context, generation Generation) (Generation, error) {
	if s == nil || s.submissions == nil || generation.ID == "" {
		return generation, nil
	}
	submission, err := s.submissions.GetByGenerationID(ctx, generation.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return generation, nil
	}
	if err != nil {
		return Generation{}, err
	}
	if submission.Status == SubmissionAccepted {
		switch generation.Status {
		case "completed", "succeeded":
			submission, err = s.submissions.Transition(ctx, submission.RequestKey, SubmissionAccepted, SubmissionCompleted, SubmissionTransition{})
		case "failed":
			submission, err = s.submissions.Transition(ctx, submission.RequestKey, SubmissionAccepted, SubmissionFailed, SubmissionTransition{
				ErrorMessage: generation.ErrorMessage,
			})
		}
		if err != nil {
			return Generation{}, err
		}
	}
	return attachSubmission(generation, submission), nil
}

// ReconcileSubmission attaches a known upstream task to the persisted intent.
// It only performs database work; callers that discover task IDs through a
// configured safe lookup must do that GET before entering this method.
func (s *Store) ReconcileSubmission(ctx context.Context, requestKey, taskID string) (ReconciliationResult, error) {
	if s == nil || s.db == nil || s.submissions == nil {
		return ReconciliationResult{}, errors.New("video store is not configured")
	}
	requestKey, err := normalizeSubmissionRequestKey(requestKey)
	if err != nil {
		return ReconciliationResult{}, err
	}
	taskID, err = normalizeUpstreamTaskID(taskID)
	if err != nil {
		return ReconciliationResult{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ReconciliationResult{}, err
	}
	defer tx.Rollback()

	submission, err := scanSubmission(tx.QueryRowContext(ctx, `
		SELECT `+submissionSelectColumns+`
		  FROM video_generation_submissions
		 WHERE request_key=$1
		 FOR UPDATE`, requestKey))
	if err != nil {
		return ReconciliationResult{}, err
	}
	if submission.UpstreamTaskID != "" && submission.UpstreamTaskID != taskID {
		return ReconciliationResult{}, reconciliationTaskConflict(requestKey, taskID)
	}
	if submission.Status != SubmissionSubmitting &&
		submission.Status != SubmissionUnknownOutcome &&
		submission.Status != SubmissionReconciled {
		return ReconciliationResult{}, &SubmissionTransitionError{
			Code:       "submission_transition_invalid",
			RequestKey: requestKey,
			From:       submission.Status,
			To:         SubmissionReconciled,
			Current:    submission.Status,
		}
	}

	generation, err := reconcileGeneration(ctx, tx, submission, taskID)
	if err != nil {
		return ReconciliationResult{}, err
	}
	if generation.TaskID != taskID {
		return ReconciliationResult{}, reconciliationTaskConflict(requestKey, taskID)
	}

	if submission.Status != SubmissionReconciled ||
		submission.UpstreamTaskID == "" ||
		submission.GenerationID == "" {
		generationID, _, normalizeErr := normalizeSubmissionID("generationId", generation.ID, false)
		if normalizeErr != nil {
			return ReconciliationResult{}, normalizeErr
		}
		submission, err = scanSubmission(tx.QueryRowContext(ctx, `
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
			string(submission.Status),
			string(SubmissionReconciled),
			taskID,
			generationID,
			"",
		))
		if err != nil {
			if isSubmissionConstraint(err, upstreamTaskSubmissionConstraint) {
				return ReconciliationResult{}, reconciliationTaskConflict(requestKey, taskID)
			}
			return ReconciliationResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return ReconciliationResult{}, err
	}
	generation = attachSubmission(generation, submission)
	return ReconciliationResult{Submission: submission, Generation: generation}, nil
}

func reconcileGeneration(ctx context.Context, tx *sql.Tx, submission Submission, taskID string) (Generation, error) {
	if submission.GenerationID != "" {
		generation, err := generationByID(ctx, tx, submission.GenerationID)
		if err == nil {
			return generation, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return Generation{}, err
		}
	}
	generation, err := generationByTaskID(ctx, tx, taskID)
	if err == nil {
		return generation, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Generation{}, err
	}
	var snapshot generationSubmissionSnapshot
	if err := json.Unmarshal(submission.RequestSnapshot, &snapshot); err != nil {
		return Generation{}, submissionValidationError("request_snapshot_invalid", "requestSnapshot", "已保存的视频生成请求快照无法用于对账。")
	}
	id, err := insertGenerationRecord(ctx, tx, generationRecordInput{
		Model:       snapshot.Model,
		Prompt:      snapshot.Prompt,
		ImageURL:    firstString(snapshot.Images),
		Images:      snapshot.Images,
		Videos:      snapshot.Videos,
		Audios:      snapshot.Audios,
		TaskID:      taskID,
		Seconds:     snapshot.Seconds,
		AspectRatio: snapshot.AspectRatio,
		Status:      "queued",
		ProjectID:   submission.ProjectID,
		ShotID:      submission.ShotID,
	})
	if err != nil {
		return Generation{}, err
	}
	return generationByID(ctx, tx, id)
}

type generationQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func insertGenerationRecord(ctx context.Context, queryer generationQueryer, input generationRecordInput) (string, error) {
	projectID, _, err := normalizeSubmissionID("projectId", input.ProjectID, true)
	if err != nil {
		return "", err
	}
	shotID, _, err := normalizeSubmissionID("shotId", input.ShotID, true)
	if err != nil {
		return "", err
	}
	images, err := json.Marshal(input.Images)
	if err != nil {
		return "", err
	}
	videos, err := json.Marshal(input.Videos)
	if err != nil {
		return "", err
	}
	audios, err := json.Marshal(input.Audios)
	if err != nil {
		return "", err
	}
	var id string
	err = queryer.QueryRowContext(ctx, `
		INSERT INTO video_generations (
			provider, model, prompt, image_url, used_images, used_videos, used_audios,
			task_id, seconds, aspect_ratio, status, project_id, shot_id
		) VALUES ('newapi',$1,$2,$3,$4::jsonb,$5::jsonb,$6::jsonb,$7,$8,$9,$10,$11,$12)
		RETURNING id::text`,
		input.Model,
		input.Prompt,
		input.ImageURL,
		string(images),
		string(videos),
		string(audios),
		input.TaskID,
		input.Seconds,
		input.AspectRatio,
		input.Status,
		projectID,
		shotID,
	).Scan(&id)
	return id, err
}

func generationByID(ctx context.Context, queryer generationQueryer, id string) (Generation, error) {
	return scanGeneration(queryer.QueryRowContext(ctx, `
		SELECT `+generationSelectColumns+`
		  FROM video_generations
		 WHERE id=$1`, id))
}

func generationByTaskID(ctx context.Context, queryer generationQueryer, taskID string) (Generation, error) {
	return scanGeneration(queryer.QueryRowContext(ctx, `
		SELECT `+generationSelectColumns+`
		  FROM video_generations
		 WHERE task_id=$1
		 ORDER BY id
		 LIMIT 1`, taskID))
}

const generationSelectColumns = `
	id::text, provider, model, prompt, image_url, task_id, seconds, aspect_ratio,
	COALESCE(video_asset_id::text,''), video_url, duration, fps, width, height,
	status, error_message, create_time, update_time`

func scanGeneration(scanner submissionScanner) (Generation, error) {
	var item Generation
	var createTime time.Time
	var updateTime time.Time
	if err := scanner.Scan(
		&item.ID,
		&item.Provider,
		&item.Model,
		&item.Prompt,
		&item.ImageURL,
		&item.TaskID,
		&item.Seconds,
		&item.AspectRatio,
		&item.VideoAssetID,
		&item.VideoURL,
		&item.Duration,
		&item.FPS,
		&item.Width,
		&item.Height,
		&item.Status,
		&item.ErrorMessage,
		&createTime,
		&updateTime,
	); err != nil {
		return Generation{}, err
	}
	item.CreateTime = formatTime(createTime)
	item.UpdateTime = formatTime(updateTime)
	return item, nil
}

func normalizeUpstreamTaskID(taskID string) (string, error) {
	trimmed := strings.TrimSpace(taskID)
	if trimmed == "" || len(trimmed) > 256 || containsURLControlCharacter(trimmed) {
		return "", submissionValidationError("upstream_task_id_invalid", "upstreamTaskId", "中转站任务 ID 无效。")
	}
	return trimmed, nil
}

func reconciliationTaskConflict(requestKey, taskID string) *ReconciliationTaskConflictError {
	return &ReconciliationTaskConflictError{
		Code:       "reconciliation_task_conflict",
		RequestKey: requestKey,
		TaskID:     taskID,
	}
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func attachSubmission(generation Generation, submission Submission) Generation {
	generation.RequestKey = submission.RequestKey
	generation.SubmissionID = submission.ID
	generation.SubmissionStatus = submission.Status
	return generation
}

func newVideoRequestKey() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("create video request key: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	raw := hex.EncodeToString(value[:])
	return raw[0:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:32], nil
}

func hashGenerationValue(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
