package server

import "testing"

func TestAgentBackendMenusExposeDistributionRoute(t *testing.T) {
	menus := agentBackendMenus()
	if len(menus) != 1 {
		t.Fatalf("agent menus length=%d, want 1", len(menus))
	}
	root := menus[0]
	if root.Name != "AppManage" || root.Path != "/app" {
		t.Fatalf("root menu = %+v", root)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root children length=%d, want 1", len(root.Children))
	}
	child := root.Children[0]
	if child.Name != "AppDistributionManagement" {
		t.Fatalf("child route name=%q", child.Name)
	}
	if child.Path != "/app/distribution" {
		t.Fatalf("child path=%q", child.Path)
	}
	if child.Component != "/app/distribution-management" {
		t.Fatalf("child component=%q", child.Component)
	}
	if child.AuthCode != "Agent:Distribution:View" {
		t.Fatalf("child authCode=%q", child.AuthCode)
	}
}
