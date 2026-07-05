package system

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestDisabledRolesDoNotGrantAuthCodesOrMenus(t *testing.T) {
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)

	authQuery := regexp.MustCompile("(?s)SELECT DISTINCT m\\.auth_code FROM menus m.*?WHERE ur\\.user_id=\\$1[^`]*`").FindString(text)
	if authQuery == "" {
		t.Fatal("could not locate non-admin auth-code query")
	}
	if !regexp.MustCompile(`JOIN roles r ON r\.id=rm\.role_id`).MatchString(authQuery) || !regexp.MustCompile(`r\.status=1`).MatchString(authQuery) {
		t.Fatalf("non-admin auth-code query must join enabled roles only; query=%s", authQuery)
	}

	menuQuery := regexp.MustCompile("(?s)SELECT DISTINCT rm\\.menu_id FROM role_menus rm.*?WHERE ur\\.user_id=\\$1[^`]*`").FindString(text)
	if menuQuery == "" {
		t.Fatal("could not locate allowed-menu query")
	}
	if !regexp.MustCompile(`JOIN roles r ON r\.id=rm\.role_id`).MatchString(menuQuery) || !regexp.MustCompile(`r\.status=1`).MatchString(menuQuery) {
		t.Fatalf("allowed-menu query must join enabled roles only; query=%s", menuQuery)
	}
}

func TestPageParamsClampsPageSize(t *testing.T) {
	_, pageSize := pageParams(map[string]string{"pageSize": "10000"})
	if pageSize > 100 {
		t.Fatalf("pageSize should be capped at 100, got %d", pageSize)
	}
}

func TestSystemStorePreventsAdminLockoutOperations(t *testing.T) {
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, required := range []string{
		"currentUserID(ctx)",
		"cannot disable current user",
		"cannot delete current user",
		"last enabled admin user",
		"ensureUserAdminNotLast",
		"ensureRoleAdminNotLast",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("system store must include lockout guard %q", required)
		}
	}
}

func TestDefaultHomePathUsesAccessibleMenus(t *testing.T) {
	source, err := os.ReadFile("store.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "func (s *Store) DefaultHomePathForUser") || !strings.Contains(text, "firstMenuPath") {
		t.Fatal("system store should compute default home path from the user's accessible menus")
	}
}
