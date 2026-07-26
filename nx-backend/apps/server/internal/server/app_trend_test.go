package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAppTrendMessageSignalsExcludeNonChatScenes(t *testing.T) {
	registerAppTrendSceneDriverOnce.Do(func() {
		sql.Register(appTrendSceneDriverName, appTrendSceneDriver{})
	})
	database, err := sql.Open(appTrendSceneDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	signals := map[string]appTrendDaySignals{}
	server := &Server{db: database}
	if err := server.addAppTrendMessageSignals(context.Background(), signals, 7, 9, start, start.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("addAppTrendMessageSignals returned error: %v", err)
	}
	if signals["2026-07-01"].UserMessages != 1 {
		t.Fatalf("signals = %+v, want one regular chat message", signals)
	}
}

const appTrendSceneDriverName = "app_trend_scene_test"

var registerAppTrendSceneDriverOnce sync.Once

type appTrendSceneDriver struct{}

func (appTrendSceneDriver) Open(string) (driver.Conn, error) { return appTrendSceneConn{}, nil }

type appTrendSceneConn struct{}

func (appTrendSceneConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appTrendSceneConn) Close() error                        { return nil }
func (appTrendSceneConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (appTrendSceneConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	normalized := strings.Join(strings.Fields(query), " ")
	want := "SELECT m.role, m.content, m.favorite, m.feedback, m.create_time FROM app_chat_sessions s JOIN app_chat_messages m ON m.session_id = s.id WHERE s.app_user_id = $1 AND s.card_id = $2 AND s.scene = 'chat' AND m.create_time >= $3 AND m.create_time < $4 ORDER BY m.create_time"
	if normalized != want {
		return nil, errors.New("trend message query is missing the exact regular-chat scene boundary: " + normalized)
	}
	if len(args) != 4 || args[0].Value != int64(7) || args[1].Value != int64(9) {
		return nil, errors.New("unexpected trend query arguments")
	}
	return &appTrendSceneRows{values: [][]driver.Value{{"user", "普通聊天", false, "", time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)}}}, nil
}

type appTrendSceneRows struct {
	values [][]driver.Value
	index  int
}

func (*appTrendSceneRows) Columns() []string {
	return []string{"role", "content", "favorite", "feedback", "create_time"}
}
func (*appTrendSceneRows) Close() error { return nil }
func (r *appTrendSceneRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

func TestBuildAppTrendSeriesUsesRealDailySignals(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 2)
	signals := map[string]appTrendDaySignals{
		"2026-07-01": {
			UserMessages: 2,
			StressHits:   3,
		},
		"2026-07-02": {
			UserMessages:      1,
			AssistantMessages: 1,
			UserChars:         200,
			EnergyHits:        2,
			RelationshipHits:  2,
			AwarenessHits:     1,
			FavoriteHelpful:   1,
			Checkins:          1,
			Memories:          1,
		},
	}

	series := buildAppTrendSeries(start, end, signals)

	if got := appTrendValue(t, series, "stress", "2026-07-01"); got != 74 {
		t.Fatalf("expected high stress day to score 74, got %.1f", got)
	}
	if got := appTrendValue(t, series, "energy", "2026-07-01"); got != 46 {
		t.Fatalf("expected stress-heavy day to lower energy to 46, got %.1f", got)
	}
	if got := appTrendValue(t, series, "relationship", "2026-07-02"); got != 72 {
		t.Fatalf("expected relationship signals to score 72, got %.1f", got)
	}
	if got := appTrendValue(t, series, "awareness", "2026-07-02"); got != 77 {
		t.Fatalf("expected awareness signals to score 77, got %.1f", got)
	}
}

func TestBuildAppTrendSeriesUsesNeutralBaselineWithoutSignals(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 3)

	series := buildAppTrendSeries(start, end, nil)

	for _, item := range []struct {
		dimension string
		want      float64
	}{
		{"stress", 48},
		{"energy", 54},
		{"relationship", 52},
		{"awareness", 50},
	} {
		if got := appTrendValue(t, series, item.dimension, "2026-07-02"); got != item.want {
			t.Fatalf("expected neutral %s baseline %.1f, got %.1f", item.dimension, item.want, got)
		}
	}
}

func appTrendValue(t *testing.T, series []appTrendSeries, dimension, date string) float64 {
	t.Helper()
	for _, item := range series {
		if item.Dimension != dimension {
			continue
		}
		for _, point := range item.Points {
			if point.Date == date {
				return point.Value
			}
		}
	}
	t.Fatalf("missing trend point dimension=%s date=%s", dimension, date)
	return 0
}
