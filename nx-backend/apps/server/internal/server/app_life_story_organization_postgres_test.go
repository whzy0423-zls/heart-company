package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/lifestory"
)

func TestPostgresLifeStoryPrepareExtractsOrganizationsIntoFactCard(t *testing.T) {
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

	phone := fmt.Sprintf("151%08d", time.Now().UnixNano()%100_000_000)
	var userID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users(phone) VALUES($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = database.ExecContext(context.Background(), `DELETE FROM app_users WHERE id=$1`, userID)
	})

	storyStore := lifestory.NewStore(database)
	story, err := storyStore.CreateStory(ctx, userID, lifestory.CreateStoryInput{
		Materials: []lifestory.Material{{
			SourceType: lifestory.MaterialText,
			Text:       "我曾在北京大学学习，后来加入腾讯公司。",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/app/life-stories/%d/prepare", story.ID), nil)
	response := httptest.NewRecorder()
	server := &Server{
		lifeStories: storyStore,
		ragGen: &preferenceJSONGenerator{name: `{
			"organizations":[{"name":"北京大学"},{"name":"腾讯公司"}],
			"questions":[
				{"prompt":"你在北京大学第一次独自走进校园时，最清晰的画面是什么？"},
				{"prompt":"从毕业到加入腾讯公司的那段时间，哪个决定最影响后来的你？"}
			]
		}`},
	}

	server.appLifeStoryPrepare(response, request, userID, story.ID)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data struct {
			Facts lifestory.FactCard `json:"facts"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	assertExtractedOrganizations(t, payload.Data.Facts.Organizations)
	assertDynamicPreparationQuestions(t, payload.Data.Facts.Questions)

	persisted, err := storyStore.Get(ctx, userID, story.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertExtractedOrganizations(t, persisted.FactCard.Organizations)
	assertDynamicPreparationQuestions(t, persisted.FactCard.Questions)
}

func assertExtractedOrganizations(t *testing.T, organizations []lifestory.FactOrganization) {
	t.Helper()
	if len(organizations) != 2 {
		t.Fatalf("organizations=%+v want two entries", organizations)
	}
	for index, wantName := range []string{"北京大学", "腾讯公司"} {
		organization := organizations[index]
		if organization.Name != wantName || organization.ID == "" || organization.RedactionMode != "blurred" {
			t.Fatalf("organization[%d]=%+v", index, organization)
		}
	}
}

func assertDynamicPreparationQuestions(t *testing.T, questions []lifestory.Question) {
	t.Helper()
	if len(questions) != 2 {
		t.Fatalf("questions=%+v want two dynamic entries", questions)
	}
	for index, question := range questions {
		if question.ID == "" || question.ID == "turning_point" || question.ID == "ending" {
			t.Fatalf("question[%d] does not have a server-generated dynamic ID: %+v", index, question)
		}
		if question.Sequence != index+1 || question.Answer != "" || question.Skipped {
			t.Fatalf("question[%d] has invalid state: %+v", index, question)
		}
	}
	if questions[0].Prompt != "你在北京大学第一次独自走进校园时，最清晰的画面是什么？" ||
		questions[1].Prompt != "从毕业到加入腾讯公司的那段时间，哪个决定最影响后来的你？" {
		t.Fatalf("dynamic prompts were not preserved: %+v", questions)
	}
}
