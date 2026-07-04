package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Store struct {
	db *sql.DB
}

type Entry struct {
	OperatorID   int64
	OperatorName string
	Action       string
	TargetType   string
	TargetID     string
	IP           string
	UserAgent    string
	Before       any
	After        any
	Summary      string
}

type Log struct {
	ID           int64           `json:"id"`
	OperatorID   int64           `json:"operatorId"`
	OperatorName string          `json:"operatorName"`
	Action       string          `json:"action"`
	TargetType   string          `json:"targetType"`
	TargetID     string          `json:"targetId"`
	IP           string          `json:"ip"`
	UserAgent    string          `json:"userAgent"`
	Before       json.RawMessage `json:"before"`
	After        json.RawMessage `json:"after"`
	Summary      string          `json:"summary"`
	CreateTime   string          `json:"createTime"`
}

type PageResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Record(ctx context.Context, entry Entry) error {
	if s == nil || s.db == nil {
		return nil
	}
	beforeJSON, err := marshalJSON(entry.Before)
	if err != nil {
		return err
	}
	afterJSON, err := marshalJSON(entry.After)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO admin_operation_logs
		(operator_id, operator_name, action, target_type, target_id, ip, user_agent, before_data, after_data, summary)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8::jsonb,$9::jsonb,$10)`,
		entry.OperatorID,
		entry.OperatorName,
		entry.Action,
		entry.TargetType,
		entry.TargetID,
		entry.IP,
		entry.UserAgent,
		beforeJSON,
		afterJSON,
		entry.Summary,
	)
	return err
}

func (s *Store) List(ctx context.Context, query map[string]string) (PageResult[Log], error) {
	if s == nil || s.db == nil {
		return PageResult[Log]{Items: []Log{}}, nil
	}
	where, args := buildWhere(query)
	page, pageSize := pageParams(query)

	var total int
	countQuery := "SELECT count(*) FROM admin_operation_logs WHERE " + where
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return PageResult[Log]{}, err
	}

	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (page-1)*pageSize)
	limitIndex := len(args) + 1
	offsetIndex := len(args) + 2
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT id, COALESCE(operator_id, 0), operator_name, action, target_type, target_id, ip, user_agent,
		       before_data, after_data, summary, to_char(create_time AT TIME ZONE 'Asia/Shanghai', 'YYYY/MM/DD HH24:MI:SS')
		FROM admin_operation_logs
		WHERE %s
		ORDER BY create_time DESC, id DESC
		LIMIT $%d OFFSET $%d`, where, limitIndex, offsetIndex), listArgs...)
	if err != nil {
		return PageResult[Log]{}, err
	}
	defer rows.Close()

	items := []Log{}
	for rows.Next() {
		var item Log
		if err := rows.Scan(&item.ID, &item.OperatorID, &item.OperatorName, &item.Action, &item.TargetType, &item.TargetID, &item.IP, &item.UserAgent, &item.Before, &item.After, &item.Summary, &item.CreateTime); err != nil {
			return PageResult[Log]{}, err
		}
		if len(item.Before) == 0 {
			item.Before = json.RawMessage(`{}`)
		}
		if len(item.After) == 0 {
			item.After = json.RawMessage(`{}`)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return PageResult[Log]{}, err
	}
	return PageResult[Log]{Items: items, Total: total}, nil
}

func marshalJSON(value any) (string, error) {
	if value == nil {
		return "{}", nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func buildWhere(query map[string]string) (string, []any) {
	where := []string{"1=1"}
	args := []any{}
	add := func(clause string, value any) {
		args = append(args, value)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if v := strings.TrimSpace(query["action"]); v != "" {
		add("action = $%d", v)
	}
	if v := strings.TrimSpace(query["targetType"]); v != "" {
		add("target_type = $%d", v)
	}
	if v := strings.TrimSpace(query["targetId"]); v != "" {
		add("target_id = $%d", v)
	}
	if v := strings.TrimSpace(query["operator"]); v != "" {
		add("operator_name ILIKE $%d", "%"+v+"%")
	}
	return strings.Join(where, " AND "), args
}

func pageParams(query map[string]string) (int, int) {
	page := parsePositive(query["page"], 1)
	pageSize := parsePositive(query["pageSize"], 20)
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func parsePositive(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
