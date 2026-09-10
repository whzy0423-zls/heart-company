package caresystem

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Collector struct{ db *sql.DB }

func NewCollector(db *sql.DB) *Collector { return &Collector{db: db} }

func (c *Collector) Collect(ctx context.Context, appUserID int64, now time.Time) ([]Evidence, Baseline, error) {
	if c == nil || c.db == nil || appUserID <= 0 {
		return nil, Baseline{}, fmt.Errorf("care collector database unavailable")
	}
	start := now.Add(-30 * 24 * time.Hour)
	rows, err := c.db.QueryContext(ctx, `
		SELECT 'main', m.role, COALESCE(NULLIF(m.delivered_text, ''), m.content), m.create_time
		FROM app_chat_messages m
		JOIN app_chat_sessions s ON s.id=m.session_id
		WHERE s.app_user_id=$1 AND s.scene='chat' AND m.create_time >= $2
		UNION ALL
		SELECT 'friend', 'friend', m.body, m.created_at
		FROM direct_messages m
		JOIN direct_conversations c ON c.id=m.conversation_id
		WHERE (c.user_low_id=$1 OR c.user_high_id=$1)
		  AND m.recalled_at IS NULL AND m.created_at >= $2
		  AND m.message_type IN ('text','sticker')
		ORDER BY 4 ASC
		LIMIT 500`, appUserID, start)
	if err != nil {
		return nil, Baseline{}, fmt.Errorf("collect care evidence: %w", err)
	}
	defer rows.Close()
	evidence := make([]Evidence, 0, 64)
	for rows.Next() {
		var item Evidence
		if err := rows.Scan(&item.Source, &item.Role, &item.Content, &item.CreatedAt); err != nil {
			return nil, Baseline{}, err
		}
		item.Content = strings.TrimSpace(item.Content)
		if item.Content != "" {
			evidence = append(evidence, item)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, Baseline{}, err
	}
	baseline, err := c.baseline(ctx, appUserID)
	return evidence, baseline, err
}

func (c *Collector) baseline(ctx context.Context, appUserID int64) (Baseline, error) {
	var level sql.NullInt64
	var evaluated sql.NullTime
	err := c.db.QueryRowContext(ctx, `
		SELECT care_level, care_evaluated_at
		FROM app_users WHERE id=$1`, appUserID).Scan(&level, &evaluated)
	if err != nil {
		return Baseline{}, err
	}
	var out Baseline
	if level.Valid {
		value := int(level.Int64)
		out.Level = &value
	}
	if evaluated.Valid {
		value := evaluated.Time
		out.EvaluatedAt = &value
	}
	return out, nil
}
