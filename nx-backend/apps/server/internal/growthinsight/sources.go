package growthinsight

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"time"
)

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

type sourceSet struct {
	Evidence      []Evidence
	Coverage      Coverage
	Fingerprint   string
	From          time.Time
	Through       time.Time
	Latest        time.Time
	BehaviorCount int
}

// Raw user material is the only chat input. Generated text and private skill sessions never enter this query.
const sourceQuery = `WITH all_sources AS (
SELECT 'message:'||m.id AS id,'message' AS kind,
 CASE WHEN btrim(m.transcript)<>'' THEN m.transcript ELSE m.content END AS text,m.create_time AS occurred_at
FROM app_chat_messages m JOIN app_chat_sessions s ON s.id=m.session_id
WHERE s.app_user_id=$1 AND s.card_id=$2 AND s.scene='chat' AND s.skill_version_id IS NULL AND m.role='user'
 AND btrim(CASE WHEN btrim(m.transcript)<>'' THEN m.transcript ELSE m.content END)<>''
UNION ALL
SELECT 'assessment:'||q.id,'assessment',json_build_object('primaryType',q.primary_type,'wingType',q.wing_type,'answers',q.answers)::text,q.create_time FROM app_quiz_submissions q
JOIN app_user_cards c ON c.submission_id=q.id WHERE c.id=$2 AND c.app_user_id=$1 AND q.app_user_id=$1
UNION ALL
SELECT 'action:'||a.id,'action_outcome',json_build_object('actionTitle',a.title,'status',a.status,'outcome',a.outcome,'note',a.note)::text,a.updated_at
FROM app_growth_insight_actions a WHERE a.app_user_id=$1 AND a.card_id=$2 AND a.status<>'pending'
UNION ALL
SELECT 'correction:'||c.id,'correction',json_build_object('kind',c.kind,'note',c.note)::text,c.created_at
FROM app_growth_insight_corrections c WHERE c.app_user_id=$1 AND c.card_id=$2
), eligible AS (
SELECT * FROM all_sources WHERE occurred_at<=$3::timestamptz AND (occurred_at >= $3::timestamptz-interval '30 days' OR kind='assessment')
)
SELECT id,kind,left(text,2000),occurred_at,md5(text),char_length(text),
 (SELECT count(*) FROM all_sources WHERE occurred_at<=$3),
 (SELECT count(*) FROM eligible),
 (SELECT max(occurred_at) FROM eligible)
FROM eligible ORDER BY (kind='assessment') DESC,occurred_at DESC,id DESC LIMIT 80`

func collect(ctx context.Context, q queryer, userID, cardID int64, now time.Time) (sourceSet, error) {
	v := sourceSet{Evidence: []Evidence{}, Coverage: Coverage{WindowDays: 30}}
	rows, err := q.QueryContext(ctx, sourceQuery, userID, cardID, now.UTC())
	if err != nil {
		return v, err
	}
	defer rows.Close()
	h := sha256.New()
	remaining := 24000
	total, eligible := 0, 0
	for rows.Next() {
		var e Evidence
		var digest string
		var length int
		if err = rows.Scan(&e.ID, &e.Kind, &e.Text, &e.OccurredAt, &digest, &length, &total, &eligible, &v.Latest); err != nil {
			return v, err
		}
		fmt.Fprintf(h, "%s|%s|%s|%s\n", e.ID, e.Kind, stamp(e.OccurredAt), digest)
		if remaining <= 0 {
			continue
		}
		runes := []rune(e.Text)
		if len(runes) > remaining {
			runes = runes[:remaining]
		}
		if len(runes) < length {
			v.Coverage.TruncatedCount++
		}
		e.Text = string(runes)
		remaining -= len(runes)
		if e.Kind != "assessment" {
			v.BehaviorCount++
		}
		v.Evidence = append(v.Evidence, e)
		if v.From.IsZero() || e.OccurredAt.Before(v.From) {
			v.From = e.OccurredAt
		}
		if e.OccurredAt.After(v.Through) {
			v.Through = e.OccurredAt
		}
	}
	if err = rows.Err(); err != nil {
		return v, err
	}
	fmt.Fprintf(h, "total:%d;eligible:%d", total, eligible)
	v.Fingerprint = hex.EncodeToString(h.Sum(nil))
	v.Coverage.EligibleCount = total
	v.Coverage.IncludedCount = len(v.Evidence)
	v.Coverage.OmittedCount = total - len(v.Evidence)
	v.Coverage.Truncated = v.Coverage.OmittedCount > 0 || v.Coverage.TruncatedCount > 0
	sort.Slice(v.Evidence, func(i, j int) bool { return v.Evidence[i].OccurredAt.Before(v.Evidence[j].OccurredAt) })
	return v, nil
}
