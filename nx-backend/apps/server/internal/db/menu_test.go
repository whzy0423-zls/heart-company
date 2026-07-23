package db

import (
	"strings"
	"testing"
)

func TestDefaultMenusIncludeMiniappHomeManagement(t *testing.T) {
	var foundCatalog bool
	var foundHome bool
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
		}
	}
	if !foundCatalog {
		t.Fatal("expected default menu MiniappManage")
	}
	if !foundHome {
		t.Fatal("expected default menu MiniappHome")
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

func TestDefaultMenusIncludeVideoAnalysis(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "VideoAnalysis" {
			continue
		}
		found = true
		if menu.PID != 1000 || menu.Path != "/video/analysis" || menu.Component != "/video/analysis" {
			t.Fatalf("unexpected video analysis route: %+v", menu)
		}
		if menu.AuthCode != "Video:Analysis:Manage" || menu.Title != "视频分析" {
			t.Fatalf("unexpected video analysis metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu VideoAnalysis")
	}
}

func TestDefaultMenusIncludeVideoStoryboard(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "VideoStoryboard" {
			continue
		}
		found = true
		if menu.PID != 1000 || menu.Path != "/video/storyboard" || menu.Component != "/video/storyboard" {
			t.Fatalf("unexpected video storyboard route: %+v", menu)
		}
		if menu.AuthCode != "Video:Storyboard:Manage" || menu.Title != "分镜设计" {
			t.Fatalf("unexpected video storyboard metadata: %+v", menu)
		}
	}
	if !found {
		t.Fatal("expected default menu VideoStoryboard")
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
		if menu.PID != 500 || menu.Path != "/customer/user-insights" || menu.Component != "/customer/user-insights" {
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

func TestDefaultMenusIncludeAppAnalyticsDashboard(t *testing.T) {
	var found bool
	for _, menu := range defaultMenus {
		if menu.Name != "DashboardAppAnalytics" {
			continue
		}
		found = true
		if menu.PID != 0 || menu.Path != "/dashboard/app" || menu.Component != "/dashboard/app" {
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

func TestDeprecatedMenusRemoveStaleCustomerPrivateRuleRoute(t *testing.T) {
	for _, token := range []string{
		"name = 'CustomerAppPrivateRule'",
		"path = '/customer/app-private-rules'",
		"component = '/customer/app-private-rules'",
	} {
		if !strings.Contains(deprecatedMenusSQL, token) {
			t.Fatalf("expected deprecated menu cleanup SQL to include %q", token)
		}
	}
}
