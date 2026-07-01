package videostoryboard

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestResultCarriesEditableStoryboard(t *testing.T) {
	result := Result{
		GlobalPrompt: "疗愈短片，全片柔和光线",
		StyleGuide:   []string{"柔和光", "真实纪实"},
		Shots: []Shot{
			{
				Index:          1,
				Duration:       4,
				Scene:          "室内窗边",
				Characters:     []string{"女性"},
				Assets:         []string{"窗光", "书本"},
				Action:         "低头翻书",
				Camera:         "近景缓慢推入",
				SeedancePrompt: "室内窗边女性低头翻书，近景缓慢推入，柔和光线",
			},
		},
		Title: "疗愈主题分镜",
	}

	if result.Title == "" || result.GlobalPrompt == "" {
		t.Fatalf("expected title and global prompt, got %+v", result)
	}
	if len(result.Shots) != 1 || result.Shots[0].SeedancePrompt == "" {
		t.Fatalf("expected editable shots, got %+v", result.Shots)
	}
}

func TestCleanShotsNormalizesListsAndIndexes(t *testing.T) {
	shots := cleanShots([]Shot{
		{
			Assets:     []string{"窗光", "", "窗光"},
			Characters: []string{" 女性 ", "女性"},
			Scene:      " 室内 ",
		},
		{},
	})

	if len(shots) != 1 {
		t.Fatalf("expected one non-empty shot, got %+v", shots)
	}
	if shots[0].Index != 1 || shots[0].Scene != "室内" {
		t.Fatalf("expected normalized shot index/scene, got %+v", shots[0])
	}
	if len(shots[0].Assets) != 1 || shots[0].Assets[0] != "窗光" {
		t.Fatalf("expected cleaned assets, got %+v", shots[0].Assets)
	}
	if len(shots[0].Characters) != 1 || shots[0].Characters[0] != "女性" {
		t.Fatalf("expected cleaned characters, got %+v", shots[0].Characters)
	}
}

func TestMarkRunningReturnsErrorWhenNoQueuedStoryboardClaimed(t *testing.T) {
	db, cleanup := newAffectedRowsDB(t, 0)
	defer cleanup()

	err := NewStore(db).MarkRunning(context.Background(), "123")
	if err == nil {
		t.Fatal("expected MarkRunning to reject a stale or already-running storyboard")
	}
	if !strings.Contains(err.Error(), "分镜任务不可运行") {
		t.Fatalf("expected helpful mark-running error, got %v", err)
	}
}

func TestMarkRunningSucceedsWhenQueuedStoryboardClaimed(t *testing.T) {
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
	name := "videostoryboard_affected_rows_test_" + strconv.Itoa(affectedRowsDriverCounter)
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
