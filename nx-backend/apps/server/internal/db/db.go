package db

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed schema.sql
var schemaSQL string

// Open 连接 PostgreSQL，执行迁移并播种初始数据。
// dsn 形如：postgres://user:pass@host:5432/dbname?sslmode=disable
// adminUser/adminPassword 用于首次播种超级管理员账号。
func Open(ctx context.Context, dsn, adminUser, adminPassword string) (*sql.DB, error) {
	database, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	database.SetMaxOpenConns(20)
	database.SetMaxIdleConns(5)
	database.SetConnMaxLifetime(time.Hour)

	// 等待数据库就绪（容器编排下 server 可能比 postgres 先起）
	if err := waitReady(ctx, database); err != nil {
		return nil, err
	}

	if _, err := database.ExecContext(ctx, schemaSQL); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	if err := seed(ctx, database, adminUser, adminPassword); err != nil {
		return nil, fmt.Errorf("seed: %w", err)
	}

	return database, nil
}

func waitReady(ctx context.Context, database *sql.DB) error {
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := database.PingContext(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return fmt.Errorf("database not ready: %w", lastErr)
}

// seed 补齐初始数据，保证幂等、不覆盖用户后续修改。
func seed(ctx context.Context, database *sql.DB, adminUser, adminPassword string) error {
	if err := seedMenus(ctx, database); err != nil {
		return err
	}
	if err := removeDeprecatedMenus(ctx, database); err != nil {
		return err
	}
	if err := seedRoles(ctx, database); err != nil {
		return err
	}
	if err := seedMindQuotes(ctx, database); err != nil {
		return err
	}
	return seedAdmin(ctx, database, adminUser, adminPassword)
}

type seedMenu struct {
	ID        int64
	PID       int64
	Name      string
	Path      string
	Component string
	AuthCode  string
	Type      string
	Sort      int
	Icon      string
	Title     string
}

// 默认菜单树：官网管理 + 系统管理。id 固定，便于角色绑定与幂等。
var defaultMenus = []seedMenu{
	{ID: 200, PID: 0, Name: "DashboardAnalytics", Path: "/dashboard/analytics", Component: "/dashboard/analytics", AuthCode: "Analytics:Overview", Type: "menu", Sort: 1, Icon: "lucide:chart-column", Title: "数据概览"},
	{ID: 201, PID: 0, Name: "DashboardGameResults", Path: "/dashboard/game-results", Component: "/dashboard/game-results", AuthCode: "Analytics:GameResults", Type: "menu", Sort: 2, Icon: "lucide:gamepad-2", Title: "小游戏统计"},
	{ID: 300, PID: 0, Name: "WebsiteManage", Path: "/website", Type: "catalog", Sort: 10, Icon: "lucide:globe-2", Title: "官网管理"},
	{ID: 301, PID: 300, Name: "WebsiteOverview", Path: "/website/overview", Component: "/site-config/overview", AuthCode: "Website:Read", Type: "menu", Sort: 1, Icon: "lucide:layout-dashboard", Title: "管理概览"},
	{ID: 302, PID: 300, Name: "WebsiteSiteSettings", Path: "/website/site", Component: "/site-config/site", AuthCode: "Website:Write", Type: "menu", Sort: 2, Icon: "lucide:settings-2", Title: "站点设置"},
	{ID: 304, PID: 300, Name: "WebsiteHome", Path: "/website/home", Component: "/site-config/home", AuthCode: "Website:Write", Type: "menu", Sort: 4, Icon: "lucide:home", Title: "首页管理"},
	{ID: 305, PID: 300, Name: "WebsiteCourses", Path: "/website/courses", Component: "/site-config/courses", AuthCode: "Website:Write", Type: "menu", Sort: 5, Icon: "lucide:book-open", Title: "课程管理"},
	{ID: 306, PID: 300, Name: "WebsiteTeacher", Path: "/website/teacher", Component: "/site-config/teacher", AuthCode: "Website:Write", Type: "menu", Sort: 6, Icon: "lucide:user-round", Title: "老师管理"},
	{ID: 307, PID: 300, Name: "WebsiteStages", Path: "/website/stages", Component: "/site-config/stages", AuthCode: "Website:Write", Type: "menu", Sort: 7, Icon: "lucide:layers-3", Title: "三阶段"},
	{ID: 308, PID: 300, Name: "WebsiteEnterprise", Path: "/website/enterprise", Component: "/site-config/enterprise", AuthCode: "Website:Write", Type: "menu", Sort: 8, Icon: "lucide:building-2", Title: "企业课程"},
	{ID: 309, PID: 300, Name: "WebsiteQuotes", Path: "/website/quotes", Component: "/site-config/quotes", AuthCode: "Website:Write", Type: "menu", Sort: 9, Icon: "lucide:quote", Title: "语录互动"},
	{ID: 310, PID: 300, Name: "WebsiteTypes", Path: "/website/types", Component: "/site-config/types", AuthCode: "Website:Write", Type: "menu", Sort: 10, Icon: "lucide:circle-dot", Title: "九型数据"},
	{ID: 311, PID: 300, Name: "WebsiteSignup", Path: "/website/signup", Component: "/site-config/signup", AuthCode: "Website:Write", Type: "menu", Sort: 11, Icon: "lucide:clipboard-edit", Title: "报名表单"},
	{ID: 312, PID: 300, Name: "WebsiteJson", Path: "/website/json", Component: "/site-config/json", AuthCode: "Website:Write", Type: "menu", Sort: 12, Icon: "lucide:braces", Title: "JSON 高级"},
	{ID: 314, PID: 300, Name: "WebsiteMindQuotes", Path: "/website/mind-quotes", Component: "/site-config/mind-quotes", AuthCode: "Website:Write", Type: "menu", Sort: 13, Icon: "lucide:sparkles", Title: "心语管理"},
	{ID: 500, PID: 0, Name: "CustomerManage", Path: "/customer", Type: "catalog", Sort: 15, Icon: "lucide:contact-round", Title: "客户管理"},
	{ID: 501, PID: 500, Name: "CustomerSignupLeads", Path: "/customer/signups", Component: "/site-config/signup-leads", AuthCode: "Customer:Signup:List", Type: "menu", Sort: 1, Icon: "lucide:inbox", Title: "报名信息"},
	{ID: 600, PID: 0, Name: "MessageCenter", Path: "/message", Type: "catalog", Sort: 18, Icon: "lucide:bell-ring", Title: "消息中心"},
	{ID: 601, PID: 600, Name: "MessageManagement", Path: "/message/management", Component: "/message/management", AuthCode: "Message:Manage:List", Type: "menu", Sort: 1, Icon: "lucide:mail-check", Title: "消息管理"},
	{ID: 700, PID: 0, Name: "VoiceCenter", Path: "/voice", Type: "catalog", Sort: 19, Icon: "lucide:audio-lines", Title: "人声管理"},
	{ID: 701, PID: 700, Name: "VoiceProfiles", Path: "/voice/profiles", Component: "/voice/profiles", AuthCode: "Voice:Profile:Manage", Type: "menu", Sort: 1, Icon: "lucide:mic-vocal", Title: "人声管理"},
	{ID: 702, PID: 700, Name: "VoiceTest", Path: "/voice/test", Component: "/voice/test", AuthCode: "Voice:Test:Manage", Type: "menu", Sort: 2, Icon: "lucide:headphones", Title: "声音测试"},
	{ID: 703, PID: 700, Name: "VoiceContent", Path: "/voice/content", Component: "/voice/content", AuthCode: "Voice:Content:Manage", Type: "menu", Sort: 3, Icon: "lucide:file-audio", Title: "内容转语音"},
	{ID: 800, PID: 0, Name: "RAGCenter", Path: "/rag", Type: "catalog", Sort: 19, Icon: "lucide:brain-circuit", Title: "RAG 知识库"},
	{ID: 801, PID: 800, Name: "RAGKnowledge", Path: "/rag/knowledge", Component: "/rag/knowledge", AuthCode: "RAG:Knowledge:Manage", Type: "menu", Sort: 1, Icon: "lucide:library-big", Title: "知识库管理"},
	{ID: 900, PID: 0, Name: "ReadingCenter", Path: "/reading", Type: "catalog", Sort: 19, Icon: "lucide:book-open-text", Title: "阅读管理"},
	{ID: 901, PID: 900, Name: "ReadingArticles", Path: "/reading/articles", Component: "/reading/articles", AuthCode: "Reading:Article:Manage", Type: "menu", Sort: 1, Icon: "lucide:newspaper", Title: "文章管理"},
	{ID: 400, PID: 0, Name: "SystemManage", Path: "/system", Type: "catalog", Sort: 20, Icon: "lucide:shield-check", Title: "系统管理"},
	{ID: 401, PID: 400, Name: "SystemUser", Path: "/system/user", Component: "/system/user/list", AuthCode: "System:User:List", Type: "menu", Sort: 1, Icon: "lucide:users", Title: "用户管理"},
	{ID: 402, PID: 400, Name: "SystemRole", Path: "/system/role", Component: "/system/role/list", AuthCode: "System:Role:List", Type: "menu", Sort: 2, Icon: "lucide:user-cog", Title: "角色管理"},
	{ID: 403, PID: 400, Name: "SystemMenu", Path: "/system/menu", Component: "/system/menu/list", AuthCode: "System:Menu:List", Type: "menu", Sort: 3, Icon: "lucide:panel-left", Title: "菜单权限"},
	{ID: 404, PID: 400, Name: "SystemBranding", Path: "/system/branding", Component: "/system/branding", AuthCode: "System:Branding", Type: "menu", Sort: 4, Icon: "lucide:palette", Title: "后台品牌"},
}

func seedMenus(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range defaultMenus {
		meta := fmt.Sprintf(`{"icon":%q,"title":%q}`, m.Icon, m.Title)
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO menus (id, pid, name, path, component, auth_code, type, status, sort, meta)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,1,$8,$9::jsonb)
			 ON CONFLICT (id) DO UPDATE
			 SET pid=EXCLUDED.pid,
			     name=EXCLUDED.name,
			     path=EXCLUDED.path,
			     component=EXCLUDED.component,
			     auth_code=EXCLUDED.auth_code,
			     type=EXCLUDED.type,
			     sort=EXCLUDED.sort,
			     meta=EXCLUDED.meta`,
			m.ID, m.PID, m.Name, m.Path, m.Component, m.AuthCode, m.Type, m.Sort, meta,
		); err != nil {
			return err
		}
	}
	// 让序列跳过手工指定的固定 id，避免后续自增冲突。
	if _, err := tx.ExecContext(ctx,
		`SELECT setval(pg_get_serial_sequence('menus','id'), (SELECT max(id) FROM menus))`); err != nil {
		return err
	}
	return tx.Commit()
}

func removeDeprecatedMenus(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx,
		`DELETE FROM menus
		 WHERE id = 303
		    OR id = 313
		    OR name = 'WebsiteNavigation'
		    OR name = 'WebsiteSignupLeads'
		    OR path = '/website/navigation'
		    OR path = '/website/signup-leads'
		    OR component = '/site-config/navigation'`,
	)
	return err
}

func seedRoles(ctx context.Context, database *sql.DB) error {
	var count int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM roles").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		_, err := database.ExecContext(ctx,
			`INSERT INTO role_menus (role_id, menu_id)
			 SELECT r.id, m.id FROM roles r CROSS JOIN menus m
			 WHERE r.code='admin'
			 ON CONFLICT DO NOTHING`,
		)
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 超级管理员：拥有全部菜单。
	var adminRoleID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO roles (code, name, remark, status) VALUES ('admin','超级管理员','拥有全部后台权限',1) RETURNING id`,
	).Scan(&adminRoleID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO role_menus (role_id, menu_id) SELECT $1, id FROM menus`, adminRoleID); err != nil {
		return err
	}
	return tx.Commit()
}

// seedMindQuotes 仅当 mind_quotes 为空时，导入默认分组与 PDF 提炼的 27 条心语。
// 心语默认不分组（group_id=NULL），由后台手动归入 脑/心/腹 等组。幂等。
func seedMindQuotes(ctx context.Context, database *sql.DB) error {
	var count int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM mind_quotes").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 默认分组（仅当 mind_groups 为空时建）
	var groupCount int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM mind_groups").Scan(&groupCount); err != nil {
		return err
	}
	if groupCount == 0 {
		for _, g := range defaultMindGroups {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO mind_groups (name, intro, sort, status) VALUES ($1,$2,$3,'enabled')`,
				g.Name, g.Intro, g.Sort); err != nil {
				return err
			}
		}
	}

	for _, q := range defaultMindQuotes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO mind_quotes (group_id, title, content, prompt, sort, status)
			 VALUES (NULL, $1, $2, $3, $4, 'enabled')`,
			q.Title, q.Content, q.Prompt, q.Sort); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func seedAdmin(ctx context.Context, database *sql.DB, adminUser, adminPassword string) error {
	if adminUser == "" {
		adminUser = "admin"
	}
	if adminPassword == "" {
		adminPassword = "123456"
	}

	var exists bool
	if err := database.QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM users WHERE username=$1)", adminUser).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var userID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO users (username, password_hash, nickname, status) VALUES ($1,$2,'超级管理员',1) RETURNING id`,
		adminUser, string(hash),
	).Scan(&userID); err != nil {
		return err
	}

	var adminRoleID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE code='admin'`).Scan(&adminRoleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if adminRoleID != 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2)`, userID, adminRoleID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
