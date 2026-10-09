package growthinsight

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Progress counts all eligible feedback, not just the bounded model-input selection.
func loadFeedbackProgress(ctx context.Context, q rowQueryer, userID, cardID, reportID int64, evidence []Evidence, now time.Time) (*FeedbackProgress, error) {
	if evidence == nil {
		evidence = []Evidence{}
	}
	encoded, err := json.Marshal(evidence)
	if err != nil {
		return nil, err
	}
	var progress FeedbackProgress
	err = q.QueryRowContext(ctx, `WITH scope AS (
SELECT c.app_user_id FROM app_growth_insight_consents c
JOIN app_users u ON u.id=c.app_user_id AND u.status='active'
JOIN app_user_cards card ON card.app_user_id=c.app_user_id AND card.id=$2 AND card.card_type='primary' AND card.status='active'
WHERE c.app_user_id=$1 AND c.enabled AND COALESCE(c.published_report_id,0)=$3
), feedback AS (`+feedbackSourcesQuery+`)
SELECT count(f.id),count(f.id) FILTER(WHERE EXISTS(
SELECT 1 FROM jsonb_array_elements($4::jsonb) e
WHERE e->>'id'=f.id AND e->>'text'=f.text AND (e->>'occurredAt')::timestamptz=f.occurred_at))
FROM scope LEFT JOIN feedback f ON f.occurred_at >= $5::timestamptz-interval '30 days' AND f.occurred_at <= $5::timestamptz
GROUP BY scope.app_user_id`, userID, cardID, reportID, encoded, now.UTC()).Scan(&progress.TotalCount, &progress.IncludedCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	progress.PendingCount = progress.TotalCount - progress.IncludedCount
	return &progress, nil
}
