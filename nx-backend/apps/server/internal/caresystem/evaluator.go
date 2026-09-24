package caresystem

import (
	"context"
	"database/sql"
	"time"
)

type Evaluator struct {
	collector *Collector
	store     *Store
	db        *sql.DB
}

func NewEvaluator(db *sql.DB) *Evaluator {
	return &Evaluator{collector: NewCollector(db), store: NewStore(db), db: db}
}

func (e *Evaluator) Evaluate(ctx context.Context, appUserID int64) error {
	now := time.Now()
	evidence, baseline, err := e.collector.Collect(ctx, appUserID, now)
	if err != nil {
		return err
	}
	result := Score(evidence, baseline)
	result.AppUserID = appUserID
	result.WindowStart = now.Add(-30 * 24 * time.Hour)
	result.WindowEnd = now
	result.KnowledgeVersion = e.knowledgeVersion(ctx)
	return e.store.Save(ctx, result)
}

func (e *Evaluator) knowledgeVersion(ctx context.Context) string {
	var version string
	_ = e.db.QueryRowContext(ctx, `SELECT COALESCE(to_char(max(update_time), 'YYYYMMDDHH24MISS'), '') FROM app_chat_knowledge_bindings WHERE status='enabled'`).Scan(&version)
	return version
}

func (e *Evaluator) EnqueueAndEvaluate(ctx context.Context, appUserID int64) {
	if e == nil || e.store == nil {
		return
	}
	if err := e.store.Enqueue(ctx, appUserID); err != nil {
		return
	}
	go func() {
		workCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = e.Evaluate(workCtx, appUserID)
	}()
}

func (e *Evaluator) EnqueueSession(ctx context.Context, sessionID int64) {
	if e == nil || e.db == nil || sessionID <= 0 {
		return
	}
	var userID int64
	if err := e.db.QueryRowContext(ctx, `SELECT app_user_id FROM app_chat_sessions WHERE id=$1`, sessionID).Scan(&userID); err == nil {
		e.EnqueueAndEvaluate(ctx, userID)
	}
}

func (e *Evaluator) Sweep(ctx context.Context) error {
	if e == nil || e.db == nil {
		return nil
	}
	ids, err := e.store.ClaimDue(ctx, 20)
	if err != nil {
		return err
	}
	for _, userID := range ids {
		if err := e.Evaluate(ctx, userID); err != nil {
			_ = e.store.SaveFailure(ctx, userID, err)
		}
	}
	return nil
}
