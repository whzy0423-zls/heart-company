package videoanalysis

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestResultCarriesSpeechInsights(t *testing.T) {
	result := Result{
		AudioSummary:   "视频中主要讨论自我认知和行动计划。",
		HasSpeech:      true,
		SpeechKeywords: []string{"目标", "复盘"},
		SpeechOutline:  []string{"介绍问题背景", "提出行动建议"},
		SpeechTopics:   []string{"自我认知", "行动计划"},
	}

	if !result.HasSpeech {
		t.Fatal("expected result to carry speech detection")
	}
	if result.AudioSummary == "" {
		t.Fatal("expected result to carry audio summary")
	}
	if len(result.SpeechTopics) != 2 || len(result.SpeechKeywords) != 2 || len(result.SpeechOutline) != 2 {
		t.Fatalf("expected result to carry speech lists, got %+v", result)
	}
}

func TestMarkRunningReturnsErrorWhenNoQueuedJobClaimed(t *testing.T) {
	db, cleanup := newAffectedRowsDB(t, 0)
	defer cleanup()

	err := NewStore(db).MarkRunning(context.Background(), "123")
	if err == nil {
		t.Fatal("expected MarkRunning to reject a stale or already-running job")
	}
	if !strings.Contains(err.Error(), "视频分析任务不可运行") {
		t.Fatalf("expected helpful mark-running error, got %v", err)
	}
}

func TestMarkRunningSucceedsWhenQueuedJobClaimed(t *testing.T) {
	db, cleanup := newAffectedRowsDB(t, 1)
	defer cleanup()

	if err := NewStore(db).MarkRunning(context.Background(), "123"); err != nil {
		t.Fatalf("MarkRunning returned error: %v", err)
	}
}

var affectedRowsDriverCounter int

func newAffectedRowsDB(t *testing.T, rows int64) (*sql.DB, func()) {
	t.Helper()
	affectedRowsDriversMu.Lock()
	affectedRowsDriverCounter++
	name := "videoanalysis_affected_rows_test_" + strings.Repeat("x", affectedRowsDriverCounter)
	affectedRowsDrivers[name] = rows
	affectedRowsDriversMu.Unlock()
	sql.Register(name, affectedRowsDriver{})
	db, err := sql.Open(name, name)
	if err != nil {
		t.Fatal(err)
	}
	return db, func() {
		db.Close()
		affectedRowsDriversMu.Lock()
		delete(affectedRowsDrivers, name)
		affectedRowsDriversMu.Unlock()
	}
}

var (
	affectedRowsDriversMu sync.Mutex
	affectedRowsDrivers   = map[string]int64{}
)

type affectedRowsDriver struct{}

func (affectedRowsDriver) Open(name string) (driver.Conn, error) {
	affectedRowsDriversMu.Lock()
	rows, ok := affectedRowsDrivers[name]
	affectedRowsDriversMu.Unlock()
	if !ok {
		return nil, errors.New("unknown affected rows driver")
	}
	return affectedRowsConn{rows: rows}, nil
}

type affectedRowsConn struct {
	rows int64
}

func (affectedRowsConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare is not implemented")
}

func (affectedRowsConn) Close() error {
	return nil
}

func (affectedRowsConn) Begin() (driver.Tx, error) {
	return nil, errors.New("Begin is not implemented")
}

func (c affectedRowsConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(c.rows), nil
}

func (affectedRowsConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return nil, io.EOF
}
