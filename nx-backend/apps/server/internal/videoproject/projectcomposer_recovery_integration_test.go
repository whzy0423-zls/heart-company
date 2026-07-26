package videoproject

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	dbstore "nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

func TestRecoverInterruptedComposeJobsReleasesProjectForRetry(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run compose recovery integration test")
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

	suffix := time.Now().UnixNano()
	projectIDs := make([]int64, 0, 3)
	generationIDs := make([]int64, 0, 1)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		for _, projectID := range projectIDs {
			_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_projects WHERE id=$1`, projectID)
		}
		for _, generationID := range generationIDs {
			_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_generations WHERE id=$1`, generationID)
		}
	})

	seedProject := func(status string) int64 {
		t.Helper()
		var projectID int64
		if err := database.QueryRowContext(ctx, `
			INSERT INTO video_projects (name, compose_status)
			VALUES ($1,$2) RETURNING id`, fmt.Sprintf("compose-recovery-%d-%d", suffix, len(projectIDs)), status,
		).Scan(&projectID); err != nil {
			t.Fatal(err)
		}
		projectIDs = append(projectIDs, projectID)
		return projectID
	}
	seedJob := func(projectID int64, status string) int64 {
		t.Helper()
		var jobID int64
		if err := database.QueryRowContext(ctx, `
			INSERT INTO video_compose_jobs (project_id, status, compose_input_snapshot)
			VALUES ($1,$2,'{}'::jsonb) RETURNING id`, projectID, status,
		).Scan(&jobID); err != nil {
			t.Fatal(err)
		}
		return jobID
	}

	queuedProjectID := seedProject("composing")
	processingProjectID := seedProject("composing")
	completedProjectID := seedProject("completed")
	queuedJobID := seedJob(queuedProjectID, "queued")
	processingJobID := seedJob(processingProjectID, "processing")
	completedJobID := seedJob(completedProjectID, "completed")

	var generationID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_generations (status, video_url, shot_revision)
		VALUES ('completed','/api/uploads/missing-recovery-test.mp4',1)
		RETURNING id`,
	).Scan(&generationID); err != nil {
		t.Fatal(err)
	}
	generationIDs = append(generationIDs, generationID)
	var shotID int64
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_shots (
			project_id, order_num, name, generation_revision,
			selected_generation_id, status
		) VALUES ($1,1,'recovery shot',1,$2,'completed')
		RETURNING id`, queuedProjectID, generationID,
	).Scan(&shotID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		UPDATE video_generations SET project_id=$1, shot_id=$2 WHERE id=$3`,
		queuedProjectID, shotID, generationID,
	); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database)
	recovered, err := store.RecoverInterruptedComposeJobs(ctx, "服务重启导致合成中断，请重试")
	if err != nil {
		t.Fatalf("recover interrupted compose jobs: %v", err)
	}
	if recovered != 2 {
		t.Fatalf("recovered jobs = %d, want 2", recovered)
	}

	for _, jobID := range []int64{queuedJobID, processingJobID} {
		var status, errorMessage string
		if err := database.QueryRowContext(ctx,
			`SELECT status, error_message FROM video_compose_jobs WHERE id=$1`, jobID,
		).Scan(&status, &errorMessage); err != nil {
			t.Fatal(err)
		}
		if status != "failed" || !strings.Contains(errorMessage, "服务重启") || !strings.Contains(errorMessage, "重试") {
			t.Fatalf("interrupted job %d not recoverable: status=%q error=%q", jobID, status, errorMessage)
		}
	}
	for _, projectID := range []int64{queuedProjectID, processingProjectID} {
		var status string
		if err := database.QueryRowContext(ctx,
			`SELECT compose_status FROM video_projects WHERE id=$1`, projectID,
		).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "failed" {
			t.Fatalf("interrupted project %d compose_status = %q, want failed", projectID, status)
		}
	}
	var completedJobStatus, completedProjectStatus string
	if err := database.QueryRowContext(ctx,
		`SELECT status FROM video_compose_jobs WHERE id=$1`, completedJobID,
	).Scan(&completedJobStatus); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		`SELECT compose_status FROM video_projects WHERE id=$1`, completedProjectID,
	).Scan(&completedProjectStatus); err != nil {
		t.Fatal(err)
	}
	if completedJobStatus != "completed" || completedProjectStatus != "completed" {
		t.Fatalf("completed compose changed: job=%q project=%q", completedJobStatus, completedProjectStatus)
	}

	composer := NewProjectComposer(store, nil, uploadasset.NewStore(database), t.TempDir())
	job, err := composer.StartCompose(ctx, fmt.Sprint(queuedProjectID), ComposeProjectInput{Transition: "none"})
	if err != nil {
		t.Fatalf("start compose after recovery: %v", err)
	}
	if job.ID == "" {
		t.Fatal("start compose after recovery returned no job id")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := database.QueryRowContext(ctx,
			`SELECT status FROM video_compose_jobs WHERE id=$1::bigint`, job.ID,
		).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "completed" || status == "failed" {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("retry compose job did not reach a terminal state before cleanup")
}
