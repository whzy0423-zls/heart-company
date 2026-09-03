package lifestory

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
)

// Run with LIFESTORY_POSTGRES_DSN pointing at an isolated test database. The
// test is skipped in normal unit-test runs where PostgreSQL is unavailable.
func TestPostgresLifeStoryPersistenceLifecycle(t *testing.T) {
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
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, "199"+time.Now().Format("150405.000000")).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, userID)
	store := NewStore(database)
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{Title: "测试", Materials: []Material{{SourceType: MaterialText, Text: "请联系 first@example.com，我经历了一次改变。"}}})
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := store.SavePrepared(ctx, userID, story.ID,
		FactCard{Confirmed: true, ConfirmedAt: "2026-01-01T00:00:00Z"},
		Outline{Confirmed: true, ConfirmedAt: "2026-01-01T00:00:00Z"}, "questions-1", story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.FactCard.Confirmed || prepared.FactCard.ConfirmedAt != "" || prepared.Outline.Confirmed || prepared.Outline.ConfirmedAt != "" {
		t.Fatalf("prepared data retained stale confirmation: facts=%+v outline=%+v", prepared.FactCard, prepared.Outline)
	}
	story = prepared
	facts := FactCard{Confirmed: true, Characters: []FactCharacter{{Alias: "我"}}, Events: []FactEvent{{Description: "改变", Confirmed: true}}}
	if _, err := store.ConfirmFacts(ctx, userID, story.ID, facts, story.Revision); err != nil {
		t.Fatal(err)
	}
	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	outline := Outline{Confirmed: true, Perspective: PerspectiveFirst, Tone: ToneWarm, StoryStyle: StoryStyleFairyTale, StoryStyleSelected: true, Chapters: []OutlineChapter{{Order: 1, Title: "一"}, {Order: 2, Title: "二"}, {Order: 3, Title: "三"}, {Order: 4, Title: "四"}}}
	if _, err := store.ConfirmOutline(ctx, userID, story.ID, outline, story.Revision); err != nil {
		t.Fatal(err)
	}
	job, reused, err := store.CreateJob(ctx, userID, story.ID, "request-1")
	if err != nil || reused {
		t.Fatalf("create job err=%v reused=%v", err, reused)
	}
	var snapshot StorySnapshot
	if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Outline.StoryStyle != StoryStyleFairyTale {
		t.Fatalf("job snapshot storyStyle=%q, want %q", snapshot.Outline.StoryStyle, StoryStyleFairyTale)
	}
	if _, reused, err := store.CreateJob(ctx, userID, story.ID, "request-1"); err != nil || !reused {
		t.Fatalf("duplicate job err=%v reused=%v", err, reused)
	}
	var originalCiphertext []byte
	if err := database.QueryRowContext(ctx, `SELECT ciphertext FROM app_life_story_token_maps WHERE job_id=$1`, job.ID).Scan(&originalCiphertext); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_story_materials
		SET text='请联系 second@example.com，我经历了一次改变。',
		transcript='请联系 second@example.com，我经历了一次改变。'
		WHERE story_id=$1 AND app_user_id=$2`, story.ID, userID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.CreateJob(ctx, userID, story.ID, "request-1"); !errors.Is(err, ErrPayloadConflict) {
		t.Fatalf("different original PII reused request key: %v", err)
	}
	var retainedCiphertext []byte
	if err := database.QueryRowContext(ctx, `SELECT ciphertext FROM app_life_story_token_maps WHERE job_id=$1`, job.ID).Scan(&retainedCiphertext); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(retainedCiphertext, originalCiphertext) {
		t.Fatal("idempotency conflict replaced the original encrypted token map")
	}
	tokenMap, err := decryptTokenMap(retainedCiphertext, store.tokenKey)
	if err != nil {
		t.Fatal(err)
	}
	foundFirst, foundSecond := false, false
	for _, replacement := range tokenMap {
		foundFirst = foundFirst || replacement.Value == "first@example.com"
		foundSecond = foundSecond || replacement.Value == "second@example.com"
	}
	if !foundFirst || foundSecond {
		t.Fatalf("retained token map first=%v second=%v map=%#v", foundFirst, foundSecond, tokenMap)
	}
	quota, err := NewQuotaStore(database).Reserve(ctx, userID, job.ID, "")
	if err != nil || quota.Remaining != 0 {
		t.Fatalf("reserve err=%v quota=%+v", err, quota)
	}
	claimed, err := store.ClaimNextJob(ctx, 5)
	if err != nil || claimed.ID != job.ID {
		t.Fatalf("claim err=%v job=%+v", err, claimed)
	}
	if _, err := store.GetProgress(ctx, userID, story.ID); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	worker := NewWorker(WorkerConfig{Store: store, Generator: NewGenerator(GeneratorConfig{Completer: &fakeCompleter{raw: generatedJSON()}}), Quota: NewQuotaStore(database), GenerationTimeout: time.Minute})
	if err := worker.processClaimed(ctx, claimed); err != nil {
		t.Fatal(err)
	}
	completed, err := store.Get(ctx, userID, story.ID)
	if err != nil || completed.Status != StatusCompleted || completed.CurrentVersion == nil {
		t.Fatalf("completed story err=%v story=%+v", err, completed)
	}
	if completed.CurrentVersion.StoryStyle != StoryStyleFairyTale {
		t.Fatalf("published version storyStyle=%q, want %q", completed.CurrentVersion.StoryStyle, StoryStyleFairyTale)
	}
	var persistedStyle StoryStyle
	if err := database.QueryRowContext(ctx, `SELECT story_style FROM app_life_story_versions WHERE id=$1`, completed.CurrentVersion.ID).Scan(&persistedStyle); err != nil {
		t.Fatal(err)
	}
	if persistedStyle != StoryStyleFairyTale {
		t.Fatalf("persisted story_style=%q, want %q", persistedStyle, StoryStyleFairyTale)
	}
	quota, err = NewQuotaStore(database).Snapshot(ctx, userID, "")
	if err != nil || quota.Consumed != 1 || quota.Remaining != 0 {
		t.Fatalf("unexpected committed quota err=%v quota=%+v", err, quota)
	}
}

func TestPostgresLifeStoryStyleSelectionPresenceLifecycle(t *testing.T) {
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

	var userID int64
	phone := fmt.Sprintf("153%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)

	store := NewStore(database)
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{Materials: []Material{{SourceType: MaterialText, Text: "一次真实的改变"}}})
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, FactCard{
		Characters: []FactCharacter{{Alias: "我"}},
		Events:     []FactEvent{{Description: "一次真实的改变", Confirmed: true}},
	}, story.Revision)
	if err != nil {
		t.Fatal(err)
	}

	legacyOutline := `{"perspective":"first_person","tone":"warm","chapters":[{"order":1,"title":"一"},{"order":2,"title":"二"},{"order":3,"title":"三"},{"order":4,"title":"四"}],"confirmed":false,"version":1}`
	if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET outline=$1::jsonb WHERE id=$2`, legacyOutline, story.ID); err != nil {
		t.Fatal(err)
	}
	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	if story.Outline.StoryStyle != StoryStyleRealistic || story.Outline.StoryStyleSelected {
		t.Fatalf("legacy outline style=%q selected=%v, want realistic false", story.Outline.StoryStyle, story.Outline.StoryStyleSelected)
	}
	apiRaw, err := json.Marshal(story.Outline)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(apiRaw, []byte(`"storyStyle":"realistic"`)) || !bytes.Contains(apiRaw, []byte(`"storyStyleSelected":false`)) {
		t.Fatalf("legacy API outline did not expose effective style and presence: %s", apiRaw)
	}
	for _, explicitStyle := range []StoryStyle{StoryStyleRealistic, StoryStyleMyth} {
		explicitOutline := strings.Replace(legacyOutline, `"tone":"warm"`, fmt.Sprintf(`"tone":"warm","storyStyle":%q`, explicitStyle), 1)
		if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET outline=$1::jsonb WHERE id=$2`, explicitOutline, story.ID); err != nil {
			t.Fatal(err)
		}
		explicitStory, err := store.Get(ctx, userID, story.ID)
		if err != nil {
			t.Fatal(err)
		}
		if explicitStory.Outline.StoryStyle != explicitStyle || !explicitStory.Outline.StoryStyleSelected {
			t.Fatalf("database explicit %q style=%q selected=%v, want selected", explicitStyle, explicitStory.Outline.StoryStyle, explicitStory.Outline.StoryStyleSelected)
		}
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET outline=$1::jsonb WHERE id=$2`, legacyOutline, story.ID); err != nil {
		t.Fatal(err)
	}
	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}

	story, err = store.SaveOutline(ctx, userID, story.ID, story.Outline, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if story.Outline.StoryStyle != StoryStyleRealistic || story.Outline.StoryStyleSelected {
		t.Fatalf("save lost unselected state: %+v", story.Outline)
	}
	story, err = store.ConfirmStoredOutline(ctx, userID, story.ID, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if story.Outline.StoryStyle != StoryStyleRealistic || story.Outline.StoryStyleSelected {
		t.Fatalf("confirm lost unselected state: %+v", story.Outline)
	}
	job, reused, err := store.CreateJob(ctx, userID, story.ID, fmt.Sprintf("presence-%d", time.Now().UnixNano()))
	if err != nil || reused {
		t.Fatalf("create job err=%v reused=%v", err, reused)
	}
	var snapshot StorySnapshot
	if err := json.Unmarshal(job.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Outline.StoryStyle != StoryStyleRealistic || snapshot.Outline.StoryStyleSelected {
		t.Fatalf("job snapshot lost unselected state: %+v", snapshot.Outline)
	}
	if _, err := store.CancelJob(ctx, userID, story.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	selectedOutline := story.Outline
	selectedOutline.StoryStyle = StoryStyleMyth
	selectedOutline.StoryStyleSelected = true
	story, err = store.SaveOutline(ctx, userID, story.ID, selectedOutline, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	unselectedWrite := story.Outline
	unselectedWrite.StoryStyle = StoryStyleRealistic
	unselectedWrite.StoryStyleSelected = false
	story, err = store.SaveOutline(ctx, userID, story.ID, unselectedWrite, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if story.Outline.StoryStyle != StoryStyleMyth || !story.Outline.StoryStyleSelected {
		t.Fatalf("unselected write erased selected style: %+v", story.Outline)
	}
	story, err = store.ConfirmStoredOutline(ctx, userID, story.ID, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	selectedJob, reused, err := store.CreateJob(ctx, userID, story.ID, fmt.Sprintf("presence-selected-%d", time.Now().UnixNano()))
	if err != nil || reused {
		t.Fatalf("create selected job err=%v reused=%v", err, reused)
	}
	if err := json.Unmarshal(selectedJob.InputSnapshot, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Outline.StoryStyle != StoryStyleMyth || !snapshot.Outline.StoryStyleSelected {
		t.Fatalf("job snapshot lost selected state: %+v", snapshot.Outline)
	}
}

func TestPostgresLifeStoryQuestionAnswerStateRecovery(t *testing.T) {
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
	store := NewStore(database)
	userSequence := int64(0)
	prepareStory := func(t *testing.T, facts FactCard, questionSetID string) Story {
		t.Helper()
		userSequence++
		phone := fmt.Sprintf("157%08d", (time.Now().UnixNano()+userSequence)%100_000_000)
		var userID int64
		if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
		})
		story, err := store.CreateStory(ctx, userID, CreateStoryInput{Materials: []Material{{SourceType: MaterialText, Text: "一次真实经历"}}})
		if err != nil {
			t.Fatal(err)
		}
		story, err = store.SavePrepared(ctx, userID, story.ID, facts, Outline{}, questionSetID, story.Revision)
		if err != nil {
			t.Fatal(err)
		}
		return story
	}

	t.Run("blank answer does not poison question", func(t *testing.T) {
		story := prepareStory(t, FactCard{Questions: []Question{
			{ID: "first", Prompt: "第一个问题"},
			{ID: "second", Prompt: "第二个问题"},
		}}, "set-blank")
		story, err := store.AnswerQuestion(ctx, story.AppUserID, story.ID, "set-blank", "first", "第一个回答", false)
		if err != nil {
			t.Fatal(err)
		}
		beforeVersion := story.FactCard.Version

		if _, err := store.AnswerQuestion(ctx, story.AppUserID, story.ID, "set-blank", "second", "   ", false); !errors.Is(err, ErrValidation) {
			t.Fatalf("blank answer err=%v, want ErrValidation", err)
		}
		persisted, err := store.Get(ctx, story.AppUserID, story.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.FactCard.Version != beforeVersion {
			t.Fatalf("blank answer advanced facts version to %d, want %d", persisted.FactCard.Version, beforeVersion)
		}
		second := persisted.FactCard.Questions[1]
		if second.AnsweredAt != "" || second.Answer != "" || second.Skipped {
			t.Fatalf("blank answer mutated second question: %+v", second)
		}

		persisted, err = store.AnswerQuestion(ctx, story.AppUserID, story.ID, "set-blank", "second", "ignored while skipped", true)
		if err != nil {
			t.Fatal(err)
		}
		second = persisted.FactCard.Questions[1]
		if persisted.FactCard.Version != beforeVersion+1 || second.Answer != "" || !second.Skipped || second.AnsweredAt == "" {
			t.Fatalf("skip after blank rejection did not complete question: version=%d question=%+v", persisted.FactCard.Version, second)
		}
	})

	for _, tt := range []struct {
		name   string
		answer string
		skip   bool
	}{
		{name: "non-empty answer", answer: "修正后的回答"},
		{name: "skip", skip: true},
	} {
		t.Run("repairs legacy incomplete state with "+tt.name, func(t *testing.T) {
			story := prepareStory(t, FactCard{Questions: []Question{{
				ID: "legacy", Prompt: "历史问题", AnsweredAt: "2026-08-30T00:00:00Z",
			}}}, "set-legacy-"+strings.ReplaceAll(tt.name, " ", "-"))
			beforeVersion := story.FactCard.Version

			updated, err := store.AnswerQuestion(ctx, story.AppUserID, story.ID, story.FactCard.QuestionSetID, "legacy", tt.answer, tt.skip)
			if err != nil {
				t.Fatal(err)
			}
			question := updated.FactCard.Questions[0]
			if updated.FactCard.Version != beforeVersion+1 || question.Answer != tt.answer || question.Skipped != tt.skip || question.AnsweredAt == "" {
				t.Fatalf("legacy question was not repaired: version=%d question=%+v", updated.FactCard.Version, question)
			}
		})
	}

	t.Run("stale preparation cannot overwrite an answered question", func(t *testing.T) {
		story := prepareStory(t, FactCard{Questions: []Question{{
			ID: "turning_point", Prompt: "这段经历中，哪个瞬间让你决定做出改变？", Sequence: 1,
		}}}, "set-before-answer")
		staleFacts := story.FactCard
		staleOutline := story.Outline
		answered, err := store.AnswerQuestion(ctx, story.AppUserID, story.ID, story.FactCard.QuestionSetID, "turning_point", "我登上列车的那一刻", false)
		if err != nil {
			t.Fatal(err)
		}

		if _, err := store.SavePrepared(ctx, story.AppUserID, story.ID, staleFacts, staleOutline, "set-stale-prepare", story.Revision); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale preparation error=%v want ErrConflict", err)
		}
		persisted, err := store.Get(ctx, story.AppUserID, story.ID)
		if err != nil {
			t.Fatal(err)
		}
		if persisted.FactCard.Version != answered.FactCard.Version || persisted.FactCard.QuestionSetID != "set-before-answer" || persisted.FactCard.Questions[0].Answer != "我登上列车的那一刻" {
			t.Fatalf("stale preparation overwrote answered facts: %+v", persisted.FactCard)
		}
	})
}

func TestPostgresLifeStorySafetyBlockedInputMustChangeBeforeNewJob(t *testing.T) {
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

	var userID int64
	phone := fmt.Sprintf("187%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, userID)

	store := NewStore(database)
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{
		Materials: []Material{{SourceType: MaterialText, Text: "一次真实的改变"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	facts := FactCard{
		Characters: []FactCharacter{{Alias: "我"}},
		Events:     []FactEvent{{Description: "一次真实的改变", Confirmed: true}},
	}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, facts, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	outline := Outline{
		Perspective: PerspectiveFirst,
		Tone:        ToneWarm,
		Chapters: []OutlineChapter{
			{Order: 1, Title: "一"}, {Order: 2, Title: "二"},
			{Order: 3, Title: "三"}, {Order: 4, Title: "四"},
		},
	}
	story, err = store.ConfirmOutline(ctx, userID, story.ID, outline, story.Revision)
	if err != nil {
		t.Fatal(err)
	}

	blockedInput := GenerationInput{
		RequestKey:     fmt.Sprintf("safety-blocked-%d", time.Now().UnixNano()),
		FactsVersion:   story.FactCard.Version,
		OutlineVersion: story.Outline.Version,
	}
	blocked, reused, err := store.CreateGenerationJobWithInput(ctx, userID, story.ID, blockedInput)
	if err != nil || reused {
		t.Fatalf("create blocked input job err=%v reused=%v", err, reused)
	}
	claimed, err := store.ClaimNextJob(ctx, time.Minute)
	if err != nil || claimed.ID != blocked.ID {
		t.Fatalf("claim err=%v job=%+v", err, claimed)
	}
	if _, err := store.BlockJob(ctx, claimed.ID, claimed.ClaimToken, "safety", "adjust input"); err != nil {
		t.Fatal(err)
	}

	retryInput := blockedInput
	retryInput.RequestKey = fmt.Sprintf("safety-retry-%d", time.Now().UnixNano())
	if _, _, err := store.CreateGenerationJobWithInput(ctx, userID, story.ID, retryInput); !errors.Is(err, ErrInvalidState) {
		t.Fatalf("unchanged safety-blocked input err=%v, want ErrInvalidState", err)
	}
	var jobCount int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_life_story_jobs WHERE story_id=$1 AND app_user_id=$2`, story.ID, userID).Scan(&jobCount); err != nil {
		t.Fatal(err)
	}
	if jobCount != 1 {
		t.Fatalf("unchanged safety-blocked input created %d jobs, want 1", jobCount)
	}

	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.SaveFactCard(ctx, userID, story.ID, facts, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, story.FactCard, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if story.FactCard.Version == blockedInput.FactsVersion {
		t.Fatalf("fact card version did not change: %d", story.FactCard.Version)
	}

	retryInput.RequestKey = fmt.Sprintf("safety-edited-%d", time.Now().UnixNano())
	retryInput.FactsVersion = story.FactCard.Version
	retryInput.OutlineVersion = story.Outline.Version
	queued, reused, err := store.CreateGenerationJobWithInput(ctx, userID, story.ID, retryInput)
	if err != nil || reused || queued.Status != JobQueued {
		t.Fatalf("create job after editing facts err=%v reused=%v job=%+v", err, reused, queued)
	}
}

func TestPostgresLifeStoryOutboxLeaseAndExpiredTokenCleanup(t *testing.T) {
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
	var userID int64
	phone := fmt.Sprintf("177%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, userID)
	var storyID, versionID, jobID, futureJobID, outboxID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,status,stage) VALUES($1,'completed','reading') RETURNING id`, userID).Scan(&storyID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_versions(story_id,app_user_id,version_no,status,perspective,tone) VALUES($1,$2,1,'published','first_person','warm') RETURNING id`, storyID, userID).Scan(&versionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs(story_id,app_user_id,request_key,status,version_id) VALUES($1,$2,$3,'succeeded',$4) RETURNING id`, storyID, userID, fmt.Sprintf("outbox-%d", time.Now().UnixNano()), versionID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs(story_id,app_user_id,request_key,status) VALUES($1,$2,$3,'failed') RETURNING id`, storyID, userID, fmt.Sprintf("future-token-%d", time.Now().UnixNano())).Scan(&futureJobID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_outbox(app_user_id,story_id,job_id,version_id,source_key) VALUES($1,$2,$3,$4,$5) RETURNING id`, userID, storyID, jobID, versionID, fmt.Sprintf("outbox-source-%d", time.Now().UnixNano())).Scan(&outboxID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `INSERT INTO app_life_story_token_maps(app_user_id,story_id,job_id,ciphertext,expires_at) VALUES($1,$2,$3,$4,now()-interval '1 minute'),($1,$2,$5,$4,now()+interval '1 hour')`, userID, storyID, jobID, []byte{1, 2, 3}, futureJobID); err != nil {
		t.Fatal(err)
	}

	store := NewStore(database)
	deleted, err := store.PurgeExpiredTokenMaps(ctx, 10)
	if err != nil || deleted != 1 {
		t.Fatalf("purge expired token maps deleted=%d err=%v", deleted, err)
	}
	var remaining int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_life_story_token_maps WHERE app_user_id=$1`, userID).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatalf("remaining token maps=%d err=%v", remaining, err)
	}

	first, err := store.ClaimOutbox(ctx, time.Minute)
	if err != nil || first.ID != outboxID || first.ClaimToken == "" || first.Attempts != 1 {
		t.Fatalf("first outbox claim=%+v err=%v", first, err)
	}
	if err := store.MarkOutboxPublished(ctx, outboxID, "wrong-token"); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong claim token error=%v want ErrConflict", err)
	}
	if err := store.MarkOutboxPublished(ctx, outboxID, first.ClaimToken, errors.New("delivery failed")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ClaimOutbox(ctx, time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("outbox ignored retry backoff: %v", err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_story_outbox SET next_attempt_at=now()-interval '1 second' WHERE id=$1`, outboxID); err != nil {
		t.Fatal(err)
	}
	second, err := store.ClaimOutbox(ctx, time.Minute)
	if err != nil || second.ClaimToken == first.ClaimToken || second.Attempts != 2 {
		t.Fatalf("second outbox claim=%+v err=%v", second, err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_story_outbox SET lease_until=now()-interval '1 second' WHERE id=$1`, outboxID); err != nil {
		t.Fatal(err)
	}
	third, err := store.ClaimOutbox(ctx, time.Minute)
	if err != nil || third.ClaimToken == second.ClaimToken || third.Attempts != 3 {
		t.Fatalf("expired lease reclaim=%+v err=%v", third, err)
	}
	if err := store.MarkOutboxPublished(ctx, outboxID, third.ClaimToken); err != nil {
		t.Fatal(err)
	}
	var published bool
	if err := database.QueryRowContext(ctx, `SELECT published_at IS NOT NULL FROM app_life_story_outbox WHERE id=$1`, outboxID).Scan(&published); err != nil || !published {
		t.Fatalf("published=%v err=%v", published, err)
	}
}

func TestPostgresGenerationGuardSerializesAccountDeletion(t *testing.T) {
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
	var userID, storyID, jobID int64
	phone := fmt.Sprintf("166%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,status,stage) VALUES($1,'generating','generating') RETURNING id`, userID).Scan(&storyID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs(story_id,app_user_id,request_key,status,claim_token,lease_until) VALUES($1,$2,$3,'running','claim',now()+interval '1 minute') RETURNING id`, storyID, userID, fmt.Sprintf("guard-%d", time.Now().UnixNano())).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database)
	release, err := store.acquireGenerationGuard(ctx, Job{ID: jobID, StoryID: storyID, AppUserID: userID, Status: JobRunning, ClaimToken: "claim"})
	if err != nil {
		t.Fatal(err)
	}
	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := database.ExecContext(ctx, `UPDATE app_users SET status='disabled' WHERE id=$1`, userID)
		updateDone <- updateErr
	}()
	select {
	case err := <-updateDone:
		t.Fatalf("account mutation bypassed generation guard: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	release()
	if err := <-updateDone; err != nil {
		t.Fatal(err)
	}
	if release, err := store.acquireGenerationGuard(ctx, Job{ID: jobID, StoryID: storyID, AppUserID: userID, Status: JobRunning, ClaimToken: "claim"}); !errors.Is(err, ErrInactiveUser) {
		if release != nil {
			release()
		}
		t.Fatalf("guard after account disable error=%v want ErrInactiveUser", err)
	}
}

func TestPostgresLifeStoryTerminalTransitionsReleaseQuota(t *testing.T) {
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
	var userID int64
	phone := fmt.Sprintf("188%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(ctx, `DELETE FROM app_users WHERE id=$1`, userID)

	store := NewStore(database)
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{Materials: []Material{{SourceType: MaterialText, Text: "一次真实的改变"}}})
	if err != nil {
		t.Fatal(err)
	}
	facts := FactCard{Characters: []FactCharacter{{Alias: "我"}}, Events: []FactEvent{{Description: "改变", Confirmed: true}}}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, facts, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	outline := Outline{Perspective: PerspectiveFirst, Tone: ToneWarm, Chapters: []OutlineChapter{
		{Order: 1, Title: "一"}, {Order: 2, Title: "二"}, {Order: 3, Title: "三"}, {Order: 4, Title: "四"},
	}}
	story, err = store.ConfirmOutline(ctx, userID, story.ID, outline, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	quotaStore := NewQuotaStore(database)
	assertQuota := func(reserved, consumed int) {
		t.Helper()
		quota, snapshotErr := quotaStore.Snapshot(ctx, userID, "")
		if snapshotErr != nil {
			t.Fatal(snapshotErr)
		}
		if quota.Reserved != reserved || quota.Consumed != consumed {
			t.Fatalf("quota=%+v, want reserved=%d consumed=%d", quota, reserved, consumed)
		}
	}
	createJob := func(key string) Job {
		t.Helper()
		job, reused, createErr := store.CreateJob(ctx, userID, story.ID, key)
		if createErr != nil || reused {
			t.Fatalf("create %s err=%v reused=%v", key, createErr, reused)
		}
		assertQuota(1, 0)
		return job
	}

	cancelled := createJob("terminal-cancel")
	if _, err := store.CancelJob(ctx, userID, story.ID, cancelled.ID); err != nil {
		t.Fatal(err)
	}
	assertQuota(0, 0)

	failed := createJob("terminal-fail")
	claimed, err := store.ClaimNextJob(ctx, 5*time.Minute)
	if err != nil || claimed.ID != failed.ID {
		t.Fatalf("claim err=%v job=%+v", err, claimed)
	}
	if _, err := store.FailJob(ctx, claimed.ID, claimed.ClaimToken, "test", "failed", false); err != nil {
		t.Fatal(err)
	}
	assertQuota(0, 0)

	rejected := createJob("terminal-reject")
	if _, err := store.RejectQueuedJob(ctx, userID, story.ID, rejected.ID, "test", "rejected"); err != nil {
		t.Fatal(err)
	}
	assertQuota(0, 0)

	blocked := createJob("terminal-block")
	claimed, err = store.ClaimNextJob(ctx, 5*time.Minute)
	if err != nil || claimed.ID != blocked.ID {
		t.Fatalf("claim err=%v job=%+v", err, claimed)
	}
	sensitiveOutput := strings.Replace(generatedJSON(), `"body":"`, `"summary":"writer@example.com","body":"`, 1)
	worker := NewWorker(WorkerConfig{
		Store: store,
		Generator: NewGenerator(GeneratorConfig{
			Completer: &fakeCompleter{raw: sensitiveOutput},
		}),
		Quota: NewQuotaStore(database),
	})
	if err := worker.processClaimed(ctx, claimed); !errors.Is(err, ErrSafetyBlocked) {
		t.Fatalf("worker safety error=%v, want ErrSafetyBlocked", err)
	}
	blocked, err = store.GetJob(ctx, userID, story.ID, blocked.ID)
	if err != nil || blocked.Status != JobSafetyBlocked || blocked.ErrorCategory != "safety" {
		t.Fatalf("blocked job=%+v err=%v", blocked, err)
	}
	assertQuota(0, 0)

	story, err = store.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	facts.Events[0].Description = "改变之后重新确认"
	story, err = store.SaveFactCard(ctx, userID, story.ID, facts, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, story.FactCard, story.Revision)
	if err != nil {
		t.Fatal(err)
	}

	tampered := createJob("terminal-tampered")
	claimed, err = store.ClaimNextJob(ctx, 5*time.Minute)
	if err != nil || claimed.ID != tampered.ID {
		t.Fatalf("claim err=%v job=%+v", err, claimed)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_story_jobs SET input_snapshot=jsonb_set(input_snapshot,'{storyId}','999'::jsonb) WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	version, err := ParseGeneratedVersion(generatedJSON())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.FinalizeJob(ctx, claimed, version, ""); !errors.Is(err, ErrPayloadConflict) {
		t.Fatalf("finalize tampered snapshot err=%v, want ErrPayloadConflict", err)
	}
	if _, err := store.FailJob(ctx, claimed.ID, claimed.ClaimToken, "invalid_input", "tampered", false); err != nil {
		t.Fatal(err)
	}
	assertQuota(0, 0)

	emptyStory, err := store.CreateStory(ctx, userID, CreateStoryInput{Materials: []Material{{SourceType: MaterialText, Text: "稍后再写"}}})
	if err != nil {
		t.Fatal(err)
	}
	emptyStory, err = store.SaveDraft(ctx, userID, emptyStory.ID, emptyStory.DraftVersion, DraftInput{Materials: []Material{}})
	if err != nil {
		t.Fatalf("save explicit empty draft: %v", err)
	}
	if emptyStory.MaterialCount != 0 || len(emptyStory.Materials) != 0 {
		t.Fatalf("empty draft retained materials: %+v", emptyStory.Materials)
	}
}

func TestPostgresExpiredRunningJobAtMaxAttemptsIsReapedAndReleasesQuota(t *testing.T) {
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

	var userID int64
	phone := fmt.Sprintf("155%08d", time.Now().UnixNano()%100_000_000)
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)

	store := NewStoreWithTokenKey(database, "test-secret")
	story, err := store.CreateStory(ctx, userID, CreateStoryInput{Materials: []Material{{
		SourceType: MaterialText,
		Text:       "联系 writer@example.com 后，我经历了一次真实的改变。",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.ConfirmFacts(ctx, userID, story.ID, FactCard{
		Characters: []FactCharacter{{Alias: "我"}},
		Events:     []FactEvent{{Description: "一次真实的改变", Confirmed: true}},
	}, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	story, err = store.ConfirmOutline(ctx, userID, story.ID, Outline{
		Perspective: PerspectiveFirst,
		Tone:        ToneWarm,
		Chapters: []OutlineChapter{
			{Order: 1, Title: "一"}, {Order: 2, Title: "二"},
			{Order: 3, Title: "三"}, {Order: 4, Title: "四"},
		},
	}, story.Revision)
	if err != nil {
		t.Fatal(err)
	}
	job, reused, err := store.CreateJob(ctx, userID, story.ID, fmt.Sprintf("exhausted-%d", time.Now().UnixNano()))
	if err != nil || reused {
		t.Fatalf("create job err=%v reused=%v", err, reused)
	}
	quota, err := NewQuotaStore(database).Snapshot(ctx, userID, "")
	if err != nil || quota.Reserved != 1 || quota.Consumed != 0 {
		t.Fatalf("reserved quota=%+v err=%v", quota, err)
	}
	var tokenMaps int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_life_story_token_maps WHERE job_id=$1`, job.ID).Scan(&tokenMaps); err != nil || tokenMaps != 1 {
		t.Fatalf("token maps=%d err=%v", tokenMaps, err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_story_jobs
		SET status='running',attempt=max_attempts,claim_token='dead-worker',
		lease_until=now()-interval '1 second' WHERE id=$1`, job.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE app_life_stories SET status='generating',stage='generating' WHERE id=$1`, story.ID); err != nil {
		t.Fatal(err)
	}

	if claimed, err := store.ClaimNextJob(ctx, time.Minute); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("claim returned job=%+v err=%v, want exhausted job reap and sql.ErrNoRows", claimed, err)
	}
	var status JobStatus
	var attempt, maxAttempts int
	var category, claimToken string
	var leaseUntil, finishedAt sql.NullTime
	if err := database.QueryRowContext(ctx, `SELECT status,attempt,max_attempts,error_category,claim_token,lease_until,finished_at
		FROM app_life_story_jobs WHERE id=$1`, job.ID).Scan(&status, &attempt, &maxAttempts, &category, &claimToken, &leaseUntil, &finishedAt); err != nil {
		t.Fatal(err)
	}
	if status != JobFailed || attempt != maxAttempts || category != "attempts_exhausted" || claimToken != "" || leaseUntil.Valid || !finishedAt.Valid {
		t.Fatalf("exhausted job status=%s attempt=%d/%d category=%q claim=%q lease=%v finished=%v", status, attempt, maxAttempts, category, claimToken, leaseUntil, finishedAt)
	}
	quota, err = NewQuotaStore(database).Snapshot(ctx, userID, "")
	if err != nil || quota.Reserved != 0 || quota.Consumed != 0 {
		t.Fatalf("released quota=%+v err=%v", quota, err)
	}
	var releases int
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_story_quota_ledger WHERE job_id=$1 AND entry_type='release'`, job.ID).Scan(&releases); err != nil || releases != 1 {
		t.Fatalf("release ledger rows=%d err=%v", releases, err)
	}
	var storyStatus StoryStatus
	var storyStage StoryStage
	if err := database.QueryRowContext(ctx, `SELECT status,stage FROM app_life_stories WHERE id=$1`, story.ID).Scan(&storyStatus, &storyStage); err != nil {
		t.Fatal(err)
	}
	if storyStatus != StatusFailed || storyStage != StageFailed {
		t.Fatalf("story status=%s stage=%s", storyStatus, storyStage)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_life_story_token_maps WHERE job_id=$1`, job.ID).Scan(&tokenMaps); err != nil || tokenMaps != 0 {
		t.Fatalf("terminal token maps=%d err=%v", tokenMaps, err)
	}

	_, _ = store.ClaimNextJob(ctx, time.Minute)
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM app_story_quota_ledger WHERE job_id=$1 AND entry_type='release'`, job.ID).Scan(&releases); err != nil || releases != 1 {
		t.Fatalf("duplicate release ledger rows=%d err=%v", releases, err)
	}
	if _, reused, err := store.CreateJob(ctx, userID, story.ID, fmt.Sprintf("after-reap-%d", time.Now().UnixNano())); err != nil || reused {
		t.Fatalf("new job after exhausted reap err=%v reused=%v", err, reused)
	}
}

func TestPostgresLifeStoryDeleteSerializesWithJobTransitions(t *testing.T) {
	dsn := os.Getenv("LIFESTORY_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("LIFESTORY_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "", "")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	actions := []struct {
		name    string
		running bool
		run     func(context.Context, *Store, int64, int64, int64, string) error
	}{
		{
			name: "claim",
			run: func(ctx context.Context, store *Store, _, _, jobID int64, _ string) error {
				job, err := store.ClaimNextJob(ctx, time.Minute)
				if err == nil && job.ID != jobID {
					return fmt.Errorf("claimed job %d, want %d", job.ID, jobID)
				}
				return err
			},
		},
		{
			name: "cancel",
			run: func(ctx context.Context, store *Store, userID, storyID, jobID int64, _ string) error {
				_, err := store.CancelJob(ctx, userID, storyID, jobID)
				return err
			},
		},
		{
			name: "reject",
			run: func(ctx context.Context, store *Store, userID, storyID, jobID int64, _ string) error {
				_, err := store.RejectQueuedJob(ctx, userID, storyID, jobID, "test", "rejected")
				return err
			},
		},
		{
			name:    "fail",
			running: true,
			run: func(ctx context.Context, store *Store, _, _, jobID int64, claimToken string) error {
				_, err := store.FailJob(ctx, jobID, claimToken, "test", "failed", false)
				return err
			},
		},
		{
			name:    "block",
			running: true,
			run: func(ctx context.Context, store *Store, _, _, jobID int64, claimToken string) error {
				_, err := store.BlockJob(ctx, jobID, claimToken, "safety", "blocked")
				return err
			},
		},
	}

	for actionIndex, action := range actions {
		t.Run(action.name, func(t *testing.T) {
			phone := fmt.Sprintf("154%08d", (time.Now().UnixNano()+int64(actionIndex))%100_000_000)
			var userID, storyID, jobID, ledgerID int64
			if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
			})
			storyStatus, storyStage := StatusQueued, StageQueued
			jobStatus, claimToken := JobQueued, ""
			if action.running {
				storyStatus, storyStage = StatusGenerating, StageGenerating
				jobStatus, claimToken = JobRunning, "transition-claim"
			}
			if err := database.QueryRowContext(ctx, `INSERT INTO app_life_stories(app_user_id,status,stage)
				VALUES($1,$2,$3) RETURNING id`, userID, storyStatus, storyStage).Scan(&storyID); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRowContext(ctx, `INSERT INTO app_life_story_jobs
				(story_id,app_user_id,request_key,status,claim_token,lease_until)
				VALUES($1,$2,$3,$4,$5,CASE WHEN $4='running' THEN now()+interval '1 minute' ELSE NULL END)
				RETURNING id`, storyID, userID, fmt.Sprintf("delete-lock-%s-%d", action.name, time.Now().UnixNano()), jobStatus, claimToken).Scan(&jobID); err != nil {
				t.Fatal(err)
			}
			var periodID int64
			if err := database.QueryRowContext(ctx, `INSERT INTO app_story_quota_periods
				(app_user_id,period_key,quota_limit,reserved) VALUES($1,$2,1,1) RETURNING id`,
				userID, fmt.Sprintf("delete-lock-%d", time.Now().UnixNano())).Scan(&periodID); err != nil {
				t.Fatal(err)
			}
			if err := database.QueryRowContext(ctx, `INSERT INTO app_story_quota_ledger
				(app_user_id,period_id,job_id,entry_type,amount,idempotency_key)
				VALUES($1,$2,$3,'reserve',1,$4) RETURNING id`, userID, periodID, jobID,
				fmt.Sprintf("delete-lock-reserve-%d", jobID)).Scan(&ledgerID); err != nil {
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

			store := NewStore(database)
			deleteDone := make(chan error, 1)
			go func() { deleteDone <- store.DeleteStory(ctx, userID, storyID) }()
			if !waitForPostgresRowLock(ctx, database, `SELECT id FROM app_users WHERE id=$1 FOR UPDATE`, userID, 2*time.Second) {
				t.Fatal("DeleteStory did not acquire the user lock")
			}

			actionDone := make(chan error, 1)
			go func() { actionDone <- action.run(ctx, store, userID, storyID, jobID, claimToken) }()
			// On the buggy path the transition acquires the job before waiting for
			// DeleteStory's story/ledger locks. After the fix it waits on the user.
			_ = waitForPostgresRowLock(ctx, database, `SELECT id FROM app_life_story_jobs WHERE id=$1 FOR UPDATE`, jobID, 300*time.Millisecond)
			if err := blocker.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				t.Fatal(err)
			}

			var deleteErr, transitionErr error
			select {
			case deleteErr = <-deleteDone:
			case <-ctx.Done():
				t.Fatalf("DeleteStory did not finish: %v", ctx.Err())
			}
			select {
			case transitionErr = <-actionDone:
			case <-ctx.Done():
				t.Fatalf("%s did not finish: %v", action.name, ctx.Err())
			}
			if deleteErr != nil {
				t.Fatalf("DeleteStory raced with %s: %v", action.name, deleteErr)
			}
			if transitionErr != nil && !errors.Is(transitionErr, sql.ErrNoRows) &&
				!errors.Is(transitionErr, ErrNotFound) && !errors.Is(transitionErr, ErrConflict) {
				t.Fatalf("%s returned a database concurrency error: %v", action.name, transitionErr)
			}
		})
	}
}

func TestPostgresDeleteStoryDoesNotDeletePrefixSiblingNotifications(t *testing.T) {
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
	phone := fmt.Sprintf("153%08d", time.Now().UnixNano()%100_000_000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	defer database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)

	targetStoryID := int64(1_000_000_000 + time.Now().UnixNano()%100_000_000)
	siblingStoryID := targetStoryID * 10
	if _, err := database.ExecContext(ctx, `INSERT INTO app_life_stories(id,app_user_id,status,stage)
		VALUES($1,$3,'completed','reading'),($2,$3,'completed','reading')`, targetStoryID, siblingStoryID, userID); err != nil {
		t.Fatal(err)
	}
	targetLink := fmt.Sprintf("/life-stories/%d/read", targetStoryID)
	siblingLink := fmt.Sprintf("/life-stories/%d/read", siblingStoryID)
	if _, err := database.ExecContext(ctx, `INSERT INTO app_notifications
		(app_user_id,kind,title,deep_link,source_key) VALUES
		($1,'life_story','target',$2,$3),($1,'life_story','sibling',$4,$5)`,
		userID, targetLink, fmt.Sprintf("target-%d", targetStoryID), siblingLink, fmt.Sprintf("sibling-%d", siblingStoryID)); err != nil {
		t.Fatal(err)
	}
	if err := NewStore(database).DeleteStory(ctx, userID, targetStoryID); err != nil {
		t.Fatal(err)
	}
	var targetCount, siblingCount int
	if err := database.QueryRowContext(ctx, `SELECT
		count(*) FILTER (WHERE deep_link=$2),count(*) FILTER (WHERE deep_link=$3)
		FROM app_notifications WHERE app_user_id=$1`, userID, targetLink, siblingLink).Scan(&targetCount, &siblingCount); err != nil {
		t.Fatal(err)
	}
	if targetCount != 0 || siblingCount != 1 {
		t.Fatalf("notifications after delete: target=%d sibling=%d", targetCount, siblingCount)
	}
}

func waitForPostgresRowLock(ctx context.Context, database *sql.DB, query string, id int64, timeout time.Duration) bool {
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
