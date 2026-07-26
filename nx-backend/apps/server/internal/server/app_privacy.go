package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"nine-xing/nx-backend/apps/server/internal/appuser"
	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appPrivacyPolicyResponse struct {
	Title       string `json:"title"`
	Version     string `json:"version"`
	EffectiveAt string `json:"effectiveAt"`
	Content     string `json:"content"`
}

type appPrivacyExportResponse struct {
	GeneratedAt  string                 `json:"generatedAt"`
	User         appuser.User           `json:"user"`
	Cards        []appPrivacyCard       `json:"cards"`
	Memories     []appMemoryItem        `json:"memories"`
	Preferences  []appPrivacyPreference `json:"preferences"`
	SessionCount int                    `json:"sessionCount"`
	MessageCount int                    `json:"messageCount"`
}

type appPrivacyCard struct {
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

type appPrivacyPreference struct {
	ID          int64  `json:"id"`
	Category    string `json:"category"`
	Slot        string `json:"slot"`
	Instruction string `json:"instruction"`
	SourceText  string `json:"sourceText"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
}

func (s *Server) appPrivacyPolicy(w http.ResponseWriter, _ *http.Request) {
	httpx.OK(w, appPrivacyPolicyResponse{
		Title:       "九型芯之力 App 隐私政策",
		Version:     "2026-07-23",
		EffectiveAt: "2026-07-23",
		Content: `我们仅为账号登录、九型测评、成长卡片、对话服务、学习到的沟通偏好、消息推送和服务改进处理必要信息。

使用芯之力语音对话时，服务会临时处理你提交的音频，用于语音识别（ASR）、生成回答和语音合成（TTS）。我们的自有后台不持久化保存原始录音。即使页面不展示文字，成功完成并保存的芯之力对话仍会将转写文字、AI 回答和回答来源保存为隐藏对话历史。服务可能保存你明确表达的沟通偏好，并在回答时使用已有对话历史和已有专属记忆（如适用），以保持上下文并提供更贴合你的后续回答。

根据后台配置，完成语音识别（ASR）、语音合成（TTS）或回答生成所必需的音频、文字及上下文可能会发送给相应的第三方模型服务提供方处理。第三方的处理范围和保留规则受我们与其约定及其适用政策约束；我们不会将“页面不展示文字”等同于“不处理或不保存文字”。

你可以在 App 内导出个人数据、清空记忆或注销账号。清空记忆会删除当前账号的专属记忆和沟通偏好，但不会删除隐藏对话历史。

注销后，主业务库中的聊天会话及消息、专属记忆、沟通偏好、兼容报告、签到记录和设备令牌会被删除；成长卡片仅标记删除，相关数据行可能为保持关联一致性而保留，但不再正常展示或使用。刷新令牌会被撤销，账号会停用并匿名化登录标识，分析记录会与账号解除关联。

订单、会员和交易记录，以及依法或履约需留存的其他记录，可能继续保留；备份和安全运维日志也可能按适用法规和实际系统配置的周期保留，期满后删除或去标识。你可以通过隐私渠道查询当前适用的保留期限。`,
	})
}

func (s *Server) appPrivacyExport(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := s.appUsers.FindByID(r.Context(), userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "user not found")
		return
	}
	cards, err := s.appPrivacyCards(r, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	memories, err := s.appPrivacyMemories(r, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	preferences, err := s.appPrivacyPreferences(r, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	sessionCount, messageCount, err := s.appPrivacyChatCounts(r, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}

	httpx.OK(w, appPrivacyExportResponse{
		GeneratedAt:  appMemoryTime(time.Now()),
		User:         user,
		Cards:        cards,
		Memories:     memories,
		Preferences:  preferences,
		SessionCount: sessionCount,
		MessageCount: messageCount,
	})
}

func (s *Server) appPrivacyDeleteMemories(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), `DELETE FROM app_memories WHERE app_user_id = $1`, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if _, err := tx.ExecContext(r.Context(), `DELETE FROM app_user_preferences WHERE app_user_id = $1`, userInfo.ID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	deleted, _ := res.RowsAffected()
	httpx.OK(w, map[string]int64{"deleted": deleted})
}

func (s *Server) appPrivacyDeleteAccount(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	defer tx.Rollback()
	var lockedUserID int64
	if err := tx.QueryRowContext(r.Context(),
		`SELECT id FROM app_users WHERE id = $1 FOR UPDATE`, userInfo.ID,
	).Scan(&lockedUserID); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}

	steps := []string{
		`DELETE FROM app_memories WHERE app_user_id = $1`,
		`DELETE FROM app_user_preferences WHERE app_user_id = $1`,
		`DELETE FROM app_chat_sessions WHERE app_user_id = $1`,
		`DELETE FROM app_compatibility_reports WHERE app_user_id = $1`,
		`DELETE FROM app_daily_checkins WHERE app_user_id = $1`,
		`DELETE FROM app_device_tokens WHERE app_user_id = $1`,
		`UPDATE app_user_cards SET status = 'deleted', update_time = now() WHERE app_user_id = $1 AND status <> 'deleted'`,
		`UPDATE app_analytics_events SET app_user_id = NULL WHERE app_user_id = $1`,
		`UPDATE app_refresh_tokens SET revoked = true WHERE app_user_id = $1 AND revoked = false`,
	}
	for _, query := range steps {
		if _, err := tx.ExecContext(r.Context(), query, userInfo.ID); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "server error")
			return
		}
	}
	res, err := tx.ExecContext(r.Context(),
		`UPDATE app_users
		    SET phone = 'deleted-' || id::text || '-' || floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint::text,
		        nickname = '',
		        avatar = '',
		        status = 'disabled',
		        member_level = 'free',
		        update_time = now()
		  WHERE id = $1 AND status = 'active'`,
		userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		httpx.Fail(w, http.StatusNotFound, "user not found")
		return
	}
	if err := tx.Commit(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, map[string]bool{"deleted": true})
}

func (s *Server) appPrivacyCards(r *http.Request, appUserID int64) ([]appPrivacyCard, error) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, app_user_id, card_type, name, relation, enneagram, wing, profile, status, create_time, update_time
		   FROM app_user_cards
		  WHERE app_user_id = $1
		  ORDER BY CASE WHEN card_type = 'primary' THEN 0 ELSE 1 END, create_time, id`,
		appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cards := []appPrivacyCard{}
	for rows.Next() {
		var card appPrivacyCard
		var createTime, updateTime time.Time
		if err := rows.Scan(&card.ID, &card.AppUserID, &card.CardType, &card.Name, &card.Relation, &card.MainType, &card.WingType, &card.Profile, &card.Status, &createTime, &updateTime); err != nil {
			return nil, err
		}
		card.CreateTime = appMemoryTime(createTime)
		card.UpdateTime = appMemoryTime(updateTime)
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (s *Server) appPrivacyMemories(r *http.Request, appUserID int64) ([]appMemoryItem, error) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, card_id, content, status, source_time, create_time, update_time
		   FROM app_memories
		  WHERE app_user_id = $1
		  ORDER BY update_time DESC, id DESC`,
		appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	memories := []appMemoryItem{}
	for rows.Next() {
		var item appMemoryItem
		var source sql.NullTime
		var createTime, updateTime time.Time
		if err := rows.Scan(&item.ID, &item.CardID, &item.Content, &item.Status, &source, &createTime, &updateTime); err != nil {
			return nil, err
		}
		if source.Valid {
			item.SourceTime = appMemoryTime(source.Time)
		}
		item.CreateTime = appMemoryTime(createTime)
		item.UpdateTime = appMemoryTime(updateTime)
		memories = append(memories, item)
	}
	return memories, rows.Err()
}

func (s *Server) appPrivacyPreferences(r *http.Request, appUserID int64) ([]appPrivacyPreference, error) {
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, category, slot, instruction, source_text, create_time, update_time
		   FROM app_user_preferences
		  WHERE app_user_id = $1
		  ORDER BY category, slot, id`,
		appUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	preferences := []appPrivacyPreference{}
	for rows.Next() {
		var preference appPrivacyPreference
		var createTime, updateTime time.Time
		if err := rows.Scan(
			&preference.ID,
			&preference.Category,
			&preference.Slot,
			&preference.Instruction,
			&preference.SourceText,
			&createTime,
			&updateTime,
		); err != nil {
			return nil, err
		}
		preference.CreateTime = appMemoryTime(createTime)
		preference.UpdateTime = appMemoryTime(updateTime)
		preferences = append(preferences, preference)
	}
	return preferences, rows.Err()
}

func (s *Server) appPrivacyChatCounts(r *http.Request, appUserID int64) (int, int, error) {
	var sessionCount int
	var messageCount int
	err := s.db.QueryRowContext(r.Context(),
		`SELECT
		    count(DISTINCT s.id),
		    count(m.id)
		   FROM app_chat_sessions s
		   LEFT JOIN app_chat_messages m ON m.session_id = s.id
		  WHERE s.app_user_id = $1`,
		appUserID).Scan(&sessionCount, &messageCount)
	return sessionCount, messageCount, err
}
