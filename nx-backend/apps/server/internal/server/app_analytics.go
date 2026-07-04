package server

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/httpx"
)

type appAnalyticsEventInput struct {
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
	TS     json.RawMessage `json:"ts"`
}

func (s *Server) appAnalyticsEvent(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var body appAnalyticsEventInput
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid request body")
		return
	}
	event := strings.TrimSpace(body.Event)
	if event == "" {
		httpx.Fail(w, http.StatusBadRequest, "event required")
		return
	}
	if len(event) > 128 {
		event = event[:128]
	}
	params := body.Params
	if len(params) == 0 || strings.TrimSpace(string(params)) == "null" {
		params = json.RawMessage(`{}`)
	}
	clientTS, err := parseAppAnalyticsTS(body.TS)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "invalid ts")
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		`INSERT INTO app_analytics_events (app_user_id, event, params, client_ts, ip, user_agent)
		 VALUES ($1, $2, $3::jsonb, $4, $5, $6)`,
		userInfo.ID,
		event,
		params,
		clientTS,
		clientIP(r),
		truncateHeader(r.UserAgent(), 512),
	)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, map[string]bool{"stored": true})
}

func parseAppAnalyticsTS(raw json.RawMessage) (sql.NullTime, error) {
	var out sql.NullTime
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return out, nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return out, nil
		}
		t, err := time.Parse(time.RFC3339Nano, asString)
		if err != nil {
			return out, err
		}
		out.Time = t
		out.Valid = true
		return out, nil
	}
	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err != nil {
		return out, err
	}
	if math.IsNaN(asNumber) || math.IsInf(asNumber, 0) || asNumber <= 0 {
		return out, nil
	}
	seconds := int64(asNumber)
	nanos := int64((asNumber - float64(seconds)) * 1e9)
	if asNumber > 1e12 {
		seconds = int64(asNumber) / 1000
		nanos = (int64(asNumber) % 1000) * int64(time.Millisecond)
	}
	out.Time = time.Unix(seconds, nanos).UTC()
	out.Valid = true
	return out, nil
}

func truncateHeader(value string, max int) string {
	value = strings.TrimSpace(value)
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
