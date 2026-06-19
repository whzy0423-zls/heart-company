package signup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var mainlandMobileRE = regexp.MustCompile(`^1[3-9]\d{9}$`)

const (
	ContactTypePhone  = "phone"
	ContactTypeWechat = "wechat"
)

type Store struct {
	db *sql.DB
}

type Lead struct {
	Contact        string `json:"contact"`
	ContactType    string `json:"contactType"`
	CreateTime     string `json:"createTime"`
	FollowNote     string `json:"followNote"`
	FollowStatus   string `json:"followStatus"`
	GameResultID   string `json:"gameResultId"`
	ID             string `json:"id"`
	Interest       string `json:"interest"`
	IP             string `json:"ip"`
	LandingPage    string `json:"landingPage"`
	Message        string `json:"message"`
	Name           string `json:"name"`
	NextFollowTime string `json:"nextFollowTime"`
	Owner          string `json:"owner"`
	Referrer       string `json:"referrer"`
	SourcePath     string `json:"sourcePath"`
	UTMCampaign    string `json:"utmCampaign"`
	UTMContent     string `json:"utmContent"`
	UTMMedium      string `json:"utmMedium"`
	UTMSource      string `json:"utmSource"`
	UTMTerm        string `json:"utmTerm"`
	UserAgent      string `json:"userAgent"`
	VisitorID      string `json:"visitorId"`
}

type LeadInput struct {
	Contact      string `json:"contact"`
	ContactType  string `json:"contactType"`
	GameResultID string `json:"gameResultId"`
	Interest     string `json:"interest"`
	Message      string `json:"message"`
	Name         string `json:"name"`
	AttributionInput
}

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

type FollowInput struct {
	Content        string `json:"content"`
	FollowNote     string `json:"followNote"`
	NextFollowTime string `json:"nextFollowTime"`
	Owner          string `json:"owner"`
	Status         string `json:"status"`
}

type TimelineItem struct {
	Content        string `json:"content"`
	CreateTime     string `json:"createTime"`
	NextFollowTime string `json:"nextFollowTime"`
	Operator       string `json:"operator"`
	Owner          string `json:"owner"`
	Status         string `json:"status"`
	Type           string `json:"type"`
}

type AttributionInput struct {
	LandingPage string `json:"landingPage"`
	Referrer    string `json:"referrer"`
	SourcePath  string `json:"sourcePath"`
	UTMCampaign string `json:"utmCampaign"`
	UTMContent  string `json:"utmContent"`
	UTMMedium   string `json:"utmMedium"`
	UTMSource   string `json:"utmSource"`
	UTMTerm     string `json:"utmTerm"`
	VisitorID   string `json:"visitorId"`
}

type VisitTrace struct {
	CreateTime string `json:"createTime"`
	Path       string `json:"path"`
	Referrer   string `json:"referrer"`
	Title      string `json:"title"`
}

type GameProfile struct {
	Centers    any    `json:"centers"`
	CreateTime string `json:"createTime"`
	Gender     string `json:"gender"`
	ID         string `json:"id"`
	ResultType int    `json:"resultType"`
	Score      any    `json:"score"`
	SecondType int    `json:"secondType"`
}

type Detail struct {
	GameResult  *GameProfile   `json:"gameResult"`
	Lead        Lead           `json:"lead"`
	Timeline    []TimelineItem `json:"timeline"`
	VisitTraces []VisitTrace   `json:"visitTraces"`
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

func (s *Store) Create(ctx context.Context, input LeadInput, r *http.Request) (Lead, error) {
	name := strings.TrimSpace(input.Name)
	contact := strings.TrimSpace(input.Contact)
	if name == "" {
		return Lead{}, errors.New("name is required")
	}
	contactType, normalizedContact, err := normalizeContact(input.ContactType, contact)
	if err != nil {
		return Lead{}, err
	}
	attribution := normalizeAttribution(input.AttributionInput)

	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var id int64
	var createTime time.Time
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Lead{}, err
	}
	defer func() { _ = tx.Rollback() }()

	gameResultID := parseOptionalInt64(input.GameResultID)
	if gameResultID <= 0 {
		gameResultID = findLatestGameResultID(c, tx, attribution.VisitorID)
	}
	err = tx.QueryRowContext(c,
		`INSERT INTO signups
		 (name, contact_type, contact, interest, message, visitor_id, source_path, landing_page, referrer,
		  utm_source, utm_medium, utm_campaign, utm_content, utm_term, game_result_id, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		 RETURNING id, create_time`,
		name,
		contactType,
		normalizedContact,
		strings.TrimSpace(input.Interest),
		strings.TrimSpace(input.Message),
		attribution.VisitorID,
		attribution.SourcePath,
		attribution.LandingPage,
		attribution.Referrer,
		attribution.UTMSource,
		attribution.UTMMedium,
		attribution.UTMCampaign,
		attribution.UTMContent,
		attribution.UTMTerm,
		nullInt64(gameResultID),
		clientIP(r),
		strings.TrimSpace(r.UserAgent()),
	).Scan(&id, &createTime)
	if err != nil {
		return Lead{}, err
	}

	lead := Lead{
		Contact:      normalizedContact,
		ContactType:  contactType,
		CreateTime:   formatTime(createTime),
		FollowStatus: "pending",
		GameResultID: formatOptionalID(gameResultID),
		ID:           strconv.FormatInt(id, 10),
		Interest:     strings.TrimSpace(input.Interest),
		IP:           clientIP(r),
		LandingPage:  attribution.LandingPage,
		Message:      strings.TrimSpace(input.Message),
		Name:         name,
		Referrer:     attribution.Referrer,
		SourcePath:   attribution.SourcePath,
		UTMCampaign:  attribution.UTMCampaign,
		UTMContent:   attribution.UTMContent,
		UTMMedium:    attribution.UTMMedium,
		UTMSource:    attribution.UTMSource,
		UTMTerm:      attribution.UTMTerm,
		UserAgent:    strings.TrimSpace(r.UserAgent()),
		VisitorID:    attribution.VisitorID,
	}

	if _, err := tx.ExecContext(c,
		`INSERT INTO messages (type, title, content, business_id, business_type, target_path)
		 VALUES ('signup', $1, $2, $3, 'signup', '/message/management?type=signup')`,
		"新的报名信息",
		lead.Name+" / "+contactTypeLabel(lead.ContactType)+": "+lead.Contact,
		lead.ID,
	); err != nil {
		return Lead{}, err
	}
	if err := tx.Commit(); err != nil {
		return Lead{}, err
	}
	return lead, nil
}

func normalizeContact(contactType string, contact string) (string, string, error) {
	contactType = strings.TrimSpace(contactType)
	if contactType == "" {
		contactType = ContactTypePhone
	}
	contact = strings.TrimSpace(contact)
	if contact == "" {
		return "", "", errors.New("请输入联系方式")
	}
	switch contactType {
	case ContactTypePhone:
		phone, ok := normalizePhone(contact)
		if !ok {
			return "", "", errors.New("请输入正确的手机号")
		}
		return ContactTypePhone, phone, nil
	case ContactTypeWechat:
		return ContactTypeWechat, contact, nil
	default:
		return "", "", errors.New("请选择联系方式类型")
	}
}

func normalizeAttribution(input AttributionInput) AttributionInput {
	result := AttributionInput{
		LandingPage: truncate(strings.TrimSpace(input.LandingPage), 1024),
		Referrer:    truncate(strings.TrimSpace(input.Referrer), 1024),
		SourcePath:  truncate(strings.TrimSpace(input.SourcePath), 512),
		UTMCampaign: truncate(strings.TrimSpace(input.UTMCampaign), 128),
		UTMContent:  truncate(strings.TrimSpace(input.UTMContent), 128),
		UTMMedium:   truncate(strings.TrimSpace(input.UTMMedium), 128),
		UTMSource:   truncate(strings.TrimSpace(input.UTMSource), 128),
		UTMTerm:     truncate(strings.TrimSpace(input.UTMTerm), 128),
		VisitorID:   truncate(strings.TrimSpace(input.VisitorID), 128),
	}
	if result.SourcePath == "" {
		result.SourcePath = "/"
	}
	return result
}

func normalizePhone(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer(" ", "", "-", "", "－", "").Replace(value)
	if !mainlandMobileRE.MatchString(value) {
		return "", false
	}
	return value, true
}

func (s *Store) List(ctx context.Context, query map[string]string) (PageResult[Lead], error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if keyword := strings.TrimSpace(query["keyword"]); keyword != "" {
		args = append(args, "%"+strings.ToLower(keyword)+"%")
		index := strconv.Itoa(len(args))
		where = append(where, "(lower(name) LIKE $"+index+" OR lower(contact_type) LIKE $"+index+" OR lower(contact) LIKE $"+index+" OR lower(interest) LIKE $"+index+" OR lower(message) LIKE $"+index+")")
	}
	if status := strings.TrimSpace(query["status"]); status != "" {
		args = append(args, status)
		where = append(where, "follow_status = $"+strconv.Itoa(len(args)))
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM signups WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Lead]{}, err
	}

	page, pageSize := pageParams(query)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		"SELECT id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time, visitor_id, source_path, landing_page, referrer, utm_source, utm_medium, utm_campaign, utm_content, utm_term, COALESCE(game_result_id::text,''), ip, user_agent, create_time FROM signups WHERE "+cond+
			" ORDER BY create_time DESC, id DESC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)),
		args...,
	)
	if err != nil {
		return PageResult[Lead]{}, err
	}
	defer rows.Close()

	items := []Lead{}
	for rows.Next() {
		var lead Lead
		var id int64
		var createTime time.Time
		var nextFollow sql.NullTime
		if err := rows.Scan(&id, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextFollow, &lead.VisitorID, &lead.SourcePath, &lead.LandingPage, &lead.Referrer, &lead.UTMSource, &lead.UTMMedium, &lead.UTMCampaign, &lead.UTMContent, &lead.UTMTerm, &lead.GameResultID, &lead.IP, &lead.UserAgent, &createTime); err != nil {
			return PageResult[Lead]{}, err
		}
		if lead.ContactType == "" {
			lead.ContactType = ContactTypePhone
		}
		lead.ID = strconv.FormatInt(id, 10)
		lead.CreateTime = formatTime(createTime)
		lead.NextFollowTime = formatNullableTime(nextFollow)
		items = append(items, lead)
	}
	if err := rows.Err(); err != nil {
		return PageResult[Lead]{}, err
	}
	return PageResult[Lead]{Items: items, Total: total}, nil
}

func (s *Store) visitTraces(ctx context.Context, visitorID string) ([]VisitTrace, error) {
	visitorID = strings.TrimSpace(visitorID)
	if visitorID == "" {
		return []VisitTrace{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT path, title, referrer, create_time
		   FROM site_visits
		  WHERE visitor_id=$1
		  ORDER BY create_time DESC, id DESC
		  LIMIT 20`,
		visitorID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []VisitTrace{}
	for rows.Next() {
		var item VisitTrace
		var createTime time.Time
		if err := rows.Scan(&item.Path, &item.Title, &item.Referrer, &createTime); err != nil {
			return nil, err
		}
		item.CreateTime = formatTime(createTime)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) gameProfile(ctx context.Context, id string) (*GameProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, nil
	}
	var item GameProfile
	var createTime time.Time
	var scoreRaw []byte
	var centersRaw []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT id::text, gender, result_type, second_type, score, centers, create_time
		   FROM game_results
		  WHERE id=$1`,
		id,
	).Scan(&item.ID, &item.Gender, &item.ResultType, &item.SecondType, &scoreRaw, &centersRaw, &createTime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.CreateTime = formatTime(createTime)
	item.Score = jsonValue(scoreRaw, map[string]any{})
	item.Centers = jsonValue(centersRaw, []any{})
	return &item, nil
}

func (s *Store) Detail(ctx context.Context, id string) (Detail, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var lead Lead
	var leadID int64
	var createTime time.Time
	var nextFollow sql.NullTime
	if err := s.db.QueryRowContext(c,
		`SELECT id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time,
		        visitor_id, source_path, landing_page, referrer, utm_source, utm_medium, utm_campaign, utm_content, utm_term, COALESCE(game_result_id::text,''),
		        ip, user_agent, create_time
		 FROM signups WHERE id=$1`,
		id,
	).Scan(&leadID, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextFollow, &lead.VisitorID, &lead.SourcePath, &lead.LandingPage, &lead.Referrer, &lead.UTMSource, &lead.UTMMedium, &lead.UTMCampaign, &lead.UTMContent, &lead.UTMTerm, &lead.GameResultID, &lead.IP, &lead.UserAgent, &createTime); err != nil {
		return Detail{}, err
	}
	lead.ID = strconv.FormatInt(leadID, 10)
	lead.CreateTime = formatTime(createTime)
	lead.NextFollowTime = formatNullableTime(nextFollow)
	timeline := []TimelineItem{{
		Content:    "客户提交报名信息",
		CreateTime: lead.CreateTime,
		Type:       "created",
	}}

	rows, err := s.db.QueryContext(c,
		`SELECT status, owner, content, next_follow_time, operator, create_time
		 FROM signup_followups WHERE signup_id=$1 ORDER BY create_time DESC, id DESC`,
		leadID,
	)
	if err != nil {
		return Detail{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item TimelineItem
		var next sql.NullTime
		var t time.Time
		if err := rows.Scan(&item.Status, &item.Owner, &item.Content, &next, &item.Operator, &t); err != nil {
			return Detail{}, err
		}
		item.Type = "followup"
		item.CreateTime = formatTime(t)
		item.NextFollowTime = formatNullableTime(next)
		timeline = append(timeline, item)
	}
	if err := rows.Err(); err != nil {
		return Detail{}, err
	}
	visits, err := s.visitTraces(c, lead.VisitorID)
	if err != nil {
		return Detail{}, err
	}
	gameResult, err := s.gameProfile(c, lead.GameResultID)
	if err != nil {
		return Detail{}, err
	}
	return Detail{GameResult: gameResult, Lead: lead, Timeline: timeline, VisitTraces: visits}, nil
}

func (s *Store) Follow(ctx context.Context, id string, input FollowInput, operator string) (Lead, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	status := normalizeStatus(input.Status)
	owner := strings.TrimSpace(input.Owner)
	content := strings.TrimSpace(input.Content)
	note := strings.TrimSpace(input.FollowNote)
	nextFollow, err := parseOptionalTime(input.NextFollowTime)
	if err != nil {
		return Lead{}, err
	}

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Lead{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var currentStatus string
	if err := tx.QueryRowContext(c, `SELECT follow_status FROM signups WHERE id=$1`, id).Scan(&currentStatus); err != nil {
		return Lead{}, err
	}
	if currentStatus == "deal" && status == "deal" {
		return Lead{}, errors.New("已成交线索不能继续跟进，请先重新打开线索")
	}

	var nextArg any
	if nextFollow.Valid {
		nextArg = nextFollow.Time
	}
	var lead Lead
	var leadID int64
	var createTime time.Time
	var nextStored sql.NullTime
	err = tx.QueryRowContext(c,
		`UPDATE signups
		 SET follow_status=$1, owner=$2, follow_note=$3, next_follow_time=$4, update_time=now()
		 WHERE id=$5
		 RETURNING id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time,
		           visitor_id, source_path, landing_page, referrer, utm_source, utm_medium, utm_campaign, utm_content, utm_term, COALESCE(game_result_id::text,''),
		           ip, user_agent, create_time`,
		status, owner, note, nextArg, id,
	).Scan(&leadID, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextStored, &lead.VisitorID, &lead.SourcePath, &lead.LandingPage, &lead.Referrer, &lead.UTMSource, &lead.UTMMedium, &lead.UTMCampaign, &lead.UTMContent, &lead.UTMTerm, &lead.GameResultID, &lead.IP, &lead.UserAgent, &createTime)
	if err != nil {
		return Lead{}, err
	}
	lead.ID = strconv.FormatInt(leadID, 10)
	lead.CreateTime = formatTime(createTime)
	lead.NextFollowTime = formatNullableTime(nextStored)

	if content != "" || note != "" || status != "" || owner != "" || nextFollow.Valid {
		if _, err := tx.ExecContext(c,
			`INSERT INTO signup_followups (signup_id, status, owner, content, next_follow_time, operator)
			 VALUES ($1,$2,$3,$4,$5,$6)`,
			leadID, status, owner, firstNonEmpty(content, note), nextArg, operator,
		); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Lead{}, err
	}
	return lead, nil
}

func pageParams(query map[string]string) (int, int) {
	page, _ := strconv.Atoi(query["page"])
	pageSize, _ := strconv.Atoi(query["pageSize"])
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

func formatNullableTime(t sql.NullTime) string {
	if !t.Valid {
		return ""
	}
	return formatTime(t.Time)
}

func parseOptionalTime(value string) (sql.NullTime, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return sql.NullTime{}, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02T15:04:05Z07:00", time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return sql.NullTime{Time: t, Valid: true}, nil
		}
	}
	return sql.NullTime{}, errors.New("下次跟进时间格式不正确")
}

func normalizeStatus(status string) string {
	switch strings.TrimSpace(status) {
	case "contacted", "interested", "deal", "invalid":
		return strings.TrimSpace(status)
	default:
		return "pending"
	}
}

func contactTypeLabel(contactType string) string {
	if contactType == ContactTypeWechat {
		return "微信号"
	}
	return "手机号"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findLatestGameResultID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, visitorID string) int64 {
	visitorID = strings.TrimSpace(visitorID)
	if visitorID == "" {
		return 0
	}
	var id int64
	err := q.QueryRowContext(ctx,
		`SELECT id
		   FROM game_results
		  WHERE visitor_id=$1
		  ORDER BY create_time DESC, id DESC
		  LIMIT 1`,
		visitorID,
	).Scan(&id)
	if err != nil {
		return 0
	}
	return id
}

func parseOptionalInt64(value string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return id
}

func nullInt64(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func formatOptionalID(value int64) string {
	if value <= 0 {
		return ""
	}
	return strconv.FormatInt(value, 10)
}

func jsonValue(raw []byte, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fallback
	}
	return value
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}

func clientIP(r *http.Request) string {
	for _, header := range []string{"X-Forwarded-For", "X-Real-IP"} {
		value := strings.TrimSpace(r.Header.Get(header))
		if value == "" {
			continue
		}
		parts := strings.Split(value, ",")
		if ip := strings.TrimSpace(parts[0]); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
