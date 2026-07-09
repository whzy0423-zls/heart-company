package profilecalibration

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// DailyQuizPushStats returns one business day's admin summary for automatic
// daily quiz reminders and answer completion.
func (s *Store) DailyQuizPushStats(ctx context.Context, date string) (DailyQuizPushStats, error) {
	date = strings.TrimSpace(date)
	out := DailyQuizPushStats{Date: date}
	if s == nil || s.db == nil || date == "" {
		return out, ErrInvalidInput
	}
	var eligibleUsers, pushedUsers, answeredUsers, completedUsers, totalAnswers, pendingReports int64
	if err := s.db.QueryRowContext(ctx, `
		WITH eligible AS (
			SELECT DISTINCT c.app_user_id
			FROM app_user_cards c
			JOIN app_users u ON u.id = c.app_user_id
			WHERE c.card_type = 'primary'
			  AND c.status = 'active'
			  AND u.status = 'active'
			  AND c.create_time < $1::date
			  AND EXISTS (SELECT 1 FROM app_device_tokens dt WHERE dt.app_user_id = c.app_user_id)
			  AND NOT EXISTS (
				SELECT 1 FROM app_reassessment_jobs j
				WHERE j.app_user_id = c.app_user_id
				  AND j.card_id = c.id
				  AND j.status IN ('pending','generating','generated')
			  )
		),
		batches AS (
			SELECT *
			FROM app_daily_quiz_batches b
			WHERE b.quiz_date = $1::date
		),
		reports AS (
			SELECT COUNT(*) AS pending_reassessment_reports
			FROM app_reassessment_jobs j
			JOIN app_users u ON u.id = j.app_user_id
			WHERE j.status = 'generated'
			  AND j.push_sent_at IS NULL
			  AND u.status = 'active'
		)
		SELECT
			(SELECT COUNT(*) FROM eligible) AS eligible_users,
			(SELECT COUNT(DISTINCT app_user_id) FROM batches WHERE push_sent_at IS NOT NULL) AS pushed_users,
			(SELECT COUNT(DISTINCT app_user_id) FROM batches WHERE answered_count > 0) AS answered_users,
			(SELECT COUNT(DISTINCT app_user_id) FROM batches WHERE completed = true OR answered_count >= 5) AS completed_users,
			COALESCE((SELECT SUM(answered_count) FROM batches), 0) AS total_answers,
			(SELECT pending_reassessment_reports FROM reports) AS pending_reassessment_reports
	`, date).Scan(&eligibleUsers, &pushedUsers, &answeredUsers, &completedUsers, &totalAnswers, &pendingReports); err != nil {
		return out, err
	}
	out.EligibleUsers = int(eligibleUsers)
	out.PushedUsers = int(pushedUsers)
	out.Pushed = pushedUsers > 0
	out.AnsweredUsers = int(answeredUsers)
	out.CompletedUsers = int(completedUsers)
	out.TotalAnswers = int(totalAnswers)
	out.PendingReassessmentReports = int(pendingReports)
	return out, nil
}

// ListDailyQuizPushRecords returns paged daily quiz batch records for the admin
// console. A record exists once a daily batch has been generated, either by the
// automatic reminder job or by the user opening the daily quiz page.
func (s *Store) ListDailyQuizPushRecords(ctx context.Context, date string, page, pageSize int) ([]DailyQuizPushRecord, int, error) {
	date = strings.TrimSpace(date)
	if s == nil || s.db == nil || date == "" {
		return nil, 0, ErrInvalidInput
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM app_daily_quiz_batches b
		WHERE b.quiz_date = $1::date
	`, date).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			b.app_user_id,
			COALESCE(u.phone, '') AS phone,
			COALESCE(u.nickname, '') AS nickname,
			b.card_id,
			COALESCE(c.name, '') AS card_name,
			b.quiz_date,
			b.id AS batch_id,
			(b.push_sent_at IS NOT NULL) AS pushed,
			b.push_sent_at,
			b.answered_count,
			b.completed,
			b.completed_at
		FROM app_daily_quiz_batches b
		JOIN app_users u ON u.id = b.app_user_id
		LEFT JOIN app_user_cards c ON c.id = b.card_id
		WHERE b.quiz_date = $1::date
		ORDER BY b.push_sent_at DESC NULLS LAST, b.id DESC
		LIMIT $2 OFFSET $3
	`, date, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]DailyQuizPushRecord, 0)
	for rows.Next() {
		var (
			item        DailyQuizPushRecord
			quizDate    time.Time
			pushSentAt  sql.NullTime
			completedAt sql.NullTime
		)
		if err := rows.Scan(
			&item.AppUserID,
			&item.Phone,
			&item.Nickname,
			&item.CardID,
			&item.CardName,
			&quizDate,
			&item.BatchID,
			&item.Pushed,
			&pushSentAt,
			&item.AnsweredCount,
			&item.Completed,
			&completedAt,
		); err != nil {
			return nil, 0, err
		}
		item.QuizDate = quizDate.Format("2006-01-02")
		item.PushSentAt = formatDailyQuizAdminTime(pushSentAt)
		item.CompletedAt = formatDailyQuizAdminTime(completedAt)
		item.Status = dailyQuizPushRecordStatus(item)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func dailyQuizPushRecordStatus(item DailyQuizPushRecord) string {
	switch {
	case item.Completed || item.AnsweredCount >= DailyQuestionCount:
		return "completed"
	case item.AnsweredCount > 0:
		return "answered"
	case item.Pushed:
		return "pushed"
	default:
		return "created"
	}
}

func formatDailyQuizAdminTime(value sql.NullTime) string {
	if !value.Valid || value.Time.IsZero() {
		return ""
	}
	return value.Time.Format("2006/01/02 15:04:05")
}
