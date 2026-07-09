package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestRecordAppProfileEvidencePersistsRoundScopedEvidence(t *testing.T) {
	registerAppProfileEvidenceTestDriver()
	appProfileEvidenceRecorder.reset()
	database, err := sql.Open(appProfileEvidenceTestDriverName, "record")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	s := &Server{db: database}

	s.recordAppProfileEvidence(context.Background(), 7, 123, "chat", 55, "我总是担心关系不稳定，会反复确认对方态度。")

	query := appProfileEvidenceRecorder.exec()
	for _, want := range []string{
		"INSERT INTO app_profile_evidence",
		"round_no",
		"source_type",
		"trait_scores",
		"type_scores",
		"emotion_scores",
		"behavior_scores",
		"confidence",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("expected evidence insert to contain %q, query:\n%s", want, query)
		}
	}
}

func TestRecordAppProfileEvidenceVerifiesCardOwnership(t *testing.T) {
	source := readServerSource(t, "app_profile_evidence.go")
	for _, want := range []string{"FROM app_user_cards", "id=$1", "app_user_id=$2", "status='active'"} {
		if !strings.Contains(source, want) {
			t.Fatalf("expected evidence recorder to verify card ownership with %q", want)
		}
	}
}

func TestChatAndVoicePathsRecordProfileEvidence(t *testing.T) {
	chatSource := readServerSource(t, "app_chat.go")
	if !strings.Contains(chatSource, "recordAppProfileEvidenceAsync") || !strings.Contains(chatSource, "\"chat\"") {
		t.Fatalf("expected chat ask path to record chat profile evidence")
	}
	voiceSource := readServerSource(t, "app_voice.go")
	if !strings.Contains(voiceSource, "recordAppProfileEvidenceAsync") || !strings.Contains(voiceSource, "\"voice_text\"") || !strings.Contains(voiceSource, "cardId") {
		t.Fatalf("expected voice recognize path to record card-scoped voice_text evidence")
	}
}

func readServerSource(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}

var appProfileEvidenceRecorder = &appProfileEvidenceRecording{}

const appProfileEvidenceTestDriverName = "app_profile_evidence_test"

var appProfileEvidenceTestDriverRegistered bool

func registerAppProfileEvidenceTestDriver() {
	if appProfileEvidenceTestDriverRegistered {
		return
	}
	appProfileEvidenceTestDriverRegistered = true
	sql.Register(appProfileEvidenceTestDriverName, appProfileEvidenceDriver{})
}

type appProfileEvidenceRecording struct {
	mu        sync.Mutex
	execQuery string
}

func (r *appProfileEvidenceRecording) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execQuery = ""
}

func (r *appProfileEvidenceRecording) setExec(query string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.execQuery = query
}

func (r *appProfileEvidenceRecording) exec() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.execQuery
}

type appProfileEvidenceDriver struct{}

func (appProfileEvidenceDriver) Open(string) (driver.Conn, error) {
	return appProfileEvidenceConn{}, nil
}

type appProfileEvidenceConn struct{}

func (appProfileEvidenceConn) Prepare(string) (driver.Stmt, error)      { return nil, nil }
func (appProfileEvidenceConn) Close() error                             { return nil }
func (appProfileEvidenceConn) Begin() (driver.Tx, error)                { return nil, nil }
func (appProfileEvidenceConn) CheckNamedValue(*driver.NamedValue) error { return nil }

func (appProfileEvidenceConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	appProfileEvidenceRecorder.setExec(query)
	return driver.RowsAffected(1), nil
}

func (appProfileEvidenceConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return &appProfileEvidenceRows{columns: []string{"round_no"}, rows: [][]driver.Value{{int64(2)}}}, nil
}

type appProfileEvidenceRows struct {
	columns []string
	rows    [][]driver.Value
	idx     int
}

func (r appProfileEvidenceRows) Columns() []string { return r.columns }
func (r appProfileEvidenceRows) Close() error      { return nil }
func (r *appProfileEvidenceRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.idx])
	r.idx++
	return nil
}

var _ driver.ExecerContext = appProfileEvidenceConn{}
var _ driver.QueryerContext = appProfileEvidenceConn{}
var _ driver.NamedValueChecker = appProfileEvidenceConn{}
