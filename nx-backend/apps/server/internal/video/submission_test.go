package video

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestSubmissionTransitions(t *testing.T) {
	statuses := []SubmissionStatus{
		SubmissionPrepared,
		SubmissionSubmitting,
		SubmissionAccepted,
		SubmissionUnknownOutcome,
		SubmissionReconciled,
		SubmissionCompleted,
		SubmissionFailed,
		SubmissionCancelled,
	}
	allowed := map[SubmissionStatus]map[SubmissionStatus]bool{
		SubmissionPrepared:       {SubmissionSubmitting: true, SubmissionCancelled: true},
		SubmissionSubmitting:     {SubmissionAccepted: true, SubmissionUnknownOutcome: true},
		SubmissionUnknownOutcome: {SubmissionReconciled: true},
		SubmissionAccepted:       {SubmissionCompleted: true, SubmissionFailed: true},
		SubmissionReconciled:     {SubmissionCompleted: true, SubmissionFailed: true},
	}

	for _, from := range statuses {
		for _, to := range statuses {
			from, to := from, to
			t.Run(string(from)+"_to_"+string(to), func(t *testing.T) {
				got := canTransitionSubmission(from, to)
				if got != allowed[from][to] {
					t.Fatalf("transition %s -> %s allowed=%v, want %v", from, to, got, allowed[from][to])
				}
			})
		}
	}
}

func TestSubmissionSameRequestKeyIsIdempotent(t *testing.T) {
	repo := newMemorySubmissionRepository()
	store := newSubmissionStore(repo)
	input := PrepareSubmissionInput{
		RequestKey:      "11111111-1111-4111-8111-111111111111",
		ShotID:          "42",
		RequestSnapshot: []byte(`{"prompt":"first"}`),
	}

	first, err := store.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Prepare(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.RequestKey != second.RequestKey {
		t.Fatalf("same request key created a second identity: first=%+v second=%+v", first, second)
	}
	if repo.insertCalls != 2 || len(repo.rows) != 1 {
		t.Fatalf("expected one persisted row after duplicate insert, calls=%d rows=%d", repo.insertCalls, len(repo.rows))
	}
}

func TestSubmissionConcurrentNewKeyRejectedUntilTerminal(t *testing.T) {
	repo := newMemorySubmissionRepository()
	store := newSubmissionStore(repo)
	firstInput := PrepareSubmissionInput{RequestKey: "11111111-1111-4111-8111-111111111111", ShotID: "42"}
	if _, err := store.Prepare(context.Background(), firstInput); err != nil {
		t.Fatal(err)
	}

	const attempts = 8
	var wg sync.WaitGroup
	errCh := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Prepare(context.Background(), PrepareSubmissionInput{
				RequestKey: "22222222-2222-4222-8222-222222222222",
				ShotID:     "42",
			})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		var active *ActiveSubmissionError
		if !errors.As(err, &active) {
			t.Fatalf("expected active submission error, got %v", err)
		}
	}

	if _, err := store.Transition(context.Background(), firstInput.RequestKey, SubmissionPrepared, SubmissionCancelled); err != nil {
		t.Fatal(err)
	}
	created, err := store.Prepare(context.Background(), PrepareSubmissionInput{
		RequestKey: "33333333-3333-4333-8333-333333333333",
		ShotID:     "42",
	})
	if err != nil {
		t.Fatalf("terminal transition should release the active lock: %v", err)
	}
	if created.RequestKey != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("unexpected new submission: %+v", created)
	}
}

func TestSubmissionReconcileIsIdempotentAndRejectsConflictingTask(t *testing.T) {
	repo := newMemorySubmissionRepository()
	store := newSubmissionStore(repo)
	requestKey := "11111111-1111-4111-8111-111111111111"
	if _, err := store.Prepare(context.Background(), PrepareSubmissionInput{RequestKey: requestKey, ShotID: "42"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), requestKey, SubmissionPrepared, SubmissionSubmitting); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Transition(context.Background(), requestKey, SubmissionSubmitting, SubmissionUnknownOutcome); err != nil {
		t.Fatal(err)
	}

	first, err := store.Reconcile(context.Background(), requestKey, "task-7", "77")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Reconcile(context.Background(), requestKey, "task-7", "77")
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || second.Status != SubmissionReconciled || second.GenerationID != "77" {
		t.Fatalf("unexpected idempotent reconcile result: first=%+v second=%+v", first, second)
	}

	_, err = store.Reconcile(context.Background(), requestKey, "task-other", "88")
	var conflict *ReconciliationTaskConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected task conflict, got %v", err)
	}
}

func TestSubmissionStoreRestartRecoversActiveRequest(t *testing.T) {
	repo := newMemorySubmissionRepository()
	requestKey := "11111111-1111-4111-8111-111111111111"
	firstStore := newSubmissionStore(repo)
	if _, err := firstStore.Prepare(context.Background(), PrepareSubmissionInput{RequestKey: requestKey, ShotID: "42"}); err != nil {
		t.Fatal(err)
	}
	if _, err := firstStore.Transition(context.Background(), requestKey, SubmissionPrepared, SubmissionSubmitting); err != nil {
		t.Fatal(err)
	}

	restarted := newSubmissionStore(repo)
	active, err := restarted.FindActiveByShot(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if active.RequestKey != requestKey || active.Status != SubmissionSubmitting {
		t.Fatalf("restart lost active submission: %+v", active)
	}
	_, err = restarted.Prepare(context.Background(), PrepareSubmissionInput{
		RequestKey: "22222222-2222-4222-8222-222222222222",
		ShotID:     "42",
	})
	var activeErr *ActiveSubmissionError
	if !errors.As(err, &activeErr) {
		t.Fatalf("restarted store should reject a new key, got %v", err)
	}
	if _, err := restarted.Transition(context.Background(), requestKey, SubmissionSubmitting, SubmissionUnknownOutcome); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Reconcile(context.Background(), requestKey, "task-7", "77"); err != nil {
		t.Fatal(err)
	}
}

type memorySubmissionRepository struct {
	mu          sync.Mutex
	rows        map[string]Submission
	nextID      int64
	insertCalls int
}

func newMemorySubmissionRepository() *memorySubmissionRepository {
	return &memorySubmissionRepository{rows: map[string]Submission{}, nextID: 1}
}

func (r *memorySubmissionRepository) Insert(_ context.Context, input PrepareSubmissionInput) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.insertCalls++
	if _, ok := r.rows[input.RequestKey]; ok {
		return Submission{}, errSubmissionRequestKeyConstraint
	}
	for _, row := range r.rows {
		if row.ShotID == input.ShotID && row.Status.Active() {
			return Submission{}, errSubmissionActiveShotConstraint
		}
	}
	row := Submission{
		ID:              r.nextID,
		RequestKey:      input.RequestKey,
		ShotID:          input.ShotID,
		Status:          SubmissionPrepared,
		RequestSnapshot: append([]byte(nil), input.RequestSnapshot...),
	}
	r.nextID++
	r.rows[input.RequestKey] = row
	return row, nil
}

func (r *memorySubmissionRepository) GetByRequestKey(_ context.Context, requestKey string) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[requestKey]
	if !ok {
		return Submission{}, ErrSubmissionNotFound
	}
	return row, nil
}

func (r *memorySubmissionRepository) GetByID(_ context.Context, id string) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if fmt.Sprint(row.ID) == id {
			return row, nil
		}
	}
	return Submission{}, ErrSubmissionNotFound
}

func (r *memorySubmissionRepository) GetByGenerationID(_ context.Context, generationID string) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.GenerationID == generationID {
			return row, nil
		}
	}
	return Submission{}, ErrSubmissionNotFound
}

func (r *memorySubmissionRepository) FindActiveByShot(_ context.Context, shotID string) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, row := range r.rows {
		if row.ShotID == shotID && row.Status.Active() {
			return row, nil
		}
	}
	return Submission{}, ErrSubmissionNotFound
}

func (r *memorySubmissionRepository) CompareAndSwap(
	_ context.Context,
	requestKey string,
	from SubmissionStatus,
	to SubmissionStatus,
	patch SubmissionPatch,
) (Submission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	row, ok := r.rows[requestKey]
	if !ok {
		return Submission{}, ErrSubmissionNotFound
	}
	if row.Status != from {
		return Submission{}, errSubmissionCompareAndSwap
	}
	row.Status = to
	if patch.TaskID != nil {
		row.TaskID = *patch.TaskID
	}
	if patch.GenerationID != nil {
		row.GenerationID = *patch.GenerationID
	}
	if patch.ErrorMessage != nil {
		row.ErrorMessage = *patch.ErrorMessage
	}
	r.rows[requestKey] = row
	return row, nil
}
