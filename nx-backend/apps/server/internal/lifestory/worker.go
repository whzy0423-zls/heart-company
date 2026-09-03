package lifestory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

type CompletionEvent struct {
	Story   Story
	Job     Job
	Version Version
	Quota   QuotaSnapshot
}

type WorkerConfig struct {
	Store             *Store
	Generator         *Generator
	Quota             *QuotaStore
	Lease             time.Duration
	GenerationTimeout time.Duration
	PollInterval      time.Duration
	MaxAttempts       int
	Concurrency       int
	WorkerIDPrefix    string
	QuotaPeriod       string
	OnCompleted       func(context.Context, CompletionEvent) error
}

type Worker struct {
	store             *Store
	generator         *Generator
	quota             *QuotaStore
	lease             time.Duration
	generationTimeout time.Duration
	pollInterval      time.Duration
	maxAttempts       int
	concurrency       int
	workerIDPrefix    string
	quotaPeriod       string
	onCompleted       func(context.Context, CompletionEvent) error
	stop              context.CancelFunc
	wg                sync.WaitGroup
}

func NewWorker(config WorkerConfig) *Worker {
	lease := config.Lease
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	timeout := config.GenerationTimeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	poll := config.PollInterval
	if poll <= 0 {
		poll = 5 * time.Second
	}
	maxAttempts := config.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 3
	}
	concurrency := config.Concurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	if concurrency > 2 {
		concurrency = 2
	}
	workerPrefix := strings.TrimSpace(config.WorkerIDPrefix)
	if workerPrefix == "" {
		workerPrefix = "life-story-worker"
	}
	quota := config.Quota
	if quota == nil && config.Store != nil {
		quota = config.Store.QuotaStore()
	}
	return &Worker{store: config.Store, generator: config.Generator, quota: quota, lease: lease,
		generationTimeout: timeout, pollInterval: poll, maxAttempts: maxAttempts,
		concurrency: concurrency, workerIDPrefix: workerPrefix,
		quotaPeriod: quotaPeriod(config.QuotaPeriod), onCompleted: config.OnCompleted}
}

// Start launches one durable polling loop. Calling Start more than once is a
// no-op while the previous loop is alive.
func (w *Worker) Start(ctx context.Context) {
	if w == nil || w.store == nil || w.store.db == nil || w.generator == nil {
		return
	}
	if w.stop != nil {
		return
	}
	loopCtx, cancel := context.WithCancel(ctx)
	w.stop = cancel
	w.wg.Add(w.concurrency)
	for i := 0; i < w.concurrency; i++ {
		workerID := fmt.Sprintf("%s-%d", w.workerIDPrefix, i+1)
		go func(id string) {
			defer w.wg.Done()
			w.runLoop(loopCtx, id)
		}(workerID)
	}
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	if w.stop != nil {
		w.stop()
		w.wg.Wait()
		w.stop = nil
	}
}

func (w *Worker) runLoop(ctx context.Context, workerID string) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if _, err := w.runOnceWithWorker(ctx, workerID); err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, context.Canceled) {
			log.Printf("life story worker failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// RunOnce claims and processes at most one job. It returns true when a job
// was claimed, false when no claimable row exists.
func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	return w.runOnceWithWorker(ctx, "")
}

func (w *Worker) runOnceWithWorker(ctx context.Context, workerID string) (bool, error) {
	if w == nil || w.store == nil {
		return false, ErrNilDB
	}
	job, err := w.store.ClaimNextJobWithWorker(ctx, w.lease, workerID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := w.processClaimedSafe(ctx, job); err != nil {
		return true, err
	}
	return true, nil
}

func (w *Worker) processClaimedSafe(ctx context.Context, job Job) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			retry := job.Attempt < minInt(job.MaxAttempts, w.maxAttempts)
			_, _ = w.store.FailJob(ctx, job.ID, job.ClaimToken, "worker_panic", "故事生成服务异常，请稍后重试", retry)
			err = errors.New("life story worker panic")
		}
	}()
	return w.processClaimed(ctx, job)
}

func (w *Worker) processClaimed(ctx context.Context, job Job) error {
	if w.generator == nil {
		_, _ = w.store.FailJob(ctx, job.ID, job.ClaimToken, "configuration", "故事生成服务暂不可用", false)
		return errors.New("life story generator is not configured")
	}
	if w.quota == nil {
		w.quota = w.store.QuotaStore()
	}
	if _, err := w.quota.Reserve(ctx, job.AppUserID, job.ID, w.quotaPeriod); err != nil {
		if errors.Is(err, ErrQuotaExhausted) {
			_, _ = w.store.FailJob(ctx, job.ID, job.ClaimToken, "quota_exhausted", "故事生成次数已用完", false)
		}
		return err
	}
	var snapshot StorySnapshot
	if err := w.store.loadJobSnapshot(ctx, job, &snapshot); err != nil {
		_, _ = w.store.FailJob(ctx, job.ID, job.ClaimToken, "invalid_input", "故事资料已失效，请重新确认事实卡和提纲", false)
		return err
	}
	tokenMap, err := w.store.loadJobTokenMap(ctx, job)
	if err != nil {
		_, _ = w.store.FailJob(ctx, job.ID, job.ClaimToken, "invalid_input", "故事隐私映射已失效，请重新发起生成", false)
		return err
	}
	version, err := w.generateWithLease(ctx, job, snapshot, tokenMap)
	if err != nil {
		if errors.Is(err, ErrSafetyBlocked) {
			if _, blockErr := w.store.BlockJob(ctx, job.ID, job.ClaimToken, "safety", "这段内容需要调整后才能生成故事"); blockErr != nil {
				return blockErr
			}
			return err
		}
		retry := job.Attempt < minInt(job.MaxAttempts, w.maxAttempts)
		_, failErr := w.store.FailJob(ctx, job.ID, job.ClaimToken, "generation_failed", publicGenerationError(err), retry)
		if failErr != nil {
			return failErr
		}
		return err
	}
	version.StoryID = job.StoryID
	version.Number = 0 // FinalizeJob allocates the next number under lock.
	event, err := w.store.FinalizeJob(ctx, job, version, w.quotaPeriod)
	if err != nil {
		if errors.Is(err, ErrInactiveUser) {
			return err
		}
		retry := job.Attempt < minInt(job.MaxAttempts, w.maxAttempts)
		category := "publish_failed"
		message := "故事发布失败，请稍后重试"
		if errors.Is(err, ErrPayloadConflict) {
			retry = false
			category = "invalid_input"
			message = "故事资料校验失败，请重新确认后再生成"
		}
		if _, failErr := w.store.FailJob(ctx, job.ID, job.ClaimToken, category, message, retry); failErr != nil && !errors.Is(failErr, ErrConflict) {
			return failErr
		}
		return err
	}
	if w.onCompleted != nil {
		if notifyErr := w.onCompleted(ctx, event); notifyErr != nil {
			// Story and quota are already durable. The outbox remains pending and
			// can be replayed by a later dispatcher.
			log.Printf("life story completion notification failed")
		}
	}
	return nil
}

func (w *Worker) generateWithLease(ctx context.Context, job Job, snapshot StorySnapshot, tokenMap TokenMap) (Version, error) {
	genCtx, cancel := context.WithTimeout(ctx, w.generationTimeout)
	defer cancel()
	releaseGuard, err := w.store.acquireGenerationGuard(genCtx, job)
	if err != nil {
		return Version{}, err
	}
	defer releaseGuard()

	heartbeatStop := make(chan struct{})
	heartbeatDone := make(chan error, 1)
	interval := w.lease / 3
	if interval < 10*time.Millisecond {
		interval = 10 * time.Millisecond
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatStop:
				heartbeatDone <- nil
				return
			case <-genCtx.Done():
				heartbeatDone <- nil
				return
			case <-ticker.C:
				renewed, renewErr := w.store.RenewJobLease(genCtx, job.ID, job.ClaimToken, w.lease)
				if renewErr != nil {
					select {
					case <-heartbeatStop:
						heartbeatDone <- nil
						return
					default:
					}
					heartbeatDone <- renewErr
					cancel()
					return
				}
				if !renewed {
					heartbeatDone <- ErrConflict
					cancel()
					return
				}
			}
		}
	}()
	version, generateErr := w.generator.GenerateTokenized(genCtx, snapshot, tokenMap)
	close(heartbeatStop)
	cancel()
	heartbeatErr := <-heartbeatDone
	if heartbeatErr != nil {
		return Version{}, heartbeatErr
	}
	return version, generateErr
}

func publicGenerationError(err error) string {
	if err == nil {
		return "故事生成失败，请稍后重试"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "故事生成超时，请稍后重试"
	}
	return "故事生成失败，请稍后重试"
}

func minInt(a, b int) int {
	if a <= 0 {
		return b
	}
	if b <= 0 || a < b {
		return a
	}
	return b
}

func (s *Store) loadJobSnapshot(ctx context.Context, job Job, target *StorySnapshot) error {
	if target == nil {
		return errors.New("nil story snapshot target")
	}
	if s == nil || s.db == nil {
		return ErrNilDB
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT input_snapshot FROM app_life_story_jobs WHERE id=$1 AND app_user_id=$2 AND story_id=$3`, job.ID, job.AppUserID, job.StoryID).Scan(&raw)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return err
	}
	target.Outline.StoryStyle, err = NormalizeStoryStyle(target.Outline.StoryStyle)
	if err != nil {
		return err
	}
	return ValidateSnapshot(*target)
}

// FinalizeJob atomically verifies the active user and claim token, publishes
// a new version, commits the quota reservation, and creates an outbox event.
func (s *Store) FinalizeJob(ctx context.Context, job Job, version Version, period string) (CompletionEvent, error) {
	if s == nil || s.db == nil {
		return CompletionEvent{}, ErrNilDB
	}
	if job.ID <= 0 || job.StoryID <= 0 || job.AppUserID <= 0 || strings.TrimSpace(job.ClaimToken) == "" {
		return CompletionEvent{}, ErrInvalidState
	}
	var err error
	version.StoryStyle, err = NormalizeStoryStyle(version.StoryStyle)
	if err != nil {
		return CompletionEvent{}, err
	}
	if err := ValidateVersion(version); err != nil {
		return CompletionEvent{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return CompletionEvent{}, err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR UPDATE`, job.AppUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return CompletionEvent{}, ErrInactiveUser
	} else if err != nil {
		return CompletionEvent{}, err
	}
	if strings.TrimSpace(userStatus) != "active" {
		// Keep the cancellation durable while the user lock is held. Account
		// deletion and worker finalization therefore cannot race to a result.
		_, _ = tx.ExecContext(ctx, `UPDATE app_life_story_jobs SET status='cancelled',claim_token='',lease_until=NULL,finished_at=now(),updated_at=now() WHERE id=$1 AND status='running' AND claim_token=$2`, job.ID, job.ClaimToken)
		if _, _, releaseErr := releaseQuotaTx(ctx, tx, job.AppUserID, job.ID); releaseErr != nil {
			return CompletionEvent{}, releaseErr
		}
		if deleteErr := deleteJobTokenMapTx(ctx, tx, job.ID); deleteErr != nil {
			return CompletionEvent{}, deleteErr
		}
		if _, updateErr := tx.ExecContext(ctx, `UPDATE app_life_stories SET status=CASE WHEN current_version_id IS NULL THEN 'cancelled' ELSE 'completed' END,stage=CASE WHEN current_version_id IS NULL THEN 'cancelled' ELSE 'reading' END,updated_at=now() WHERE id=$1 AND app_user_id=$2 AND status IN ('queued','generating')`, job.StoryID, job.AppUserID); updateErr != nil {
			return CompletionEvent{}, updateErr
		}
		if err := tx.Commit(); err != nil {
			return CompletionEvent{}, err
		}
		return CompletionEvent{}, ErrInactiveUser
	}
	var lockedJob Job
	if lockedJob, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE id=$1 AND story_id=$2 AND app_user_id=$3 FOR UPDATE`, job.ID, job.StoryID, job.AppUserID)); errors.Is(err, sql.ErrNoRows) {
		return CompletionEvent{}, ErrNotFound
	} else if err != nil {
		return CompletionEvent{}, err
	}
	if lockedJob.Status != JobRunning || lockedJob.ClaimToken != job.ClaimToken {
		if lockedJob.Status == JobSucceeded && lockedJob.VersionID > 0 {
			published, getErr := getVersionByID(ctx, tx, job.AppUserID, lockedJob.VersionID)
			if getErr != nil {
				return CompletionEvent{}, getErr
			}
			var committedPeriod int64
			quota := QuotaSnapshot{}
			if scanErr := tx.QueryRowContext(ctx, `SELECT period_id FROM app_story_quota_ledger WHERE app_user_id=$1 AND job_id=$2 AND entry_type='commit'`, job.AppUserID, job.ID).Scan(&committedPeriod); scanErr == nil {
				quota, _ = quotaSnapshotByID(ctx, tx, job.AppUserID, committedPeriod)
			}
			if err := tx.Commit(); err != nil {
				return CompletionEvent{}, err
			}
			story, storyErr := s.Get(ctx, job.AppUserID, job.StoryID)
			if storyErr != nil {
				return CompletionEvent{}, storyErr
			}
			return CompletionEvent{Story: story, Job: lockedJob, Version: published, Quota: quota}, nil
		}
		return CompletionEvent{}, ErrConflict
	}
	if job.PayloadHash != "" && job.PayloadHash != lockedJob.PayloadHash {
		return CompletionEvent{}, ErrConflict
	}
	if job.SnapshotHash != "" && job.SnapshotHash != lockedJob.SnapshotHash {
		return CompletionEvent{}, ErrConflict
	}
	expectedSnapshotHash := lockedJob.SnapshotHash
	if expectedSnapshotHash == "" {
		// Compatibility for rows created before snapshot_hash was introduced.
		expectedSnapshotHash = lockedJob.PayloadHash
	}
	if expectedSnapshotHash == "" || snapshotPayloadHash(lockedJob.InputSnapshot) != expectedSnapshotHash {
		return CompletionEvent{}, ErrPayloadConflict
	}
	var nextNumber int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version_no),0)+1 FROM app_life_story_versions WHERE story_id=$1 AND app_user_id=$2`, job.StoryID, job.AppUserID).Scan(&nextNumber); err != nil {
		return CompletionEvent{}, err
	}
	version.Number = nextNumber
	version.Status = VersionPublished
	chapters, _ := json.Marshal(version.Chapters)
	config := version.GenerationConfig
	if len(config) == 0 {
		config = []byte(`{}`)
	}
	var versionID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO app_life_story_versions
	 (story_id,app_user_id,version_no,status,perspective,tone,story_style,chapters,reflection,character_count,word_count,model,generation_config)
	 VALUES($1,$2,$3,'published',$4,$5,$6,$7::jsonb,$8,$9,$10,$11,$12::jsonb) RETURNING id`,
		job.StoryID, job.AppUserID, version.Number, version.Perspective, version.Tone, version.StoryStyle,
		string(chapters), version.Reflection, version.CharacterCount, version.WordCount, version.Model, string(config)).Scan(&versionID); err != nil {
		return CompletionEvent{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_life_story_versions SET status='superseded' WHERE story_id=$1 AND app_user_id=$2 AND id<>$3 AND status='published'`, job.StoryID, job.AppUserID, versionID); err != nil {
		return CompletionEvent{}, err
	}
	storyUpdate, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status='completed',stage='reading',current_version_id=$1,revision=revision+1,updated_at=now() WHERE id=$2 AND app_user_id=$3 AND status IN ('generating','queued')`, versionID, job.StoryID, job.AppUserID)
	if err != nil {
		return CompletionEvent{}, err
	}
	if affected, _ := storyUpdate.RowsAffected(); affected != 1 {
		return CompletionEvent{}, ErrConflict
	}
	// Commit the reservation under the same user/job lock. The helper uses the
	// reservation's original period rather than the current month.
	quota, err := commitQuotaTx(ctx, tx, job.AppUserID, job.ID, versionID)
	if err != nil {
		return CompletionEvent{}, err
	}
	var completedJob Job
	completedJob, err = scanJob(tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET status='succeeded',progress=100,version_id=$1,claim_token='',lease_until=NULL,finished_at=now(),updated_at=now(),error_category='',error_message='' WHERE id=$2 AND status='running' AND claim_token=$3 RETURNING `+jobColumns, versionID, job.ID, job.ClaimToken))
	if errors.Is(err, sql.ErrNoRows) {
		return CompletionEvent{}, ErrConflict
	}
	if err != nil {
		return CompletionEvent{}, err
	}
	payload, _ := json.Marshal(map[string]any{"storyId": job.StoryID, "versionId": versionID, "jobId": job.ID})
	sourceKey := fmt.Sprintf("life-story:%d:%d", job.StoryID, versionID)
	if _, err := tx.ExecContext(ctx, `INSERT INTO app_life_story_outbox(app_user_id,story_id,job_id,version_id,event_type,source_key,payload) VALUES($1,$2,$3,$4,'completed',$5,$6::jsonb) ON CONFLICT(job_id,event_type) DO UPDATE SET source_key=EXCLUDED.source_key,payload=EXCLUDED.payload`, job.AppUserID, job.StoryID, job.ID, versionID, sourceKey, string(payload)); err != nil {
		return CompletionEvent{}, err
	}
	if err := deleteJobTokenMapTx(ctx, tx, job.ID); err != nil {
		return CompletionEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return CompletionEvent{}, err
	}
	version.ID, version.StoryID = versionID, job.StoryID
	story, storyErr := s.Get(ctx, job.AppUserID, job.StoryID)
	if storyErr != nil {
		return CompletionEvent{}, storyErr
	}
	return CompletionEvent{Story: story, Job: completedJob, Version: version, Quota: quota}, nil
}

func (s *Store) BlockJob(ctx context.Context, jobID int64, token, category, message string) (Job, error) {
	if s == nil || s.db == nil {
		return Job{}, ErrNilDB
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback()
	if _, err := discoverAndLockJobUserShared(ctx, tx, jobID); err != nil {
		return Job{}, err
	}
	var job Job
	job, err = scanJob(tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM app_life_story_jobs WHERE id=$1 FOR UPDATE`, jobID))
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	if err != nil {
		return Job{}, err
	}
	if job.Status != JobRunning || strings.TrimSpace(token) == "" || job.ClaimToken != token {
		return Job{}, ErrConflict
	}
	job, err = scanJob(tx.QueryRowContext(ctx, `UPDATE app_life_story_jobs SET status='safety_blocked',error_category=$1,error_message=$2,claim_token='',lease_until=NULL,finished_at=now(),updated_at=now() WHERE id=$3 AND status='running' AND claim_token=$4 RETURNING `+jobColumns, strings.TrimSpace(category), strings.TrimSpace(message), jobID, token))
	if err != nil {
		return Job{}, err
	}
	storyUpdate, err := tx.ExecContext(ctx, `UPDATE app_life_stories SET status='safety_blocked',stage='safety_blocked',updated_at=now() WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL`, job.StoryID, job.AppUserID)
	if err != nil {
		return Job{}, err
	}
	if affected, _ := storyUpdate.RowsAffected(); affected != 1 {
		return Job{}, ErrNotFound
	}
	if _, _, err := releaseQuotaTx(ctx, tx, job.AppUserID, job.ID); err != nil {
		return Job{}, err
	}
	if err := deleteJobTokenMapTx(ctx, tx, job.ID); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return job, nil
}

type OutboxEvent struct {
	ID         int64           `json:"id"`
	AppUserID  int64           `json:"appUserId"`
	StoryID    int64           `json:"storyId"`
	JobID      int64           `json:"jobId"`
	VersionID  int64           `json:"versionId"`
	EventType  string          `json:"eventType"`
	Payload    json.RawMessage `json:"payload"`
	ClaimToken string          `json:"-"`
	Attempts   int             `json:"attempts,omitempty"`
}

func (s *Store) ClaimOutbox(ctx context.Context, leaseArg ...time.Duration) (OutboxEvent, error) {
	if s == nil || s.db == nil {
		return OutboxEvent{}, ErrNilDB
	}
	lease := 2 * time.Minute
	if len(leaseArg) > 0 && leaseArg[0] > 0 {
		lease = leaseArg[0]
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return OutboxEvent{}, err
	}
	defer tx.Rollback()
	var event OutboxEvent
	var claimToken string
	if err := tx.QueryRowContext(ctx, `SELECT id,app_user_id,story_id,job_id,version_id,event_type,payload,attempts
		FROM app_life_story_outbox
		WHERE published_at IS NULL AND next_attempt_at <= now()
		  AND (lease_until IS NULL OR lease_until < now())
		ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&event.ID, &event.AppUserID, &event.StoryID, &event.JobID, &event.VersionID, &event.EventType, &event.Payload, &event.Attempts); err != nil {
		return OutboxEvent{}, err
	}
	claimToken = randomToken()
	if err := tx.QueryRowContext(ctx, `UPDATE app_life_story_outbox
		SET attempts=attempts+1,claim_token=$1,lease_until=now()+$2::interval
		WHERE id=$3 AND published_at IS NULL
		RETURNING attempts`, claimToken, fmt.Sprintf("%f seconds", lease.Seconds()), event.ID).Scan(&event.Attempts); err != nil {
		return OutboxEvent{}, err
	}
	event.ClaimToken = claimToken
	if err := tx.Commit(); err != nil {
		return OutboxEvent{}, err
	}
	return event, nil
}

// MarkOutboxPublished accepts both the current (id, claimToken, err) form and
// the legacy (id, err) form. A token-bearing claim is always required for a
// claimed row; the legacy form can only finalize an unclaimed legacy row.
func (s *Store) MarkOutboxPublished(ctx context.Context, id int64, args ...any) error {
	if s == nil || s.db == nil {
		return ErrNilDB
	}
	var claimToken string
	var publishErr error
	for _, arg := range args {
		switch value := arg.(type) {
		case string:
			claimToken = strings.TrimSpace(value)
		case error:
			publishErr = value
		case nil:
			// A nil error is a successful publish marker.
		}
	}
	whereToken := `(($2 = '' AND claim_token='') OR ($2 <> '' AND claim_token=$2))`
	if publishErr == nil {
		result, err := s.db.ExecContext(ctx, `UPDATE app_life_story_outbox
			SET published_at=now(),last_error='',claim_token='',lease_until=NULL
			WHERE id=$1 AND published_at IS NULL AND `+whereToken, id, claimToken)
		if err != nil {
			return err
		}
		if affected, _ := result.RowsAffected(); affected != 1 {
			return ErrConflict
		}
		return nil
	}
	message := strings.TrimSpace(publishErr.Error())
	if len([]rune(message)) > 500 {
		message = string([]rune(message)[:500])
	}
	result, err := s.db.ExecContext(ctx, `UPDATE app_life_story_outbox
		SET last_error=$1,claim_token='',lease_until=NULL,
		    next_attempt_at=now()+make_interval(secs => LEAST(300, CAST(POWER(2, LEAST(attempts, 8)) AS INTEGER)))
		WHERE id=$2 AND published_at IS NULL AND (($3 = '' AND claim_token='') OR ($3 <> '' AND claim_token=$3))`, message, id, claimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrConflict
	}
	return nil
}

// PublishOutboxNotification validates the claim and live story while holding
// the same user lock used by story deletion. Inbox insertion and outbox
// acknowledgement then commit together, so a claimed in-memory event cannot
// leave a notification behind after its story is deleted.
func (s *Store) PublishOutboxNotification(ctx context.Context, event OutboxEvent, title, content, deepLink, sourceKey string) error {
	if s == nil || s.db == nil {
		return ErrNilDB
	}
	if event.ID <= 0 || event.AppUserID <= 0 || event.StoryID <= 0 || event.JobID <= 0 || event.VersionID <= 0 || strings.TrimSpace(event.ClaimToken) == "" {
		return ErrInvalidState
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userStatus string
	if err := tx.QueryRowContext(ctx, `SELECT status FROM app_users WHERE id=$1 FOR SHARE`, event.AppUserID).Scan(&userStatus); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var storyID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_life_stories
		WHERE id=$1 AND app_user_id=$2 AND deleted_at IS NULL FOR SHARE`, event.StoryID, event.AppUserID).Scan(&storyID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	var outboxID int64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM app_life_story_outbox
		WHERE id=$1 AND app_user_id=$2 AND story_id=$3 AND job_id=$4 AND version_id=$5
		  AND published_at IS NULL AND claim_token=$6 FOR UPDATE`,
		event.ID, event.AppUserID, event.StoryID, event.JobID, event.VersionID, event.ClaimToken).Scan(&outboxID); errors.Is(err, sql.ErrNoRows) {
		return ErrConflict
	} else if err != nil {
		return err
	}
	if strings.TrimSpace(userStatus) == "active" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO app_notifications
			(app_user_id,kind,title,content,deep_link,source_key)
			VALUES($1,'life_story',$2,$3,$4,$5)
			ON CONFLICT (app_user_id,source_key) WHERE source_key<>''
			DO UPDATE SET source_key=EXCLUDED.source_key`, event.AppUserID,
			strings.TrimSpace(title), strings.TrimSpace(content), strings.TrimSpace(deepLink), strings.TrimSpace(sourceKey)); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE app_life_story_outbox
		SET published_at=now(),last_error='',claim_token='',lease_until=NULL
		WHERE id=$1 AND published_at IS NULL AND claim_token=$2`, outboxID, event.ClaimToken)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected != 1 {
		return ErrConflict
	}
	return tx.Commit()
}
