package db

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
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
	if err := seedQuizQuestions(ctx, database); err != nil {
		return err
	}
	return seedAdmin(ctx, database, adminUser, adminPassword)
}

type seedMenu struct {
	ID         int64
	PID        int64
	Name       string
	Path       string
	Component  string
	AuthCode   string
	Type       string
	Sort       int
	Icon       string
	Title      string
	HideInMenu bool
	ActiveMenu string
	ActivePath string
}

// 默认菜单树：官网管理 + 系统管理。id 固定，便于角色绑定与幂等。
var defaultMenus = []seedMenu{
	{ID: 200, PID: 0, Name: "DashboardAnalytics", Path: "/dashboard/analytics", Component: "/dashboard/analytics", AuthCode: "Analytics:Overview", Type: "menu", Sort: 1, Icon: "lucide:chart-column", Title: "数据概览"},
	{ID: 201, PID: 0, Name: "DashboardGameResults", Path: "/dashboard/game-results", Component: "/dashboard/game-results", AuthCode: "Analytics:GameResults", Type: "menu", Sort: 2, Icon: "lucide:gamepad-2", Title: "小游戏统计"},
	{ID: 202, PID: 0, Name: "DashboardAppAnalytics", Path: "/dashboard/app", Component: "/dashboard/app", AuthCode: "Analytics:App:Overview", Type: "menu", Sort: 3, Icon: "lucide:smartphone", Title: "App 数据看板"},
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
	{ID: 315, PID: 300, Name: "WebsiteAppReleases", Path: "/website/app-releases", Component: "/site-config/app-releases", AuthCode: "Website:AppReleases:List", Type: "menu", Sort: 14, Icon: "lucide:package-open", Title: "App 版本"},
	{ID: 316, PID: 315, Name: "WebsiteAppReleasesWrite", AuthCode: "Website:AppReleases:Write", Type: "button", Sort: 1, Icon: "lucide:pencil", Title: "管理 App 版本"},
	{ID: 1300, PID: 0, Name: "MiniappManage", Path: "/miniapp", Type: "catalog", Sort: 12, Icon: "lucide:smartphone", Title: "小程序管理"},
	{ID: 1301, PID: 1300, Name: "MiniappHome", Path: "/miniapp/home", Component: "/miniapp/home", AuthCode: "Website:Write", Type: "menu", Sort: 1, Icon: "lucide:images", Title: "首页管理"},
	{ID: 500, PID: 0, Name: "CustomerManage", Path: "/customer", Type: "catalog", Sort: 15, Icon: "lucide:contact-round", Title: "客户管理"},
	{ID: 501, PID: 500, Name: "CustomerSignupLeads", Path: "/customer/signups", Component: "/site-config/signup-leads", AuthCode: "Customer:Signup:List", Type: "menu", Sort: 1, Icon: "lucide:inbox", Title: "报名信息"},
	{ID: 502, PID: 500, Name: "CustomerAppUsers", Path: "/customer/app-users", Component: "/customer/app-users", AuthCode: "Customer:App:List", Type: "menu", Sort: 2, Icon: "lucide:smartphone", Title: "App 客户"},
	{ID: 503, PID: 502, Name: "CustomerAppUsersEdit", AuthCode: "Customer:App:Write", Type: "button", Sort: 1, Icon: "lucide:pencil", Title: "编辑 App 客户"},
	{ID: 504, PID: 500, Name: "CustomerUserInsights", Path: "/customer/user-insights", Component: "/customer/user-insights", AuthCode: "Customer:UserInsights:List", Type: "menu", Sort: 3, Icon: "lucide:user-search", Title: "用户提炼数据"},
	{ID: 505, PID: 500, Name: "CustomerAppOrders", Path: "/customer/app-orders", Component: "/customer/app-orders", AuthCode: "Customer:AppOrders:List", Type: "menu", Sort: 4, Icon: "lucide:receipt-text", Title: "App 订单"},
	{ID: 506, PID: 505, Name: "CustomerAppOrdersGrant", AuthCode: "Customer:AppOrders:Write", Type: "button", Sort: 1, Icon: "lucide:badge-check", Title: "补发订单权益"},
	{ID: 507, PID: 500, Name: "CustomerAppChat", Path: "/customer/app-chat", Component: "/customer/app-chat", AuthCode: "Customer:AppChat:List", Type: "menu", Sort: 5, Icon: "lucide:messages-square", Title: "聊天质检"},
	{ID: 508, PID: 500, Name: "CustomerAppMemory", Path: "/customer/app-memories", Component: "/customer/app-memories", AuthCode: "Customer:AppMemory:List", Type: "menu", Sort: 6, Icon: "lucide:database-zap", Title: "私库记忆"},
	{ID: 509, PID: 508, Name: "CustomerAppMemoryWrite", AuthCode: "Customer:AppMemory:Write", Type: "button", Sort: 1, Icon: "lucide:pencil", Title: "管理私库记忆"},
	{ID: 510, PID: 500, Name: "CustomerQuizQuestions", Path: "/customer/quiz-questions", Component: "/quiz/questions", AuthCode: "Website:Write", Type: "menu", Sort: 7, Icon: "lucide:list-checks", Title: "测评题库"},
	{ID: 1200, PID: 0, Name: "ProfileCalibration", Path: "/profile-calibration", Type: "catalog", Sort: 17, Icon: "lucide:badge-check", Title: "画像校准"},
	{ID: 1201, PID: 1200, Name: "DailyQuizBank", Path: "/profile-calibration/daily-quiz-bank", Component: "/profile-calibration/daily-quiz-bank", AuthCode: "ProfileCalibration:DailyQuiz:Manage", Type: "menu", Sort: 1, Icon: "lucide:list-checks", Title: "每日题库管理"},
	{ID: 603, PID: 1200, Name: "DailyQuizPushRecords", Path: "/profile-calibration/daily-quiz-push", Component: "/message/daily-quiz-push", AuthCode: "ProfileCalibration:DailyQuiz:Manage", Type: "menu", Sort: 2, Icon: "lucide:send", Title: "每日题推送记录"},
	{ID: 600, PID: 0, Name: "MessageCenter", Path: "/message", Type: "catalog", Sort: 18, Icon: "lucide:bell-ring", Title: "消息中心"},
	{ID: 601, PID: 600, Name: "MessageManagement", Path: "/message/management", Component: "/message/management", AuthCode: "Message:Manage:List", Type: "menu", Sort: 1, Icon: "lucide:mail-check", Title: "消息管理"},
	{ID: 602, PID: 600, Name: "PushManagement", Path: "/message/push", Component: "/message/push", AuthCode: "Push:Manage", Type: "menu", Sort: 2, Icon: "lucide:send", Title: "推送管理"},
	{ID: 700, PID: 0, Name: "VoiceCenter", Path: "/voice", Type: "catalog", Sort: 19, Icon: "lucide:audio-lines", Title: "人声管理"},
	{ID: 701, PID: 700, Name: "VoiceProfiles", Path: "/voice/profiles", Component: "/voice/profiles", AuthCode: "Voice:Profile:Manage", Type: "menu", Sort: 1, Icon: "lucide:mic-vocal", Title: "人声管理"},
	{ID: 702, PID: 700, Name: "VoiceTest", Path: "/voice/test", Component: "/voice/test", AuthCode: "Voice:Test:Manage", Type: "menu", Sort: 2, Icon: "lucide:headphones", Title: "声音测试"},
	{ID: 703, PID: 700, Name: "VoiceContent", Path: "/voice/content", Component: "/voice/content", AuthCode: "Voice:Content:Manage", Type: "menu", Sort: 3, Icon: "lucide:file-audio", Title: "内容转语音"},
	{ID: 800, PID: 0, Name: "RAGCenter", Path: "/rag", Type: "catalog", Sort: 19, Icon: "lucide:brain-circuit", Title: "RAG 知识库"},
	{ID: 801, PID: 800, Name: "RAGKnowledge", Path: "/rag/knowledge", Component: "/rag/knowledge", AuthCode: "RAG:Knowledge:Manage", Type: "menu", Sort: 1, Icon: "lucide:library-big", Title: "知识库管理"},
	{ID: 900, PID: 0, Name: "ReadingCenter", Path: "/reading", Type: "catalog", Sort: 19, Icon: "lucide:book-open-text", Title: "阅读管理"},
	{ID: 901, PID: 900, Name: "ReadingArticles", Path: "/reading/articles", Component: "/reading/articles", AuthCode: "Reading:Article:Manage", Type: "menu", Sort: 1, Icon: "lucide:newspaper", Title: "文章管理"},
	{ID: 1000, PID: 0, Name: "VideoCenter", Path: "/video", Type: "catalog", Sort: 19, Icon: "lucide:clapperboard", Title: "视频生成"},
	{ID: 1001, PID: 1000, Name: "VideoGenerate", Path: "/video/generate", Component: "/video/generate", AuthCode: "Video:Generate:Manage", Type: "menu", Sort: 1, Icon: "lucide:film", Title: "视频生成"},
	{ID: 1002, PID: 1000, Name: "VideoAssets", Path: "/video/assets", Component: "/video/assets", AuthCode: "Video:Asset:Manage", Type: "menu", Sort: 2, Icon: "lucide:boxes", Title: "资产库"},
	{ID: 1003, PID: 1000, Name: "VideoAnalysis", Path: "/video/analysis", Component: "/video/analysis", AuthCode: "Video:Analysis:Manage", Type: "menu", Sort: 3, Icon: "lucide:scan-search", Title: "视频分析"},
	{ID: 1004, PID: 1000, Name: "VideoStoryboard", Path: "/video/storyboard", Component: "/video/storyboard", AuthCode: "Video:Storyboard:Manage", Type: "menu", Sort: 4, Icon: "lucide:panels-top-left", Title: "分镜设计"},
	{ID: 1005, PID: 1000, Name: "VideoOverview", Path: "/video/overview", Component: "/video/overview", AuthCode: "Video:Generation:Overview", Type: "menu", Sort: 5, Icon: "lucide:bar-chart-2", Title: "生成概览"},
	{ID: 1008, PID: 1000, Name: "VideoProduction", Path: "/video/production", Component: "/video/production/index", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 6, Icon: "lucide:clapperboard", Title: "制片工作台"},
	{ID: 1006, PID: 1000, Name: "VideoProjects", Path: "/video/projects", Component: "/video/projects", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 7, Icon: "lucide:folder-kanban", Title: "项目列表"},
	{ID: 1009, PID: 1000, Name: "VideoProductionShort", Path: "/video/production/short", Component: "/video/production/short", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 8, Icon: "lucide:badge-play", Title: "短片制工作台", HideInMenu: true, ActivePath: "/video/production"},
	{ID: 1007, PID: 1000, Name: "VideoProjectWorkbench", Path: "/video/projects/:id/workbench", Component: "/video/projects/workbench", AuthCode: "Video:Project:Manage", Type: "menu", Sort: 9, Icon: "lucide:panel-top", Title: "项目工作台详情", HideInMenu: true, ActivePath: "/video/projects"},
	{ID: 1100, PID: 0, Name: "ModelSettings", Path: "/settings", Type: "catalog", Sort: 21, Icon: "lucide:cpu", Title: "模型配置"},
	{ID: 1101, PID: 1100, Name: "ModelPairing", Path: "/settings/model", Component: "/settings/model", AuthCode: "System:Model:Config", Type: "menu", Sort: 1, Icon: "lucide:plug-zap", Title: "模型配对"},
	{ID: 1102, PID: 1100, Name: "AdminModelConfig", Path: "/settings/admin-model", Component: "/settings/model", AuthCode: "System:Model:Config", Type: "menu", Sort: 2, Icon: "lucide:bot", Title: "管理端大模型配置"},
	{ID: 400, PID: 0, Name: "SystemManage", Path: "/system", Type: "catalog", Sort: 20, Icon: "lucide:shield-check", Title: "系统管理"},
	{ID: 401, PID: 400, Name: "SystemUser", Path: "/system/user", Component: "/system/user/list", AuthCode: "System:User:List", Type: "menu", Sort: 1, Icon: "lucide:users", Title: "用户管理"},
	{ID: 402, PID: 400, Name: "SystemRole", Path: "/system/role", Component: "/system/role/list", AuthCode: "System:Role:List", Type: "menu", Sort: 2, Icon: "lucide:user-cog", Title: "角色管理"},
	{ID: 403, PID: 400, Name: "SystemMenu", Path: "/system/menu", Component: "/system/menu/list", AuthCode: "System:Menu:List", Type: "menu", Sort: 3, Icon: "lucide:panel-left", Title: "菜单权限"},
	{ID: 404, PID: 400, Name: "SystemBranding", Path: "/system/branding", Component: "/system/branding", AuthCode: "System:Branding", Type: "menu", Sort: 4, Icon: "lucide:palette", Title: "后台品牌"},
	{ID: 414, PID: 400, Name: "SystemAudit", Path: "/system/audit", Component: "/system/audit/list", AuthCode: "System:Audit:List", Type: "menu", Sort: 5, Icon: "lucide:scroll-text", Title: "操作审计"},
	{ID: 405, PID: 401, Name: "SystemUserCreate", AuthCode: "System:User:Create", Type: "button", Sort: 1, Icon: "lucide:user-plus", Title: "新增用户"},
	{ID: 406, PID: 401, Name: "SystemUserUpdate", AuthCode: "System:User:Update", Type: "button", Sort: 2, Icon: "lucide:user-pen", Title: "编辑用户"},
	{ID: 407, PID: 401, Name: "SystemUserDelete", AuthCode: "System:User:Delete", Type: "button", Sort: 3, Icon: "lucide:user-x", Title: "删除用户"},
	{ID: 408, PID: 402, Name: "SystemRoleCreate", AuthCode: "System:Role:Create", Type: "button", Sort: 1, Icon: "lucide:plus", Title: "新增角色"},
	{ID: 409, PID: 402, Name: "SystemRoleUpdate", AuthCode: "System:Role:Update", Type: "button", Sort: 2, Icon: "lucide:pencil", Title: "编辑角色"},
	{ID: 410, PID: 402, Name: "SystemRoleDelete", AuthCode: "System:Role:Delete", Type: "button", Sort: 3, Icon: "lucide:trash-2", Title: "删除角色"},
	{ID: 411, PID: 403, Name: "SystemMenuCreate", AuthCode: "System:Menu:Create", Type: "button", Sort: 1, Icon: "lucide:plus", Title: "新增菜单"},
	{ID: 412, PID: 403, Name: "SystemMenuUpdate", AuthCode: "System:Menu:Update", Type: "button", Sort: 2, Icon: "lucide:pencil", Title: "编辑菜单"},
	{ID: 413, PID: 403, Name: "SystemMenuDelete", AuthCode: "System:Menu:Delete", Type: "button", Sort: 3, Icon: "lucide:trash-2", Title: "删除菜单"},
}

func seedMenus(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	for _, m := range defaultMenus {
		metaMap := map[string]any{"icon": m.Icon, "title": m.Title}
		if m.HideInMenu {
			metaMap["hideInMenu"] = true
		}
		// activePath is consumed by the Vben menu; activeMenu is kept for legacy seeded menu compatibility.
		if m.ActiveMenu != "" {
			metaMap["activeMenu"] = m.ActiveMenu
			metaMap["activePath"] = m.ActiveMenu
		}
		if m.ActivePath != "" {
			metaMap["activeMenu"] = m.ActivePath
			metaMap["activePath"] = m.ActivePath
		}
		metaRaw, _ := json.Marshal(metaMap)
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
			m.ID, m.PID, m.Name, m.Path, m.Component, m.AuthCode, m.Type, m.Sort, string(metaRaw),
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

const deprecatedMenusSQL = `DELETE FROM menus
 WHERE id = 303
    OR id = 313
    OR name = 'WebsiteNavigation'
    OR name = 'WebsiteSignupLeads'
    OR name = 'CustomerAppPrivateRule'
    OR path = '/website/navigation'
    OR path = '/website/signup-leads'
    OR path = '/customer/app-private-rules'
    OR component = '/site-config/navigation'
    OR component = '/customer/app-private-rules'`

func removeDeprecatedMenus(ctx context.Context, database *sql.DB) error {
	_, err := database.ExecContext(ctx, deprecatedMenusSQL)
	return err
}

func seedRoles(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// 超级管理员：拥有全部菜单。
	var adminRoleID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO roles (code, name, remark, status)
		 VALUES ('admin','超级管理员','拥有全部后台权限',1)
		 ON CONFLICT (code) DO UPDATE
		   SET name=EXCLUDED.name,
		       remark=EXCLUDED.remark,
		       status=1
		 RETURNING id`,
	).Scan(&adminRoleID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO role_menus (role_id, menu_id)
		 SELECT $1, id FROM menus
		 ON CONFLICT (role_id, menu_id) DO NOTHING`, adminRoleID); err != nil {
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

	return tx.Commit()
}

// quizSeedOption 播种用选项：id 锚定 a/b/c/d，weights 为 typeID->分值。
type quizSeedOption struct {
	ID      string      `json:"id"`
	Text    string      `json:"text"`
	Weights map[int]int `json:"weights"`
}

type quizSeedQuestion struct {
	Body    string
	Options []quizSeedOption
}

// seedQuizQuestions 播种九型测评题库（12 道情境题，来源：miniapp enneagramGame.js）。
// 幂等：仅当 app_quiz_questions 为空时写入。
func seedQuizQuestions(ctx context.Context, database *sql.DB) error {
	var count int
	if err := database.QueryRowContext(ctx, "SELECT count(*) FROM app_quiz_questions").Scan(&count); err != nil {
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

	for i, q := range defaultQuizQuestions {
		optsJSON, err := json.Marshal(q.Options)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO app_quiz_questions (sort, body, options, dimension, status)
			 VALUES ($1, $2, $3, '', 'enabled')`,
			(i+1)*10, q.Body, optsJSON); err != nil {
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

	var userID int64
	if err := database.QueryRowContext(ctx,
		"SELECT id FROM users WHERE username=$1", adminUser).Scan(&userID); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}

	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if userID == 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		if err := tx.QueryRowContext(ctx,
			`INSERT INTO users (username, password_hash, nickname, status) VALUES ($1,$2,'超级管理员',1) RETURNING id`,
			adminUser, string(hash),
		).Scan(&userID); err != nil {
			return err
		}
	}

	var adminRoleID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE code='admin'`).Scan(&adminRoleID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if adminRoleID != 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, adminRoleID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
