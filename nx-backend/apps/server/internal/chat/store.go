package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	ID              int64           `json:"id"`
	SessionID       int64           `json:"sessionId"`
	Role            string          `json:"role"`
	Content         string          `json:"content"`
	Sources         json.RawMessage `json:"sources"`
	Favorite        bool            `json:"favorite"`
	Feedback        string          `json:"feedback"`
	MessageType     string          `json:"messageType"`
	AudioDurationMs int             `json:"audioDurationMs,omitempty"`
	AudioURL        string          `json:"audioUrl,omitempty"`
	AudioAssetID    int64           `json:"-"`
	Transcript      string          `json:"-"`
	CreateTime      string          `json:"createTime"`
}

func (m Message) EffectiveContent() string {
	if strings.TrimSpace(m.MessageType) == "voice" {
		return strings.TrimSpace(m.Transcript)
	}
	return strings.TrimSpace(m.Content)
}

type ConversationState struct {
	Summary                 string
	SummaryThroughMessageID int64
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

func scanMessage(row interface{ Scan(...interface{}) error }) (Message, error) {
	var message Message
	var audioAssetID sql.NullInt64
	var createTime time.Time
	err := row.Scan(
		&message.ID,
		&message.SessionID,
		&message.Role,
		&message.Content,
		&message.Sources,
		&message.Favorite,
		&message.Feedback,
		&message.MessageType,
		&audioAssetID,
		&message.AudioDurationMs,
		&message.Transcript,
		&createTime,
	)
	if err != nil {
		return message, err
	}
	message.AudioAssetID = audioAssetID.Int64
	message.CreateTime = formatTime(createTime)
	if message.MessageType == "voice" && message.AudioAssetID > 0 {
		message.AudioURL = fmt.Sprintf("/api/app/chat/messages/%d/audio", message.ID)
	}
	return message, nil
}

// ListSessions 返回用户所有会话（按最近更新倒序）。
func (s *Store) ListSessions(ctx context.Context, appUserID int64) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE app_user_id = $1 AND scene = 'chat'
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
	return s.GetOrCreateSceneSession(ctx, appUserID, cardID, "chat")
}

// GetOrCreateSceneSession finds the most recent session for an explicitly
// selected internal scene. Public chat callers must use GetOrCreateSession.
func (s *Store) GetOrCreateSceneSession(ctx context.Context, appUserID, cardID int64, scene string) (Session, error) {
	scene = strings.TrimSpace(scene)
	if scene == "" {
		scene = "chat"
	}
	if scene == "xinzhili_voice" {
		return s.getOrCreateSerializedSceneSession(ctx, appUserID, cardID, scene)
	}
	return getOrCreateSceneSession(ctx, s.db, appUserID, cardID, scene)
}

type sceneSessionQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getOrCreateSceneSession(ctx context.Context, queryer sceneSessionQueryer, appUserID, cardID int64, scene string) (Session, error) {
	sess, err := scanSession(queryer.QueryRowContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE app_user_id = $1 AND card_id = $2 AND scene = $3
		 ORDER BY updated_at DESC, id DESC LIMIT 1`,
		appUserID, cardID, scene,
	))
	if err == nil {
		return sess, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return sess, err
	}
	sess, err = scanSession(queryer.QueryRowContext(ctx,
		`INSERT INTO app_chat_sessions (app_user_id, card_id, scene) VALUES ($1, $2, $3)
		 RETURNING id, app_user_id, card_id, title, updated_at, create_time`,
		appUserID, cardID, scene))
	return sess, err
}

func (s *Store) getOrCreateSerializedSceneSession(ctx context.Context, appUserID, cardID int64, scene string) (sess Session, retErr error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return Session{}, fmt.Errorf("begin scene session transaction: %w", err)
	}
	defer func() {
		rollbackErr := tx.Rollback()
		if rollbackErr == nil || errors.Is(rollbackErr, sql.ErrTxDone) {
			return
		}
		wrapped := fmt.Errorf("rollback scene session transaction: %w", rollbackErr)
		if retErr != nil {
			retErr = errors.Join(retErr, wrapped)
			return
		}
		retErr = wrapped
	}()
	lockKey := fmt.Sprintf("app-chat-scene-session:%d:%d:%s", appUserID, cardID, scene)
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey); err != nil {
		return Session{}, fmt.Errorf("lock scene session: %w", err)
	}
	sess, err = getOrCreateSceneSession(ctx, tx, appUserID, cardID, scene)
	if err != nil {
		return Session{}, err
	}
	if err := tx.Commit(); err != nil {
		return Session{}, fmt.Errorf("commit scene session transaction: %w", err)
	}
	return sess, nil
}

// ResolveSceneSession creates/resumes a scene conversation only when all four
// isolation dimensions match. A non-zero conversationID is never silently
// replaced by another card's or scene's latest session.
func (s *Store) ResolveSceneSession(ctx context.Context, appUserID, cardID int64, scene string, conversationID int64) (Session, error) {
	scene = strings.TrimSpace(scene)
	if appUserID <= 0 || cardID <= 0 || scene == "" || conversationID < 0 {
		return Session{}, ErrNotFound
	}
	if conversationID == 0 {
		return s.GetOrCreateSceneSession(ctx, appUserID, cardID, scene)
	}
	session, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions
		 WHERE id=$1 AND app_user_id=$2 AND card_id=$3 AND scene=$4`,
		conversationID, appUserID, cardID, scene,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNotFound
	}
	return session, err
}

// SaveSceneUserText stores only hidden transcript text; no audio asset or
// public voice message is created for the realtime xinzhili scene.
func (s *Store) SaveSceneUserText(ctx context.Context, sessionID int64, text, mode string) (int64, error) {
	text = strings.TrimSpace(text)
	mode = strings.TrimSpace(mode)
	if sessionID <= 0 || text == "" || mode == "" {
		return 0, errors.New("chat: invalid scene user message")
	}
	var messageID int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages(session_id, role, content, sources, message_type, xinzhili_mode)
		 SELECT id, 'user', $2, '[]'::jsonb, 'text', $3
		 FROM app_chat_sessions WHERE id=$1 AND scene='xinzhili_voice'
		 RETURNING id`, sessionID, text, mode,
	).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err == nil {
		_, err = s.db.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID)
	}
	return messageID, err
}

// CreateSceneAssistant creates the delivery record only after transport has
// accepted the first playable segment. delivered_text starts empty.
func (s *Store) CreateSceneAssistant(ctx context.Context, sessionID int64, content, mode string) (int64, error) {
	content = strings.TrimSpace(content)
	mode = strings.TrimSpace(mode)
	if sessionID <= 0 || content == "" || mode == "" {
		return 0, errors.New("chat: invalid scene assistant message")
	}
	var messageID int64
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages(session_id, role, content, sources, message_type, delivery_status, delivered_text, xinzhili_mode)
		 SELECT id, 'assistant', $2, '[]'::jsonb, 'text', 'sent', '', $3
		 FROM app_chat_sessions WHERE id=$1 AND scene='xinzhili_voice'
		 RETURNING id`, sessionID, content, mode,
	).Scan(&messageID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return messageID, err
}

// AcknowledgeSceneAssistant advances delivered_text monotonically. Callers
// must provide the exact concatenated text represented by acknowledged audio
// segments; arbitrary or shrinking prefixes are rejected.
func (s *Store) AcknowledgeSceneAssistant(ctx context.Context, messageID int64, deliveredText string, complete bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var content string
	var previous sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT m.content, m.delivered_text
		 FROM app_chat_messages m JOIN app_chat_sessions s ON s.id=m.session_id
		 WHERE m.id=$1 AND m.role='assistant' AND s.scene='xinzhili_voice'
		 FOR UPDATE OF m`, messageID,
	).Scan(&content, &previous)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	deliveredText = strings.TrimSpace(deliveredText)
	if !strings.HasPrefix(deliveredText, previous.String) ||
		(!strings.HasPrefix(content, deliveredText) && !strings.HasPrefix(deliveredText, content)) {
		return errors.New("chat: invalid delivered text prefix")
	}
	if complete && deliveredText != content {
		return errors.New("chat: completed delivery must equal final content")
	}
	status := "sent"
	if complete {
		status = "played"
	}
	if _, err = tx.ExecContext(ctx,
		`UPDATE app_chat_messages
		 SET content=CASE WHEN length($2) > length(content) THEN $2 ELSE content END,
		     delivered_text=$2,
		     delivery_status=CASE WHEN delivery_status='played' THEN 'played' ELSE $3 END
		 WHERE id=$1`,
		messageID, deliveredText, status,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CompleteSceneAssistant(ctx context.Context, messageID int64, content string, sources json.RawMessage) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return errors.New("chat: assistant content is required")
	}
	if sources == nil {
		sources = json.RawMessage("[]")
	}
	result, err := s.db.ExecContext(ctx,
		`UPDATE app_chat_messages m SET content=$2, sources=$3
		 FROM app_chat_sessions s
		 WHERE m.id=$1 AND m.session_id=s.id AND m.role='assistant' AND s.scene='xinzhili_voice'`,
		messageID, content, sources,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSession 按 id+用户 返回会话，防越权。
func (s *Store) GetSession(ctx context.Context, appUserID, sessionID int64) (Session, error) {
	sess, err := scanSession(s.db.QueryRowContext(ctx,
		`SELECT id, app_user_id, card_id, title, updated_at, create_time
		 FROM app_chat_sessions WHERE id = $1 AND app_user_id = $2 AND scene = 'chat'`,
		sessionID, appUserID))
	if errors.Is(err, sql.ErrNoRows) {
		return sess, ErrNotFound
	}
	return sess, err
}

// ListMessages 返回会话的全部消息（按时间正序）。
func (s *Store) ListMessages(ctx context.Context, sessionID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.session_id, m.role, m.content, m.sources, m.favorite, m.feedback,
		        m.message_type, m.audio_asset_id, m.audio_duration_ms, m.transcript, m.create_time
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id = m.session_id
		 WHERE m.session_id = $1 AND s.scene = 'chat' ORDER BY m.create_time, m.id`,
		sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ListRecentMessages 返回会话最近的有限条消息，并恢复为时间正序。
func (s *Store) ListRecentMessages(ctx context.Context, sessionID int64, limit int) ([]Message, error) {
	if sessionID <= 0 || limit <= 0 {
		return nil, nil
	}
	if limit > 50 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, sources, favorite, feedback,
		        message_type, audio_asset_id, audio_duration_ms, transcript, create_time
		 FROM app_chat_messages
		 WHERE session_id = $1
		   AND role IN ('user', 'assistant')
		   AND ((message_type = 'voice' AND btrim(transcript) <> '')
		        OR (message_type <> 'voice' AND btrim(content) <> ''))
		 ORDER BY create_time DESC, id DESC
		 LIMIT $2`,
		sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for left, right := 0, len(out)-1; left < right; left, right = left+1, right-1 {
		out[left], out[right] = out[right], out[left]
	}
	return out, nil
}

func (s *Store) GetConversationState(ctx context.Context, sessionID int64) (ConversationState, error) {
	var state ConversationState
	err := s.db.QueryRowContext(ctx,
		`SELECT context_summary, context_summary_through_message_id
		 FROM app_chat_sessions WHERE id = $1`,
		sessionID,
	).Scan(&state.Summary, &state.SummaryThroughMessageID)
	return state, err
}

func (s *Store) ListMessagesAfter(ctx context.Context, sessionID, afterMessageID int64) ([]Message, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, session_id, role, content, sources, favorite, feedback,
		        message_type, audio_asset_id, audio_duration_ms, transcript, create_time
		 FROM app_chat_messages
		 WHERE session_id = $1 AND id > $2
		 ORDER BY id`,
		sessionID, afterMessageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, message)
	}
	return out, rows.Err()
}

func (s *Store) UpdateConversationSummary(ctx context.Context, sessionID, expectedThroughMessageID int64, summary string, throughMessageID int64) (bool, error) {
	result, err := s.db.ExecContext(ctx,
		`UPDATE app_chat_sessions
		 SET context_summary = $2, context_summary_through_message_id = $3
		 WHERE id = $1 AND context_summary_through_message_id = $4`,
		sessionID, summary, throughMessageID, expectedThroughMessageID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
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

// SaveVoicePair atomically stores a playable user voice message and the AI text answer.
// The transcript is server-only context and is never exposed through Message JSON.
func (s *Store) SaveVoicePair(ctx context.Context, sessionID, audioAssetID int64, durationMs int, transcript, answer string, sources json.RawMessage) (int64, int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	if sources == nil {
		sources = json.RawMessage("[]")
	}
	var userID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages
		 (session_id, role, content, sources, message_type, audio_asset_id, audio_duration_ms, transcript)
		 VALUES ($1,'user','','[]','voice',$2,$3,$4)
		 RETURNING id`,
		sessionID, audioAssetID, durationMs, strings.TrimSpace(transcript),
	).Scan(&userID)
	if err != nil {
		return 0, 0, err
	}

	var assistantID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages (session_id, role, content, sources)
		 VALUES ($1,'assistant',$2,$3)
		 RETURNING id`,
		sessionID, answer, sources,
	).Scan(&assistantID)
	if err != nil {
		return 0, 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID); err != nil {
		return 0, 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, 0, err
	}
	return userID, assistantID, nil
}

// SaveVoiceMessage atomically stores a playable user voice message without an
// assistant response. It is used when the audio is valid but contains no
// recognizable speech.
func (s *Store) SaveVoiceMessage(ctx context.Context, sessionID, audioAssetID int64, durationMs int, transcript string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var userID int64
	err = tx.QueryRowContext(ctx,
		`INSERT INTO app_chat_messages
		 (session_id, role, content, sources, message_type, audio_asset_id, audio_duration_ms, transcript)
		 VALUES ($1,'user','','[]','voice',$2,$3,$4)
		 RETURNING id`,
		sessionID, audioAssetID, durationMs, strings.TrimSpace(transcript),
	).Scan(&userID)
	if err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE app_chat_sessions SET updated_at=now() WHERE id=$1`, sessionID); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return userID, nil
}

// GetVoiceAudioAssetID returns the backing asset only when the App user owns
// the voice message's session.
func (s *Store) GetVoiceAudioAssetID(ctx context.Context, appUserID, messageID int64) (int64, error) {
	var assetID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT m.audio_asset_id
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id = m.session_id
		 WHERE m.id = $1 AND s.app_user_id = $2
		   AND s.scene = 'chat'
		   AND m.role = 'user' AND m.message_type = 'voice'
		   AND m.audio_asset_id IS NOT NULL`,
		messageID, appUserID,
	).Scan(&assetID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return assetID, err
}

// GetVoiceTranscript returns the hidden ASR transcript only when the App user
// owns the voice message's session.
func (s *Store) GetVoiceTranscript(ctx context.Context, appUserID, messageID int64) (string, error) {
	var transcript string
	err := s.db.QueryRowContext(ctx,
		`SELECT m.transcript
		 FROM app_chat_messages m
		 JOIN app_chat_sessions s ON s.id = m.session_id
		 WHERE m.id = $1 AND s.app_user_id = $2
		   AND s.scene = 'chat'
		   AND m.role = 'user' AND m.message_type = 'voice'`,
		messageID, appUserID,
	).Scan(&transcript)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return strings.TrimSpace(transcript), err
}

// SetFeedback 设置某条 AI 消息的反馈（'helpful' | 'inaccurate' | 'continue' | ”）。
// 通过 session→app_user_id 联结校验归属，防越权。
func (s *Store) SetFeedback(ctx context.Context, appUserID, messageID int64, feedback string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE app_chat_messages m SET feedback = $3
		 FROM app_chat_sessions s
		 WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'assistant'`,
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
		 WHERE m.id = $1 AND m.session_id = s.id AND s.app_user_id = $2 AND s.scene = 'chat' AND m.role = 'assistant'
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
		 WHERE s.app_user_id = $1 AND s.scene = 'chat' AND m.favorite = true
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
		   AND s.scene = 'chat'
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
