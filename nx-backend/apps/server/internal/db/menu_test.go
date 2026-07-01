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
