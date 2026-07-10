package video

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
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	submissionKeyOne = "11111111-1111-4111-8111-111111111111"
	submissionKeyTwo = "22222222-2222-4222-8222-222222222222"
)

func TestSubmissionAllowsDeclaredTransitions(t *testing.T) {
	cases := []struct {
		from SubmissionStatus
		to   SubmissionStatus
	}{
		{SubmissionPrepared, SubmissionSubmitting},
		{SubmissionSubmitting, SubmissionAccepted},
		{SubmissionAccepted, SubmissionCompleted},
		{SubmissionAccepted, SubmissionFailed},
		{SubmissionSubmitting, SubmissionUnknownOutcome},
		{SubmissionUnknownOutcome, SubmissionReconciled},
		{SubmissionUnknownOutcome, SubmissionCancelled},
		{SubmissionPrepared, SubmissionCancelled},
		{SubmissionSubmitting, SubmissionFailed},
		{SubmissionSubmitting, SubmissionCancelled},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			if err := validateSubmissionTransition(tc.from, tc.to); err != nil {
				t.Fatalf("expected transition to be allowed: %v", err)
			}
		})
	}
}

func TestSubmissionRejectsInvalidTransitions(t *testing.T) {
	cases := []struct {
		from SubmissionStatus
		to   SubmissionStatus
	}{
		{SubmissionPrepared, SubmissionCompleted},
		{SubmissionSubmitting, SubmissionPrepared},
		{SubmissionAccepted, SubmissionSubmitting},
		{SubmissionCompleted, SubmissionSubmitting},
		{SubmissionCancelled, SubmissionPrepared},
	}

	for _, tc := range cases {
		t.Run(string(tc.from)+"_to_"+string(tc.to), func(t *testing.T) {
			err := validateSubmissionTransition(tc.from, tc.to)
			var transitionErr *SubmissionTransitionError
			if !errors.As(err, &transitionErr) {
				t.Fatalf("error = %T, want *SubmissionTransitionError: %v", err, err)
			}
			if transitionErr.Code != "submission_transition_invalid" {
				t.Fatalf("code = %q", transitionErr.Code)
			}
		})
	}
}

func TestSubmissionPrepareReusesRequestKeyAndLocksActiveShot(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	first, created, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9"))
	if err != nil {
		t.Fatal(err)
	}
	if !created || first.ID == "" || first.Status != SubmissionPrepared {
		t.Fatalf("unexpected first submission: created=%v submission=%+v", created, first)
	}

	replayed, created, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9"))
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("same request key must reuse the persisted submission")
	}
	if replayed.ID != first.ID {
		t.Fatalf("replayed id = %q, want %q", replayed.ID, first.ID)
	}

	_, _, err = store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyTwo, "9"))
	var activeErr *ActiveSubmissionError
	if !errors.As(err, &activeErr) {
		t.Fatalf("error = %T, want *ActiveSubmissionError: %v", err, err)
	}
	if activeErr.Code != "shot_submission_active" || activeErr.Existing.RequestKey != submissionKeyOne {
		t.Fatalf("unexpected active error: %+v", activeErr)
	}

	active, err := store.FindActiveByShot(ctx, "9")
	if err != nil {
		t.Fatal(err)
	}
	if active.RequestKey != submissionKeyOne {
		t.Fatalf("active request key = %q", active.RequestKey)
	}
}

func TestSubmissionRejectsRequestKeyReusedForDifferentIntent(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	input := testPrepareSubmissionInput(submissionKeyOne, "9")
	if _, _, err := store.Prepare(ctx, input); err != nil {
		t.Fatal(err)
	}
	input.RequestHash = "different-request-hash"
	_, _, err := store.Prepare(ctx, input)
	var conflict *RequestKeyConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T, want *RequestKeyConflictError: %v", err, err)
	}
	if conflict.Code != "request_key_payload_conflict" {
		t.Fatalf("code = %q", conflict.Code)
	}
}

func TestSubmissionTerminalStateReleasesShotForNewVersion(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	if _, _, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(ctx, submissionKeyOne, SubmissionPrepared, SubmissionSubmitting, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}
	accepted, err := store.Transition(ctx, submissionKeyOne, SubmissionSubmitting, SubmissionAccepted, SubmissionTransition{
		UpstreamTaskID: "task-42",
		GenerationID:   "42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.UpstreamTaskID != "task-42" || accepted.GenerationID != "42" {
		t.Fatalf("accepted linkage = %+v", accepted)
	}
	if _, err := store.Transition(ctx, submissionKeyOne, SubmissionAccepted, SubmissionCompleted, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.FindActiveByShot(ctx, "9"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected terminal submission to release active lock, got %v", err)
	}

	second, created, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyTwo, "9"))
	if err != nil {
		t.Fatal(err)
	}
	if !created || second.RequestKey != submissionKeyTwo {
		t.Fatalf("expected a new version after terminal state, got created=%v submission=%+v", created, second)
	}
}

func TestSubmissionTransitionUsesCompareAndSwap(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	if _, _, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(ctx, submissionKeyOne, SubmissionPrepared, SubmissionSubmitting, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}

	_, err := store.Transition(ctx, submissionKeyOne, SubmissionPrepared, SubmissionCancelled, SubmissionTransition{})
	var transitionErr *SubmissionTransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("error = %T, want *SubmissionTransitionError: %v", err, err)
	}
	if transitionErr.Code != "submission_state_conflict" || transitionErr.Current != SubmissionSubmitting {
		t.Fatalf("unexpected transition conflict: %+v", transitionErr)
	}
}

func TestSubmissionClaimSubmittingHasOneWinner(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	if _, _, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9")); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	type claimResult struct {
		claimed bool
		err     error
	}
	results := make(chan claimResult, 2)
	for range 2 {
		go func() {
			<-start
			_, claimed, err := store.ClaimSubmitting(ctx, submissionKeyOne)
			results <- claimResult{claimed: claimed, err: err}
		}()
	}
	close(start)

	winners := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.claimed {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners = %d, want 1", winners)
	}
	current, err := store.GetByRequestKey(ctx, submissionKeyOne)
	if err != nil {
		t.Fatal(err)
	}
	if current.Status != SubmissionSubmitting {
		t.Fatalf("status = %q, want submitting", current.Status)
	}
}

func TestSubmissionReconcileIsIdempotentAndRejectsDifferentTask(t *testing.T) {
	database := openSubmissionTestDB(t)
	store := NewSubmissionStore(database)
	ctx := context.Background()

	if _, _, err := store.Prepare(ctx, testPrepareSubmissionInput(submissionKeyOne, "9")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(ctx, submissionKeyOne, SubmissionPrepared, SubmissionSubmitting, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(ctx, submissionKeyOne, SubmissionSubmitting, SubmissionUnknownOutcome, SubmissionTransition{}); err != nil {
		t.Fatal(err)
	}

	first, err := store.Reconcile(ctx, submissionKeyOne, "task-42", "42")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Reconcile(ctx, submissionKeyOne, "task-42", "42")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Status != SubmissionReconciled {
		t.Fatalf("reconciliation was not idempotent: first=%+v second=%+v", first, second)
	}

	_, err = store.Reconcile(ctx, submissionKeyOne, "task-other", "43")
	var conflict *ReconciliationTaskConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %T, want *ReconciliationTaskConflictError: %v", err, err)
	}
	if conflict.Code != "reconciliation_task_conflict" {
		t.Fatalf("code = %q", conflict.Code)
	}
}

func testPrepareSubmissionInput(requestKey, shotID string) PrepareSubmissionInput {
	return PrepareSubmissionInput{
		RequestKey:        requestKey,
		ProjectID:         "3",
		ShotID:            shotID,
		RequestHash:       "request-hash",
		PromptHash:        "prompt-hash",
		CapabilityVersion: "capability-v1",
		RequestSnapshot:   []byte(`{"model":"video-ds-2.0","prompt":"safe snapshot"}`),
	}
}

func init() {
	sql.Register("video-submission-test", submissionTestDriver{})
}

type submissionTestState struct {
	mu                        sync.Mutex
	nextID                    int64
	byKey                     map[string]Submission
	recordTaskFailures        int
	acceptTransitionFailures  int
	unknownTransitionFailures int
	cancelTransitionFailures  int
}

type submissionTestDriver struct{}

type submissionTestConnector struct {
	state *submissionTestState
}

type submissionTestConn struct {
	state *submissionTestState
}

type submissionTestRows struct {
	columns []string
	values  []driver.Value
	read    bool
}

type submissionTestResult int64

var (
	submissionStatesMu sync.Mutex
	submissionStates   = map[string]*submissionTestState{}
)

func openSubmissionTestDB(t *testing.T) *sql.DB {
	t.Helper()
	name := strings.ReplaceAll(t.Name(), "/", "_")
	state := &submissionTestState{byKey: map[string]Submission{}}
	submissionStatesMu.Lock()
	submissionStates[name] = state
	submissionStatesMu.Unlock()
	t.Cleanup(func() {
		submissionStatesMu.Lock()
		delete(submissionStates, name)
		submissionStatesMu.Unlock()
	})
	database, err := sql.Open("video-submission-test", name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func (submissionTestDriver) Open(name string) (driver.Conn, error) {
	submissionStatesMu.Lock()
	defer submissionStatesMu.Unlock()
	state := submissionStates[name]
	if state == nil {
		return nil, errors.New("missing submission test state")
	}
	return &submissionTestConn{state: state}, nil
}

func (submissionTestDriver) OpenConnector(name string) (driver.Connector, error) {
	submissionStatesMu.Lock()
	defer submissionStatesMu.Unlock()
	state := submissionStates[name]
	if state == nil {
		return nil, errors.New("missing submission test state")
	}
	return submissionTestConnector{state: state}, nil
}

func (c submissionTestConnector) Connect(context.Context) (driver.Conn, error) {
	return &submissionTestConn{state: c.state}, nil
}

func (submissionTestConnector) Driver() driver.Driver {
	return submissionTestDriver{}
}

func (c *submissionTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported")
}

func (c *submissionTestConn) Close() error { return nil }

func (c *submissionTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c *submissionTestConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	q := strings.Join(strings.Fields(query), " ")
	c.state.mu.Lock()
	defer c.state.mu.Unlock()

	switch {
	case strings.HasPrefix(q, "INSERT INTO video_generation_submissions"):
		requestKey := submissionNamedString(args, 1)
		if _, exists := c.state.byKey[requestKey]; exists {
			return nil, sql.ErrNoRows
		}
		shotID := submissionNamedString(args, 3)
		for _, item := range c.state.byKey {
			if shotID != "" && item.ShotID == shotID && isActiveSubmissionStatus(item.Status) {
				return nil, &pgconn.PgError{
					Code:           "23505",
					ConstraintName: activeShotSubmissionConstraint,
				}
			}
		}
		c.state.nextID++
		item := Submission{
			ID:                strconv.FormatInt(c.state.nextID, 10),
			RequestKey:        requestKey,
			ProjectID:         submissionNamedString(args, 2),
			ShotID:            shotID,
			RequestHash:       submissionNamedString(args, 4),
			PromptHash:        submissionNamedString(args, 5),
			CapabilityVersion: submissionNamedString(args, 6),
			RequestSnapshot:   append([]byte(nil), submissionNamedBytes(args, 7)...),
			Status:            SubmissionPrepared,
		}
		c.state.byKey[requestKey] = item
		return submissionRow(item), nil
	case strings.Contains(q, "FROM video_generation_submissions WHERE request_key=$1"):
		item, ok := c.state.byKey[submissionNamedString(args, 1)]
		if !ok {
			return nil, sql.ErrNoRows
		}
		return submissionRow(item), nil
	case strings.Contains(q, "FROM video_generation_submissions WHERE generation_id=$1"):
		generationID := submissionNamedString(args, 1)
		for _, item := range c.state.byKey {
			if item.GenerationID == generationID {
				return submissionRow(item), nil
			}
		}
		return nil, sql.ErrNoRows
	case strings.Contains(q, "FROM video_generation_submissions WHERE shot_id=$1"):
		shotID := submissionNamedString(args, 1)
		for _, item := range c.state.byKey {
			if item.ShotID == shotID && isActiveSubmissionStatus(item.Status) {
				return submissionRow(item), nil
			}
		}
		return nil, sql.ErrNoRows
	case strings.HasPrefix(q, "UPDATE video_generation_submissions SET status=$3"):
		if submissionNamedString(args, 3) == string(SubmissionAccepted) && c.state.acceptTransitionFailures > 0 {
			c.state.acceptTransitionFailures--
			return nil, errors.New("injected accepted linkage failure")
		}
		if submissionNamedString(args, 3) == string(SubmissionUnknownOutcome) && c.state.unknownTransitionFailures > 0 {
			c.state.unknownTransitionFailures--
			return nil, errors.New("injected unknown outcome persistence failure")
		}
		if submissionNamedString(args, 3) == string(SubmissionCancelled) && c.state.cancelTransitionFailures > 0 {
			c.state.cancelTransitionFailures--
			return nil, errors.New("injected cancelled state persistence failure")
		}
		requestKey := submissionNamedString(args, 1)
		item, ok := c.state.byKey[requestKey]
		if !ok || item.Status != SubmissionStatus(submissionNamedString(args, 2)) {
			return nil, sql.ErrNoRows
		}
		item.Status = SubmissionStatus(submissionNamedString(args, 3))
		if taskID := submissionNamedString(args, 4); taskID != "" {
			item.UpstreamTaskID = taskID
		}
		if generationID := submissionNamedString(args, 5); generationID != "" {
			item.GenerationID = generationID
		}
		item.ErrorMessage = submissionNamedString(args, 6)
		c.state.byKey[requestKey] = item
		return submissionRow(item), nil
	case strings.HasPrefix(q, "UPDATE video_generation_submissions SET upstream_task_id=$2"):
		if c.state.recordTaskFailures > 0 {
			c.state.recordTaskFailures--
			return nil, errors.New("injected upstream task linkage failure")
		}
		requestKey := submissionNamedString(args, 1)
		item, ok := c.state.byKey[requestKey]
		taskID := submissionNamedString(args, 2)
		if !ok || item.Status != SubmissionSubmitting || (item.UpstreamTaskID != "" && item.UpstreamTaskID != taskID) {
			return nil, sql.ErrNoRows
		}
		item.UpstreamTaskID = taskID
		c.state.byKey[requestKey] = item
		return submissionRow(item), nil
	default:
		return nil, errors.New("unexpected submission query: " + q)
	}
}

func (c *submissionTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return submissionTestResult(0), nil
}

func (r *submissionTestRows) Columns() []string { return r.columns }

func (r *submissionTestRows) Close() error { return nil }

func (r *submissionTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	copy(dest, r.values)
	r.read = true
	return nil
}

func (submissionTestResult) LastInsertId() (int64, error) { return 0, nil }

func (r submissionTestResult) RowsAffected() (int64, error) { return int64(r), nil }

func submissionRow(item Submission) driver.Rows {
	now := time.Now()
	return &submissionTestRows{
		columns: []string{
			"id", "request_key", "project_id", "shot_id", "request_hash", "prompt_hash",
			"capability_version", "request_snapshot", "status", "upstream_task_id",
			"generation_id", "error_message", "create_time", "update_time",
		},
		values: []driver.Value{
			item.ID, item.RequestKey, item.ProjectID, item.ShotID, item.RequestHash, item.PromptHash,
			item.CapabilityVersion, []byte(item.RequestSnapshot), string(item.Status), item.UpstreamTaskID,
			item.GenerationID, item.ErrorMessage, now, now,
		},
	}
}

func submissionNamedString(args []driver.NamedValue, ordinal int) string {
	for _, arg := range args {
		if arg.Ordinal != ordinal || arg.Value == nil {
			continue
		}
		switch value := arg.Value.(type) {
		case string:
			return value
		case []byte:
			return string(value)
		case int64:
			return strconv.FormatInt(value, 10)
		}
	}
	return ""
}

func submissionNamedBytes(args []driver.NamedValue, ordinal int) []byte {
	for _, arg := range args {
		if arg.Ordinal != ordinal || arg.Value == nil {
			continue
		}
		switch value := arg.Value.(type) {
		case []byte:
			return value
		case string:
			return []byte(value)
		}
	}
	return nil
}
