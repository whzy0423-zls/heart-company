package server

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appMemoryItem struct {
	ID         int64  `json:"id"`
	CardID     int64  `json:"cardId"`
	Content    string `json:"content"`
	Status     string `json:"status"`
	SourceTime string `json:"sourceTime,omitempty"`
	CreateTime string `json:"createTime"`
	UpdateTime string `json:"updateTime"`
}

func appMemoryTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func (s *Server) appCardMemories(w http.ResponseWriter, r *http.Request, appUserID int64, idText string) {
	cardID, err := strconv.ParseInt(strings.Trim(idText, "/"), 10, 64)
	if err != nil || cardID <= 0 {
		httpx.Fail(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := s.quiz.GetCard(r.Context(), appUserID, cardID); err != nil {
		httpx.Fail(w, http.StatusNotFound, "card not found")
		return
	}
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, card_id, content, status, source_time, create_time, update_time
		 FROM app_memories
		 WHERE app_user_id = $1 AND card_id = $2
		 ORDER BY update_time DESC, id DESC`,
		appUserID, cardID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	defer rows.Close()
	items := []appMemoryItem{}
	for rows.Next() {
		var item appMemoryItem
		var source sql.NullTime
		var createTime, updateTime time.Time
		if err := rows.Scan(&item.ID, &item.CardID, &item.Content, &item.Status, &source, &createTime, &updateTime); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "server error")
			return
		}
		if source.Valid {
			item.SourceTime = appMemoryTime(source.Time)
		}
		item.CreateTime = appMemoryTime(createTime)
		item.UpdateTime = appMemoryTime(updateTime)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, items)
}

func appMemoryIDFromPath(path, suffix string) (int64, bool) {
	rest := strings.TrimPrefix(path, "/api/app/memories/")
	if suffix != "" {
		rest = strings.TrimSuffix(rest, "/"+suffix)
	}
	id, err := strconv.ParseInt(strings.Trim(rest, "/"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Server) appMemoryDelete(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := appMemoryIDFromPath(r.URL.Path, "")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		`DELETE FROM app_memories WHERE id = $1 AND app_user_id = $2`,
		id, userInfo.ID)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		httpx.Fail(w, http.StatusNotFound, "memory not found")
		return
	}
	httpx.OK(w, true)
}

func (s *Server) appMemoryStatus(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, ok := appMemoryIDFromPath(r.URL.Path, "status")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid memory id")
		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.Status != "active" && body.Status != "disabled" {
		httpx.Fail(w, http.StatusBadRequest, "invalid status")
		return
	}
	res, err := s.db.ExecContext(r.Context(),
		`UPDATE app_memories SET status = $3, update_time = now()
		 WHERE id = $1 AND app_user_id = $2`,
		id, userInfo.ID, body.Status)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		httpx.Fail(w, http.StatusNotFound, "memory not found")
		return
	}
	httpx.OK(w, map[string]string{"status": body.Status})
}

func (s *Server) appMemoryRouter(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/status") {
		if r.Method != http.MethodPut {
			httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		s.appMemoryStatus(w, r)
		return
	}
	if r.Method == http.MethodDelete {
		s.appMemoryDelete(w, r)
		return
	}
	httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
}
