package system

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Store 基于 PostgreSQL 的系统管理存储（用户 / 角色 / 菜单）。
type Store struct {
	db *sql.DB
}

func NewStore(database *sql.DB) *Store {
	return &Store{db: database}
}

const queryTimeout = 10 * time.Second

func (s *Store) ctx(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	return context.WithTimeout(parent, queryTimeout)
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006/01/02 15:04:05")
}

// ---------------- Users ----------------

func (s *Store) ListUsers(ctx context.Context, query map[string]string) (PageResult[User], error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if kw := strings.TrimSpace(query["username"]); kw != "" {
		args = append(args, "%"+strings.ToLower(kw)+"%")
		where = append(where, "(lower(username) LIKE $"+strconv.Itoa(len(args))+" OR lower(nickname) LIKE $"+strconv.Itoa(len(args))+")")
	}
	if st := query["status"]; st != "" {
		if n, err := strconv.Atoi(st); err == nil {
			args = append(args, n)
			where = append(where, "status = $"+strconv.Itoa(len(args)))
		}
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM users WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[User]{}, err
	}

	page, pageSize := pageParams(query)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		"SELECT id, username, avatar, nickname, email, phone, remark, status, create_time FROM users WHERE "+cond+
			" ORDER BY id ASC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return PageResult[User]{}, err
	}
	defer rows.Close()

	items := []User{}
	ids := []int64{}
	for rows.Next() {
		var u User
		var id int64
		var ct time.Time
		if err := rows.Scan(&id, &u.Username, &u.Avatar, &u.Nickname, &u.Email, &u.Phone, &u.Remark, &u.Status, &ct); err != nil {
			return PageResult[User]{}, err
		}
		u.ID = strconv.FormatInt(id, 10)
		u.CreateTime = fmtTime(ct)
		u.RoleIds = []string{}
		items = append(items, u)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return PageResult[User]{}, err
	}

	// 批量取每个用户的角色 id
	roleMap, err := s.rolesForUsers(c, ids)
	if err != nil {
		return PageResult[User]{}, err
	}
	for i := range items {
		if rs, ok := roleMap[items[i].ID]; ok {
			items[i].RoleIds = rs
		}
	}
	return PageResult[User]{Items: items, Total: total}, nil
}

func (s *Store) rolesForUsers(ctx context.Context, userIDs []int64) (map[string][]string, error) {
	result := map[string][]string{}
	if len(userIDs) == 0 {
		return result, nil
	}
	inClause, args := inPlaceholders(userIDs)
	rows, err := s.db.QueryContext(ctx,
		"SELECT user_id, role_id FROM user_roles WHERE user_id IN ("+inClause+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid, rid int64
		if err := rows.Scan(&uid, &rid); err != nil {
			return nil, err
		}
		key := strconv.FormatInt(uid, 10)
		result[key] = append(result[key], strconv.FormatInt(rid, 10))
	}
	return result, rows.Err()
}

func (s *Store) SaveUser(ctx context.Context, input User) (User, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	status := input.Status
	if status == 0 {
		status = 1
	}

	var userID int64
	if input.ID == "" {
		// 新建：密码必填
		if strings.TrimSpace(input.Password) == "" {
			return User{}, errors.New("新建用户必须设置密码")
		}
		hash, herr := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if herr != nil {
			return User{}, herr
		}
		if err := tx.QueryRowContext(c,
			`INSERT INTO users (username, password_hash, avatar, nickname, email, phone, remark, status)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
			input.Username, string(hash), input.Avatar, input.Nickname, input.Email, input.Phone, input.Remark, status,
		).Scan(&userID); err != nil {
			return User{}, err
		}
	} else {
		userID, err = strconv.ParseInt(input.ID, 10, 64)
		if err != nil {
			return User{}, errors.New("invalid user id")
		}
		if _, err := tx.ExecContext(c,
			`UPDATE users SET username=$1, avatar=$2, nickname=$3, email=$4, phone=$5, remark=$6, status=$7 WHERE id=$8`,
			input.Username, input.Avatar, input.Nickname, input.Email, input.Phone, input.Remark, status, userID,
		); err != nil {
			return User{}, err
		}
		// 仅当传了新密码才更新
		if strings.TrimSpace(input.Password) != "" {
			hash, herr := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
			if herr != nil {
				return User{}, herr
			}
			if _, err := tx.ExecContext(c, `UPDATE users SET password_hash=$1 WHERE id=$2`, string(hash), userID); err != nil {
				return User{}, err
			}
		}
	}

	// 重置角色关联
	if _, err := tx.ExecContext(c, `DELETE FROM user_roles WHERE user_id=$1`, userID); err != nil {
		return User{}, err
	}
	for _, rid := range input.RoleIds {
		roleID, perr := strconv.ParseInt(rid, 10, 64)
		if perr != nil {
			continue
		}
		if _, err := tx.ExecContext(c, `INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, roleID); err != nil {
			return User{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	input.ID = strconv.FormatInt(userID, 10)
	input.Password = ""
	return input, nil
}

func (s *Store) DeleteUser(ctx context.Context, id string) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) CurrentUserProfile(ctx context.Context, id int64, homePath string) (CurrentUserProfile, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	var profile CurrentUserProfile
	err := s.db.QueryRowContext(c,
		`SELECT id, username, avatar, nickname, email, phone, remark FROM users WHERE id=$1 AND status=1`,
		id,
	).Scan(&profile.ID, &profile.Username, &profile.Avatar, &profile.RealName, &profile.Email, &profile.Phone, &profile.Remark)
	if err != nil {
		return CurrentUserProfile{}, err
	}
	profile.UserID = strconv.FormatInt(profile.ID, 10)
	profile.HomePath = homePath
	if profile.HomePath == "" {
		profile.HomePath = "/website/overview"
	}

	rows, err := s.db.QueryContext(c,
		`SELECT r.code FROM roles r JOIN user_roles ur ON ur.role_id=r.id WHERE ur.user_id=$1 AND r.status=1 ORDER BY r.id ASC`,
		id)
	if err != nil {
		return CurrentUserProfile{}, err
	}
	defer rows.Close()
	profile.Roles = []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return CurrentUserProfile{}, err
		}
		profile.Roles = append(profile.Roles, code)
	}
	if err := rows.Err(); err != nil {
		return CurrentUserProfile{}, err
	}
	return profile, nil
}

func (s *Store) UpdateCurrentUserProfile(ctx context.Context, id int64, input ProfileUpdate, homePath string) (CurrentUserProfile, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	username := strings.TrimSpace(input.Username)
	realName := strings.TrimSpace(input.RealName)
	if username == "" {
		return CurrentUserProfile{}, errors.New("username is required")
	}
	if realName == "" {
		return CurrentUserProfile{}, errors.New("realName is required")
	}

	res, err := s.db.ExecContext(c,
		`UPDATE users
		 SET username=$1, avatar=$2, nickname=$3, email=$4, phone=$5, remark=$6
		 WHERE id=$7`,
		username,
		strings.TrimSpace(input.Avatar),
		realName,
		strings.TrimSpace(input.Email),
		strings.TrimSpace(input.Phone),
		strings.TrimSpace(input.Remark),
		id,
	)
	if err != nil {
		return CurrentUserProfile{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return CurrentUserProfile{}, sql.ErrNoRows
	}
	return s.CurrentUserProfile(ctx, id, homePath)
}

// ---------------- Roles ----------------

func (s *Store) ListRoles(ctx context.Context, query map[string]string) (PageResult[Role], error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	where := []string{"1=1"}
	args := []any{}
	if kw := strings.TrimSpace(query["name"]); kw != "" {
		args = append(args, "%"+strings.ToLower(kw)+"%")
		where = append(where, "(lower(name) LIKE $"+strconv.Itoa(len(args))+" OR lower(code) LIKE $"+strconv.Itoa(len(args))+")")
	}
	if st := query["status"]; st != "" {
		if n, err := strconv.Atoi(st); err == nil {
			args = append(args, n)
			where = append(where, "status = $"+strconv.Itoa(len(args)))
		}
	}
	cond := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRowContext(c, "SELECT count(*) FROM roles WHERE "+cond, args...).Scan(&total); err != nil {
		return PageResult[Role]{}, err
	}

	page, pageSize := pageParams(query)
	offset := (page - 1) * pageSize
	args = append(args, pageSize, offset)
	rows, err := s.db.QueryContext(c,
		"SELECT id, code, name, remark, status, create_time FROM roles WHERE "+cond+
			" ORDER BY id ASC LIMIT $"+strconv.Itoa(len(args)-1)+" OFFSET $"+strconv.Itoa(len(args)), args...)
	if err != nil {
		return PageResult[Role]{}, err
	}
	defer rows.Close()

	items := []Role{}
	ids := []int64{}
	for rows.Next() {
		var r Role
		var id int64
		var ct time.Time
		if err := rows.Scan(&id, &r.Code, &r.Name, &r.Remark, &r.Status, &ct); err != nil {
			return PageResult[Role]{}, err
		}
		r.ID = strconv.FormatInt(id, 10)
		r.CreateTime = fmtTime(ct)
		r.MenuIds = []int64{}
		items = append(items, r)
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return PageResult[Role]{}, err
	}

	menuMap, err := s.menusForRoles(c, ids)
	if err != nil {
		return PageResult[Role]{}, err
	}
	for i := range items {
		if ms, ok := menuMap[items[i].ID]; ok {
			items[i].MenuIds = ms
		}
	}
	return PageResult[Role]{Items: items, Total: total}, nil
}

func (s *Store) menusForRoles(ctx context.Context, roleIDs []int64) (map[string][]int64, error) {
	result := map[string][]int64{}
	if len(roleIDs) == 0 {
		return result, nil
	}
	inClause, args := inPlaceholders(roleIDs)
	rows, err := s.db.QueryContext(ctx,
		"SELECT role_id, menu_id FROM role_menus WHERE role_id IN ("+inClause+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var rid, mid int64
		if err := rows.Scan(&rid, &mid); err != nil {
			return nil, err
		}
		key := strconv.FormatInt(rid, 10)
		result[key] = append(result[key], mid)
	}
	return result, rows.Err()
}

func (s *Store) SaveRole(ctx context.Context, input Role) (Role, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(c, nil)
	if err != nil {
		return Role{}, err
	}
	defer func() { _ = tx.Rollback() }()

	status := input.Status
	if status == 0 {
		status = 1
	}

	var roleID int64
	if input.ID == "" {
		if err := tx.QueryRowContext(c,
			`INSERT INTO roles (code, name, remark, status) VALUES ($1,$2,$3,$4) RETURNING id`,
			input.Code, input.Name, input.Remark, status,
		).Scan(&roleID); err != nil {
			return Role{}, err
		}
	} else {
		roleID, err = strconv.ParseInt(input.ID, 10, 64)
		if err != nil {
			return Role{}, errors.New("invalid role id")
		}
		if _, err := tx.ExecContext(c,
			`UPDATE roles SET code=$1, name=$2, remark=$3, status=$4 WHERE id=$5`,
			input.Code, input.Name, input.Remark, status, roleID,
		); err != nil {
			return Role{}, err
		}
	}

	if _, err := tx.ExecContext(c, `DELETE FROM role_menus WHERE role_id=$1`, roleID); err != nil {
		return Role{}, err
	}
	for _, mid := range input.MenuIds {
		if _, err := tx.ExecContext(c, `INSERT INTO role_menus (role_id, menu_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, roleID, mid); err != nil {
			return Role{}, err
		}
	}

	if err := tx.Commit(); err != nil {
		return Role{}, err
	}
	input.ID = strconv.FormatInt(roleID, 10)
	return input, nil
}

func (s *Store) DeleteRole(ctx context.Context, id string) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c, `DELETE FROM roles WHERE id=$1`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ---------------- Menus ----------------

// listMenusFlat 返回扁平菜单（按 sort 排序），内部用。
func (s *Store) listMenusFlat(ctx context.Context) ([]MenuItem, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, pid, name, path, component, auth_code, type, status, sort, meta FROM menus ORDER BY sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []MenuItem{}
	for rows.Next() {
		var m MenuItem
		var metaRaw []byte
		if err := rows.Scan(&m.ID, &m.PID, &m.Name, &m.Path, &m.Component, &m.AuthCode, &m.Type, &m.Status, &m.Sort, &metaRaw); err != nil {
			return nil, err
		}
		m.Meta = map[string]any{}
		if len(metaRaw) > 0 {
			_ = json.Unmarshal(metaRaw, &m.Meta)
		}
		items = append(items, m)
	}
	return items, rows.Err()
}

// ListMenus 返回树形菜单（用于前端菜单页与角色权限树）。
func (s *Store) ListMenus(ctx context.Context) ([]MenuItem, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	flat, err := s.listMenusFlat(c)
	if err != nil {
		return nil, err
	}
	return buildTree(flat, 0), nil
}

func buildTree(flat []MenuItem, pid int64) []MenuItem {
	nodes := []MenuItem{}
	for _, m := range flat {
		if m.PID == pid {
			children := buildTree(flat, m.ID)
			if len(children) > 0 {
				m.Children = children
			}
			nodes = append(nodes, m)
		}
	}
	return nodes
}

func (s *Store) SaveMenu(ctx context.Context, input MenuItem) (MenuItem, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	status := input.Status
	if status == 0 {
		status = 1
	}
	if input.Type == "" {
		input.Type = "menu"
	}
	if input.Meta == nil {
		input.Meta = map[string]any{"title": input.Name}
	}
	metaRaw, _ := json.Marshal(input.Meta)

	if input.ID == 0 {
		if err := s.db.QueryRowContext(c,
			`INSERT INTO menus (pid, name, path, component, auth_code, type, status, sort, meta)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) RETURNING id`,
			input.PID, input.Name, input.Path, input.Component, input.AuthCode, input.Type, status, input.Sort, string(metaRaw),
		).Scan(&input.ID); err != nil {
			return MenuItem{}, err
		}
		return input, nil
	}

	if _, err := s.db.ExecContext(c,
		`UPDATE menus SET pid=$1, name=$2, path=$3, component=$4, auth_code=$5, type=$6, status=$7, sort=$8, meta=$9::jsonb WHERE id=$10`,
		input.PID, input.Name, input.Path, input.Component, input.AuthCode, input.Type, status, input.Sort, string(metaRaw), input.ID,
	); err != nil {
		return MenuItem{}, err
	}
	return input, nil
}

func (s *Store) DeleteMenu(ctx context.Context, id int64) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	res, err := s.db.ExecContext(c,
		`WITH RECURSIVE descendants AS (
			SELECT id FROM menus WHERE id=$1
			UNION ALL
			SELECT m.id FROM menus m
			INNER JOIN descendants d ON m.pid=d.id
		)
		DELETE FROM menus WHERE id IN (SELECT id FROM descendants)`,
		id,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) MenuNameExists(ctx context.Context, name string, id int64) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var exists bool
	err := s.db.QueryRowContext(c, `SELECT EXISTS(SELECT 1 FROM menus WHERE name=$1 AND id<>$2)`, name, id).Scan(&exists)
	return exists, err
}

func (s *Store) MenuPathExists(ctx context.Context, path string, id int64) (bool, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()
	var exists bool
	err := s.db.QueryRowContext(c, `SELECT EXISTS(SELECT 1 FROM menus WHERE path=$1 AND id<>$2)`, path, id).Scan(&exists)
	return exists, err
}

// ---------------- RBAC 查询（供登录/菜单/权限码用）----------------

// AuthUser 校验用户名密码，成功返回用户基本信息与角色 code 列表。
func (s *Store) AuthUser(ctx context.Context, username, password string) (id int64, nickname string, roleCodes []string, ok bool, err error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	var hash string
	var status int
	row := s.db.QueryRowContext(c, `SELECT id, password_hash, nickname, status FROM users WHERE username=$1`, username)
	if err = row.Scan(&id, &hash, &nickname, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, "", nil, false, nil
		}
		return 0, "", nil, false, err
	}
	if status != 1 {
		return 0, "", nil, false, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return 0, "", nil, false, nil
	}
	roleCodes, err = s.roleCodesForUser(c, id)
	if err != nil {
		return 0, "", nil, false, err
	}
	return id, nickname, roleCodes, true, nil
}

func (s *Store) roleCodesForUser(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.code FROM roles r JOIN user_roles ur ON ur.role_id=r.id WHERE ur.user_id=$1 AND r.status=1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codes := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

// MenusForUser 返回该用户（按角色）可见的菜单树。admin 角色可见全部。
func (s *Store) MenusForUser(ctx context.Context, userID int64, roleCodes []string) ([]MenuItem, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	flat, err := s.listMenusFlat(c)
	if err != nil {
		return nil, err
	}
	// 只保留启用的菜单
	enabled := make([]MenuItem, 0, len(flat))
	for _, m := range flat {
		if m.Status == 1 {
			enabled = append(enabled, m)
		}
	}

	if hasAdmin(roleCodes) {
		return buildTree(enabled, 0), nil
	}

	allowed, err := s.allowedMenuIDs(c, userID)
	if err != nil {
		return nil, err
	}
	filtered := make([]MenuItem, 0, len(enabled))
	for _, m := range enabled {
		if allowed[m.ID] {
			filtered = append(filtered, m)
		}
	}
	return buildTree(filtered, 0), nil
}

// AuthCodesForUser 返回该用户的权限码集合（菜单 auth_code 去重）。
func (s *Store) AuthCodesForUser(ctx context.Context, userID int64, roleCodes []string) ([]string, error) {
	c, cancel := s.ctx(ctx)
	defer cancel()

	var rows *sql.Rows
	var err error
	if hasAdmin(roleCodes) {
		rows, err = s.db.QueryContext(c, `SELECT DISTINCT auth_code FROM menus WHERE auth_code <> '' AND status=1`)
	} else {
		rows, err = s.db.QueryContext(c,
			`SELECT DISTINCT m.auth_code FROM menus m
			 JOIN role_menus rm ON rm.menu_id=m.id
			 JOIN user_roles ur ON ur.role_id=rm.role_id
			 WHERE ur.user_id=$1 AND m.auth_code <> '' AND m.status=1`, userID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codes := []string{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}

func (s *Store) allowedMenuIDs(ctx context.Context, userID int64) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT rm.menu_id FROM role_menus rm
		 JOIN user_roles ur ON ur.role_id=rm.role_id
		 WHERE ur.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	allowed := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		allowed[id] = true
		// 同时放行其父级，保证目录可见
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.expandParents(ctx, allowed)
}

// expandParents 把已选菜单的所有父级目录也标记为可见。
func (s *Store) expandParents(ctx context.Context, allowed map[int64]bool) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, pid FROM menus`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	pidOf := map[int64]int64{}
	for rows.Next() {
		var id, pid int64
		if err := rows.Scan(&id, &pid); err != nil {
			return nil, err
		}
		pidOf[id] = pid
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for id := range allowed {
		cur := id
		for {
			pid := pidOf[cur]
			if pid == 0 || allowed[pid] {
				break
			}
			allowed[pid] = true
			cur = pid
		}
	}
	return allowed, nil
}

func hasAdmin(roleCodes []string) bool {
	for _, c := range roleCodes {
		if c == "admin" {
			return true
		}
	}
	return false
}

// ---------------- helpers ----------------

func pageParams(query map[string]string) (int, int) {
	page, _ := strconv.Atoi(query["page"])
	pageSize, _ := strconv.Atoi(query["pageSize"])
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	return page, pageSize
}

// inPlaceholders 为 IN (...) 生成 $1,$2,... 占位符与对应参数。
func inPlaceholders(values []int64) (string, []any) {
	parts := make([]string, len(values))
	args := make([]any, len(values))
	for i, v := range values {
		parts[i] = "$" + strconv.Itoa(i+1)
		args[i] = v
	}
	return strings.Join(parts, ","), args
}
