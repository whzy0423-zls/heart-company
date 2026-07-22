package chat

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetOrCreateSceneSessionSerializesXinzhiliCreation(t *testing.T) {
	state := &sceneSessionTxState{}
	store := newSceneSessionTxStore(t, state)

	session, err := store.GetOrCreateSceneSession(context.Background(), 7, 9, "xinzhili_voice")
	if err != nil {
		t.Fatalf("GetOrCreateSceneSession: %v", err)
	}
	if session.ID != 42 {
		t.Fatalf("session id = %d, want 42", session.ID)
	}
	if state.beginCount != 1 || state.lockCount != 1 || state.commitCount != 1 {
		t.Fatalf("transaction counts = begin:%d lock:%d commit:%d, want 1/1/1", state.beginCount, state.lockCount, state.commitCount)
	}
	if state.isolation != driver.IsolationLevel(sql.LevelReadCommitted) {
		t.Fatalf("isolation = %d, want READ COMMITTED", state.isolation)
	}
	if state.lockKey != "app-chat-scene-session:7:9:xinzhili_voice" {
		t.Fatalf("lock key = %q, want complete scene key", state.lockKey)
	}
	if state.rollbackCount != 0 {
		t.Fatalf("rollback count = %d, want 0 after commit", state.rollbackCount)
	}
	if state.selectCount != 1 || state.insertCount != 1 {
		t.Fatalf("query counts = select:%d insert:%d, want 1/1", state.selectCount, state.insertCount)
	}
}

func TestGetOrCreateSceneSessionRollsBackWhenXinzhiliLockFails(t *testing.T) {
	lockErr := errors.New("lock unavailable")
	rollbackErr := errors.New("rollback failed")
	state := &sceneSessionTxState{lockErr: lockErr, rollbackErr: rollbackErr}
	store := newSceneSessionTxStore(t, state)

	_, err := store.GetOrCreateSceneSession(context.Background(), 7, 9, "xinzhili_voice")
	if !errors.Is(err, lockErr) {
		t.Fatalf("error = %v, want lock error", err)
	}
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("error = %v, want rollback error to be preserved", err)
	}
	if state.rollbackCount != 1 || state.commitCount != 0 {
		t.Fatalf("transaction counts = rollback:%d commit:%d, want 1/0", state.rollbackCount, state.commitCount)
	}
	if state.selectCount != 0 || state.insertCount != 0 {
		t.Fatalf("queries continued after lock failure: select:%d insert:%d", state.selectCount, state.insertCount)
	}
}

func TestGetOrCreateSceneSessionPropagatesTransactionDriverErrors(t *testing.T) {
	tests := []struct {
		name      string
		state     *sceneSessionTxState
		wantErr   error
		rollbacks int
	}{
		{name: "begin", state: &sceneSessionTxState{beginErr: errors.New("begin failed")}, wantErr: errors.New("begin failed")},
		{name: "select", state: &sceneSessionTxState{selectErr: errors.New("select failed")}, wantErr: errors.New("select failed"), rollbacks: 1},
		{name: "insert", state: &sceneSessionTxState{insertErr: errors.New("insert failed")}, wantErr: errors.New("insert failed"), rollbacks: 1},
		{name: "commit", state: &sceneSessionTxState{commitErr: errors.New("commit failed")}, wantErr: errors.New("commit failed")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newSceneSessionTxStore(t, tt.state)
			_, err := store.GetOrCreateSceneSession(context.Background(), 7, 9, "xinzhili_voice")
			if err == nil || !strings.Contains(err.Error(), tt.wantErr.Error()) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if tt.state.rollbackCount != tt.rollbacks {
				t.Fatalf("rollback count = %d, want %d", tt.state.rollbackCount, tt.rollbacks)
			}
		})
	}
}

func TestGetOrCreateSceneSessionHonorsCanceledContext(t *testing.T) {
	state := &sceneSessionTxState{}
	store := newSceneSessionTxStore(t, state)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.GetOrCreateSceneSession(ctx, 7, 9, "xinzhili_voice")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context canceled", err)
	}
	if state.beginCount != 0 {
		t.Fatalf("driver began %d transactions after cancellation", state.beginCount)
	}
}

func TestGetOrCreateSceneSessionDoesNotSerializeRegularChat(t *testing.T) {
	state := &sceneSessionTxState{existing: true}
	store := newSceneSessionTxStore(t, state)

	session, err := store.GetOrCreateSceneSession(context.Background(), 7, 9, "chat")
	if err != nil {
		t.Fatalf("GetOrCreateSceneSession: %v", err)
	}
	if session.ID != 41 {
		t.Fatalf("session id = %d, want existing chat session 41", session.ID)
	}
	if state.beginCount != 0 || state.lockCount != 0 || state.commitCount != 0 {
		t.Fatalf("regular chat unexpectedly used singleton transaction: begin:%d lock:%d commit:%d", state.beginCount, state.lockCount, state.commitCount)
	}
}

func TestGetOrCreateSceneSessionConcurrentPostgres(t *testing.T) {
	database := openChatBoundaryTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userID, cardID := createSceneSessionTestUserAndCard(t, ctx, database, "same")
	store := NewStore(database)

	const workers = 32
	start := make(chan struct{})
	results := make(chan int64, workers)
	errs := make(chan error, workers)
	var ready sync.WaitGroup
	ready.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			ready.Done()
			<-start
			session, err := store.GetOrCreateSceneSession(ctx, userID, cardID, "xinzhili_voice")
			if err != nil {
				errs <- err
				return
			}
			results <- session.ID
		}()
	}
	ready.Wait()
	close(start)

	var sessionID int64
	for i := 0; i < workers; i++ {
		select {
		case err := <-errs:
			t.Fatalf("concurrent GetOrCreateSceneSession: %v", err)
		case got := <-results:
			if sessionID == 0 {
				sessionID = got
			}
			if got != sessionID {
				t.Fatalf("session id = %d, want all calls to return %d", got, sessionID)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}

	var count int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM app_chat_sessions WHERE app_user_id=$1 AND card_id=$2 AND scene='xinzhili_voice'`,
		userID, cardID,
	).Scan(&count); err != nil {
		t.Fatalf("count xinzhili sessions: %v", err)
	}
	if count != 1 {
		t.Fatalf("xinzhili session count = %d, want 1", count)
	}
}

func TestGetOrCreateSceneSessionPostgresKeepsKeysAndChatRowsDistinct(t *testing.T) {
	database := openChatBoundaryTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	userA, cardA := createSceneSessionTestUserAndCard(t, ctx, database, "key-a")
	userB, cardB := createSceneSessionTestUserAndCard(t, ctx, database, "key-b")
	var cardA2 int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_user_cards (app_user_id, card_type, name) VALUES ($1,'other','key-a-2') RETURNING id`,
		userA,
	).Scan(&cardA2); err != nil {
		t.Fatalf("create second card: %v", err)
	}
	store := NewStore(database)

	a, err := store.GetOrCreateSceneSession(ctx, userA, cardA, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	differentCard, err := store.GetOrCreateSceneSession(ctx, userA, cardA2, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	b, err := store.GetOrCreateSceneSession(ctx, userB, cardB, "xinzhili_voice")
	if err != nil {
		t.Fatal(err)
	}
	otherScene, err := store.GetOrCreateSceneSession(ctx, userA, cardA, "another_hidden_scene")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == differentCard.ID || a.ID == b.ID || a.ID == otherScene.ID || differentCard.ID == b.ID || differentCard.ID == otherScene.ID || b.ID == otherScene.ID {
		t.Fatalf("distinct keys were merged: a=%d card=%d b=%d scene=%d", a.ID, differentCard.ID, b.ID, otherScene.ID)
	}

	var oldChatID, newChatID int64
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_chat_sessions (app_user_id, card_id, scene, updated_at) VALUES ($1,$2,'chat',now()-interval '1 minute') RETURNING id`,
		userA, cardA,
	).Scan(&oldChatID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_chat_sessions (app_user_id, card_id, scene, updated_at) VALUES ($1,$2,'chat',now()) RETURNING id`,
		userA, cardA,
	).Scan(&newChatID); err != nil {
		t.Fatal(err)
	}
	chat, err := store.GetOrCreateSceneSession(ctx, userA, cardA, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if chat.ID != newChatID {
		t.Fatalf("chat session id = %d, want newest existing row %d (older %d remains valid)", chat.ID, newChatID, oldChatID)
	}
	var chatCount int
	if err := database.QueryRowContext(ctx,
		`SELECT count(*) FROM app_chat_sessions WHERE app_user_id=$1 AND card_id=$2 AND scene='chat'`,
		userA, cardA,
	).Scan(&chatCount); err != nil {
		t.Fatal(err)
	}
	if chatCount != 2 {
		t.Fatalf("chat row count = %d, want existing multi-session rows preserved", chatCount)
	}
}

func createSceneSessionTestUserAndCard(t *testing.T, ctx context.Context, database *sql.DB, label string) (int64, int64) {
	t.Helper()
	var userID, cardID int64
	phone := fmt.Sprintf("scene-race-%s-%d", label, time.Now().UnixNano())
	if err := database.QueryRowContext(ctx, `INSERT INTO app_users (phone) VALUES ($1) RETURNING id`, phone).Scan(&userID); err != nil {
		t.Fatalf("create app user: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = database.ExecContext(cleanupCtx, `DELETE FROM app_users WHERE id=$1`, userID)
	})
	if err := database.QueryRowContext(ctx,
		`INSERT INTO app_user_cards (app_user_id, card_type, name) VALUES ($1,'other',$2) RETURNING id`,
		userID, label,
	).Scan(&cardID); err != nil {
		t.Fatalf("create app user card: %v", err)
	}
	return userID, cardID
}

const sceneSessionTxDriverName = "chat_scene_session_tx_test"

var (
	registerSceneSessionTxDriverOnce sync.Once
	sceneSessionTxStates             sync.Map
	sceneSessionTxStateSeq           atomic.Int64
)

type sceneSessionTxState struct {
	existing      bool
	beginErr      error
	lockErr       error
	selectErr     error
	insertErr     error
	commitErr     error
	rollbackErr   error
	beginCount    int
	isolation     driver.IsolationLevel
	lockKey       string
	lockCount     int
	selectCount   int
	insertCount   int
	commitCount   int
	rollbackCount int
}

func newSceneSessionTxStore(t *testing.T, state *sceneSessionTxState) *Store {
	t.Helper()
	registerSceneSessionTxDriverOnce.Do(func() { sql.Register(sceneSessionTxDriverName, sceneSessionTxDriver{}) })
	key := strconv.FormatInt(sceneSessionTxStateSeq.Add(1), 10)
	sceneSessionTxStates.Store(key, state)
	t.Cleanup(func() { sceneSessionTxStates.Delete(key) })
	database, err := sql.Open(sceneSessionTxDriverName, key)
	if err != nil {
		t.Fatal(err)
	}
	database.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = database.Close() })
	return NewStore(database)
}

type sceneSessionTxDriver struct{}

func (sceneSessionTxDriver) Open(name string) (driver.Conn, error) {
	value, _ := sceneSessionTxStates.Load(name)
	state, _ := value.(*sceneSessionTxState)
	return &sceneSessionTxConn{state: state}, nil
}

type sceneSessionTxConn struct{ state *sceneSessionTxState }

func (c *sceneSessionTxConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *sceneSessionTxConn) Close() error                        { return nil }
func (c *sceneSessionTxConn) Begin() (driver.Tx, error)           { return c.begin() }
func (c *sceneSessionTxConn) BeginTx(_ context.Context, opts driver.TxOptions) (driver.Tx, error) {
	c.state.isolation = opts.Isolation
	return c.begin()
}
func (c *sceneSessionTxConn) begin() (driver.Tx, error) {
	c.state.beginCount++
	if c.state.beginErr != nil {
		return nil, c.state.beginErr
	}
	return &sceneSessionTx{state: c.state}, nil
}
func (c *sceneSessionTxConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "pg_advisory_xact_lock") {
		c.state.lockCount++
		if len(args) != 1 {
			return nil, fmt.Errorf("lock args = %v, want one key", args)
		}
		c.state.lockKey, _ = args[0].Value.(string)
		if c.state.lockErr != nil {
			return nil, c.state.lockErr
		}
	}
	return driver.RowsAffected(1), nil
}
func (c *sceneSessionTxConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	now := time.Unix(100, 0)
	switch {
	case strings.Contains(query, "SELECT id, app_user_id"):
		c.state.selectCount++
		if c.state.selectErr != nil {
			return nil, c.state.selectErr
		}
		if !c.state.existing {
			return &sceneSessionTxRows{columns: sceneSessionTxColumns()}, nil
		}
		return &sceneSessionTxRows{columns: sceneSessionTxColumns(), values: [][]driver.Value{{int64(41), int64(7), int64(9), "", now, now}}}, nil
	case strings.Contains(query, "INSERT INTO app_chat_sessions"):
		c.state.insertCount++
		if c.state.insertErr != nil {
			return nil, c.state.insertErr
		}
		return &sceneSessionTxRows{columns: sceneSessionTxColumns(), values: [][]driver.Value{{int64(42), int64(7), int64(9), "", now, now}}}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type sceneSessionTx struct{ state *sceneSessionTxState }

func (tx *sceneSessionTx) Commit() error {
	tx.state.commitCount++
	return tx.state.commitErr
}
func (tx *sceneSessionTx) Rollback() error {
	tx.state.rollbackCount++
	return tx.state.rollbackErr
}

type sceneSessionTxRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func sceneSessionTxColumns() []string {
	return []string{"id", "app_user_id", "card_id", "title", "updated_at", "create_time"}
}
func (r *sceneSessionTxRows) Columns() []string { return r.columns }
func (r *sceneSessionTxRows) Close() error      { return nil }
func (r *sceneSessionTxRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}

var _ driver.Conn = (*sceneSessionTxConn)(nil)
var _ driver.ConnBeginTx = (*sceneSessionTxConn)(nil)
var _ driver.ExecerContext = (*sceneSessionTxConn)(nil)
var _ driver.QueryerContext = (*sceneSessionTxConn)(nil)
var _ driver.Tx = (*sceneSessionTx)(nil)
var _ driver.Rows = (*sceneSessionTxRows)(nil)
