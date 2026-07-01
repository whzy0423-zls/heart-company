package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrNotFound = errors.New("chat: not found")

type Session struct {
	ID         int64  `json:"id"`
	AppUserID  int64  `json:"appUserId"`
	CardID     int64  `json:"cardId"`
	Title      string `json:"title"`
	UpdatedAt  string `json:"updatedAt"`
	CreateTime string `json:"createTime"`
}

type Message struct {
	ID         int64           `json:"id"`
	SessionID  int64           `json:"sessionId"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Sources    json.RawMessage `json:"sources"`
	Favorite   bool            `json:"favorite"`
	Feedback   string          `json:"feedback"`
	CreateTime string          `json:"createTime"`
}

type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func scanSession(row interface{ Scan(...interface{}) error }) (Session, error) {
	var s Session
	var updatedAt, createTime time.Time
	err := row.Scan(&s.ID, &s.AppUserID, &s.CardID, &s.Title, &updatedAt, &createTime)
	s.UpdatedAt = formatTime(updatedAt)
	s.CreateTime = formatTime(createTime)
	return s, err
}

// ListSessions 返回用户所有会话（按最近更新倒序）。
func (s *Store) ListSessions(ctx context.Context, appUserID int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE app_user_id = $1
		 ORDER BY updated_at DESC`, appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		sess, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, sess)
	}
	return out, rows.Err()
}

// GetOrCreateSession 找到 card 的最近会话，若无则新建。
func (s *Store) GetOrCreateSession(ctx context.Context, appUserID, cardID int64) (Session, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE app_user_id = $1 AND card_id = $2
		 ORDER BY updated_at DESC LIMIT 1`,
		appUserID, cardID,
	))
	if err == nil {
		return sess, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return sess, err
	}
	// 新建
	sess, err = scanSession(s.db.QueryRowContext(ctx,
		`INSERT INTO app_chat_sessions (app_user_id, card_id) VALUES ($1, $2)
		 RETURNING id, app_user_id, card_id, title, updated_at, create_time`,
		appUserID, cardID))
	return sess, err
}

// GetSession 按 id+用户 返回会话，防越权。
func (s *Store) GetSession(ctx context.Context, appUserID, sessionID int64) (Session, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE id = $1 AND app_user_id = $2`,
		sessionID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return sess, ErrNotFound
	}
	return sess, err
}

// ListMessages 返回会话的全部消息（按时间正序）。
func (s *Store) ListMessages(ctx context.Context, sessionID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, sources, favorite, feedback, create_time
		 FROM app_chat_messages WHERE session_id = $1 ORDER BY create_time, id`,
		sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		var createTime time.Time
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Role, &m.Content, &m.Sources, &m.Favorite, &m.Feedback, &createTime); err != nil {
			return nil, err
		}
		m.CreateTime = formatTime(createTime)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SavePair 在事务中保存用户消息 + AI回答，并刷新 session.updated_at。
// 返回 AI 回答的消息 id，供反馈 / 收藏定位。
func (s *Store) SavePair(ctx context.Context, sessionID int64, question, answer string, sources json.RawMessage) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	emptySources := json.RawMessage("[]")
	if sources == nil {
		sources = emptySources
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO app_chat_messages (session_id, role, content, sources) VALUES ($1,'user',$2,'[]')`,
		sessionID, question)
	if err != nil {
		return 0, err
	}
	var assistantID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages (session_id, role, content, sources) VALUES ($1,'assistant',$2,$3)
		 RETURNING id`,
		sessionID, answer, sources).Scan(&assistantID)
	if err != nil {
		return 0, err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return assistantID, nil
}

// SetFeedback 设置某条 AI 消息的反馈（'helpful' | 'inaccurate' | 'continue' | ”）。
// 通过 session→app_user_id 联结校验归属，防越权。
func (s *Store) SetFeedback(ctx context.Context, appUserID, messageID int64, feedback string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE app_chat_messages m SET feedback = $3
		 FROM app_chat_sessions s
		 WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND m.role = 'assistant'`,
		messageID, appUserID, feedback)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ToggleFavorite 切换某条 AI 消息的收藏状态，返回切换后的状态。
func (s *Store) ToggleFavorite(ctx context.Context, appUserID, messageID int64) (bool, error) {
	var favorite bool
	err := s.db.QueryRowContext(ctx,
		`UPDATE app_chat_messages m SET favorite = NOT m.favorite
		 FROM app_chat_sessions s
		 WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND m.role = 'assistant'
		 RETURNING m.favorite`,
		messageID, appUserID).Scan(&favorite)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return favorite, err
}

// FavoriteItem 收藏项：AI 消息 + 其所属会话 / 卡片信息。
type FavoriteItem struct {
	ID         int64           `json:"id"`
	SessionID  int64           `json:"sessionId"`
	CardID     int64           `json:"cardId"`
	Content    string          `json:"content"`
	Sources    json.RawMessage `json:"sources"`
	CreateTime string          `json:"createTime"`
}

// ListFavorites 返回用户的收藏回答；cardID>0 时按卡片过滤。
func (s *Store) ListFavorites(ctx context.Context, appUserID, cardID int64) ([]FavoriteItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.session_id, s.card_id, m.content, m.sources, m.create_time
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id = m.session_id
		 WHERE s.app_user_id = $1 AND m.favorite = true
		   AND ($2 = 0 OR s.card_id = $2)
		 ORDER BY m.create_time DESC, m.id DESC`,
		appUserID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []FavoriteItem
	for rows.Next() {
		var it FavoriteItem
		var createTime time.Time
		if err := rows.Scan(&it.ID, &it.SessionID, &it.CardID, &it.Content, &it.Sources, &createTime); err != nil {
			return nil, err
		}
		it.CreateTime = formatTime(createTime)
		out = append(out, it)
	}
	return out, rows.Err()
}

// SearchResult 历史搜索结果：命中的消息 + 所属会话 / 卡片。
type SearchResult struct {
	ID         int64           `json:"id"`
	SessionID  int64           `json:"sessionId"`
	CardID     int64           `json:"cardId"`
	Role       string          `json:"role"`
	Content    string          `json:"content"`
	Sources    json.RawMessage `json:"sources"`
	Favorite   bool            `json:"favorite"`
	CreateTime string          `json:"createTime"`
}

// SearchMessages 按关键词搜索用户历史问答；cardID>0 时按卡片隔离，结果不跨卡片混淆。
func (s *Store) SearchMessages(ctx context.Context, appUserID, cardID int64, keyword string) ([]SearchResult, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.session_id, s.card_id, m.role, m.content, m.sources, m.favorite, m.create_time
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id = m.session_id
		 WHERE s.app_user_id = $1
		   AND ($2 = 0 OR s.card_id = $2)
		   AND m.content ILIKE '%' || $3 || '%'
		 ORDER BY m.create_time DESC, m.id DESC
		 LIMIT 100`,
		appUserID, cardID, keyword)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SearchResult
	for rows.Next() {
		var it SearchResult
		var createTime time.Time
		if err := rows.Scan(&it.ID, &it.SessionID, &it.CardID, &it.Role, &it.Content, &it.Sources, &it.Favorite, &createTime); err != nil {
			return nil, err
		}
		it.CreateTime = formatTime(createTime)
		out = append(out, it)
	}
	return out, rows.Err()
}
