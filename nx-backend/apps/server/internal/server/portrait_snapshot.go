package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"nine-xing/nx-backend/apps/server/internal/quiz"
)

const portraitRefreshInterval = 7 * 24 * time.Hour

// portraitSnapshot is the persisted freshness metadata for one card. The
// response body itself is kept in Payload so the generated content and its
// timestamp stay in sync across server instances and restarts.
type portraitSnapshot struct {
	CardID           int64
	SourceUpdateTime string
	GeneratedAt      time.Time
}

func portraitRefreshDue(snapshot portraitSnapshot, card quiz.Card, now time.Time) bool {
	if snapshot.CardID <= 0 || snapshot.GeneratedAt.IsZero() {
		return true
	}
	if snapshot.SourceUpdateTime != card.UpdateTime {
		return true
	}
	return !now.Before(snapshot.GeneratedAt.Add(portraitRefreshInterval))
}

func portraitTimestamp(t time.Time) string {
	return t.UTC().Format("2006/01/02 15:04:05")
}

func withPortraitSnapshotTimes(resp *portraitResp, generatedAt time.Time) {
	if resp == nil || generatedAt.IsZero() {
		return
	}
	resp.UpdatedAt = portraitTimestamp(generatedAt)
	resp.NextUpdateAt = portraitTimestamp(generatedAt.Add(portraitRefreshInterval))
}

// portraitForCard returns a persisted portrait until its seven-day window
// expires. A changed card update time invalidates the old snapshot so a newly
// completed assessment is reflected immediately.
func (s *Server) portraitForCard(ctx context.Context, userID int64, card quiz.Card) (portraitResp, error) {
	if card.MainType <= 0 {
		return buildPortrait(card), nil
	}
	if s == nil || s.db == nil {
		return portraitResp{}, errors.New("portrait snapshot store unavailable")
	}

	var sourceUpdateTime string
	var payload []byte
	var generatedAt time.Time
	err := s.db.QueryRowContext(ctx, `
		SELECT source_update_time, payload, generated_at
		FROM app_growth_portrait_snapshots
		WHERE card_id=$1 AND app_user_id=$2`, card.ID, userID).
		Scan(&sourceUpdateTime, &payload, &generatedAt)
	if err == nil {
		snapshot := portraitSnapshot{
			CardID:           card.ID,
			SourceUpdateTime: sourceUpdateTime,
			GeneratedAt:      generatedAt,
		}
		if !portraitRefreshDue(snapshot, card, time.Now().UTC()) {
			var cached portraitResp
			if decodeErr := json.Unmarshal(payload, &cached); decodeErr != nil {
				return portraitResp{}, decodeErr
			}
			withPortraitSnapshotTimes(&cached, generatedAt)
			return cached, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return portraitResp{}, err
	}

	return s.refreshPortraitSnapshot(ctx, userID, card)
}

func (s *Server) refreshPortraitSnapshot(ctx context.Context, userID int64, card quiz.Card) (portraitResp, error) {
	generatedAt := time.Now().UTC()
	resp := buildPortrait(card)
	withPortraitSnapshotTimes(&resp, generatedAt)
	payload, err := json.Marshal(resp)
	if err != nil {
		return portraitResp{}, err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO app_growth_portrait_snapshots
			(card_id, app_user_id, source_update_time, payload, generated_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (card_id) DO UPDATE SET
			app_user_id=EXCLUDED.app_user_id,
			source_update_time=EXCLUDED.source_update_time,
			payload=EXCLUDED.payload,
			generated_at=EXCLUDED.generated_at`,
		card.ID, userID, card.UpdateTime, payload, generatedAt)
	if err != nil {
		return portraitResp{}, err
	}
	return resp, nil
}
