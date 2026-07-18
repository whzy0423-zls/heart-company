package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestSaveVoicePairPersistsHiddenTranscriptAndAudioMetadata(t *testing.T) {
	registerVoiceStoreDriver()
	database, err := sql.Open(voiceStoreDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	userID, assistantID, err := NewStore(database).SaveVoicePair(
		context.Background(), 42, 88, 3200, "孩子最近不愿意沟通", "可以先倾听她", json.RawMessage(`[]`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if userID != 11 || assistantID != 12 {
		t.Fatalf("unexpected message ids: user=%d assistant=%d", userID, assistantID)
	}
}

func TestSaveVoiceMessagePersistsWithoutAssistantPair(t *testing.T) {
	registerVoiceStoreDriver()
	database, err := sql.Open(voiceStoreDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	userID, err := NewStore(database).SaveVoiceMessage(
		context.Background(), 42, 88, 2500, "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if userID != 11 {
		t.Fatalf("user message id = %d, want 11", userID)
	}
}

func TestGetVoiceAudioAssetIDChecksAppOwnership(t *testing.T) {
	registerVoiceStoreDriver()
	database, err := sql.Open(voiceStoreDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	assetID, err := NewStore(database).GetVoiceAudioAssetID(context.Background(), 7, 11)
	if err != nil {
		t.Fatal(err)
	}
	if assetID != 88 {
		t.Fatalf("asset id = %d, want 88", assetID)
	}
}

func TestGetVoiceTranscriptChecksOwnershipAndReturnsTrimmedTranscript(t *testing.T) {
	database := openVoiceTranscriptStoreDatabase(t, voiceTranscriptQueryResult{
		value: "  孩子最近不愿意沟通 \n",
	})

	transcript, err := NewStore(database).GetVoiceTranscript(context.Background(), 7, 11)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "孩子最近不愿意沟通" {
		t.Fatalf("transcript = %q, want trimmed transcript", transcript)
	}
}

func TestGetVoiceTranscriptReturnsEmptyTranscript(t *testing.T) {
	database := openVoiceTranscriptStoreDatabase(t, voiceTranscriptQueryResult{value: " \n\t "})

	transcript, err := NewStore(database).GetVoiceTranscript(context.Background(), 7, 11)
	if err != nil {
		t.Fatal(err)
	}
	if transcript != "" {
		t.Fatalf("transcript = %q, want empty string", transcript)
	}
}

func TestGetVoiceTranscriptMapsNoRowsToNotFound(t *testing.T) {
	database := openVoiceTranscriptStoreDatabase(t, voiceTranscriptQueryResult{err: sql.ErrNoRows})

	_, err := NewStore(database).GetVoiceTranscript(context.Background(), 7, 11)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestGetVoiceTranscriptPreservesDatabaseErrors(t *testing.T) {
	wantErr := errors.New("database unavailable")
	database := openVoiceTranscriptStoreDatabase(t, voiceTranscriptQueryResult{err: wantErr})

	_, err := NewStore(database).GetVoiceTranscript(context.Background(), 7, 11)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}

const voiceTranscriptStoreDriverName = "chat_voice_transcript_store_test"

var (
	registerVoiceTranscriptStoreDriverOnce sync.Once
	voiceTranscriptQueryMu                 sync.Mutex
	voiceTranscriptQueryResults            = map[string]voiceTranscriptQueryResult{}
	voiceTranscriptQuerySequence           int
)

type voiceTranscriptQueryResult struct {
	value string
	err   error
}

func openVoiceTranscriptStoreDatabase(t *testing.T, result voiceTranscriptQueryResult) *sql.DB {
	t.Helper()
	registerVoiceTranscriptStoreDriverOnce.Do(func() {
		sql.Register(voiceTranscriptStoreDriverName, voiceTranscriptStoreDriver{})
	})

	voiceTranscriptQueryMu.Lock()
	voiceTranscriptQuerySequence++
	dsn := fmt.Sprintf("voice-transcript-%d", voiceTranscriptQuerySequence)
	voiceTranscriptQueryResults[dsn] = result
	voiceTranscriptQueryMu.Unlock()

	database, err := sql.Open(voiceTranscriptStoreDriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = database.Close()
		voiceTranscriptQueryMu.Lock()
		delete(voiceTranscriptQueryResults, dsn)
		voiceTranscriptQueryMu.Unlock()
	})
	return database
}

type voiceTranscriptStoreDriver struct{}

func (voiceTranscriptStoreDriver) Open(dsn string) (driver.Conn, error) {
	voiceTranscriptQueryMu.Lock()
	result, ok := voiceTranscriptQueryResults[dsn]
	voiceTranscriptQueryMu.Unlock()
	if !ok {
		return nil, errors.New("missing voice transcript query result")
	}
	return &voiceTranscriptStoreConn{result: result}, nil
}

type voiceTranscriptStoreConn struct {
	result voiceTranscriptQueryResult
}

func (*voiceTranscriptStoreConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*voiceTranscriptStoreConn) Close() error                        { return nil }
func (*voiceTranscriptStoreConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c *voiceTranscriptStoreConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	requiredSQL := []string{
		"JOIN app_chat_sessions",
		"m.id = $1",
		"s.app_user_id = $2",
		"m.role = 'user'",
		"m.message_type = 'voice'",
	}
	for _, fragment := range requiredSQL {
		if !strings.Contains(query, fragment) {
			return nil, fmt.Errorf("voice transcript lookup missing SQL fragment %q", fragment)
		}
	}
	if len(args) != 2 || args[0].Value != int64(11) || args[1].Value != int64(7) {
		return nil, errors.New("voice transcript ownership lookup arguments mismatch")
	}
	if c.result.err != nil {
		return nil, c.result.err
	}
	return &singleValueRows{value: c.result.value}, nil
}

const voiceStoreDriverName = "chat_voice_store_test"

var registerVoiceStoreDriverOnce sync.Once

func registerVoiceStoreDriver() {
	registerVoiceStoreDriverOnce.Do(func() { sql.Register(voiceStoreDriverName, voiceStoreDriver{}) })
}

type voiceStoreDriver struct{}

func (voiceStoreDriver) Open(string) (driver.Conn, error) { return &voiceStoreConn{}, nil }

type voiceStoreConn struct{}

func (*voiceStoreConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (*voiceStoreConn) Close() error                        { return nil }
func (*voiceStoreConn) Begin() (driver.Tx, error)           { return voiceStoreTx{}, nil }
func (*voiceStoreConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return voiceStoreTx{}, nil
}
func (*voiceStoreConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "UPDATE app_chat_sessions") && len(args) == 1 && args[0].Value == int64(42) {
		return driver.RowsAffected(1), nil
	}
	return nil, errors.New("unexpected voice store exec")
}
func (*voiceStoreConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "message_type, audio_asset_id, audio_duration_ms, transcript"):
		if len(args) != 4 {
			return nil, errors.New("unexpected voice insert argument count")
		}
		validSpoken := args[2].Value == int64(3200) && args[3].Value == "孩子最近不愿意沟通"
		validSilent := args[2].Value == int64(2500) && args[3].Value == ""
		if args[0].Value != int64(42) || args[1].Value != int64(88) || (!validSpoken && !validSilent) {
			return nil, errors.New("unexpected voice insert arguments")
		}
		return &singleValueRows{value: int64(11)}, nil
	case strings.Contains(query, "'assistant'"):
		return &singleValueRows{value: int64(12)}, nil
	case strings.Contains(query, "JOIN app_chat_sessions") && strings.Contains(query, "audio_asset_id"):
		if len(args) != 2 || args[0].Value != int64(11) || args[1].Value != int64(7) {
			return nil, errors.New("ownership lookup arguments mismatch")
		}
		return &singleValueRows{value: int64(88)}, nil
	default:
		return nil, errors.New("unexpected voice store query")
	}
}

type voiceStoreTx struct{}

func (voiceStoreTx) Commit() error   { return nil }
func (voiceStoreTx) Rollback() error { return nil }

type singleValueRows struct {
	value driver.Value
	done  bool
}

func (*singleValueRows) Columns() []string { return []string{"id"} }
func (*singleValueRows) Close() error      { return nil }
func (r *singleValueRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	dest[0] = r.value
	r.done = true
	return nil
}
