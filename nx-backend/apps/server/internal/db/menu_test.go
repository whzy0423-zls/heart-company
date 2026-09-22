package db

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultMenusIncludeMiniappHomeManagement(t *testing.T) {
	var foundCatalog bool
	var foundHome bool
	var foundLearn bool
	for _, menu := range defaultMenus {
		switch menu.Name {
		case "MiniappManage":
			foundCatalog = true
			if menu.ID != 1300 || menu.PID != 0 || menu.Path != "/miniapp" || menu.Type != "catalog" || menu.Sort != 12 || menu.Icon != "lucide:smartphone" || menu.Title != "小程序管理" {
				t.Fatalf("unexpected miniapp management catalog: %+v", menu)
			}
		case "MiniappHome":
			foundHome = true
			if menu.ID != 1301 || menu.PID != 1300 || menu.Path != "/miniapp/home" || menu.Component != "/miniapp/home" || menu.AuthCode != "Website:Write" || menu.Type != "menu" || menu.Sort != 1 || menu.Icon != "lucide:images" || menu.Title != "首页管理" {
				t.Fatalf("unexpected miniapp home management menu: %+v", menu)
			}
		case "MiniappLearn":
			foundLearn = true
			if menu.ID != 1302 || menu.PID != 1300 || menu.Path != "/miniapp/learn" || menu.Component != "/miniapp/learn" || menu.AuthCode != "Website:Write" || menu.Type != "menu" || menu.Sort != 2 || menu.Icon != "lucide:book-open" || menu.Title != "学习页管理" {
				t.Fatalf("unexpected miniapp learn management menu: %+v", menu)
			}
		}
	}
	if !foundCatalog {
		t.Fatal("expected default menu MiniappManage")
	}
	if !foundHome {
		t.Fatal("expected default menu MiniappHome")
	}
	if !foundLearn {
		t.Fatal("expected default menu MiniappLearn")
	}
}

func TestDefaultMenusIncludeMiniappOrdersUnderMiniappManagement(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.ID != 513 {
			continue
		}
		if menu.PID != 1300 || menu.Name != "MiniappOrders" {
			t.Fatalf("expected miniapp orders under MiniappManage: %+v", menu)
		}
		if menu.Path != "/miniapp/orders" || menu.Component != "/customer/miniapp-orders" {
			t.Fatalf("unexpected miniapp orders route: %+v", menu)
		}
		if menu.AuthCode != "Customer:MiniappOrders:List" || menu.Type != "menu" || menu.Sort != 3 {
			t.Fatalf("unexpected miniapp orders metadata: %+v", menu)
		}
		return
	}
	t.Fatal("expected default menu MiniappOrders with fixed id 513")
}

func TestDefaultMenusIncludeAppPaymentMode(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.Name != "AppPaymentMode" {
			continue
		}
		if menu.PID != 1500 || menu.Path != "/third-party-payment/mode" || menu.Component != "/third-party-payment/mode" {
			t.Fatalf("unexpected payment mode route: %+v", menu)
		}
		if menu.AuthCode != "System:Payment:Config" || menu.Title != "支付模式" || menu.Type != "menu" {
			t.Fatalf("unexpected payment mode metadata: %+v", menu)
		}
		return
	}
	t.Fatal("expected default menu AppPaymentMode")
}

func TestDefaultMenusIncludeAppReleaseManagement(t *testing.T) {
	var foundList bool
	var foundWrite bool
	for _, menu := range defaultMenus {
		switch menu.ID {
		case 315:
			foundList = true
			if menu.PID != 1600 || menu.Name != "WebsiteAppReleases" {
				t.Fatalf("unexpected App release menu identity: %+v", menu)
			}
			if menu.Path != "/website/app-releases" || menu.Component != "/site-config/app-releases" {
				t.Fatalf("unexpected App release menu route: %+v", menu)
			}
			if menu.AuthCode != "Website:AppReleases:List" || menu.Type != "menu" || menu.Title != "App 版本" {
				t.Fatalf("unexpected App release menu metadata: %+v", menu)
			}
		case 316:
			foundWrite = true
			if menu.PID != 315 || menu.Type != "button" || menu.AuthCode != "Website:AppReleases:Write" {
				t.Fatalf("unexpected App release write permission: %+v", menu)
			}
		}
	}
	if !foundList {
		t.Fatal("expected default menu WebsiteAppReleases")
	}
	if !foundWrite {
		t.Fatal("expected App release write permission button")
	}
}

func TestDefaultMenusIncludeRAGKnowledgeManagement(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "RAGKnowledge" {
			continue
		}
		found = true
		if menu.Path != "/rag/knowledge" || menu.Component != "/rag/knowledge" {
			t.Fatalf("unexpected RAG knowledge route: %+v", menu)
		}
		if menu.AuthCode != "RAG:Knowledge:Manage" || menu.Title != "知识库管理" {
			t.Fatalf("unexpected RAG knowledge metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu RAGKnowledge")
	}
}

func TestDefaultMenusIncludeDailyQuizPushRecords(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "DailyQuizPushRecords" {
			continue
		}
		found = true
		if menu.PID != 1200 || menu.Path != "/profile-calibration/daily-quiz-push" || menu.Component != "/message/daily-quiz-push" {
			t.Fatalf("unexpected daily quiz push records route: %+v", menu)
		}
		if menu.AuthCode != "ProfileCalibration:DailyQuiz:Manage" || menu.Type != "menu" || menu.Title != "每日题推送记录" {
			t.Fatalf("unexpected daily quiz push records metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu DailyQuizPushRecords")
	}
}

func TestDefaultMenusIncludeDailyQuizBankManagement(t *testing.T) {
	var foundCatalog bool
	var foundBank bool
	for _, menu := range defaultMenus {
		switch menu.Name {
		case "ProfileCalibration":
			foundCatalog = true
			if menu.PID != 0 || menu.Path != "/profile-calibration" || menu.Type != "catalog" || menu.Title != "画像校准" {
				t.Fatalf("unexpected profile calibration catalog: %+v", menu)
			}
		case "DailyQuizBank":
			foundBank = true
			if menu.PID != 1200 || menu.Path != "/profile-calibration/daily-quiz-bank" || menu.Component != "/profile-calibration/daily-quiz-bank" {
				t.Fatalf("unexpected daily quiz bank route: %+v", menu)
			}
			if menu.AuthCode != "ProfileCalibration:DailyQuiz:Manage" || menu.Type != "menu" || menu.Title != "每日题库管理" {
				t.Fatalf("unexpected daily quiz bank metadata: %+v", menu)
			}
		}
	}
	if !foundCatalog {
		t.Fatal("expected default menu ProfileCalibration")
	}
	if !foundBank {
		t.Fatal("expected default menu DailyQuizBank")
	}
}

func TestDefaultMenusIncludeAdminModelConfig(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "AdminModelConfig" {
			continue
		}
		found = true
		if menu.PID != 1100 || menu.Path != "/settings/admin-model" || menu.Component != "/settings/model" {
			t.Fatalf("unexpected admin model config route: %+v", menu)
		}
		if menu.AuthCode != "System:Model:Config" || menu.Title != "管理端大模型配置" {
			t.Fatalf("unexpected admin model config metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu AdminModelConfig")
	}
}

func TestDefaultMenusIncludeXinzhiliModelConfig(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "XinzhiliModelConfig" {
			continue
		}
		found = true
		if menu.PID != 1100 || menu.Path != "/settings/xinzhili-model" || menu.Component != "/settings/xinzhili-model" {
			t.Fatalf("unexpected xinzhili model config route: %+v", menu)
		}
		if menu.AuthCode != "System:XinzhiliModel:Config" || menu.Title != "芯之力模型配置" {
			t.Fatalf("unexpected xinzhili model config metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu XinzhiliModelConfig")
	}
}

func TestDefaultMenusExcludeLegacyVideoAnalysis(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.Name == "VideoAnalysis" || menu.Path == "/video/analysis" {
			t.Fatalf("legacy video analysis menu remains: %+v", menu)
		}
	}
}

func TestDefaultMenusExcludeLegacyVideoStoryboard(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.Name == "VideoStoryboard" || menu.Path == "/video/storyboard" {
			t.Fatalf("legacy video storyboard menu remains: %+v", menu)
		}
	}
}

func TestDefaultMenusIncludeCustomerAppWritePermission(t *testing.T) {
	var foundList bool
	var foundWrite bool
	for _, menu := range defaultMenus {
		switch menu.Name {
		case "CustomerAppUsers":
			foundList = true
			if menu.AuthCode != "Customer:App:List" {
				t.Fatalf("unexpected App customer list permission: %+v", menu)
			}
		case "CustomerAppUsersEdit":
			foundWrite = true
			if menu.PID != 502 || menu.Type != "button" {
				t.Fatalf("expected App customer write permission to be a child button of App customers, got %+v", menu)
			}
			if menu.AuthCode != "Customer:App:Write" || menu.Title != "编辑 App 客户" {
				t.Fatalf("unexpected App customer write metadata: %+v", menu)
			}
		}
	}
	if !foundList {
		t.Fatal("expected default menu CustomerAppUsers")
	}
	if !foundWrite {
		t.Fatal("expected default menu CustomerAppUsersEdit")
	}
}

func TestDefaultMenusIncludeCustomerUserInsights(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "CustomerUserInsights" {
			continue
		}
		found = true
		if menu.PID != 1600 || menu.Path != "/customer/user-insights" || menu.Component != "/customer/user-insights" {
			t.Fatalf("unexpected user insights route: %+v", menu)
		}
		if menu.AuthCode != "Customer:UserInsights:List" || menu.Title != "用户提炼数据" {
			t.Fatalf("unexpected user insights metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu CustomerUserInsights")
	}
}

func TestDefaultMenusIncludeCustomerMiniappUsers(t *testing.T) {
	for _, menu := range defaultMenus {
		if menu.ID != 511 {
			continue
		}
		if menu.PID != 500 || menu.Name != "CustomerMiniappUsers" {
			t.Fatalf("unexpected miniapp customer menu identity: %+v", menu)
		}
		if menu.Path != "/customer/miniapp-users" || menu.Component != "/customer/miniapp-users" {
			t.Fatalf("unexpected miniapp customer menu route: %+v", menu)
		}
		if menu.AuthCode != "Customer:Miniapp:List" || menu.Type != "menu" || menu.Sort != 8 {
			t.Fatalf("unexpected miniapp customer menu permission: %+v", menu)
		}
		if menu.Icon == "" || menu.Title != "小程序客户" {
			t.Fatalf("unexpected miniapp customer menu metadata: %+v", menu)
		}
		return
	}
	t.Fatal("expected default menu CustomerMiniappUsers with fixed id 511")
}

func TestDefaultMenusIncludeAppAnalyticsDashboard(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "DashboardAppAnalytics" {
			continue
		}
		found = true
		if menu.PID != 1600 || menu.Path != "/dashboard/app" || menu.Component != "/dashboard/app" {
			t.Fatalf("unexpected App analytics dashboard route: %+v", menu)
		}
		if menu.AuthCode != "Analytics:App:Overview" || menu.Title != "App 数据看板" {
			t.Fatalf("unexpected App analytics dashboard metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu DashboardAppAnalytics")
	}
}

func TestDefaultAppMenusBelongToAppManagement(t *testing.T) {
	var foundParent bool
	for _, menu := range defaultMenus {
		if menu.Name == "AppManage" {
			foundParent = true
			if menu.ID != 1600 || menu.PID != 0 || menu.Path != "/app" || menu.Type != "catalog" || menu.Title != "App 管理" {
				t.Fatalf("unexpected App management parent: %+v", menu)
			}
		}
	}
	if !foundParent {
		t.Fatal("expected default AppManage parent menu")
	}

	wantSort := map[string]int{
		"DashboardAppAnalytics": 1,
		"CustomerAppUsers":      2,
		"CustomerUserInsights":  3,
		"CustomerAppOrders":     4,
		"WebsiteAppReleases":    6,
		"CustomerAppChat":       7,
		"CustomerAppMemory":     8,
		"CustomerQuizQuestions": 9,
		"AppPlanManagement":     12,
	}
	found := make(map[string]bool, len(wantSort))
	for _, menu := range defaultMenus {
		sort, ok := wantSort[menu.Name]
		if !ok {
			continue
		}
		found[menu.Name] = true
		if menu.PID != 1600 || menu.Sort != sort {
			t.Fatalf("expected %s under App 管理 with sort %d, got %+v", menu.Name, sort, menu)
		}
	}
	for name := range wantSort {
		if !found[name] {
			t.Fatalf("expected default App menu %s", name)
		}
	}
}

func TestDefaultMenusIncludeDistributionManagement(t *testing.T) {
	want := map[string]struct {
		path  string
		title string
		sort  int
	}{
		"AppDistributionManagement":       {path: "/app/distribution", title: "代理管理", sort: 13},
		"AppDistributionCommissions":      {path: "/app/distribution-commissions", title: "佣金明细", sort: 14},
		"AppDistributionRules":            {path: "/app/distribution-rules", title: "佣金规则", sort: 15},
		"AppDistributionSettlements":      {path: "/app/distribution-settlements", title: "分销结算", sort: 16},
		"AppDistributionPosterManagement": {path: "/app/distribution-poster", title: "海报管理", sort: 17},
	}
	found := make(map[string]bool, len(want))
	for _, menu := range defaultMenus {
		expected, ok := want[menu.Name]
		if !ok {
			continue
		}
		found[menu.Name] = true
		expectedComponent := expected.path
		if menu.Name == "AppDistributionPosterManagement" {
			expectedComponent = "/app/distribution-poster-management"
			if menu.HideInMenu {
				t.Fatal("poster menu must be visible")
			}
		}
		if menu.Name == "AppDistributionManagement" {
			expectedComponent = "/app/distribution-management"
		}
		if menu.PID != 1600 || menu.Path != expected.path || menu.Component != expectedComponent || menu.AuthCode != "Customer:App:List" || menu.Type != "menu" || menu.Sort != expected.sort || menu.Title != expected.title {
			t.Fatalf("unexpected distribution menu %s: %+v", menu.Name, menu)
		}
	}
	for name := range want {
		if !found[name] {
			t.Fatalf("expected default distribution menu %s", name)
		}
	}
}

func TestDefaultMenusIncludeAppPlanManagement(t *testing.T) {
	var foundPlan bool
	for _, menu := range defaultMenus {
		if menu.Name != "AppPlanManagement" {
			continue
		}
		foundPlan = true
		if menu.PID != 1600 || menu.Path != "/app/plan-management" || menu.Component != "/app/plan-management" || menu.AuthCode != "App:PlanManagement:View" || menu.Type != "menu" || menu.Sort != 12 || menu.Title != "套餐管理" {
			t.Fatalf("unexpected App plan menu: %+v", menu)
		}
	}
	if !foundPlan {
		t.Fatal("expected default menu AppPlanManagement")
	}
}

func TestSeedInvokesAppManagementPermissionBackfill(t *testing.T) {
	raw, err := os.ReadFile("db.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, call := range []string{
		"seedDistributionMenuBindings(ctx, database)",
		"seedAppPlanManagementMenu(ctx, database)",
	} {
		if !strings.Contains(source, call) {
			t.Fatalf("expected seed to call %s", call)
		}
	}
}

func TestDeprecatedMenusRemoveStaleCustomerPrivateRuleRoute(t *testing.T) {
	for _, token := range []string{
		"name = 'CustomerAppPrivateRule'",
		"path = '/customer/app-private-rules'",
		"component = '/customer/app-private-rules'",
		"name = 'TheoryLibrary'",
		"path = '/theory/library'",
		"component = '/theory/library'",
		"name = 'AppProducts'",
		"path = '/app/products'",
		"component = '/app/products'",
	} {
		if !strings.Contains(deprecatedMenusSQL, token) {
			t.Fatalf("expected deprecated menu cleanup SQL to include %q", token)
		}
	}
	for _, token := range []string{
		"name = 'XinzhiliModelConfig'",
		"path = '/settings/xinzhili-model'",
	} {
		if strings.Contains(deprecatedMenusSQL, token) {
			t.Fatalf("expected deprecated menu cleanup SQL not to delete restored route by %q", token)
		}
	}
}

func TestDefaultMenusIncludeTeacherClassroomPermissions(t *testing.T) {
	want := map[string]bool{"Miniapp:Classroom:List": false, "Miniapp:Classroom:Write": false, "Miniapp:Classroom:Upload": false, "Miniapp:Classroom:Publish": false, "Miniapp:Classroom:Price": false}
	for _, menu := range defaultMenus {
		if _, ok := want[menu.AuthCode]; ok {
			want[menu.AuthCode] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("missing classroom permission menu %s", code)
		}
	}
}

func TestDefaultMenusIncludeTeacherManagement(t *testing.T) {
	var foundCatalog, foundManagement bool
	for _, menu := range defaultMenus {
		switch menu.Name {
		case "MiniappTeacher":
			foundCatalog = true
			if menu.PID != 0 || menu.Path != "/teachers" || menu.Type != "catalog" || menu.Title != "老师管理" {
				t.Fatalf("unexpected teacher catalog: %+v", menu)
			}
		case "MiniappTeacherManagement":
			foundManagement = true
			if menu.PID != 1408 || menu.Path != "/teachers/manage" || menu.Component != "/teacher/index" || menu.AuthCode != "Miniapp:Teacher:Manage" || menu.Type != "menu" {
				t.Fatalf("unexpected teacher management menu: %+v", menu)
			}
		}
	}
	if !foundCatalog || !foundManagement {
		t.Fatalf("expected teacher management catalog and menu, got catalog=%v management=%v", foundCatalog, foundManagement)
	}
}
