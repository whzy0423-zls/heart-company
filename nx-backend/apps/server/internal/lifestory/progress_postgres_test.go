package lifestory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
)

func TestPostgresLifeStoryOlderClientProgressCannotOverwriteNewer(t *testing.T) {
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

	phone := fmt.Sprintf("177%08d", time.Now().UnixNano()%100_000_000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, userID)

	store := NewStore(database)
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{
		Title:     "阅读进度",
		Materials: []Material{{SourceType: MaterialText, Text: "一段真实经历"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	chapters, err := json.Marshal([]Chapter{{Order: 1, Title: "第一章", Body: "甲乙丙丁戊己庚辛"}})
	if err != nil {
		t.Fatal(err)
	}
	var versionID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_versions(story_id,app_user_id,version_no,status,chapters,character_count,word_count) VALUES($1,$2,1,'published',$3::jsonb,8,8) RETURNING id`, story.ID, userID, string(chapters)).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET current_version_id=$1,status='completed' WHERE id=$2`, versionID, story.ID); err != nil {
		t.Fatal(err)
	}

	newerAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	newer, err := store.SaveProgress(ctx, userID, story.ID, ReadingProgress{
		VersionID: versionID, ChapterIndex: 0, CharacterOffset: 7,
		ClientUpdatedAt: newerAt.Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if newer.CharacterOffset != 7 {
		t.Fatalf("newer offset=%d, want 7", newer.CharacterOffset)
	}

	stale, err := store.SaveProgress(ctx, userID, story.ID, ReadingProgress{
		VersionID: versionID, ChapterIndex: 0, CharacterOffset: 2,
		ClientUpdatedAt: newerAt.Add(-time.Minute).Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.CharacterOffset != 7 {
		t.Fatalf("stale request returned offset=%d, want persisted newer offset 7", stale.CharacterOffset)
	}

	persisted, err := store.GetProgress(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.CharacterOffset != 7 {
		t.Fatalf("stale request overwrote progress: offset=%d, want 7", persisted.CharacterOffset)
	}
}
