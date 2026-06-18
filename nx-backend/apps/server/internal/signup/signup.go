package signup

import (
	"context"
	"database/sql"
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
	ID             string `json:"id"`
	Interest       string `json:"interest"`
	IP             string `json:"ip"`
	Message        string `json:"message"`
	Name           string `json:"name"`
	NextFollowTime string `json:"nextFollowTime"`
	Owner          string `json:"owner"`
	UserAgent      string `json:"userAgent"`
}

type LeadInput struct {
	Contact     string `json:"contact"`
	ContactType string `json:"contactType"`
	Interest    string `json:"interest"`
	Message     string `json:"message"`
	Name        string `json:"name"`
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

type Detail struct {
	Lead     Lead           `json:"lead"`
	Timeline []TimelineItem `json:"timeline"`
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

	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var id int64
	var createTime time.Time
	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Lead{}, err
	}
	defer func() { _ = tx.Rollback() }()

	err = tx.QueryRowContext(c,
		`INSERT INTO signups (name, contact_type, contact, interest, message, ip, user_agent)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 RETURNING id, create_time`,
		name,
		contactType,
		normalizedContact,
		strings.TrimSpace(input.Interest),
		strings.TrimSpace(input.Message),
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
		ID:           strconv.FormatInt(id, 10),
		Interest:     strings.TrimSpace(input.Interest),
		IP:           clientIP(r),
		Message:      strings.TrimSpace(input.Message),
		Name:         name,
		UserAgent:    strings.TrimSpace(r.UserAgent()),
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
		"SELECT id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time, ip, user_agent, create_time FROM signups WHERE "+cond+
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
		if err := rows.Scan(&id, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextFollow, &lead.IP, &lead.UserAgent, &createTime); err != nil {
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

func (s *Store) Detail(ctx context.Context, id string) (Detail, error) {
	c, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var lead Lead
	var leadID int64
	var createTime time.Time
	var nextFollow sql.NullTime
	if err := s.db.QueryRowContext(c,
		`SELECT id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time, ip, user_agent, create_time
		 FROM signups WHERE id=$1`,
		id,
	).Scan(&leadID, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextFollow, &lead.IP, &lead.UserAgent, &createTime); err != nil {
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
	return Detail{Lead: lead, Timeline: timeline}, rows.Err()
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
		 RETURNING id, name, contact_type, contact, interest, message, follow_status, owner, follow_note, next_follow_time, ip, user_agent, create_time`,
		status, owner, note, nextArg, id,
	).Scan(&leadID, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.FollowStatus, &lead.Owner, &lead.FollowNote, &nextStored, &lead.IP, &lead.UserAgent, &createTime)
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
