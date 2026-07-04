package system

import "testing"

func TestRouteMenusOnlyDropsButtonPermissions(t *testing.T) {
	flat := []MenuItem{
		{ID: 500, Name: "CustomerManage", Path: "/customer", Type: "catalog", Status: 1},
		{ID: 502, PID: 500, Name: "CustomerAppUsers", Path: "/customer/app-users", Component: "/customer/app-users", Type: "menu", Status: 1},
		{ID: 503, PID: 502, Name: "CustomerAppUsersEdit", AuthCode: "Customer:App:Write", Type: "button", Status: 1},
	}

	tree := buildTree(routeMenusOnly(flat), 0)
	if len(tree) != 1 {
		t.Fatalf("expected one root route, got %d", len(tree))
	}
	if len(tree[0].Children) != 1 {
		t.Fatalf("expected only the App customer route child, got %+v", tree[0].Children)
	}
	appUsers := tree[0].Children[0]
	if appUsers.Name != "CustomerAppUsers" || appUsers.Path == "" {
		t.Fatalf("unexpected App customer route: %+v", appUsers)
	}
	if len(appUsers.Children) != 0 {
		t.Fatalf("button permissions must not be returned as route children: %+v", appUsers.Children)
	}
}
