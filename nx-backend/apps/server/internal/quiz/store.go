package quiz

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCardLimit 副卡数量超出会员权益时返回，handler 据此提示升级。
var ErrCardLimit = errors.New("quiz: secondary card limit reached")

// ErrNotFound 卡片不存在或不属于当前用户。
var ErrNotFound = errors.New("quiz: not found")

// Option 题目选项。weights 仅在后台/算分场景填充；App 端返回时会被剥离（防作弊）。
type Option struct {
	ID      string      `json:"id"`
	Text    string      `json:"text"`
	Weights map[int]int `json:"weights,omitempty"`
}

// Question 题目。App 端 options 不含权重，后台含权重。
type Question struct {
	ID          int64    `json:"id"`
	Sort        int      `json:"sort"`
	Body        string   `json:"body"`
	Options     []Option `json:"options"`
	Dimension   string   `json:"dimension"`
	Status      string   `json:"status"`
	QuizVersion string   `json:"quizVersion"`
}

// AnswerItem 客户端作答：只提交选了哪道题的哪个选项，分值由服务端按题库权重计算。
type AnswerItem struct {
	QuestionID int64  `json:"questionId"`
	OptionID   string `json:"optionId"`
}

// SubmitInput 提交测试。gender 可选（male/female），缺省按中性 1.0 加权。
type SubmitInput struct {
	Answers []AnswerItem `json:"answers"`
	Gender  string       `json:"gender"`
}

// Submission 一次测试记录。
type Submission struct {
	ID            int64           `json:"id"`
	AppUserID     int64           `json:"appUserId"`
	Answers       json.RawMessage `json:"answers"`
	Result        json.RawMessage `json:"result"`
	PrimaryType   int             `json:"primaryType"`
	SecondType    int             `json:"secondType"`
	WingType      int             `json:"wingType"`
	Gender        string          `json:"gender"`
	QuizVersion   string          `json:"quizVersion"`
	Score         json.RawMessage `json:"score"`
	AdjustedScore json.RawMessage `json:"adjustedScore"`
	Centers       json.RawMessage `json:"centers"`
	CreateTime    string          `json:"createTime"`
}

// Card 用户卡片。DB 列名为 enneagram/wing，对外 JSON 暴露为 mainType/wingType。
type Card struct {
	ID         int64           `json:"id"`
	AppUserID  int64           `json:"appUserId"`
	CardType   string          `json:"cardType"`
	Name       string          `json:"name"`
	Relation   string          `json:"relation"`
	MainType   int             `json:"mainType"`
	WingType   int             `json:"wingType"`
	Profile    json.RawMessage `json:"profile"`
	Status     string          `json:"status"`
	CreateTime string          `json:"createTime"`
	UpdateTime string          `json:"updateTime"`
}

// SubmitResult 提交测试后的返回：落库的 submission + upsert 后的主卡 + 画像。
// CardInput 用于创建/更新副卡的客户端请求体。
type CardInput struct {
	Name     string `json:"name"`
	Relation string `json:"relation"`
	MainType int    `json:"mainType"`
	WingType int    `json:"wingType"`
}

// QuestionInput 用于后台新建/更新题目。
type QuestionInput struct {
	Sort      int      `json:"sort"`
	Body      string   `json:"body"`
	Options   []Option `json:"options"`
	Dimension string   `json:"dimension"`
	Status    string   `json:"status"`
}

type SubmitResult struct {
	Submission Submission `json:"submission"`
	Card       Card       `json:"card"`
	Persona    Persona    `json:"persona"`
}

const quizVersion = "v1"

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

// stripWeights 返回不含权重的 options 副本，用于 App 端防作弊。
func stripWeights(opts []Option) []Option {
	out := make([]Option, len(opts))
	for i, o := range opts {
		out[i] = Option{ID: o.ID, Text: o.Text}
	}
	return out
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanQuestionFrom(row scanner) (Question, error) {
	var q Question
	var optsRaw []byte
	if err := row.Scan(&q.ID, &q.Sort, &q.Body, &optsRaw, &q.Dimension, &q.Status, &q.QuizVersion); err != nil {
		return q, err
	}
	if len(optsRaw) > 0 {
		if err := json.Unmarshal(optsRaw, &q.Options); err != nil {
			return q, fmt.Errorf("quiz: decode options: %w", err)
		}
	}
	return q, nil
}

func scanQuestion(rows *sql.Rows) (Question, error) { return scanQuestionFrom(rows) }

func scanQuestionRow(row *sql.Row) (Question, error) { return scanQuestionFrom(row) }

const questionCols = `id, sort, body, options, dimension, status, quiz_version`

// ListQuestions 返回启用中的题目（App 端，剥离权重防作弊）。
func (s *Store) ListQuestions(ctx context.Context) ([]Question, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+questionCols+` FROM app_quiz_questions WHERE status = 'enabled' ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Question
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		q.Options = stripWeights(q.Options)
		out = append(out, q)
	}
	return out, rows.Err()
}

// ListQuestionsAdmin 返回全部题目（后台，含权重）。
func (s *Store) ListQuestionsAdmin(ctx context.Context) ([]Question, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+questionCols+` FROM app_quiz_questions ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Question
	for rows.Next() {
		q, err := scanQuestion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, q)
	}
	return out, rows.Err()
}

// Submit 提交一次测试：从题库按选项权重累加 rawScore（不信任客户端提交的分值），
// 算分 + 构建画像，落库 submission 并 upsert 主卡，事务保证一致性。
func (s *Store) Submit(ctx context.Context, appUserID int64, in SubmitInput) (SubmitResult, error) {
	var res SubmitResult
	if len(in.Answers) == 0 {
		return res, errors.New("quiz: answers required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return res, err
	}
	defer tx.Rollback()

	// 加载启用题库的选项权重：questionID -> optionID -> map[typeID]weight
	rows, err := tx.QueryContext(ctx,
		`SELECT id, options FROM app_quiz_questions WHERE status = 'enabled'`)
	if err != nil {
		return res, err
	}
	weights := map[int64]map[string]map[int]int{}
	for rows.Next() {
		var qid int64
		var optsRaw []byte
		if err := rows.Scan(&qid, &optsRaw); err != nil {
			rows.Close()
			return res, err
		}
		var opts []Option
		if len(optsRaw) > 0 {
			if err := json.Unmarshal(optsRaw, &opts); err != nil {
				rows.Close()
				return res, fmt.Errorf("quiz: decode options: %w", err)
			}
		}
		m := map[string]map[int]int{}
		for _, o := range opts {
			m[o.ID] = o.Weights
		}
		weights[qid] = m
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return res, err
	}
	rows.Close()

	rawScore, err := validateAnswers(in.Answers, weights)
	if err != nil {
		return res, err
	}

	sr := calcType(rawScore, in.Gender)
	persona := buildPersona(sr.Type, sr.Second, sr.Wing, sr.Centers, in.Gender)

	answersJSON, _ := json.Marshal(in.Answers)
	personaJSON, _ := json.Marshal(persona)
	scoreJSON, _ := json.Marshal(sr.Score)
	adjJSON, _ := json.Marshal(sr.Adjusted)
	centersJSON, _ := json.Marshal(sr.Centers)

	var sub Submission
	sub.AppUserID = appUserID
	sub.Answers = answersJSON
	sub.Result = personaJSON
	sub.PrimaryType = sr.Type
	sub.SecondType = sr.Second
	sub.WingType = sr.Wing
	sub.Gender = in.Gender
	sub.QuizVersion = quizVersion
	sub.Score = scoreJSON
	sub.AdjustedScore = adjJSON
	sub.Centers = centersJSON

	var submissionCreateTime time.Time
	err = tx.QueryRowContext(ctx,
		`INSERT INTO app_quiz_submissions
		 (app_user_id, answers, result, primary_type, second_type, wing_type,
		  gender, quiz_version, score, adjusted_score, centers)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		 RETURNING id, create_time`,
		appUserID, answersJSON, personaJSON, sr.Type, sr.Second, sr.Wing,
		in.Gender, quizVersion, scoreJSON, adjJSON, centersJSON,
	).Scan(&sub.ID, &submissionCreateTime)
	if err != nil {
		return res, fmt.Errorf("quiz: insert submission: %w", err)
	}
	sub.CreateTime = formatTime(submissionCreateTime)

	// upsert 主卡：每个用户唯一一张 active 主卡（schema 部分唯一索引保证）。
	card, err := s.upsertPrimaryCard(ctx, tx, appUserID, sub.ID, sr.Type, sr.Wing, personaJSON)
	if err != nil {
		return res, err
	}

	if err := tx.Commit(); err != nil {
		return res, err
	}

	res.Submission = sub
	res.Card = card
	res.Persona = persona
	return res, nil
}

func validateAnswers(answers []AnswerItem, weights map[int64]map[string]map[int]int) (map[int]int, error) {
	if len(answers) == 0 {
		return nil, errors.New("quiz: answers required")
	}
	rawScore := map[int]int{}
	seen := map[int64]bool{}
	for _, a := range answers {
		if a.QuestionID <= 0 || strings.TrimSpace(a.OptionID) == "" {
			return nil, errors.New("quiz: invalid answer")
		}
		if seen[a.QuestionID] {
			return nil, errors.New("quiz: duplicate question")
		}
		seen[a.QuestionID] = true
		opts, ok := weights[a.QuestionID]
		if !ok {
			return nil, errors.New("quiz: unknown question")
		}
		w, ok := opts[a.OptionID]
		if !ok || len(w) == 0 {
			return nil, errors.New("quiz: unknown option")
		}
		for typeID, val := range w {
			rawScore[typeID] += val
		}
	}
	if len(rawScore) == 0 {
		return nil, errors.New("quiz: no valid score")
	}
	return rawScore, nil
}

// upsertPrimaryCard 在事务内创建或更新用户主卡。
func (s *Store) upsertPrimaryCard(ctx context.Context, tx *sql.Tx, appUserID, submissionID int64, mainType, wingType int, profile json.RawMessage) (Card, error) {
	var c Card
	var createTime, updateTime time.Time
	err := tx.QueryRowContext(ctx,
		`INSERT INTO app_user_cards
		 (app_user_id, card_type, name, relation, enneagram, wing, profile, status, submission_id)
		 VALUES ($1,'primary','本人','self',$2,$3,$4,'active',$5)
		 ON CONFLICT (app_user_id) WHERE card_type='primary' AND status='active'
		 DO UPDATE SET enneagram=EXCLUDED.enneagram, wing=EXCLUDED.wing,
		   profile=EXCLUDED.profile, submission_id=EXCLUDED.submission_id, update_time=now()
		 RETURNING id, app_user_id, card_type, name, relation, enneagram, wing, profile, status, create_time, update_time`,
		appUserID, mainType, wingType, profile, submissionID,
	).Scan(&c.ID, &c.AppUserID, &c.CardType, &c.Name, &c.Relation, &c.MainType, &c.WingType, &c.Profile, &c.Status, &createTime, &updateTime)
	if err != nil {
		return c, fmt.Errorf("quiz: upsert primary card: %w", err)
	}
	c.CreateTime = formatTime(createTime)
	c.UpdateTime = formatTime(updateTime)
	return c, nil
}

const cardCols = `id, app_user_id, card_type, name, relation, enneagram, wing, profile, status, create_time, update_time`

func scanCard(row interface{ Scan(...interface{}) error }) (Card, error) {
	var c Card
	var createTime, updateTime time.Time
	err := row.Scan(&c.ID, &c.AppUserID, &c.CardType, &c.Name, &c.Relation,
		&c.MainType, &c.WingType, &c.Profile, &c.Status, &createTime, &updateTime)
	c.CreateTime = formatTime(createTime)
	c.UpdateTime = formatTime(updateTime)
	return c, err
}

// LatestSubmission 返回用户最近一次测试结果，无记录返回 ErrNotFound。
func (s *Store) LatestSubmission(ctx context.Context, appUserID int64) (Submission, error) {
	var sub Submission
	var createTime time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, answers, result, primary_type, second_type, wing_type,
		        gender, quiz_version, score, adjusted_score, centers, create_time
		 FROM app_quiz_submissions WHERE app_user_id = $1
		 ORDER BY create_time DESC, id DESC LIMIT 1`,
		appUserID,
	).Scan(&sub.ID, &sub.AppUserID, &sub.Answers, &sub.Result, &sub.PrimaryType,
		&sub.SecondType, &sub.WingType, &sub.Gender, &sub.QuizVersion,
		&sub.Score, &sub.AdjustedScore, &sub.Centers, &createTime)
	if errors.Is(err, sql.ErrNoRows) {
		return sub, ErrNotFound
	}
	if err != nil {
		return sub, err
	}
	sub.CreateTime = formatTime(createTime)
	return sub, nil
}

// ListCards 返回用户全部 active 卡片，主卡在前。
func (s *Store) ListCards(ctx context.Context, appUserID int64) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cardCols+` FROM app_user_cards
		 WHERE app_user_id = $1 AND status = 'active'
		 ORDER BY CASE WHEN card_type='primary' THEN 0 ELSE 1 END, create_time`,
		appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// PrimaryCard 返回用户主卡，无则 ErrNotFound。
func (s *Store) PrimaryCard(ctx context.Context, appUserID int64) (Card, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`SELECT `+cardCols+` FROM app_user_cards
		 WHERE app_user_id = $1 AND card_type='primary' AND status='active' LIMIT 1`,
		appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// secondaryLimit 根据会员等级返回副卡上限：免费 1 张，会员 5 张。
func secondaryLimit(memberLevel string) int {
	if memberLevel == "free" || memberLevel == "" {
		return 1
	}
	return 5
}

// CreateCard 创建副卡，超出会员配额返回 ErrCardLimit。
func (s *Store) CreateCard(ctx context.Context, appUserID int64, memberLevel string, in CardInput) (Card, error) {
	var c Card
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return c, err
	}
	defer tx.Rollback()

	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT count(*) FROM app_user_cards
		 WHERE app_user_id = $1 AND card_type='secondary' AND status='active'`,
		appUserID).Scan(&count); err != nil {
		return c, err
	}
	if count >= secondaryLimit(memberLevel) {
		return c, ErrCardLimit
	}

	persona := buildPersona(in.MainType, 0, in.WingType, nil, "")
	profileJSON, _ := json.Marshal(persona)

	c, err = scanCard(tx.QueryRowContext(ctx,
		`INSERT INTO app_user_cards
		 (app_user_id, card_type, name, relation, enneagram, wing, profile, status)
		 VALUES ($1,'secondary',$2,$3,$4,$5,$6,'active')
		 RETURNING `+cardCols,
		appUserID, in.Name, in.Relation, in.MainType, in.WingType, profileJSON))
	if err != nil {
		return c, fmt.Errorf("quiz: insert card: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return c, err
	}
	return c, nil
}

// GetCard 按 id+用户 返回卡片，防止越权访问。
func (s *Store) GetCard(ctx context.Context, appUserID, cardID int64) (Card, error) {
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`SELECT `+cardCols+` FROM app_user_cards
		 WHERE id = $1 AND app_user_id = $2 AND status='active'`,
		cardID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// UpdateCard 更新副卡（仅 name/relation/type），按 id+用户 校验越权。
func (s *Store) UpdateCard(ctx context.Context, appUserID, cardID int64, in CardInput) (Card, error) {
	persona := buildPersona(in.MainType, 0, in.WingType, nil, "")
	profileJSON, _ := json.Marshal(persona)
	c, err := scanCard(s.db.QueryRowContext(ctx,
		`UPDATE app_user_cards
		 SET name=$1, relation=$2, enneagram=$3, wing=$4, profile=$5, update_time=now()
		 WHERE id=$6 AND app_user_id=$7 AND card_type='secondary' AND status='active'
		 RETURNING `+cardCols,
		in.Name, in.Relation, in.MainType, in.WingType, profileJSON, cardID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// DeleteCard 软删除副卡（主卡不可删），按 id+用户 校验越权。
func (s *Store) DeleteCard(ctx context.Context, appUserID, cardID int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE app_user_cards SET status='deleted', update_time=now()
		 WHERE id=$1 AND app_user_id=$2 AND card_type='secondary' AND status='active'`,
		cardID, appUserID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateQuestion 后台新建题目。
func (s *Store) CreateQuestion(ctx context.Context, in QuestionInput) (Question, error) {
	var q Question
	optsJSON, err := json.Marshal(in.Options)
	if err != nil {
		return q, err
	}
	status := in.Status
	if status == "" {
		status = "enabled"
	}
	q, err = scanQuestionRow(s.db.QueryRowContext(ctx,
		`INSERT INTO app_quiz_questions (sort, body, options, dimension, status, quiz_version)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 RETURNING `+questionCols,
		in.Sort, in.Body, optsJSON, in.Dimension, status, quizVersion))
	return q, err
}

// UpdateQuestion 后台更新题目。
func (s *Store) UpdateQuestion(ctx context.Context, id int64, in QuestionInput) (Question, error) {
	var q Question
	optsJSON, err := json.Marshal(in.Options)
	if err != nil {
		return q, err
	}
	status := in.Status
	if status == "" {
		status = "enabled"
	}
	q, err = scanQuestionRow(s.db.QueryRowContext(ctx,
		`UPDATE app_quiz_questions
		 SET sort=$1, body=$2, options=$3, dimension=$4, status=$5, update_time=now()
		 WHERE id=$6
		 RETURNING `+questionCols,
		in.Sort, in.Body, optsJSON, in.Dimension, status, id))
	if errors.Is(err, sql.ErrNoRows) {
		return q, ErrNotFound
	}
	return q, err
}

// DeleteQuestion 后台删除题目（软删除：置为 disabled）。
func (s *Store) DeleteQuestion(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE app_quiz_questions SET status='disabled', update_time=now() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListCardsAdmin 后台查看某用户的所有卡片（含已删除）。
func (s *Store) ListCardsAdmin(ctx context.Context, appUserID int64) ([]Card, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+cardCols+` FROM app_user_cards
		 WHERE app_user_id = $1
		 ORDER BY CASE WHEN card_type='primary' THEN 0 ELSE 1 END, create_time`,
		appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
