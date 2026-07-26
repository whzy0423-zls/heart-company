package server

import (
	"bytes"
	"context"
	"encoding/json"
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
	"nine-xing/nx-backend/apps/server/internal/video"
	"nine-xing/nx-backend/apps/server/internal/videoproject"
)

func TestVideoSubmissionRoutesRetryStartupRecoveryBeforeProviderUse(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run video submission route recovery integration test")
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

	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		http.Error(w, "provider must not be called while recovery is closed", http.StatusTeapot)
	}))
	defer provider.Close()

	var projectID, shotID string
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_projects (name) VALUES ($1) RETURNING id::text`,
		fmt.Sprintf("route-recovery-%d", time.Now().UnixNano()),
	).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var generationID string
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_projects WHERE id=$1::bigint`, projectID)
		if generationID != "" {
			_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_generations WHERE id=$1::bigint`, generationID)
		}
	})
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_shots (project_id, order_num, name, action_description)
		VALUES ($1::bigint,1,'recovery shot','recovery action') RETURNING id::text`, projectID,
	).Scan(&shotID); err != nil {
		t.Fatal(err)
	}
	requestKey := "55555555-5555-4555-8555-555555555555"
	if _, err := database.ExecContext(ctx, `
		INSERT INTO video_generation_submissions
		    (request_key, shot_id, status, request_snapshot)
		VALUES ($1::uuid,$2::bigint,'submitting',
		        '{"aspectRatio":"16:9","generationMode":"paid","images":[],"model":"paid-model","prompt":"recover","seconds":5,"shotRevision":0}'::jsonb)`,
		requestKey, shotID,
	); err != nil {
		t.Fatal(err)
	}

	videoStore := video.NewStore(database, nil, config.VideoConfig{
		APIBase: provider.URL,
		APIKey:  "must-not-be-used-before-recovery",
		Mode:    config.VideoGenerationModePaid,
	})
	s := &Server{db: database, videos: videoStore}
	var recoveryCalls atomic.Int32
	s.videoSubmissionRecovery = func(ctx context.Context) (int64, error) {
		if recoveryCalls.Add(1) == 1 {
			return 0, errors.New("temporary recovery failure")
		}
		return videoStore.RecoverInterruptedSubmissions(ctx, "上游请求结果不确定，禁止重复提交", "本地演练生成中断")
	}

	generateBody, _ := json.Marshal(map[string]string{"requestKey": "44444444-4444-4444-8444-444444444444"})
	generateRecorder := httptest.NewRecorder()
	generateRequest := httptest.NewRequest(http.MethodPost, "/api/video/shots-generate-safe/"+shotID, bytes.NewReader(generateBody))
	s.videoWorkflowGenerate(generateRecorder, generateRequest)
	if generateRecorder.Code != http.StatusServiceUnavailable || providerCalls.Load() != 0 {
		t.Fatalf("closed recovery gate reached generation: status=%d provider=%d body=%s", generateRecorder.Code, providerCalls.Load(), generateRecorder.Body.String())
	}

	workflowRecorder := httptest.NewRecorder()
	workflowRequest := httptest.NewRequest(http.MethodGet, "/api/video/projects-workflow/"+projectID, nil)
	s.videoWorkflowGet(workflowRecorder, workflowRequest)
	if workflowRecorder.Code != http.StatusOK {
		t.Fatalf("workflow after recovery status=%d body=%s", workflowRecorder.Code, workflowRecorder.Body.String())
	}
	var envelope workflowAPIResponse
	if err := json.Unmarshal(workflowRecorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	var workflow videoproject.WorkflowStatus
	if err := json.Unmarshal(envelope.Data, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.Shots) != 1 || workflow.Shots[0].Readiness != videoproject.ReadinessRecovery {
		t.Fatalf("workflow did not expose recovered submission: %+v", workflow.Shots)
	}

	reconcileBody, _ := json.Marshal(map[string]string{"taskId": "manual-task"})
	reconcileRecorder := httptest.NewRecorder()
	reconcileRequest := httptest.NewRequest(http.MethodPost, "/api/video/generation-submissions/reconcile/"+requestKey, bytes.NewReader(reconcileBody))
	s.videoWorkflowReconcile(reconcileRecorder, reconcileRequest)
	if reconcileRecorder.Code != http.StatusOK || providerCalls.Load() != 0 {
		t.Fatalf("reconcile after recovery status=%d provider=%d body=%s", reconcileRecorder.Code, providerCalls.Load(), reconcileRecorder.Body.String())
	}
	if recoveryCalls.Load() != 2 {
		t.Fatalf("recovery calls = %d, want failed startup retry plus success", recoveryCalls.Load())
	}
	if err := database.QueryRowContext(ctx, `
		SELECT status, generation_id::text FROM video_generation_submissions WHERE request_key=$1::uuid`, requestKey,
	).Scan(new(string), &generationID); err != nil {
		t.Fatal(err)
	}
}
