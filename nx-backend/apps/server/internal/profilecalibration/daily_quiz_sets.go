package profilecalibration

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// GetDailyQuizSet returns the retained daily set with current and historical
// slot versions for admin review.
func (s *Store) GetDailyQuizSet(ctx context.Context, date string) (DailyQuizSet, error) {
	date, err := normalizeQuizDate(date)
	if err != nil {
		return DailyQuizSet{}, err
	}
	set, err := s.scanDailyQuizSet(ctx, `
		SELECT id, quiz_date, status, source, model_provider, model_name, prompt, raw_response, question_ids,
		       error_message, generated_at, published_at, pushed_at
		FROM app_daily_quiz_sets
		WHERE quiz_date=$1::date
	`, date)
	if err != nil {
		return DailyQuizSet{}, err
	}
	return s.attachDailyQuizSetQuestions(ctx, set)
}

// GenerateDailyQuizSet ensures one retained 5-question set exists for date.
// Existing sets are returned unchanged to avoid mutating already visible
// questions; admins should use single-slot replacement for refinements.
func (s *Store) GenerateDailyQuizSet(ctx context.Context, date string) (DailyQuizSet, error) {
	return s.EnsureDailyQuizSet(ctx, date)
}

// EnsureDailyQuizSet creates the date's set before App users receive the noon
// push. It prefers model-generated questions and falls back to the local bank.
func (s *Store) EnsureDailyQuizSet(ctx context.Context, date string) (DailyQuizSet, error) {
	date, err := normalizeQuizDate(date)
	if err != nil {
		return DailyQuizSet{}, err
	}
	if s == nil || s.db == nil {
		return DailyQuizSet{}, ErrInvalidInput
	}
	if existing, err := s.GetDailyQuizSet(ctx, date); err == nil && len(existing.QuestionIDs) >= DailyQuestionCount {
		return existing, nil
	} else if err != nil && !errors.Is(err, ErrNotFound) {
		return DailyQuizSet{}, err
	}

	if s.dailyQuizGenerator != nil {
		generated, genErr := s.dailyQuizGenerator.GenerateDailyQuizQuestions(ctx, DailyQuizGenerationInput{Date: date, Count: DailyQuestionCount})
		if genErr == nil {
			if set, err := s.createDailyQuizSetFromGenerated(ctx, date, generated, "generated", "ai", ""); err == nil {
				return set, nil
			} else {
				genErr = err
			}
		}
		return s.createFallbackDailyQuizSet(ctx, date, genErr)
	}
	return s.createFallbackDailyQuizSet(ctx, date, nil)
}

func (s *Store) ReplaceDailyQuizQuestion(ctx context.Context, setID int64, slotNo int, reason, operator string) (DailyQuizSet, error) {
	if s == nil || s.db == nil || setID <= 0 || slotNo < 1 || slotNo > DailyQuestionCount {
		return DailyQuizSet{}, ErrInvalidInput
	}
	reason = strings.TrimSpace(reason)
	operator = strings.TrimSpace(operator)

	var generated DailyQuizGenerationResult
	var genErr error
	if s.dailyQuizGenerator != nil {
		generated, genErr = s.dailyQuizGenerator.GenerateDailyQuizQuestions(ctx, DailyQuizGenerationInput{
			Count:         1,
			SlotNo:        slotNo,
			ReplaceReason: reason,
		})
	}
	if genErr != nil || len(generated.Questions) == 0 {
		generated = fallbackGeneratedQuestion(slotNo, genErr)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DailyQuizSet{}, err
	}
	defer tx.Rollback()

	var (
		quizDate time.Time
		rawIDs   []byte
		pushedAt sql.NullTime
	)
	if err := tx.QueryRowContext(ctx, `
		SELECT quiz_date, question_ids, pushed_at
		FROM app_daily_quiz_sets
		WHERE id=$1
		FOR UPDATE
	`, setID).Scan(&quizDate, &rawIDs, &pushedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DailyQuizSet{}, ErrNotFound
		}
		return DailyQuizSet{}, err
	}
	ids := decodeIDs(rawIDs)
	if len(ids) < DailyQuestionCount {
		return DailyQuizSet{}, ErrInvalidStatus
	}
	var answered int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM app_daily_quiz_batches b
		WHERE b.quiz_date=$1::date
		  AND (b.answered_count > 0
		       OR EXISTS (SELECT 1 FROM app_daily_quiz_answers a WHERE a.batch_id=b.id))
	`, quizDate.Format("2006-01-02")).Scan(&answered); err != nil {
		return DailyQuizSet{}, err
	}
	if answered > 0 {
		return DailyQuizSet{}, ErrInvalidStatus
	}

	question, err := normalizeGeneratedQuestion(generated.Questions[0], slotNo)
	if err != nil {
		return DailyQuizSet{}, err
	}
	newQuestionID, err := insertDailyQuizQuestionTx(ctx, tx, question, slotNo)
	if err != nil {
		return DailyQuizSet{}, err
	}
	ids[slotNo-1] = newQuestionID
	idsJSON, _ := json.Marshal(ids)

	if _, err := tx.ExecContext(ctx, `
		UPDATE app_daily_quiz_question_versions
		SET is_active=false
		WHERE set_id=$1 AND slot_no=$2 AND is_active=true
	`, setID, slotNo); err != nil {
		return DailyQuizSet{}, err
	}
	var versionNo int
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_no),0)+1
		FROM app_daily_quiz_question_versions
		WHERE set_id=$1 AND slot_no=$2
	`, setID, slotNo).Scan(&versionNo); err != nil {
		return DailyQuizSet{}, err
	}
	if err := insertQuestionVersionTx(ctx, tx, setID, newQuestionID, slotNo, versionNo, question, generated, "admin_replace", operator, reason); err != nil {
		return DailyQuizSet{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_daily_quiz_sets
		SET question_ids=$2::jsonb,
		    source='admin_replace',
		    update_time=now()
		WHERE id=$1
	`, setID, string(idsJSON)); err != nil {
		return DailyQuizSet{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE app_daily_quiz_batches
		SET question_ids=$2::jsonb, update_time=now()
		WHERE quiz_date=$1::date
		  AND answered_count=0
		  AND completed=false
	`, quizDate.Format("2006-01-02"), string(idsJSON)); err != nil {
		return DailyQuizSet{}, err
	}
	if err := tx.Commit(); err != nil {
		return DailyQuizSet{}, err
	}
	return s.GetDailyQuizSet(ctx, quizDate.Format("2006-01-02"))
}

func (s *Store) MarkDailyQuizSetPushed(ctx context.Context, date string) error {
	date, err := normalizeQuizDate(date)
	if err != nil {
		return err
	}
	if s == nil || s.db == nil {
		return ErrInvalidInput
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE app_daily_quiz_sets
		SET status=CASE WHEN status='fallback' THEN status ELSE 'pushed' END,
		    pushed_at=COALESCE(pushed_at, now()),
		    update_time=now()
		WHERE quiz_date=$1::date
	`, date)
	return err
}

func (s *Store) createFallbackDailyQuizSet(ctx context.Context, date string, cause error) (DailyQuizSet, error) {
	if err := s.EnsureDefaultQuestions(ctx); err != nil {
		return DailyQuizSet{}, err
	}
	ids, err := s.selectFallbackQuestionIDs(ctx, 0, date)
	if err != nil {
		return DailyQuizSet{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DailyQuizSet{}, err
	}
	defer tx.Rollback()

	idsJSON, _ := json.Marshal(ids)
	errorMessage := ""
	if cause != nil {
		errorMessage = cause.Error()
	}
	var setID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app_daily_quiz_sets (quiz_date, status, source, question_ids, error_message, generated_at, published_at)
		VALUES ($1::date,'fallback','fallback',$2::jsonb,$3,now(),now())
		ON CONFLICT (quiz_date) DO NOTHING
		RETURNING id
	`, date, string(idsJSON), errorMessage).Scan(&setID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			return s.GetDailyQuizSet(ctx, date)
		}
		return DailyQuizSet{}, err
	}
	if err := insertVersionsForExistingQuestionsTx(ctx, tx, setID, ids, DailyQuizGenerationResult{Source: "fallback"}, "fallback"); err != nil {
		return DailyQuizSet{}, err
	}
	if err := tx.Commit(); err != nil {
		return DailyQuizSet{}, err
	}
	return s.GetDailyQuizSet(ctx, date)
}

func (s *Store) createDailyQuizSetFromGenerated(ctx context.Context, date string, generated DailyQuizGenerationResult, status, source, errorMessage string) (DailyQuizSet, error) {
	if len(generated.Questions) < DailyQuestionCount {
		return DailyQuizSet{}, fmt.Errorf("profilecalibration: generated question count less than %d", DailyQuestionCount)
	}
	if source == "" {
		source = strings.TrimSpace(generated.Source)
	}
	if source == "" {
		source = "ai"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DailyQuizSet{}, err
	}
	defer tx.Rollback()

	ids := make([]int64, 0, DailyQuestionCount)
	normalized := make([]GeneratedDailyQuizQuestion, 0, DailyQuestionCount)
	for i := 0; i < DailyQuestionCount; i++ {
		q, err := normalizeGeneratedQuestion(generated.Questions[i], i+1)
		if err != nil {
			return DailyQuizSet{}, err
		}
		id, err := insertDailyQuizQuestionTx(ctx, tx, q, i+1)
		if err != nil {
			return DailyQuizSet{}, err
		}
		ids = append(ids, id)
		normalized = append(normalized, q)
	}
	idsJSON, _ := json.Marshal(ids)
	var setID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO app_daily_quiz_sets (quiz_date, status, source, model_provider, model_name, prompt, raw_response, question_ids, error_message, generated_at, published_at)
		VALUES ($1::date,$2,$3,$4,$5,$6,$7,$8::jsonb,$9,now(),now())
		ON CONFLICT (quiz_date) DO NOTHING
		RETURNING id
	`, date, status, source, generated.ModelProvider, generated.ModelName, generated.Prompt, generated.RawResponse, string(idsJSON), errorMessage).Scan(&setID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			_ = tx.Rollback()
			return s.GetDailyQuizSet(ctx, date)
		}
		return DailyQuizSet{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE app_daily_quiz_question_versions SET is_active=false WHERE set_id=$1`, setID); err != nil {
		return DailyQuizSet{}, err
	}
	for i, q := range normalized {
		versionNo, err := nextQuestionVersionNoTx(ctx, tx, setID, i+1)
		if err != nil {
			return DailyQuizSet{}, err
		}
		if err := insertQuestionVersionTx(ctx, tx, setID, ids[i], i+1, versionNo, q, generated, source, "", ""); err != nil {
			return DailyQuizSet{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return DailyQuizSet{}, err
	}
	return s.GetDailyQuizSet(ctx, date)
}

func (s *Store) scanDailyQuizSet(ctx context.Context, query string, args ...any) (DailyQuizSet, error) {
	var (
		set         DailyQuizSet
		quizDate    time.Time
		rawIDs      []byte
		generatedAt sql.NullTime
		publishedAt sql.NullTime
		pushedAt    sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&set.ID,
		&quizDate,
		&set.Status,
		&set.Source,
		&set.ModelProvider,
		&set.ModelName,
		&set.Prompt,
		&set.RawResponse,
		&rawIDs,
		&set.ErrorMessage,
		&generatedAt,
		&publishedAt,
		&pushedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return DailyQuizSet{}, ErrNotFound
	}
	if err != nil {
		return DailyQuizSet{}, err
	}
	set.Date = quizDate.Format("2006-01-02")
	set.QuestionIDs = decodeIDs(rawIDs)
	set.GeneratedAt = formatDailyQuizSetTime(generatedAt)
	set.PublishedAt = formatDailyQuizSetTime(publishedAt)
	set.PushedAt = formatDailyQuizSetTime(pushedAt)
	return set, nil
}

func (s *Store) attachDailyQuizSetQuestions(ctx context.Context, set DailyQuizSet) (DailyQuizSet, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, question_id, slot_no, version_no, is_active, body, options, dimension, source,
		       model_provider, model_name, operator, replace_reason, create_time
		FROM app_daily_quiz_question_versions
		WHERE set_id=$1
		ORDER BY slot_no ASC, version_no DESC
	`, set.ID)
	if err != nil {
		return DailyQuizSet{}, err
	}
	defer rows.Close()
	items := make([]DailyQuizQuestionVersion, 0)
	for rows.Next() {
		var (
			item       DailyQuizQuestionVersion
			rawOptions []byte
			createTime time.Time
		)
		item.SetID = set.ID
		if err := rows.Scan(
			&item.ID,
			&item.QuestionID,
			&item.SlotNo,
			&item.VersionNo,
			&item.IsActive,
			&item.Question.Body,
			&rawOptions,
			&item.Question.Dimension,
			&item.Source,
			&item.ModelProvider,
			&item.ModelName,
			&item.Operator,
			&item.ReplaceReason,
			&createTime,
		); err != nil {
			return DailyQuizSet{}, err
		}
		item.Question.ID = item.QuestionID
		_ = json.Unmarshal(rawOptions, &item.Question.Options)
		item.CreateTime = createTime.Format("2006/01/02 15:04:05")
		_ = s.db.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM app_daily_quiz_answers a
			JOIN app_daily_quiz_batches b ON b.id=a.batch_id
			WHERE b.quiz_date=$1::date AND a.question_id=$2
		`, set.Date, item.QuestionID).Scan(&item.AnsweredCount)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return DailyQuizSet{}, err
	}
	set.Questions = items
	return set, nil
}

func insertDailyQuizQuestionTx(ctx context.Context, tx *sql.Tx, q GeneratedDailyQuizQuestion, slotNo int) (int64, error) {
	optionsJSON, _ := json.Marshal(q.Options)
	weightsJSON, _ := json.Marshal(q.TypeWeights)
	var id int64
	err := tx.QueryRowContext(ctx, `
		INSERT INTO app_daily_quiz_questions (sort, body, options, dimension, type_weights, status)
		VALUES ($1,$2,$3::jsonb,$4,$5::jsonb,'active')
		RETURNING id
	`, slotNo*10, q.Body, string(optionsJSON), q.Dimension, string(weightsJSON)).Scan(&id)
	return id, err
}

func insertVersionsForExistingQuestionsTx(ctx context.Context, tx *sql.Tx, setID int64, ids []int64, generated DailyQuizGenerationResult, source string) error {
	if _, err := tx.ExecContext(ctx, `UPDATE app_daily_quiz_question_versions SET is_active=false WHERE set_id=$1`, setID); err != nil {
		return err
	}
	for i, id := range ids {
		q, err := generatedQuestionFromExistingTx(ctx, tx, id)
		if err != nil {
			return err
		}
		versionNo, err := nextQuestionVersionNoTx(ctx, tx, setID, i+1)
		if err != nil {
			return err
		}
		if err := insertQuestionVersionTx(ctx, tx, setID, id, i+1, versionNo, q, generated, source, "", ""); err != nil {
			return err
		}
	}
	return nil
}

func nextQuestionVersionNoTx(ctx context.Context, tx *sql.Tx, setID int64, slotNo int) (int, error) {
	var versionNo int
	err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(version_no),0)+1
		FROM app_daily_quiz_question_versions
		WHERE set_id=$1 AND slot_no=$2
	`, setID, slotNo).Scan(&versionNo)
	return versionNo, err
}

func generatedQuestionFromExistingTx(ctx context.Context, tx *sql.Tx, id int64) (GeneratedDailyQuizQuestion, error) {
	var q GeneratedDailyQuizQuestion
	var rawOptions, rawWeights []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT body, dimension, options, type_weights
		FROM app_daily_quiz_questions
		WHERE id=$1 AND status='active'
	`, id).Scan(&q.Body, &q.Dimension, &rawOptions, &rawWeights); err != nil {
		return q, err
	}
	_ = json.Unmarshal(rawOptions, &q.Options)
	_ = json.Unmarshal(rawWeights, &q.TypeWeights)
	return q, nil
}

func insertQuestionVersionTx(ctx context.Context, tx *sql.Tx, setID, questionID int64, slotNo, versionNo int, q GeneratedDailyQuizQuestion, generated DailyQuizGenerationResult, source, operator, reason string) error {
	optionsJSON, _ := json.Marshal(q.Options)
	weightsJSON, _ := json.Marshal(q.TypeWeights)
	if source == "" {
		source = strings.TrimSpace(generated.Source)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO app_daily_quiz_question_versions
			(set_id, question_id, slot_no, version_no, is_active, body, options, dimension, type_weights,
			 source, model_provider, model_name, prompt, raw_response, operator, replace_reason)
		VALUES ($1,$2,$3,$4,true,$5,$6::jsonb,$7,$8::jsonb,$9,$10,$11,$12,$13,$14,$15)
	`, setID, questionID, slotNo, versionNo, q.Body, string(optionsJSON), q.Dimension, string(weightsJSON),
		source, generated.ModelProvider, generated.ModelName, generated.Prompt, generated.RawResponse, operator, reason)
	return err
}

func normalizeGeneratedQuestion(input GeneratedDailyQuizQuestion, slotNo int) (GeneratedDailyQuizQuestion, error) {
	q := GeneratedDailyQuizQuestion{
		Body:        strings.TrimSpace(input.Body),
		Dimension:   strings.TrimSpace(input.Dimension),
		Options:     append([]Option(nil), input.Options...),
		TypeWeights: input.TypeWeights,
	}
	if q.Body == "" {
		return q, ErrInvalidInput
	}
	if q.Dimension == "" {
		q.Dimension = "self_awareness"
	}
	if len(q.Options) < 3 {
		return q, ErrInvalidInput
	}
	if len(q.Options) > 4 {
		q.Options = q.Options[:4]
	}
	if q.TypeWeights == nil {
		q.TypeWeights = map[string]int{strconv.Itoa(((slotNo - 1) % 9) + 1): 1}
	}
	for i := range q.Options {
		q.Options[i].ID = strings.TrimSpace(q.Options[i].ID)
		if q.Options[i].ID == "" {
			q.Options[i].ID = string(rune('a' + i))
		}
		q.Options[i].Label = strings.TrimSpace(q.Options[i].Label)
		if q.Options[i].Label == "" {
			q.Options[i].Label = strings.ToUpper(q.Options[i].ID)
		}
		q.Options[i].Text = strings.TrimSpace(q.Options[i].Text)
		if q.Options[i].Text == "" {
			return q, ErrInvalidInput
		}
		if len(q.Options[i].TypeWeights) == 0 && len(q.Options[i].Weights) > 0 {
			q.Options[i].TypeWeights = q.Options[i].Weights
		}
		if len(q.Options[i].TypeWeights) == 0 {
			q.Options[i].TypeWeights = map[string]int{strconv.Itoa(((slotNo + i - 1) % 9) + 1): 1}
		}
		q.Options[i].Weights = nil
	}
	return q, nil
}

func fallbackGeneratedQuestion(slotNo int, cause error) DailyQuizGenerationResult {
	defaults := defaultQuestions()
	index := (slotNo - 1) % len(defaults)
	if index < 0 {
		index = 0
	}
	q := defaults[index]
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	return DailyQuizGenerationResult{
		Questions: []GeneratedDailyQuizQuestion{{
			Body:        q.Body,
			Dimension:   q.Dimension,
			Options:     q.Options,
			TypeWeights: map[string]int{strconv.Itoa(((slotNo - 1) % 9) + 1): 1},
		}},
		Source:      "fallback",
		RawResponse: reason,
	}
}

func normalizeQuizDate(date string) (string, error) {
	date = strings.TrimSpace(date)
	if date == "" {
		date = businessDate()
	}
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		return "", ErrInvalidInput
	}
	return date, nil
}

func formatDailyQuizSetTime(value sql.NullTime) string {
	if !value.Valid || value.Time.IsZero() {
		return ""
	}
	return value.Time.Format("2006/01/02 15:04:05")
}
