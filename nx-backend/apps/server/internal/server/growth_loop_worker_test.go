package server

import (
	"database/sql"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/config"
)

func TestGrowthWorkerDisabledKeepsStoreWithoutStarting(t *testing.T) {
	database, err := sql.Open("pgx", "postgres://fixture:fixture@127.0.0.1:1/growth_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	s := &Server{db: database}
	t.Cleanup(func() {
		if s.growthInsightCancel != nil {
			s.growthInsightCancel()
			s.growthInsightWorkers.Wait()
		}
	})
	s.startGrowthInsights()
	if s.growthInsights == nil {
		t.Fatal("disabled worker must preserve the growth API store")
	}
	if s.growthInsightCancel != nil {
		t.Fatal("disabled worker started a background loop")
	}
}

func TestGrowthWorkerEnabledStartsAndStops(t *testing.T) {
	database, err := sql.Open("pgx", "postgres://fixture:fixture@127.0.0.1:1/growth_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	_ = database.Close()
	s := &Server{db: database, env: config.Env{
		GrowthInsightsWorkerEnabled:         true,
		GrowthInsightsMaxOutputTokens:       4096,
		GrowthInsightsDailyAttemptLimit:     7,
		GrowthInsightsUserDailyAttemptLimit: 2,
	}}
	s.startGrowthInsights()
	if s.growthInsights == nil || s.growthInsightCancel == nil {
		t.Fatal("enabled worker was not initialized")
	}
	s.growthInsightCancel()
	s.growthInsightWorkers.Wait()
}
