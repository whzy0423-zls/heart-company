package db

import "testing"

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
