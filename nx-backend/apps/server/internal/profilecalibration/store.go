package profilecalibration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	DailyQuestionCount = 5
	RoundAnswerTarget  = 100
)

var (
	ErrNotFound      = errors.New("profilecalibration: not found")
	ErrInvalidInput  = errors.New("profilecalibration: invalid input")
	ErrIncomplete    = errors.New("profilecalibration: batch incomplete")
	ErrInvalidStatus = errors.New("profilecalibration: invalid status")
)

type Store struct {
	db                 *sql.DB
	dailyQuizGenerator DailyQuizQuestionGenerator
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// DailyQuizQuestionGenerator lets the server inject a model-backed generator
// without making the profilecalibration store depend on server/model packages.
// Implementations must return already validated questions or an error; the
// store will fall back to the built-in bank when generation is unavailable.
type DailyQuizQuestionGenerator interface {
	GenerateDailyQuizQuestions(ctx context.Context, input DailyQuizGenerationInput) (DailyQuizGenerationResult, error)
}

type DailyQuizGenerationInput struct {
	Date          string `json:"date"`
	Count         int    `json:"count"`
	SlotNo        int    `json:"slotNo,omitempty"`
	ReplaceReason string `json:"replaceReason,omitempty"`
}

type GeneratedDailyQuizQuestion struct {
	Body        string         `json:"body"`
	Dimension   string         `json:"dimension"`
	Options     []Option       `json:"options"`
	TypeWeights map[string]int `json:"typeWeights,omitempty"`
}

type DailyQuizGenerationResult struct {
	Questions     []GeneratedDailyQuizQuestion `json:"questions"`
	Prompt        string                       `json:"prompt"`
	RawResponse   string                       `json:"rawResponse"`
	ModelProvider string                       `json:"modelProvider"`
	ModelName     string                       `json:"modelName"`
	Source        string                       `json:"source"`
}

func (s *Store) SetDailyQuizQuestionGenerator(generator DailyQuizQuestionGenerator) {
	if s != nil {
		s.dailyQuizGenerator = generator
	}
}

type Option struct {
	ID          string         `json:"id"`
	Label       string         `json:"label"`
	Text        string         `json:"text"`
	TypeWeights map[string]int `json:"typeWeights,omitempty"`
	Weights     map[string]int `json:"weights,omitempty"`
}

type Question struct {
	ID        int64    `json:"id"`
	Body      string   `json:"body"`
	Dimension string   `json:"dimension,omitempty"`
	Options   []Option `json:"options"`
}

type Batch struct {
	ID            int64      `json:"id"`
	CardID        int64      `json:"cardId"`
	Date          string     `json:"date"`
	Questions     []Question `json:"questions"`
	Progress      Progress   `json:"progress"`
	AnsweredCount int        `json:"answeredCount"`
	Completed     bool       `json:"completed"`
}

type Progress struct {
	Answered       int    `json:"answered"`
	Total          int    `json:"total"`
	TodayAnswered  int    `json:"todayAnswered"`
	TodayTotal     int    `json:"todayTotal"`
	LatestReportID *int64 `json:"latestReportId,omitempty"`
}

type Report struct {
	ID                int64             `json:"id"`
	CardID            int64             `json:"cardId"`
	Status            string            `json:"status"`
	OldMainType       int               `json:"oldMainType"`
	SuggestedMainType int               `json:"suggestedMainType"`
	Confidence        float64           `json:"confidence"`
	ConfidenceLabel   string            `json:"confidenceLabel"`
	Summary           string            `json:"summary"`
	Profile           ReportProfile     `json:"profile"`
	Reasons           []string          `json:"reasons"`
	Evidence          []EvidenceSummary `json:"evidence"`
}

type ReportProfile struct {
	MainType             int    `json:"mainType"`
	PrimaryMotivation    string `json:"primaryMotivation"`
	StressPattern        string `json:"stressPattern"`
	RelationshipPattern  string `json:"relationshipPattern"`
	GrowthFocus          string `json:"growthFocus"`
	CommunicationAdvice  string `json:"communicationAdvice"`
	PrivateSignalSummary string `json:"privateSignalSummary"`
}

type EvidenceSummary struct {
	Kind       string `json:"kind"`
	Label      string `json:"label"`
	Text       string `json:"text"`
	typeScores map[int]float64
}

// DailyReminderCandidate identifies a primary card that should receive the
// second-day-and-after daily 5-question calibration reminder.
type DailyReminderCandidate struct {
	AppUserID int64 `json:"appUserId"`
	CardID    int64 `json:"cardId"`
}

// ReassessmentPushCandidate identifies a generated report that still needs an
// App push reminder.
type ReassessmentPushCandidate struct {
	ID        int64 `json:"id"`
	AppUserID int64 `json:"appUserId"`
	CardID    int64 `json:"cardId"`
}

// DailyQuizPushStats summarizes one business day's automatic daily-quiz push
// and answering progress for the admin console.
type DailyQuizPushStats struct {
	Date                       string `json:"date"`
	EligibleUsers              int    `json:"eligibleUsers"`
	Pushed                     bool   `json:"pushed"`
	PushedUsers                int    `json:"pushedUsers"`
	AnsweredUsers              int    `json:"answeredUsers"`
	CompletedUsers             int    `json:"completedUsers"`
	TotalAnswers               int    `json:"totalAnswers"`
	PendingReassessmentReports int    `json:"pendingReassessmentReports"`
}

// DailyQuizPushRecord is a user/card row shown in the admin daily-quiz push
// records page.
type DailyQuizPushRecord struct {
	AppUserID     int64  `json:"appUserId"`
	Phone         string `json:"phone"`
	Nickname      string `json:"nickname"`
	CardID        int64  `json:"cardId"`
	CardName      string `json:"cardName"`
	QuizDate      string `json:"quizDate"`
	BatchID       int64  `json:"batchId"`
	Pushed        bool   `json:"pushed"`
	PushSentAt    string `json:"pushSentAt"`
	AnsweredCount int    `json:"answeredCount"`
	Completed     bool   `json:"completed"`
	CompletedAt   string `json:"completedAt"`
	Status        string `json:"status"`
}

// DailyQuizSet is the admin-facing daily 5-question set for a business date.
type DailyQuizSet struct {
	ID            int64                      `json:"id"`
	Date          string                     `json:"date"`
	Status        string                     `json:"status"`
	Source        string                     `json:"source"`
	ModelProvider string                     `json:"modelProvider"`
	ModelName     string                     `json:"modelName"`
	Prompt        string                     `json:"prompt,omitempty"`
	RawResponse   string                     `json:"rawResponse,omitempty"`
	QuestionIDs   []int64                    `json:"questionIds"`
	ErrorMessage  string                     `json:"errorMessage,omitempty"`
	GeneratedAt   string                     `json:"generatedAt"`
	PublishedAt   string                     `json:"publishedAt"`
	PushedAt      string                     `json:"pushedAt"`
	Questions     []DailyQuizQuestionVersion `json:"questions"`
}

// DailyQuizQuestionVersion records the active or historical version of one
// slot in a daily set. Admin replacement creates a new version and question_id.
type DailyQuizQuestionVersion struct {
	ID            int64    `json:"id"`
	SetID         int64    `json:"setId"`
	QuestionID    int64    `json:"questionId"`
	SlotNo        int      `json:"slotNo"`
	VersionNo     int      `json:"versionNo"`
	IsActive      bool     `json:"isActive"`
	Question      Question `json:"question"`
	Source        string   `json:"source"`
	ModelProvider string   `json:"modelProvider"`
	ModelName     string   `json:"modelName"`
	Operator      string   `json:"operator"`
	ReplaceReason string   `json:"replaceReason"`
	AnsweredCount int      `json:"answeredCount"`
	CreateTime    string   `json:"createTime"`
}

func (s *Store) TodayBatch(ctx context.Context, appUserID, cardID int64) (any, error) {
	return s.todayBatch(ctx, appUserID, cardID, businessDate())
}

func (s *Store) TodayBatchForDate(ctx context.Context, appUserID, cardID int64, date string) (Batch, error) {
	return s.todayBatch(ctx, appUserID, cardID, strings.TrimSpace(date))
}

func (s *Store) ListDailyReminderCandidates(ctx context.Context, date string, limit int) ([]DailyReminderCandidate, error) {
	date = strings.TrimSpace(date)
	if s == nil || s.db == nil || date == "" {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.app_user_id, c.id
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
		  AND NOT EXISTS (
			SELECT 1 FROM app_daily_quiz_batches b
			WHERE b.app_user_id = c.app_user_id
			  AND b.card_id = c.id
			  AND b.quiz_date = $1::date
			  AND (b.push_sent_at IS NOT NULL OR (b.push_claimed_at IS NOT NULL AND b.push_claimed_at > now() - INTERVAL '10 minutes') OR b.completed = true OR b.answered_count >= 5)
		  )
		ORDER BY c.id ASC
		LIMIT $2
	`, date, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]DailyReminderCandidate, 0)
	for rows.Next() {
		var item DailyReminderCandidate
		if err := rows.Scan(&item.AppUserID, &item.CardID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ClaimBatchPush(ctx context.Context, batchID int64) (bool, error) {
	if s == nil || s.db == nil || batchID <= 0 {
		return false, ErrInvalidInput
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE app_daily_quiz_batches
		SET push_claimed_at=now(), update_time=now()
		WHERE id=$1
		  AND push_sent_at IS NULL
		  AND (push_claimed_at IS NULL OR push_claimed_at < now() - INTERVAL '10 minutes')
	`, batchID)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) MarkBatchPushSent(ctx context.Context, batchID int64) error {
	if s == nil || s.db == nil || batchID <= 0 {
		return ErrInvalidInput
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE app_daily_quiz_batches
		SET push_claimed_at=COALESCE(push_claimed_at,now()), push_sent_at=COALESCE(push_sent_at,now()), update_time=now()
		WHERE id=$1
	`, batchID)
	return err
}

func (s *Store) ListGeneratedReassessmentPushCandidates(ctx context.Context, limit int) ([]ReassessmentPushCandidate, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidInput
	}
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT j.id, j.app_user_id, j.card_id
		FROM app_reassessment_jobs j
		JOIN app_users u ON u.id = j.app_user_id
		WHERE j.status = 'generated'
		  AND j.push_sent_at IS NULL
		  AND (j.push_claimed_at IS NULL OR j.push_claimed_at < now() - INTERVAL '10 minutes')
		  AND u.status = 'active'
		  AND EXISTS (SELECT 1 FROM app_device_tokens dt WHERE dt.app_user_id = j.app_user_id)
		ORDER BY j.create_time ASC, j.id ASC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ReassessmentPushCandidate, 0)
	for rows.Next() {
		var item ReassessmentPushCandidate
		if err := rows.Scan(&item.ID, &item.AppUserID, &item.CardID); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ClaimReassessmentPush(ctx context.Context, id int64) (bool, error) {
	if s == nil || s.db == nil || id <= 0 {
		return false, ErrInvalidInput
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE app_reassessment_jobs
		SET push_claimed_at=now(), update_time=now()
		WHERE id=$1
		  AND push_sent_at IS NULL
		  AND (push_claimed_at IS NULL OR push_claimed_at < now() - INTERVAL '10 minutes')
	`, id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) MarkReassessmentPushSent(ctx context.Context, id int64) error {
	if s == nil || s.db == nil || id <= 0 {
		return ErrInvalidInput
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE app_reassessment_jobs
		SET push_claimed_at=COALESCE(push_claimed_at,now()), push_sent_at=COALESCE(push_sent_at,now()), update_time=now()
		WHERE id=$1
	`, id)
	return err
}

func (s *Store) Progress(ctx context.Context, appUserID, cardID int64) (any, error) {
	return s.progress(ctx, appUserID, cardID, businessDate())
}

func (s *Store) SubmitAnswer(ctx context.Context, appUserID, batchID, questionID int64, optionID string) (any, error) {
	return true, s.submitAnswer(ctx, appUserID, batchID, questionID, optionID)
}

func (s *Store) CompleteBatch(ctx context.Context, appUserID, batchID int64) (any, error) {
	return s.completeBatch(ctx, appUserID, batchID)
}

func (s *Store) Latest(ctx context.Context, appUserID, cardID int64) (any, error) {
	report, err := s.latestReport(ctx, appUserID, cardID)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (s *Store) Detail(ctx context.Context, appUserID, id int64) (any, error) {
	return s.reportByID(ctx, appUserID, id)
}

func (s *Store) Accept(ctx context.Context, appUserID, id int64) (any, error) {
	return true, s.finishReport(ctx, appUserID, id, true)
}

func (s *Store) Reject(ctx context.Context, appUserID, id int64) (any, error) {
	return true, s.finishReport(ctx, appUserID, id, false)
}

func (s *Store) todayBatch(ctx context.Context, appUserID, cardID int64, date string) (Batch, error) {
	var out Batch
	if s == nil || s.db == nil || appUserID <= 0 || cardID <= 0 {
		return out, ErrInvalidInput
	}
	if err := s.ensureCard(ctx, appUserID, cardID); err != nil {
		return out, err
	}
	roundNo, err := s.currentRound(ctx, appUserID, cardID)
	if err != nil {
		return out, err
	}
	var ids []int64
	if set, setErr := s.EnsureDailyQuizSet(ctx, date); setErr == nil && len(set.QuestionIDs) >= DailyQuestionCount {
		ids = append([]int64(nil), set.QuestionIDs[:DailyQuestionCount]...)
	} else {
		if err := s.EnsureDefaultQuestions(ctx); err != nil {
			return out, err
		}
		ids, err = s.selectQuestionIDs(ctx, cardID, date)
		if err != nil {
			return out, err
		}
	}
	idsJSON, _ := json.Marshal(ids)
	var batchID int64
	if err := s.db.QueryRowContext(ctx, `
		INSERT INTO app_daily_quiz_batches (app_user_id, card_id, quiz_date, round_no, question_ids)
		VALUES ($1,$2,$3,$4,$5::jsonb)
		ON CONFLICT (card_id, quiz_date, round_no) DO UPDATE SET update_time=now()
		RETURNING id
	`, appUserID, cardID, date, roundNo, string(idsJSON)).Scan(&batchID); err != nil {
		return out, err
	}
	return s.batchByID(ctx, appUserID, batchID, date)
}

func (s *Store) progress(ctx context.Context, appUserID, cardID int64, date string) (Progress, error) {
	p := Progress{Total: RoundAnswerTarget, TodayTotal: DailyQuestionCount}
	if s == nil || s.db == nil || appUserID <= 0 || cardID <= 0 {
		return p, ErrInvalidInput
	}
	if err := s.ensureCard(ctx, appUserID, cardID); err != nil {
		return p, err
	}
	roundNo, err := s.currentRound(ctx, appUserID, cardID)
	if err != nil {
		return p, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM app_daily_quiz_answers
		WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3
	`, appUserID, cardID, roundNo).Scan(&p.Answered); err != nil {
		return p, err
	}
	if p.Answered > RoundAnswerTarget {
		p.Answered = RoundAnswerTarget
	}
	_ = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(answered_count),0)
		FROM app_daily_quiz_batches
		WHERE app_user_id=$1 AND card_id=$2 AND quiz_date=$3 AND round_no=$4
	`, appUserID, cardID, date, roundNo).Scan(&p.TodayAnswered)
	if p.TodayAnswered > DailyQuestionCount {
		p.TodayAnswered = DailyQuestionCount
	}
	var reportID sql.NullInt64
	err = s.db.QueryRowContext(ctx, `
		SELECT id FROM app_reassessment_jobs
		WHERE app_user_id=$1 AND card_id=$2 AND status IN ('pending','generating','generated')
		ORDER BY create_time DESC, id DESC LIMIT 1
	`, appUserID, cardID).Scan(&reportID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return p, err
	}
	if reportID.Valid {
		p.LatestReportID = &reportID.Int64
	}
	return p, nil
}

func (s *Store) submitAnswer(ctx context.Context, appUserID, batchID, questionID int64, optionID string) error {
	optionID = strings.TrimSpace(optionID)
	if appUserID <= 0 || batchID <= 0 || questionID <= 0 || optionID == "" {
		return ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cardID int64
	var roundNo int
	var rawIDs []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT card_id, round_no, question_ids FROM app_daily_quiz_batches
		WHERE id=$1 AND app_user_id=$2 FOR UPDATE
	`, batchID, appUserID).Scan(&cardID, &roundNo, &rawIDs); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if !containsID(decodeIDs(rawIDs), questionID) {
		return ErrInvalidInput
	}
	delta, err := optionDeltaTx(ctx, tx, questionID, optionID)
	if err != nil {
		return err
	}
	deltaJSON, _ := json.Marshal(delta)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_daily_quiz_answers (batch_id, app_user_id, card_id, round_no, question_id, option_id, type_delta, answered_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,now())
		ON CONFLICT (batch_id, question_id)
		DO UPDATE SET option_id=EXCLUDED.option_id, type_delta=EXCLUDED.type_delta, answered_at=now()
	`, batchID, appUserID, cardID, roundNo, questionID, optionID, string(deltaJSON)); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_daily_quiz_batches b SET answered_count=sub.cnt, update_time=now()
		FROM (SELECT batch_id, COUNT(*)::int cnt FROM app_daily_quiz_answers WHERE batch_id=$1 GROUP BY batch_id) sub
		WHERE b.id=sub.batch_id
	`, batchID); err != nil {
		return err
	}
	if err := maybeCreateReassessmentTx(ctx, tx, appUserID, cardID, roundNo); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) completeBatch(ctx context.Context, appUserID, batchID int64) (Progress, error) {
	p := Progress{Total: RoundAnswerTarget, TodayTotal: DailyQuestionCount}
	if appUserID <= 0 || batchID <= 0 {
		return p, ErrInvalidInput
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return p, err
	}
	defer tx.Rollback()
	var cardID int64
	var roundNo int
	var quizDate time.Time
	var answered int
	if err := tx.QueryRowContext(ctx, `
		SELECT card_id, round_no, quiz_date, answered_count FROM app_daily_quiz_batches
		WHERE id=$1 AND app_user_id=$2 FOR UPDATE
	`, batchID, appUserID).Scan(&cardID, &roundNo, &quizDate, &answered); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrNotFound
		}
		return p, err
	}
	if answered < DailyQuestionCount {
		return p, ErrIncomplete
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_daily_quiz_batches SET completed=true, completed_at=COALESCE(completed_at,now()), update_time=now() WHERE id=$1`, batchID); err != nil {
		return p, err
	}
	behaviorScores, _ := json.Marshal(map[string]float64{"dailyQuizCompleted": 1, "completionConsistency": 0.75})
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO app_profile_evidence (app_user_id, card_id, round_no, source_type, source_id, evidence_text, behavior_scores, confidence, status)
		SELECT $1,$2,$3,'behavior',$4,'用户完成了今日 5 道画像校准题，体现出持续校准和互动稳定性。',$5::jsonb,0.62,'active'
		WHERE NOT EXISTS (
			SELECT 1 FROM app_profile_evidence WHERE app_user_id=$1 AND card_id=$2 AND source_type='behavior' AND source_id=$4
		)
	`, appUserID, cardID, roundNo, batchID, string(behaviorScores)); err != nil {
		return p, err
	}
	if err := maybeCreateReassessmentTx(ctx, tx, appUserID, cardID, roundNo); err != nil {
		return p, err
	}
	if err := tx.Commit(); err != nil {
		return p, err
	}
	return s.progress(ctx, appUserID, cardID, quizDate.Format("2006-01-02"))
}

func (s *Store) latestReport(ctx context.Context, appUserID, cardID int64) (*Report, error) {
	if err := s.ensureCard(ctx, appUserID, cardID); err != nil {
		return nil, err
	}
	return scanReport(s.db.QueryRowContext(ctx, `
		SELECT id, card_id, status, old_main_type, suggested_main_type, confidence, report_json
		FROM app_reassessment_jobs
		WHERE app_user_id=$1 AND card_id=$2 AND status IN ('pending','generating','generated','accepted','rejected')
		ORDER BY create_time DESC, id DESC LIMIT 1
	`, appUserID, cardID))
}

func (s *Store) reportByID(ctx context.Context, appUserID, id int64) (Report, error) {
	report, err := scanReport(s.db.QueryRowContext(ctx, `
		SELECT id, card_id, status, old_main_type, suggested_main_type, confidence, report_json
		FROM app_reassessment_jobs WHERE app_user_id=$1 AND id=$2
	`, appUserID, id))
	if err != nil {
		return Report{}, err
	}
	return *report, nil
}

func (s *Store) finishReport(ctx context.Context, appUserID, id int64, accept bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var cardID int64
	var status string
	var suggested int
	var confidence float64
	if err := tx.QueryRowContext(ctx, `
		SELECT card_id, status, suggested_main_type, confidence FROM app_reassessment_jobs
		WHERE id=$1 AND app_user_id=$2 FOR UPDATE
	`, id, appUserID).Scan(&cardID, &status, &suggested, &confidence); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if status == "accepted" || status == "rejected" {
		return nil
	}
	if status != "generated" {
		return ErrInvalidStatus
	}
	newStatus := "rejected"
	if accept {
		newStatus = "accepted"
		profileJSON, _ := json.Marshal(buildProfile(suggested))
		if _, err := tx.ExecContext(ctx, `UPDATE app_user_cards SET enneagram=$1, profile=$2::jsonb, update_time=now() WHERE id=$3 AND app_user_id=$4 AND status='active'`, suggested, string(profileJSON), cardID, appUserID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE app_profile_versions SET is_active=false WHERE card_id=$1`, cardID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_profile_versions (app_user_id, card_id, version, main_type, wing_type, profile_json, source, confidence, is_active)
			VALUES ($1,$2,COALESCE((SELECT MAX(version)+1 FROM app_profile_versions WHERE card_id=$2),1),$3,0,$4::jsonb,'reassessment',$5,true)
		`, appUserID, cardID, suggested, string(profileJSON), confidence); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_reassessment_jobs SET status=$1, update_time=now() WHERE id=$2`, newStatus, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) currentRound(ctx context.Context, appUserID, cardID int64) (int, error) {
	var done sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(round_no) FROM app_reassessment_jobs WHERE app_user_id=$1 AND card_id=$2 AND status IN ('accepted','rejected','expired')`, appUserID, cardID).Scan(&done); err != nil {
		return 0, err
	}
	if done.Valid {
		return int(done.Int64) + 1, nil
	}
	var open sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(round_no) FROM app_reassessment_jobs WHERE app_user_id=$1 AND card_id=$2 AND status IN ('pending','generating','generated')`, appUserID, cardID).Scan(&open); err != nil {
		return 0, err
	}
	if open.Valid && open.Int64 > 0 {
		return int(open.Int64), nil
	}
	var answered sql.NullInt64
	if err := s.db.QueryRowContext(ctx, `SELECT MAX(round_no) FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2`, appUserID, cardID).Scan(&answered); err != nil {
		return 0, err
	}
	if answered.Valid && answered.Int64 > 0 {
		return int(answered.Int64), nil
	}
	return 1, nil
}

func (s *Store) ensureCard(ctx context.Context, appUserID, cardID int64) error {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM app_user_cards WHERE id=$1 AND app_user_id=$2 AND status='active'`, cardID, appUserID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Store) EnsureDefaultQuestions(ctx context.Context) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_daily_quiz_questions WHERE status='active'`).Scan(&count); err != nil {
		return err
	}
	if count >= DailyQuestionCount {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, q := range defaultQuestions() {
		optionsJSON, _ := json.Marshal(q.Options)
		weightsJSON, _ := json.Marshal(map[string]int{strconv.Itoa((i % 9) + 1): 1})
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO app_daily_quiz_questions (sort, body, options, dimension, type_weights, status)
			SELECT $1,$2,$3::jsonb,$4,$5::jsonb,'active'
			WHERE NOT EXISTS (SELECT 1 FROM app_daily_quiz_questions WHERE body=$2)
		`, (i+1)*10, q.Body, string(optionsJSON), q.Dimension, string(weightsJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) batchByID(ctx context.Context, appUserID, batchID int64, date string) (Batch, error) {
	var out Batch
	var rawIDs []byte
	var quizDate time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT id, card_id, quiz_date, question_ids, answered_count, completed FROM app_daily_quiz_batches WHERE id=$1 AND app_user_id=$2`, batchID, appUserID).Scan(&out.ID, &out.CardID, &quizDate, &rawIDs, &out.AnsweredCount, &out.Completed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return out, ErrNotFound
		}
		return out, err
	}
	out.Date = quizDate.Format("2006-01-02")
	questions, err := s.questionsByIDs(ctx, decodeIDs(rawIDs))
	if err != nil {
		return out, err
	}
	out.Questions = questions
	p, err := s.progress(ctx, appUserID, out.CardID, date)
	if err != nil {
		return out, err
	}
	out.Progress = p
	return out, nil
}

func (s *Store) selectQuestionIDs(ctx context.Context, cardID int64, date string) ([]int64, error) {
	if ids, ok, err := s.selectDailyQuizSetQuestionIDs(ctx, date); err != nil {
		return nil, err
	} else if ok {
		return ids, nil
	}
	return s.selectFallbackQuestionIDs(ctx, cardID, date)
}

func (s *Store) selectDailyQuizSetQuestionIDs(ctx context.Context, date string) ([]int64, bool, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		return nil, false, nil
	}
	var raw []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT question_ids
		FROM app_daily_quiz_sets
		WHERE quiz_date=$1::date
		  AND status IN ('generated','published','pushed','fallback')
		ORDER BY id DESC
		LIMIT 1
	`, date).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	ids := decodeIDs(raw)
	if len(ids) < DailyQuestionCount {
		return nil, false, nil
	}
	return append([]int64(nil), ids[:DailyQuestionCount]...), true, nil
}

func (s *Store) selectFallbackQuestionIDs(ctx context.Context, cardID int64, date string) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM app_daily_quiz_questions WHERE status='active' ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	all := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		all = append(all, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(all) < DailyQuestionCount {
		return nil, fmt.Errorf("profilecalibration: invalid question bank")
	}
	start := (int(cardID) + stableDateSeed(date)) % len(all)
	ids := make([]int64, 0, DailyQuestionCount)
	for i := 0; i < len(all) && len(ids) < DailyQuestionCount; i++ {
		ids = append(ids, all[(start+i)%len(all)])
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, nil
}

func (s *Store) questionsByIDs(ctx context.Context, ids []int64) ([]Question, error) {
	if len(ids) == 0 {
		return []Question{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, body, dimension, options FROM app_daily_quiz_questions WHERE status='active'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]Question{}
	for rows.Next() {
		var q Question
		var raw []byte
		if err := rows.Scan(&q.ID, &q.Body, &q.Dimension, &raw); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(raw, &q.Options)
		for i := range q.Options {
			q.Options[i].TypeWeights = nil
			q.Options[i].Weights = nil
		}
		byID[q.ID] = q
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Question, 0, len(ids))
	for _, id := range ids {
		if q, ok := byID[id]; ok {
			out = append(out, q)
		}
	}
	return out, nil
}

func optionDeltaTx(ctx context.Context, tx *sql.Tx, questionID int64, optionID string) (map[string]int, error) {
	var raw []byte
	if err := tx.QueryRowContext(ctx, `SELECT options FROM app_daily_quiz_questions WHERE id=$1 AND status='active'`, questionID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidInput
		}
		return nil, err
	}
	var options []Option
	_ = json.Unmarshal(raw, &options)
	for _, opt := range options {
		if opt.ID == optionID {
			if len(opt.TypeWeights) > 0 {
				return opt.TypeWeights, nil
			}
			if len(opt.Weights) > 0 {
				return opt.Weights, nil
			}
			return map[string]int{"6": 1}, nil
		}
	}
	return nil, ErrInvalidInput
}

func maybeCreateReassessmentTx(ctx context.Context, tx *sql.Tx, appUserID, cardID int64, roundNo int) error {
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3`, appUserID, cardID, roundNo).Scan(&count); err != nil {
		return err
	}
	if count < RoundAnswerTarget {
		return nil
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM app_reassessment_jobs WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3 AND status IN ('pending','generating','generated')`, appUserID, cardID, roundNo).Scan(&existing); err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}
	var recentHandled int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM app_reassessment_jobs
		WHERE app_user_id=$1 AND card_id=$2
		  AND status IN ('accepted','rejected','expired')
		  AND update_time > now() - INTERVAL '14 days'
	`, appUserID, cardID).Scan(&recentHandled); err != nil {
		return err
	}
	if recentHandled > 0 {
		return nil
	}
	var oldType int
	if err := tx.QueryRowContext(ctx, `SELECT enneagram FROM app_user_cards WHERE id=$1 AND app_user_id=$2 AND status='active'`, cardID, appUserID).Scan(&oldType); err != nil {
		return err
	}
	dailySuggested, counts, err := suggestedTypeTx(ctx, tx, appUserID, cardID, roundNo, oldType)
	if err != nil {
		return err
	}
	evidence, chatCount, voiceCount, behaviorCount, err := profileEvidenceTx(ctx, tx, appUserID, cardID, roundNo)
	if err != nil {
		return err
	}
	suggested, confidence := combinedSuggestedType(dailySuggested, counts, evidence)
	report := buildReport(0, cardID, "generated", oldType, suggested, count, counts, evidence, confidence)
	j, _ := json.Marshal(report)
	var id int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app_reassessment_jobs (app_user_id, card_id, round_no, trigger_reason, evidence_window_start, evidence_window_end, daily_answer_count, chat_evidence_count, voice_evidence_count, behavior_evidence_count, old_main_type, suggested_main_type, confidence, status, report_json)
		VALUES ($1,$2,$3,'daily_quiz_100',(SELECT MIN(answered_at) FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3),(SELECT MAX(answered_at) FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3),$4,$5,$6,$7,$8,$9,$10,'generated',$11::jsonb)
		RETURNING id`, appUserID, cardID, roundNo, count, chatCount, voiceCount, behaviorCount, oldType, suggested, report.Confidence, string(j)).Scan(&id); err != nil {
		return err
	}
	report.ID = id
	j, _ = json.Marshal(report)
	_, err = tx.ExecContext(ctx, `UPDATE app_reassessment_jobs SET report_json=$2::jsonb WHERE id=$1`, id, string(j))
	return err
}

func suggestedTypeTx(ctx context.Context, tx *sql.Tx, appUserID, cardID int64, roundNo, fallback int) (int, map[int]int, error) {
	rows, err := tx.QueryContext(ctx, `SELECT type_delta FROM app_daily_quiz_answers WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3`, appUserID, cardID, roundNo)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()
	counts := map[int]int{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return 0, nil, err
		}
		var delta map[string]int
		_ = json.Unmarshal(raw, &delta)
		for k, v := range delta {
			id, _ := strconv.Atoi(k)
			if id > 0 {
				counts[id] += v
			}
		}
	}
	best, score := fallback, -1
	for i := 1; i <= 9; i++ {
		if counts[i] > score {
			best, score = i, counts[i]
		}
	}
	if best <= 0 {
		best = 6
	}
	return best, counts, rows.Err()
}

func profileEvidenceTx(ctx context.Context, tx *sql.Tx, appUserID, cardID int64, roundNo int) ([]EvidenceSummary, int, int, int, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT source_type, evidence_text, type_scores
		FROM app_profile_evidence
		WHERE app_user_id=$1 AND card_id=$2 AND round_no=$3 AND status='active'
		ORDER BY confidence DESC, create_time DESC, id DESC
		LIMIT 12
	`, appUserID, cardID, roundNo)
	if err != nil {
		return nil, 0, 0, 0, err
	}
	defer rows.Close()
	out := make([]EvidenceSummary, 0, 12)
	chatCount, voiceCount, behaviorCount := 0, 0, 0
	for rows.Next() {
		var sourceType, text string
		var rawTypeScores []byte
		if err := rows.Scan(&sourceType, &text, &rawTypeScores); err != nil {
			return nil, 0, 0, 0, err
		}
		summary, ok := evidenceSummaryFromSource(sourceType, text)
		summary.typeScores = decodeEvidenceTypeScores(rawTypeScores)
		if !ok {
			continue
		}
		switch summary.Kind {
		case "chat":
			chatCount++
		case "voice_text", "voice_emotion":
			voiceCount++
		case "behavior":
			behaviorCount++
		}
		out = append(out, summary)
	}
	return out, chatCount, voiceCount, behaviorCount, rows.Err()
}

func evidenceSummaryFromSource(sourceType, text string) (EvidenceSummary, bool) {
	sourceType = strings.TrimSpace(sourceType)
	text = strings.TrimSpace(text)
	if sourceType == "" || text == "" {
		return EvidenceSummary{}, false
	}
	if len([]rune(text)) > 180 {
		text = string([]rune(text)[:180])
	}
	switch sourceType {
	case "chat":
		return EvidenceSummary{Kind: "chat", Label: "聊天", Text: text}, true
	case "voice_text", "voice_emotion":
		label := "语音"
		return EvidenceSummary{Kind: sourceType, Label: label, Text: text}, true
	case "behavior":
		return EvidenceSummary{Kind: "behavior", Label: "行为", Text: text}, true
	case "daily_quiz":
		return EvidenceSummary{Kind: "daily_quiz", Label: "每日题", Text: text}, true
	default:
		return EvidenceSummary{Kind: sourceType, Label: "辅助证据", Text: text}, true
	}
}

func privateSignalSummary(extracted []EvidenceSummary) string {
	seen := map[string]bool{}
	labels := make([]string, 0, 3)
	for _, item := range extracted {
		label := strings.TrimSpace(item.Label)
		if label == "" || seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return "本轮信号来自每日题和互动证据的综合观察"
	}
	return "本轮参考了每日题以及" + strings.Join(labels, "、") + "等私人证据的稳定信号"
}

func scanReport(row *sql.Row) (*Report, error) {
	var id, cardID int64
	var status string
	var oldType, suggested int
	var confidence float64
	var raw []byte
	if err := row.Scan(&id, &cardID, &status, &oldType, &suggested, &confidence, &raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var r Report
	_ = json.Unmarshal(raw, &r)
	if r.Summary == "" {
		r = buildReport(id, cardID, status, oldType, suggested, RoundAnswerTarget, nil, nil, 0.76)
	}
	r.ID = id
	r.CardID = cardID
	r.Status = status
	r.OldMainType = oldType
	r.SuggestedMainType = suggested
	r.Confidence = confidence
	return &r, nil
}

func combinedSuggestedType(dailySuggested int, dailyCounts map[int]int, evidence []EvidenceSummary) (int, float64) {
	scores := map[int]float64{}
	for typ, count := range dailyCounts {
		if typ > 0 {
			scores[typ] += float64(count)
		}
	}
	if dailySuggested > 0 {
		scores[dailySuggested] += 0.01
	}
	for _, item := range evidence {
		weight := evidenceWeight(item.Kind)
		for typ, value := range item.typeScores {
			if typ > 0 && value > 0 {
				scores[typ] += value * weight
			}
		}
		for typ, value := range inferredEvidenceTypeScores(item.Text) {
			scores[typ] += value * weight
		}
	}
	best := dailySuggested
	bestScore := scores[best]
	for typ := 1; typ <= 9; typ++ {
		if scores[typ] > bestScore {
			best = typ
			bestScore = scores[typ]
		}
	}
	if best <= 0 {
		best = 6
	}
	confidence := 0.76
	if len(evidence) > 0 {
		confidence += 0.03 * float64(len(evidence))
		if best != dailySuggested {
			confidence += 0.03
		}
	}
	if confidence > 0.92 {
		confidence = 0.92
	}
	return best, confidence
}

func evidenceWeight(kind string) float64 {
	switch kind {
	case "chat":
		return 16
	case "voice_text", "voice_emotion":
		return 14
	case "behavior":
		return 8
	default:
		return 6
	}
}

func inferredEvidenceTypeScores(text string) map[int]float64 {
	text = strings.ToLower(text)
	out := map[int]float64{}
	if strings.Contains(text, "担心") || strings.Contains(text, "害怕") || strings.Contains(text, "风险") || strings.Contains(text, "确认") || strings.Contains(text, "不稳定") || strings.Contains(text, "出错") || strings.Contains(text, "安全") {
		out[6] += 1.0
	}
	if strings.Contains(text, "关系") || strings.Contains(text, "照顾") || strings.Contains(text, "连接") || strings.Contains(text, "对方") {
		out[2] += 0.45
	}
	if strings.Contains(text, "行动") || strings.Contains(text, "推进") || strings.Contains(text, "目标") || strings.Contains(text, "完成") {
		out[3] += 0.35
	}
	return out
}

func decodeEvidenceTypeScores(raw []byte) map[int]float64 {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]float64
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	out := map[int]float64{}
	for key, value := range values {
		typ, err := strconv.Atoi(key)
		if err == nil && typ > 0 && value > 0 {
			out[typ] = value
		}
	}
	return out
}

func confidenceLabel(confidence float64) string {
	switch {
	case confidence >= 0.86:
		return "高"
	case confidence >= 0.72:
		return "中高"
	case confidence >= 0.55:
		return "中"
	default:
		return "观察中"
	}
}

func buildReport(id, cardID int64, status string, oldType, suggested, count int, _ map[int]int, extracted []EvidenceSummary, confidence float64) Report {
	if suggested <= 0 {
		suggested = 6
	}
	if confidence <= 0 {
		confidence = 0.76
	}
	evidence := []EvidenceSummary{{Kind: "daily_quiz", Label: "每日题", Text: fmt.Sprintf("当前轮累计 %d/100 道有效答案。", count)}}
	if len(extracted) > 0 {
		evidence = append(evidence, extracted...)
	} else {
		evidence = append(evidence,
			EvidenceSummary{Kind: "chat", Label: "聊天", Text: "汇总近期对话中的持续关注点和压力表达作为辅助证据。"},
			EvidenceSummary{Kind: "voice_text", Label: "语音", Text: "语音转文字中的情绪语义作为辅助观察。"},
			EvidenceSummary{Kind: "behavior", Label: "行为", Text: "参考连续完成校准题、打卡和互动稳定性。"},
		)
	}
	profile := buildProfile(suggested)
	if len(extracted) > 0 {
		profile.PrivateSignalSummary = privateSignalSummary(extracted)
	}
	return Report{ID: id, CardID: cardID, Status: status, OldMainType: oldType, SuggestedMainType: suggested, Confidence: confidence, ConfidenceLabel: confidenceLabel(confidence), Summary: fmt.Sprintf("最近 %d 道画像校准题显示 %d 号倾向增强，建议确认是否更新主画像。", count, suggested), Profile: profile, Reasons: []string{fmt.Sprintf("当前轮已完成 %d 道有效每日校准题。", count), "系统结合每日题、聊天表达、语音语义与行为稳定性生成本次建议。"}, Evidence: evidence}
}

func buildProfile(t int) ReportProfile {
	if t == 6 {
		return ReportProfile{MainType: 6, PrimaryMotivation: "希望安全、确定、可预期", StressPattern: "压力下容易反复确认和预判风险", RelationshipPattern: "关系中重视稳定、信任和共同面对问题", GrowthFocus: "把担心拆成事实、猜测和下一步行动", CommunicationAdvice: "直接说明担心点，并约定确认方式", PrivateSignalSummary: "本轮信号显示安全感、确认和风险预判相关表达较稳定"}
	}
	return ReportProfile{MainType: t, PrimaryMotivation: "希望更稳定地理解自己", StressPattern: "压力下会出现可识别的惯性反应", RelationshipPattern: "关系中呈现出稳定的互动偏好", GrowthFocus: "把觉察落实到下一步行动", CommunicationAdvice: "用具体事实表达感受和需求", PrivateSignalSummary: "本轮信号来自每日题、聊天、语音和行为的综合观察"}
}

func defaultQuestions() []Question {
	dims := []string{"security", "boundary", "emotion", "action", "relationship"}
	out := make([]Question, 0, 20)
	for i := 0; i < 20; i++ {
		a := (i % 9) + 1
		b := ((i + 5) % 9) + 1
		out = append(out, Question{Body: fmt.Sprintf("最近遇到压力时，你更接近下面哪一种反应？（%02d）", i+1), Dimension: dims[i%len(dims)], Options: []Option{{ID: "a", Label: "A", Text: "我会先确认风险和安全边界，再决定下一步。", TypeWeights: map[string]int{strconv.Itoa(a): 2, "6": 1}}, {ID: "b", Label: "B", Text: "我会先照顾关系里的感受和连接。", TypeWeights: map[string]int{strconv.Itoa(b): 2, "2": 1}}, {ID: "c", Label: "C", Text: "我会先行动，把事情推进到可控状态。", TypeWeights: map[string]int{"3": 1, "8": 1}}}})
	}
	return out
}

func decodeIDs(raw []byte) []int64 { var ids []int64; _ = json.Unmarshal(raw, &ids); return ids }
func containsID(ids []int64, target int64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
func businessDate() string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	return time.Now().In(loc).Format("2006-01-02")
}
func stableDateSeed(date string) int {
	seed := 0
	for _, r := range date {
		if r >= '0' && r <= '9' {
			seed = seed*10 + int(r-'0')
		}
	}
	return seed
}
