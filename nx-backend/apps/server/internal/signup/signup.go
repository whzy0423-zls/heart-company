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
	Contact     string `json:"contact"`
	ContactType string `json:"contactType"`
	CreateTime  string `json:"createTime"`
	ID          string `json:"id"`
	Interest    string `json:"interest"`
	IP          string `json:"ip"`
	Message     string `json:"message"`
	Name        string `json:"name"`
	UserAgent   string `json:"userAgent"`
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
	err = s.db.QueryRowContext(c,
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

	return Lead{
		Contact:     normalizedContact,
		ContactType: contactType,
		CreateTime:  formatTime(createTime),
		ID:          strconv.FormatInt(id, 10),
		Interest:    strings.TrimSpace(input.Interest),
		IP:          clientIP(r),
		Message:     strings.TrimSpace(input.Message),
		Name:        name,
		UserAgent:   strings.TrimSpace(r.UserAgent()),
	}, nil
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
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM signups WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Lead]{}, err
	}

	page, pageSize := pageParams(query)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		"SELECT id, name, contact_type, contact, interest, message, ip, user_agent, create_time FROM signups WHERE "+cond+
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
		if err := rows.Scan(&id, &lead.Name, &lead.ContactType, &lead.Contact, &lead.Interest, &lead.Message, &lead.IP, &lead.UserAgent, &createTime); err != nil {
			return PageResult[Lead]{}, err
		}
		if lead.ContactType == "" {
			lead.ContactType = ContactTypePhone
		}
		lead.ID = strconv.FormatInt(id, 10)
		lead.CreateTime = formatTime(createTime)
		items = append(items, lead)
	}
	if err := rows.Err(); err != nil {
		return PageResult[Lead]{}, err
	}
	return PageResult[Lead]{Items: items, Total: total}, nil
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
