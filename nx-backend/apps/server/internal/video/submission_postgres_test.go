package video

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func TestSubmissionPostgresConcurrencyAndReconciliation(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL submission integration test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	var projectID string
	if err := database.QueryRowContext(ctx,
		`INSERT INTO video_projects (name) VALUES ('submission integration') RETURNING id::text`,
	).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	var shotID string
	if err := database.QueryRowContext(ctx,
		`INSERT INTO video_shots (project_id, name) VALUES ($1,'submission integration shot') RETURNING id::text`,
		projectID,
	).Scan(&shotID); err != nil {
		t.Fatal(err)
	}
	requestKey, err := newVideoRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := newVideoRequestKey()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_generation_submissions WHERE request_key IN ($1,$2)`, requestKey, secondKey)
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_generations WHERE task_id='task-postgres-reconcile'`)
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_shots WHERE id=$1`, shotID)
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_projects WHERE id=$1`, projectID)
	})

	snapshot, err := json.Marshal(generationSubmissionSnapshot{
		Model:             "video-ds-2.0",
		Prompt:            "postgres reconciliation",
		Seconds:           15,
		AspectRatio:       "16:9",
		CapabilityVersion: "integration-capability-v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	submissions := NewSubmissionStore(database)
	prepared, created, err := submissions.Prepare(ctx, PrepareSubmissionInput{
		RequestKey:        requestKey,
		ProjectID:         projectID,
		ShotID:            shotID,
		RequestHash:       hashGenerationValue(snapshot),
		PromptHash:        hashGenerationValue([]byte("postgres reconciliation")),
		CapabilityVersion: "integration-capability-v1",
		RequestSnapshot:   snapshot,
	})
	if err != nil || !created || prepared.Status != SubmissionPrepared {
		t.Fatalf("prepare: created=%v submission=%+v err=%v", created, prepared, err)
	}

	start := make(chan struct{})
	claims := make(chan bool, 2)
	errs := make(chan error, 2)
	var claimWG sync.WaitGroup
	for range 2 {
		claimWG.Add(1)
		go func() {
			defer claimWG.Done()
			<-start
			_, claimed, claimErr := submissions.ClaimSubmitting(ctx, requestKey)
			claims <- claimed
			errs <- claimErr
		}()
	}
	close(start)
	claimWG.Wait()
	close(claims)
	close(errs)
	winners := 0
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatal(claimErr)
		}
	}
	for claimed := range claims {
		if claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("PostgreSQL claim winners = %d, want 1", winners)
	}
	if _, err := submissions.Transition(ctx, requestKey, SubmissionSubmitting, SubmissionUnknownOutcome, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database, nil, config.VideoConfig{
		APIBase:         "https://gateway.example",
		APIKey:          "unused",
		GatewayContract: LegacyFlatContract(),
	})
	results := make(chan ReconciliationResult, 2)
	reconcileErrs := make(chan error, 2)
	start = make(chan struct{})
	var reconcileWG sync.WaitGroup
	for range 2 {
		reconcileWG.Add(1)
		go func() {
			defer reconcileWG.Done()
			<-start
			result, reconcileErr := store.ReconcileSubmission(ctx, requestKey, "task-postgres-reconcile")
			results <- result
			reconcileErrs <- reconcileErr
		}()
	}
	close(start)
	reconcileWG.Wait()
	close(results)
	close(reconcileErrs)
	for reconcileErr := range reconcileErrs {
		if reconcileErr != nil {
			t.Fatal(reconcileErr)
		}
	}
	var generationID string
	for result := range results {
		if result.Submission.Status != SubmissionReconciled || result.Generation.ID == "" {
			t.Fatalf("unexpected reconciliation result: %+v", result)
		}
		if generationID == "" {
			generationID = result.Generation.ID
		} else if result.Generation.ID != generationID {
			t.Fatalf("concurrent reconciliation generation ids = %q/%q", generationID, result.Generation.ID)
		}
	}
	var generationCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM video_generations WHERE task_id='task-postgres-reconcile'`,
	).Scan(&generationCount); err != nil {
		t.Fatal(err)
	}
	if generationCount != 1 {
		t.Fatalf("PostgreSQL generation count = %d, want 1", generationCount)
	}

	_, err = store.ReconcileSubmission(ctx, requestKey, "task-postgres-conflict")
	var conflict *ReconciliationTaskConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("conflicting task error = %T, want *ReconciliationTaskConflictError: %v", err, err)
	}
	if _, created, err := submissions.Prepare(ctx, PrepareSubmissionInput{
		RequestKey:        secondKey,
		ProjectID:         projectID,
		ShotID:            shotID,
		RequestHash:       "second-request-hash",
		PromptHash:        "second-prompt-hash",
		CapabilityVersion: "integration-capability-v1",
		RequestSnapshot:   snapshot,
	}); err != nil || !created {
		t.Fatalf("new version after reconciled: created=%v err=%v", created, err)
	}
}
