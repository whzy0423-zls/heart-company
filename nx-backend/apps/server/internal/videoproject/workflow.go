package videoproject

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

type ShotReadiness string

const (
	ReadinessReady      ShotReadiness = "ready"
	ReadinessIncomplete ShotReadiness = "incomplete"
	ReadinessGenerating ShotReadiness = "generating"
	ReadinessRecovery   ShotReadiness = "recovery"
	ReadinessCompleted  ShotReadiness = "completed"
	ReadinessStale      ShotReadiness = "stale"
	ReadinessFailed     ShotReadiness = "failed"
)

func (r ShotReadiness) CanGenerate() bool {
	return r == ReadinessReady || r == ReadinessStale || r == ReadinessFailed
}

type SelectedVersionFacts struct {
	Status       string `json:"status"`
	VideoURL     string `json:"videoUrl"`
	ShotRevision int    `json:"shotRevision"`
}

type ShotWorkflowFacts struct {
	ActionDescription  string
	GenerationRevision int
	LatestStatus       string
	LinkedTaskActive   bool
	Selected           *SelectedVersionFacts
	SubmissionStatus   string
}

func ComputeShotReadiness(facts ShotWorkflowFacts) ShotReadiness {
	submissionStatus := strings.ToLower(strings.TrimSpace(facts.SubmissionStatus))
	if submissionStatus == "unknown_outcome" {
		return ReadinessRecovery
	}
	switch submissionStatus {
	case "prepared", "submitting", "accepted", "reconciled":
		return ReadinessGenerating
	}
	if facts.LinkedTaskActive {
		return ReadinessGenerating
	}
	if strings.TrimSpace(facts.ActionDescription) == "" {
		return ReadinessIncomplete
	}
	if facts.Selected != nil {
		selectedValid := canSelectGeneration("selected", "selected", facts.Selected.Status, facts.Selected.VideoURL)
		if selectedValid {
			if facts.Selected.ShotRevision == facts.GenerationRevision {
				return ReadinessCompleted
			}
			return ReadinessStale
		}
		return ReadinessFailed
	}
	switch strings.ToLower(strings.TrimSpace(facts.LatestStatus)) {
	case "failed", "error":
		return ReadinessFailed
	}
	return ReadinessReady
}

type WorkflowStep string

const (
	StepBrief      WorkflowStep = "brief"
	StepAssets     WorkflowStep = "assets"
	StepStoryboard WorkflowStep = "storyboard"
	StepGenerate   WorkflowStep = "generate"
	StepExport     WorkflowStep = "export"
)

type WorkflowStepState string

const (
	StepComplete        WorkflowStepState = "complete"
	StepOptional        WorkflowStepState = "optional"
	StepSkippedExisting WorkflowStepState = "skipped_existing"
	StepBlocked         WorkflowStepState = "blocked"
	StepStale           WorkflowStepState = "stale"
)

type WorkflowFacts struct {
	AssetCount        int64
	FinalVideoCurrent bool
	FinalVideoURL     string
	ScriptContent     string
	ShotReadiness     []ShotReadiness
}

func ComputeWorkflowStepState(facts WorkflowFacts, step WorkflowStep) WorkflowStepState {
	hasShots := len(facts.ShotReadiness) > 0
	switch step {
	case StepBrief:
		if strings.TrimSpace(facts.ScriptContent) != "" {
			return StepComplete
		}
		if hasShots {
			return StepSkippedExisting
		}
		return StepBlocked
	case StepAssets:
		if facts.AssetCount > 0 {
			return StepComplete
		}
		if hasShots {
			return StepSkippedExisting
		}
		return StepOptional
	case StepStoryboard:
		if hasShots {
			return StepComplete
		}
		return StepBlocked
	case StepGenerate:
		if !hasShots {
			return StepBlocked
		}
		allCompleted := true
		for _, readiness := range facts.ShotReadiness {
			if readiness == ReadinessStale {
				return StepStale
			}
			if readiness != ReadinessCompleted {
				allCompleted = false
			}
		}
		if allCompleted {
			return StepComplete
		}
		return StepBlocked
	case StepExport:
		if strings.TrimSpace(facts.FinalVideoURL) == "" {
			return StepBlocked
		}
		if facts.FinalVideoCurrent {
			return StepComplete
		}
		return StepStale
	default:
		return StepBlocked
	}
}

func RecommendedWorkflowStep(facts WorkflowFacts) WorkflowStep {
	if len(facts.ShotReadiness) == 0 {
		return StepBrief
	}
	for _, readiness := range facts.ShotReadiness {
		if readiness == ReadinessIncomplete {
			return StepStoryboard
		}
	}
	for _, readiness := range facts.ShotReadiness {
		if readiness != ReadinessCompleted {
			return StepGenerate
		}
	}
	return StepExport
}

func FilterGeneratableShotIDs(readiness map[string]ShotReadiness, requested []string) []string {
	result := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, shotID := range requested {
		shotID = strings.TrimSpace(shotID)
		if _, exists := seen[shotID]; exists || !readiness[shotID].CanGenerate() {
			continue
		}
		seen[shotID] = struct{}{}
		result = append(result, shotID)
	}
	return result
}

type ScriptParagraph struct {
	Content string `json:"content"`
	Index   int    `json:"index"`
}

type ScriptImportStatus string

const (
	ScriptImportPending  ScriptImportStatus = "pending"
	ScriptImportCreated  ScriptImportStatus = "created"
	ScriptImportExisting ScriptImportStatus = "existing"
	ScriptImportFailed   ScriptImportStatus = "failed"
)

type ScriptImportItem struct {
	Error     string             `json:"error,omitempty"`
	Index     int                `json:"index"`
	ShotID    string             `json:"shotId,omitempty"`
	SourceKey string             `json:"sourceKey"`
	Status    ScriptImportStatus `json:"status"`
}

type CreateShotsFromScriptResult struct {
	Created  []ScriptImportItem `json:"created"`
	Existing []ScriptImportItem `json:"existing"`
	Failed   []ScriptImportItem `json:"failed"`
	Items    []ScriptImportItem `json:"items"`
}

func ShotSourceKey(projectID string, revision, index int, paragraph string) string {
	payload := fmt.Sprintf("%s\n%d\n%d\n%s", strings.TrimSpace(projectID), revision, index, normalizeRevisionText(paragraph))
	return fmt.Sprintf("%x", sha256.Sum256([]byte(payload)))
}

func validateScriptRevision(current, requested int) error {
	if current != requested {
		return fmt.Errorf("script_revision_conflict: current=%d requested=%d", current, requested)
	}
	return nil
}

func prepareScriptImportItems(
	projectID string,
	revision int,
	paragraphs []ScriptParagraph,
	existing map[string]string,
) []ScriptImportItem {
	items := make([]ScriptImportItem, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		content := normalizeRevisionText(paragraph.Content)
		key := ShotSourceKey(projectID, revision, paragraph.Index, content)
		item := ScriptImportItem{Index: paragraph.Index, SourceKey: key, Status: ScriptImportPending}
		if content == "" {
			item.Status = ScriptImportFailed
			item.Error = "分镜段落不能为空"
		} else if shotID := existing[key]; shotID != "" {
			item.Status = ScriptImportExisting
			item.ShotID = shotID
		}
		items = append(items, item)
	}
	return items
}

func groupScriptImportItems(items []ScriptImportItem) CreateShotsFromScriptResult {
	result := CreateShotsFromScriptResult{Items: items}
	for _, item := range items {
		switch item.Status {
		case ScriptImportCreated:
			result.Created = append(result.Created, item)
		case ScriptImportExisting:
			result.Existing = append(result.Existing, item)
		case ScriptImportFailed:
			result.Failed = append(result.Failed, item)
		}
	}
	return result
}

func (s *Store) CreateShotsFromScript(
	ctx context.Context,
	projectID string,
	scriptRevision int,
	paragraphs []ScriptParagraph,
) (CreateShotsFromScriptResult, error) {
	pid, err := parseID(projectID)
	if err != nil {
		return CreateShotsFromScriptResult{}, err
	}
	var currentRevision int
	if err := s.db.QueryRowContext(ctx,
		`SELECT script_revision FROM video_projects WHERE id=$1`, pid,
	).Scan(&currentRevision); err != nil {
		return CreateShotsFromScriptResult{}, err
	}
	if err := validateScriptRevision(currentRevision, scriptRevision); err != nil {
		return CreateShotsFromScriptResult{}, err
	}
	items := prepareScriptImportItems(projectID, scriptRevision, paragraphs, nil)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return CreateShotsFromScriptResult{}, err
	}
	defer tx.Rollback()
	for position := range items {
		item := &items[position]
		if item.Status == ScriptImportFailed {
			continue
		}
		var existingID string
		err := tx.QueryRowContext(ctx,
			`SELECT id::text FROM video_shots WHERE project_id=$1 AND source_key=$2`, pid, item.SourceKey,
		).Scan(&existingID)
		if err == nil {
			item.Status = ScriptImportExisting
			item.ShotID = existingID
			continue
		}
		if err != sql.ErrNoRows {
			item.Status = ScriptImportFailed
			item.Error = err.Error()
			continue
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("SAVEPOINT shot_item_%d", position)); err != nil {
			return CreateShotsFromScriptResult{}, err
		}
		paragraph := paragraphs[position]
		content := normalizeRevisionText(paragraph.Content)
		var shotID string
		err = tx.QueryRowContext(ctx,
			`INSERT INTO video_shots (
			   project_id, order_num, name, script_original_content, action_description,
			   source_key, source_script_revision
			 ) VALUES ($1,$2,$3,$4,$4,$5,$6)
			 RETURNING id::text`,
			pid, paragraph.Index+1, fmt.Sprintf("分镜 %d", paragraph.Index+1), content, item.SourceKey, scriptRevision,
		).Scan(&shotID)
		if err != nil {
			_, _ = tx.ExecContext(ctx, fmt.Sprintf("ROLLBACK TO SAVEPOINT shot_item_%d", position))
			_, _ = tx.ExecContext(ctx, fmt.Sprintf("RELEASE SAVEPOINT shot_item_%d", position))
			item.Status = ScriptImportFailed
			item.Error = err.Error()
			continue
		}
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("RELEASE SAVEPOINT shot_item_%d", position)); err != nil {
			return CreateShotsFromScriptResult{}, err
		}
		item.Status = ScriptImportCreated
		item.ShotID = shotID
	}
	if err := tx.Commit(); err != nil {
		return CreateShotsFromScriptResult{}, err
	}
	return groupScriptImportItems(items), nil
}

type WorkflowShotStatus struct {
	ActiveSubmission *WorkflowActiveSubmission `json:"activeSubmission,omitempty"`
	CanGenerate      bool                      `json:"canGenerate"`
	Readiness        ShotReadiness             `json:"readiness"`
	Shot             Shot                      `json:"shot"`
}

type WorkflowActiveSubmission struct {
	SubmissionID int64  `json:"submissionId"`
	RequestKey   string `json:"requestKey"`
	Status       string `json:"status"`
	TaskID       string `json:"taskId,omitempty"`
}

type WorkflowStatus struct {
	Project         Project                            `json:"project"`
	RecommendedStep WorkflowStep                       `json:"recommendedStep"`
	Shots           []WorkflowShotStatus               `json:"shots"`
	Steps           map[WorkflowStep]WorkflowStepState `json:"steps"`
}

func (s *Store) GetWorkflowStatus(ctx context.Context, projectID string) (WorkflowStatus, error) {
	project, err := s.GetProject(ctx, projectID)
	if err != nil {
		return WorkflowStatus{}, err
	}
	shots, err := s.ListShots(ctx, projectID)
	if err != nil {
		return WorkflowStatus{}, err
	}
	active := map[string]WorkflowActiveSubmission{}
	pid, err := parseID(projectID)
	if err != nil {
		return WorkflowStatus{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT shot_id::text, id, request_key::text, status, task_id
		FROM video_generation_submissions
		WHERE shot_id IN (SELECT id FROM video_shots WHERE project_id=$1)
		  AND status IN ('prepared','submitting','accepted','unknown_outcome','reconciled')`, pid)
	if err != nil && err != sql.ErrNoRows {
		return WorkflowStatus{}, err
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var shotID string
			var submission WorkflowActiveSubmission
			if err := rows.Scan(&shotID, &submission.SubmissionID, &submission.RequestKey, &submission.Status, &submission.TaskID); err != nil {
				return WorkflowStatus{}, err
			}
			active[shotID] = submission
		}
		if err := rows.Err(); err != nil {
			return WorkflowStatus{}, err
		}
	}

	statuses := make([]WorkflowShotStatus, 0, len(shots))
	readiness := make([]ShotReadiness, 0, len(shots))
	var assetCount int64 = project.CharacterCount + project.SceneCount
	for _, shot := range shots {
		assetCount += int64(len(shot.ShotAssets))
		var selected *SelectedVersionFacts
		if shot.SelectedGenerationID != "" {
			selected = &SelectedVersionFacts{
				Status:       shot.SelectedGenerationStatus,
				VideoURL:     shot.VideoURL,
				ShotRevision: shot.SelectedGenerationRevision,
			}
		}
		activeSubmission, hasActiveSubmission := active[shot.ID]
		var activeSubmissionPointer *WorkflowActiveSubmission
		if hasActiveSubmission {
			activeSubmissionPointer = &activeSubmission
		}
		state := ComputeShotReadiness(ShotWorkflowFacts{
			ActionDescription:  shot.ActionDescription,
			GenerationRevision: shot.GenerationRevision,
			LatestStatus:       shot.Status,
			LinkedTaskActive:   shot.Status == "generating",
			Selected:           selected,
			SubmissionStatus:   activeSubmission.Status,
		})
		readiness = append(readiness, state)
		statuses = append(statuses, WorkflowShotStatus{ActiveSubmission: activeSubmissionPointer, Shot: shot, Readiness: state, CanGenerate: state.CanGenerate()})
	}
	finalCurrent := false
	if project.FinalVideoURL != "" && project.FinalVideoInputHash != "" {
		var rawSnapshot []byte
		if err := s.db.QueryRowContext(ctx, `
			SELECT compose_input_snapshot
			FROM video_compose_jobs
			WHERE project_id=$1 AND status='completed' AND compose_input_hash=$2
			ORDER BY id DESC LIMIT 1`, pid, project.FinalVideoInputHash,
		).Scan(&rawSnapshot); err == nil {
			var snapshot ComposeInputSnapshot
			if json.Unmarshal(rawSnapshot, &snapshot) == nil {
				composeFacts := make([]ComposeShotFacts, 0, len(shots))
				for _, shot := range shots {
					composeFacts = append(composeFacts, ComposeShotFacts{
						GenerationRevision: shot.GenerationRevision, OrderNum: shot.OrderNum,
						SelectedGenerationID: shot.SelectedGenerationID,
						SelectedRevision:     shot.SelectedGenerationRevision,
						SelectedStatus:       shot.SelectedGenerationStatus,
						SelectedVideoURL:     shot.VideoURL, ShotID: shot.ID,
					})
				}
				current, err := BuildComposeInputSnapshot(composeFacts, ComposeProjectInput{
					Transition: snapshot.Transition, MusicURL: snapshot.MusicURL,
					EnableSubtitles: snapshot.EnableSubtitles,
					ExcludedShotIDs: snapshot.ExcludedShotIDs, PartialAcknowledged: snapshot.PartialAcknowledged,
				})
				finalCurrent = err == nil && ComposeResultIsCurrent(project.FinalVideoInputHash, current.InputHash)
			}
		}
	}
	facts := WorkflowFacts{
		AssetCount:        assetCount,
		FinalVideoCurrent: finalCurrent,
		FinalVideoURL:     project.FinalVideoURL,
		ScriptContent:     project.ScriptContent,
		ShotReadiness:     readiness,
	}
	steps := map[WorkflowStep]WorkflowStepState{}
	for _, step := range []WorkflowStep{StepBrief, StepAssets, StepStoryboard, StepGenerate, StepExport} {
		steps[step] = ComputeWorkflowStepState(facts, step)
	}
	return WorkflowStatus{
		Project:         project,
		RecommendedStep: RecommendedWorkflowStep(facts),
		Shots:           statuses,
		Steps:           steps,
	}, nil
}
