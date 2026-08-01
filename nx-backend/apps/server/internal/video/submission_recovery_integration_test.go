package video

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	dbstore "nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestRecoverInterruptedSubmissions(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run paid submission recovery integration test")
	}
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := dbstore.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var projectID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_projects (name) VALUES ($1) RETURNING id`,
		fmt.Sprintf("paid-submission-recovery-%d", time.Now().UnixNano()),
	).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var generationIDs []string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_projects WHERE id=$1`, projectID)
		for _, generationID := range generationIDs {
			_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_generations WHERE id=$1::bigint`, generationID)
		}
	})

	type seededSubmission struct {
		requestKey string
		shotID     string
		status     SubmissionStatus
		taskID     string
		error      string
		mode       string
	}
	cases := []struct {
		status SubmissionStatus
		mode   string
	}{
		{status: SubmissionPrepared, mode: config.VideoGenerationModePaid},
		{status: SubmissionSubmitting},
		{status: SubmissionSubmitting, mode: config.VideoGenerationModePaid},
		{status: SubmissionSubmitting, mode: config.VideoGenerationModeDemo},
		{status: SubmissionAccepted, mode: config.VideoGenerationModePaid},
		{status: SubmissionUnknownOutcome, mode: config.VideoGenerationModePaid},
		{status: SubmissionReconciled, mode: config.VideoGenerationModePaid},
		{status: SubmissionCompleted, mode: config.VideoGenerationModePaid},
		{status: SubmissionFailed, mode: config.VideoGenerationModePaid},
		{status: SubmissionCancelled, mode: config.VideoGenerationModePaid},
	}
	seeded := make([]seededSubmission, 0, len(cases))
	for index, testCase := range cases {
		var shotID string
		if err := database.QueryRowContext(ctx, `
			INSERT INTO video_shots (project_id, order_num, name, action_description)
			VALUES ($1,$2,$3,'recovery action') RETURNING id::text`,
			projectID, index+1, fmt.Sprintf("recovery shot %d", index+1),
		).Scan(&shotID); err != nil {
			t.Fatal(err)
		}
		requestKey := fmt.Sprintf("%08d-1111-4111-8111-%012d", index+1, index+1)
		taskID := fmt.Sprintf("task-%d", index+1)
		if testCase.status == SubmissionSubmitting && index == 1 {
			taskID = ""
		}
		snapshot := fmt.Sprintf(`{"aspectRatio":"16:9","generationMode":%q,"images":[],"model":"paid-model","prompt":"recover","seconds":5,"shotRevision":0}`, testCase.mode)
		if testCase.mode == "" {
			snapshot = `{"aspectRatio":"16:9","images":[],"model":"paid-model","prompt":"recover","seconds":5,"shotRevision":0}`
		}
		errorMessage := "original error"
		if _, err := database.ExecContext(ctx, `
			INSERT INTO video_generation_submissions
			    (request_key, shot_id, status, task_id, request_snapshot, error_message)
			VALUES ($1::uuid,$2::bigint,$3,$4,$5::jsonb,$6)`,
			requestKey, shotID, testCase.status, taskID, snapshot,
			errorMessage,
		); err != nil {
			t.Fatal(err)
		}
		seeded = append(seeded, seededSubmission{requestKey: requestKey, shotID: shotID, status: testCase.status, taskID: taskID, error: errorMessage, mode: testCase.mode})
	}

	store := NewStore(database, nil, config.VideoConfig{Mode: config.VideoGenerationModePaid})
	unknownReason := "服务重启时付费视频提交仍在处理中，上游请求结果不确定；请核对供应商任务后人工对账，禁止重复提交"
	demoReason := "服务重启导致本地演练视频生成中断，请重新生成"
	recovered, err := store.RecoverInterruptedSubmissions(ctx, unknownReason, demoReason)
	if err != nil {
		t.Fatalf("recover interrupted paid submissions: %v", err)
	}
	if recovered != 3 {
		t.Fatalf("recovered submissions = %d, want 3", recovered)
	}

	for _, item := range seeded {
		var status SubmissionStatus
		var taskID, errorMessage, model string
		if err := database.QueryRowContext(ctx, `
			SELECT status, task_id, error_message, request_snapshot->>'model'
			FROM video_generation_submissions WHERE request_key=$1::uuid`, item.requestKey,
		).Scan(&status, &taskID, &errorMessage, &model); err != nil {
			t.Fatal(err)
		}
		if model != "paid-model" {
			t.Fatalf("recovery changed provider request snapshot for %s: model=%q", item.requestKey, model)
		}
		if item.status == SubmissionSubmitting && item.mode != config.VideoGenerationModeDemo {
			if status != SubmissionUnknownOutcome || taskID != item.taskID || errorMessage != unknownReason {
				t.Fatalf("interrupted submission not recovered safely: status=%q task=%q error=%q", status, taskID, errorMessage)
			}
			continue
		}
		if item.status == SubmissionSubmitting {
			if status != SubmissionCancelled || taskID != item.taskID || errorMessage != demoReason {
				t.Fatalf("interrupted demo submission not released safely: status=%q task=%q error=%q", status, taskID, errorMessage)
			}
			continue
		}
		if status != item.status || taskID != item.taskID || errorMessage != item.error {
			t.Fatalf("unrelated submission changed: before=%+v after status=%q task=%q error=%q", item, status, taskID, errorMessage)
		}
	}

	interrupted := seeded[1]
	_, _, err = store.submissions.Prepare(ctx, PrepareSubmissionInput{
		RequestKey: "99999999-9999-4999-8999-999999999999",
		ShotID:     interrupted.shotID,
	})
	var active *ActiveSubmissionError
	if !errors.As(err, &active) || active.Existing.Status != SubmissionUnknownOutcome {
		t.Fatalf("recovered unknown outcome must retain the active lock, got %v", err)
	}

	generation, err := store.ReconcileSubmission(ctx, interrupted.requestKey, "manual-provider-task")
	if err != nil {
		t.Fatalf("manual reconcile after empty-task recovery: %v", err)
	}
	generationIDs = append(generationIDs, generation.Generation.ID)
	if generation.Generation.TaskID != "manual-provider-task" || generation.Generation.Status != "queued" {
		t.Fatalf("manual reconcile returned unexpected generation: %+v", generation)
	}
	resolved, err := store.submissions.GetByRequestKey(ctx, interrupted.requestKey)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Status != SubmissionReconciled || resolved.UpstreamTaskID != "manual-provider-task" || resolved.GenerationID != generation.Generation.ID {
		t.Fatalf("manual reconcile did not resolve recovered submission: %+v", resolved)
	}

	demoInterrupted := seeded[3]
	var providerCalls atomic.Int64
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "demo retry must remain local", http.StatusTeapot)
	}))
	defer provider.Close()
	uploader := &recordingVideoUploader{url: "/api/uploads/video/generated/recovered-demo.mp4"}
	demoStore := NewStore(database, nil, config.VideoConfig{
		APIBase: provider.URL,
		APIKey:  "must-not-be-used",
		Mode:    config.VideoGenerationModeDemo,
	}, uploader)
	demo, err := demoStore.Generate(ctx, GenerateInput{
		Prompt:     "retry local demo",
		RequestKey: "88888888-8888-4888-8888-888888888888",
		Seconds:    5,
		ShotID:     demoInterrupted.shotID,
	})
	if err != nil {
		t.Fatalf("retry demo after restart recovery: %v", err)
	}
	generationIDs = append(generationIDs, demo.ID)
	if providerCalls.Load() != 0 || demo.Provider != "demo" || demo.Status != "completed" {
		t.Fatalf("recovered demo retry escaped the local path: calls=%d generation=%+v", providerCalls.Load(), demo)
	}
}
