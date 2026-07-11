package videoproject

import (
	"context"
	"database/sql"
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

type WorkflowShotStatus struct {
	CanGenerate bool          `json:"canGenerate"`
	Readiness   ShotReadiness `json:"readiness"`
	Shot        Shot          `json:"shot"`
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
	active := map[string]string{}
	pid, err := parseID(projectID)
	if err != nil {
		return WorkflowStatus{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT shot_id::text, status
		FROM video_generation_submissions
		WHERE shot_id IN (SELECT id FROM video_shots WHERE project_id=$1)
		  AND status IN ('prepared','submitting','accepted','unknown_outcome','reconciled')`, pid)
	if err != nil && err != sql.ErrNoRows {
		return WorkflowStatus{}, err
	}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var shotID, status string
			if err := rows.Scan(&shotID, &status); err != nil {
				return WorkflowStatus{}, err
			}
			active[shotID] = status
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
		state := ComputeShotReadiness(ShotWorkflowFacts{
			ActionDescription:  shot.ActionDescription,
			GenerationRevision: shot.GenerationRevision,
			LatestStatus:       shot.Status,
			LinkedTaskActive:   shot.Status == "generating",
			Selected:           selected,
			SubmissionStatus:   active[shot.ID],
		})
		readiness = append(readiness, state)
		statuses = append(statuses, WorkflowShotStatus{Shot: shot, Readiness: state, CanGenerate: state.CanGenerate()})
	}
	facts := WorkflowFacts{
		AssetCount:        assetCount,
		FinalVideoCurrent: project.FinalVideoURL != "" && project.FinalVideoInputHash != "",
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
