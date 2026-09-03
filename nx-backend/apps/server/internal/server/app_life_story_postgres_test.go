package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appnotification"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/lifestory"
)

func TestPostgresLifeStoryPrepareDefaultsEventRedactionToBlurred(t *testing.T) {
	dsn := os.Getenv("LIFESTORY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LIFESTORY_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	database, err := db.Open(ctx, dsn, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	phone := fmt.Sprintf("150%08d", time.Now().UnixNano()%100_000_000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
	})

	storyStore := lifestory.NewStore(database)
	story, err := storyStore.CreateStory(ctx, userID, lifestory.CreateStoryInput{
		Materials: []lifestory.Material{{SourceType: lifestory.MaterialText, Text: "我第一次离开家乡去读书。"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app/life-stories/%d/prepare", story.ID), nil)
	response := httptest.NewRecorder()
	server := &Server{lifeStories: storyStore}

	server.appLifeStoryPrepare(response, request, userID, story.ID)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Facts    lifestory.FactCard `json:"facts"`
			FactCard lifestory.FactCard `json:"factCard"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	for name, facts := range map[string]lifestory.FactCard{
		"facts":    payload.Data.Facts,
		"factCard": payload.Data.FactCard,
	} {
		if len(facts.Events) != 1 || facts.Events[0].RedactionMode != "blurred" {
			t.Fatalf("%s events=%+v want one blurred event", name, facts.Events)
		}
	}
	persisted, err := storyStore.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.FactCard.Events) != 1 || persisted.FactCard.Events[0].RedactionMode != "blurred" {
		t.Fatalf("persisted events=%+v want one blurred event", persisted.FactCard.Events)
	}
}

func TestPostgresLifeStoryOutboxDispatchRequiresLiveStory(t *testing.T) {
	dsn := os.Getenv("LIFESTORY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LIFESTORY_POSTGRES_DSN is not set")
	}
	ctx := context.Background()
	database, err := db.Open(ctx, dsn, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	for _, deletedBeforeDispatch := range []bool{false, true} {
		name := "live"
		if deletedBeforeDispatch {
			name = "deleted"
		}
		t.Run(name, func(t *testing.T) {
			phone := fmt.Sprintf("152%08d", time.Now().UnixNano()%100_000_000)
			var userID, storyID, versionID, jobID int64
			if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
			})
			if err := database.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,status,stage)
				VALUES($1,'completed','reading') RETURNING id`, userID).Scan(&storyID); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_versions
				(story_id,app_user_id,version_no,status,perspective,tone)
				VALUES($1,$2,1,'published','first_person','warm') RETURNING id`, storyID, userID).Scan(&versionID); err != nil {
				t.Fatal(err)
			}
			if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET current_version_id=$1 WHERE id=$2`, versionID, storyID); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs
				(story_id,app_user_id,request_key,status,version_id)
				VALUES($1,$2,$3,'succeeded',$4) RETURNING id`, storyID, userID,
				fmt.Sprintf("dispatch-%d", time.Now().UnixNano()), versionID).Scan(&jobID); err != nil {
				t.Fatal(err)
			}
			sourceKey := fmt.Sprintf("life-story:%d:%d", storyID, versionID)
			if _, err := database.ExecContext(ctx, `INSERT INTO app_life_story_outbox
				(app_user_id,story_id,job_id,version_id,event_type,source_key)
				VALUES($1,$2,$3,$4,'completed',$5)`, userID, storyID, jobID, versionID, sourceKey); err != nil {
				t.Fatal(err)
			}

			storyStore := lifestory.NewStore(database)
			s := &Server{db: database, lifeStories: storyStore, appNotifications: appnotification.NewStore(database)}
			event, err := storyStore.ClaimOutbox(ctx, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			if deletedBeforeDispatch {
				if err := storyStore.DeleteStory(ctx, userID, storyID); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.dispatchLifeStoryOutboxEvent(ctx, event); err != nil {
				t.Fatal(err)
			}

			var notifications int
			if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_notifications
				WHERE app_user_id=$1 AND source_key=$2`, userID, sourceKey).Scan(&notifications); err != nil {
				t.Fatal(err)
			}
			wantNotifications := 1
			if deletedBeforeDispatch {
				wantNotifications = 0
			}
			if notifications != wantNotifications {
				t.Fatalf("notifications=%d want=%d", notifications, wantNotifications)
			}
			if !deletedBeforeDispatch {
				var published sql.NullTime
				if err := database.QueryRowContext(ctx, `SELECT published_at FROM app_life_story_outbox WHERE id=$1`, event.ID).Scan(&published); err != nil {
					t.Fatal(err)
				}
				if !published.Valid {
					t.Fatal("live outbox event was not marked published")
				}
			}
		})
	}
}

func TestPostgresLifeStoryOutboxDispatchSerializesWithDelete(t *testing.T) {
	dsn := os.Getenv("LIFESTORY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LIFESTORY_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	phone := fmt.Sprintf("151%08d", time.Now().UnixNano()%100_000_000)
	var userID, storyID, versionID, jobID, ledgerID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
	})
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,status,stage)
		VALUES($1,'completed','reading') RETURNING id`, userID).Scan(&storyID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_versions
		(story_id,app_user_id,version_no,status,perspective,tone)
		VALUES($1,$2,1,'published','first_person','warm') RETURNING id`, storyID, userID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET current_version_id=$1 WHERE id=$2`, versionID, storyID); err != nil {
		t.Fatal(err)
	}
	requestKey := fmt.Sprintf("dispatch-delete-%d", time.Now().UnixNano())
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs
		(story_id,app_user_id,request_key,status,version_id)
		VALUES($1,$2,$3,'succeeded',$4) RETURNING id`, storyID, userID, requestKey, versionID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	var periodID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_story_quota_periods
		(app_user_id,period_key,quota_limit,reserved) VALUES($1,$2,1,1) RETURNING id`,
		userID, requestKey).Scan(&periodID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_story_quota_ledger
		(app_user_id,period_id,job_id,entry_type,amount,idempotency_key)
		VALUES($1,$2,$3,'reserve',1,$4) RETURNING id`, userID, periodID, jobID,
		requestKey+":reserve").Scan(&ledgerID); err != nil {
		t.Fatal(err)
	}
	sourceKey := fmt.Sprintf("life-story:%d:%d", storyID, versionID)
	if _, err := database.ExecContext(ctx, `INSERT INTO app_life_story_outbox
		(app_user_id,story_id,job_id,version_id,event_type,source_key)
		VALUES($1,$2,$3,$4,'completed',$5)`, userID, storyID, jobID, versionID, sourceKey); err != nil {
		t.Fatal(err)
	}

	storyStore := lifestory.NewStore(database)
	server := &Server{db: database, lifeStories: storyStore, appNotifications: appnotification.NewStore(database)}
	event, err := storyStore.ClaimOutbox(ctx, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	blocker, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback()
	if err := blocker.QueryRowContext(ctx, `SELECT id FROM app_story_quota_ledger WHERE id=$1 FOR UPDATE`, ledgerID).Scan(&ledgerID); err != nil {
		t.Fatal(err)
	}

	deleteDone := make(chan error, 1)
	go func() { deleteDone <- storyStore.DeleteStory(ctx, userID, storyID) }()
	if !waitForServerPostgresRowLock(ctx, database, `SELECT id FROM app_users WHERE id=$1 FOR UPDATE`, userID, 2*time.Second) {
		t.Fatal("DeleteStory did not acquire the user lock")
	}
	publishDone := make(chan error, 1)
	go func() { publishDone <- server.dispatchLifeStoryOutboxEvent(ctx, event) }()
	if err := blocker.Rollback(); err != nil && err != sql.ErrTxDone {
		t.Fatal(err)
	}

	if err := <-deleteDone; err != nil {
		t.Fatalf("DeleteStory failed: %v", err)
	}
	if err := <-publishDone; err != nil {
		t.Fatalf("outbox dispatch raced with DeleteStory: %v", err)
	}
	var notifications int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_notifications
		WHERE app_user_id=$1 AND source_key=$2`, userID, sourceKey).Scan(&notifications); err != nil {
		t.Fatal(err)
	}
	if notifications != 0 {
		t.Fatalf("deleted story left %d completion notifications", notifications)
	}
}

func waitForServerPostgresRowLock(ctx context.Context, database *sql.DB, query string, id int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tx, err := database.BeginTx(ctx, nil)
		if err != nil {
			return false
		}
		_, setErr := tx.ExecContext(ctx, `SET LOCAL lock_timeout='20ms'`)
		var lockedID int64
		lockErr := error(nil)
		if setErr == nil {
			lockErr = tx.QueryRowContext(ctx, query, id).Scan(&lockedID)
		}
		_ = tx.Rollback()
		if lockErr != nil && strings.Contains(strings.ToLower(lockErr.Error()), "lock timeout") {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(10 * time.Millisecond):
		}
	}
	return false
}
