package db

import (
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
	wantSort := map[string]int{
		"DashboardAppAnalytics": 1,
		"CustomerAppUsers":      2,
		"CustomerUserInsights":  3,
		"CustomerAppOrders":     4,
		"WebsiteAppReleases":    6,
		"CustomerAppChat":       7,
		"CustomerAppMemory":     8,
		"CustomerQuizQuestions": 9,
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

func TestDeprecatedMenusRemoveStaleCustomerPrivateRuleRoute(t *testing.T) {
	for _, token := range []string{
		"name = 'CustomerAppPrivateRule'",
		"path = '/customer/app-private-rules'",
		"component = '/customer/app-private-rules'",
		"name = 'TheoryLibrary'",
		"path = '/theory/library'",
		"component = '/theory/library'",
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
