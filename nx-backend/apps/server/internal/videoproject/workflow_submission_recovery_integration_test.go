package videoproject

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	dbstore "nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/testutil"
	"nine-xing/nx-backend/apps/server/internal/video"
)

func TestRecoveredPaidSubmissionAppearsInWorkflowRecovery(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run workflow submission recovery integration test")
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

	var projectID, paidShotID, demoShotID string
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_projects (name) VALUES ($1) RETURNING id::text`,
		fmt.Sprintf("workflow-submission-recovery-%d", time.Now().UnixNano()),
	).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM video_projects WHERE id=$1::bigint`, projectID)
	})
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_shots (project_id, order_num, name, action_description)
		VALUES ($1::bigint,1,'recovery shot','recovery action') RETURNING id::text`, projectID,
	).Scan(&paidShotID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `
		INSERT INTO video_shots (project_id, order_num, name, action_description)
		VALUES ($1::bigint,2,'demo recovery shot','demo recovery action') RETURNING id::text`, projectID,
	).Scan(&demoShotID); err != nil {
		t.Fatal(err)
	}
	requestKey := "77777777-7777-4777-8777-777777777777"
	if _, err := database.ExecContext(ctx, `
		INSERT INTO video_generation_submissions
		    (request_key, shot_id, status, request_snapshot)
		VALUES ($1::uuid,$2::bigint,'submitting','{}'::jsonb)`, requestKey, paidShotID,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO video_generation_submissions
		    (request_key, shot_id, status, request_snapshot)
		VALUES ('66666666-6666-4666-8666-666666666666',$1::bigint,'submitting',
		        '{"generationMode":"demo"}'::jsonb)`, demoShotID,
	); err != nil {
		t.Fatal(err)
	}

	videoStore := video.NewStore(database, nil, config.VideoConfig{Mode: config.VideoGenerationModePaid})
	if _, err := videoStore.RecoverInterruptedSubmissions(ctx, "上游请求结果不确定，禁止重复提交", "本地演练生成中断，请重新生成"); err != nil {
		t.Fatal(err)
	}
	workflow, err := NewStore(database).GetWorkflowStatus(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	if len(workflow.Shots) != 2 {
		t.Fatalf("workflow shots = %d, want 2", len(workflow.Shots))
	}
	shot := workflow.Shots[0]
	if shot.Readiness != ReadinessRecovery || shot.CanGenerate {
		t.Fatalf("recovered submission must require manual recovery: %+v", shot)
	}
	if shot.ActiveSubmission == nil || shot.ActiveSubmission.Status != string(video.SubmissionUnknownOutcome) || shot.ActiveSubmission.RequestKey != requestKey {
		t.Fatalf("workflow lost recovered submission identity: %+v", shot.ActiveSubmission)
	}
	demoShot := workflow.Shots[1]
	if demoShot.Readiness != ReadinessReady || !demoShot.CanGenerate || demoShot.ActiveSubmission != nil {
		t.Fatalf("recovered demo submission must be locally retryable: %+v", demoShot)
	}
}
