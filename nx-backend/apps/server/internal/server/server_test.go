package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/server"
)

func TestVbenCompatibleAPI(t *testing.T) {
	handler, configPath := newTestServer(t)

	t.Run("rejects protected resources without token", func(t *testing.T) {
		response := perform(handler, http.MethodGet, "/api/user/info", "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", response.Code)
		}
	})

	t.Run("logs in and returns token", func(t *testing.T) {
		response := perform(handler, http.MethodPost, "/api/auth/login", "", map[string]string{
			"password": "123456",
			"username": "admin",
		})
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected vben success response, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if data["accessToken"] == "" {
			t.Fatal("expected accessToken")
		}
	})

	t.Run("returns user, codes, menus, and site config with token", func(t *testing.T) {
		token := loginToken(t, handler)
		for _, path := range []string{"/api/user/info", "/api/auth/codes", "/api/menu/all", "/api/site-config"} {
			response := perform(handler, http.MethodGet, path, token, nil)
			body := decodeBody(t, response)
			if response.Code != http.StatusOK || body.Code != 0 {
				t.Fatalf("%s expected success, got status=%d body=%+v", path, response.Code, body)
			}
		}
	})

	t.Run("updates current user profile", func(t *testing.T) {
		token := loginToken(t, handler)
		payload := map[string]any{
			"avatar":   "https://cdn.example.com/avatar.png",
			"email":    "admin@example.com",
			"phone":    "18800000000",
			"realName": "新的管理员",
			"remark":   "个人简介",
			"username": "admin-new",
		}

		response := perform(handler, http.MethodPut, "/api/user/profile", token, payload)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected profile update success, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if data["username"] != "admin-new" || data["realName"] != "新的管理员" || data["avatar"] != "https://cdn.example.com/avatar.png" {
			t.Fatalf("unexpected profile payload: %+v", data)
		}

		infoResponse := perform(handler, http.MethodGet, "/api/user/info", token, nil)
		infoBody := decodeBody(t, infoResponse)
		info := infoBody.Data.(map[string]any)
		if info["username"] != "admin-new" || info["realName"] != "新的管理员" || info["avatar"] != "https://cdn.example.com/avatar.png" {
			t.Fatalf("expected user info to reflect profile changes, got %+v", info)
		}
	})

	t.Run("updates site config", func(t *testing.T) {
		token := loginToken(t, handler)
		var config map[string]any
		raw, _ := os.ReadFile(configPath)
		if err := json.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		config["site"].(map[string]any)["brandName"] = "九型芯之力"

		response := perform(handler, http.MethodPut, "/api/site-config", token, config)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected save success, got status=%d body=%+v", response.Code, body)
		}

		nextRaw, _ := os.ReadFile(configPath)
		if !bytes.Contains(nextRaw, []byte("九型芯之力")) {
			t.Fatal("expected file to be updated")
		}
	})

	t.Run("provides system management apis", func(t *testing.T) {
		token := loginToken(t, handler)

		for _, path := range []string{"/api/system/user/list", "/api/system/role/list", "/api/system/menu/list"} {
			response := perform(handler, http.MethodGet, path, token, nil)
			body := decodeBody(t, response)
			if response.Code != http.StatusOK || body.Code != 0 {
				t.Fatalf("%s expected success, got status=%d body=%+v", path, response.Code, body)
			}
		}

		createUser := perform(handler, http.MethodPost, "/api/system/user", token, map[string]any{
			"email":    "test@example.com",
			"nickname": "测试用户",
			"roleIds":  []string{"2"},
			"status":   1,
			"username": "tester",
		})
		userBody := decodeBody(t, createUser)
		if createUser.Code != http.StatusOK || userBody.Code != 0 {
			t.Fatalf("expected create user success, got status=%d body=%+v", createUser.Code, userBody)
		}

		createRole := perform(handler, http.MethodPost, "/api/system/role", token, map[string]any{
			"code":    "tester",
			"menuIds": []int{1, 201},
			"name":    "测试角色",
			"remark":  "测试",
			"status":  1,
		})
		roleBody := decodeBody(t, createRole)
		if createRole.Code != http.StatusOK || roleBody.Code != 0 {
			t.Fatalf("expected create role success, got status=%d body=%+v", createRole.Code, roleBody)
		}
	})

	t.Run("forbids backend api without matching permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler)
		response := perform(handler, http.MethodGet, "/api/system/user/list", token, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing System:User:List permission, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("forbids site config read without website permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler)
		response := perform(handler, http.MethodGet, "/api/site-config", token, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing Website:Read permission, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("forbids site config update without website write permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler)
		var config map[string]any
		raw, _ := os.ReadFile(configPath)
		if err := json.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		config["site"].(map[string]any)["brandName"] = "低权限写入"

		response := perform(handler, http.MethodPut, "/api/site-config", token, config)
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing Website:Write permission, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("stores public signup submissions and lists them in admin", func(t *testing.T) {
		create := perform(handler, http.MethodPost, "/api/public/signups", "", map[string]any{
			"contact":  "13800000000",
			"interest": "九型基础课",
			"message":  "想了解课程安排",
			"name":     "王同学",
		})
		createBody := decodeBody(t, create)
		if create.Code != http.StatusOK || createBody.Code != 0 {
			t.Fatalf("expected signup create success, got status=%d body=%+v", create.Code, createBody)
		}

		token := loginToken(t, handler)
		list := perform(handler, http.MethodGet, "/api/signups/list?keyword=王同学", token, nil)
		listBody := decodeBody(t, list)
		if list.Code != http.StatusOK || listBody.Code != 0 {
			t.Fatalf("expected signup list success, got status=%d body=%+v", list.Code, listBody)
		}
		data := listBody.Data.(map[string]any)
		if data["total"].(float64) < 1 {
			t.Fatalf("expected at least one signup, got %+v", data)
		}
	})

	t.Run("model config includes video analysis model", func(t *testing.T) {
		token := loginToken(t, handler)
		response := perform(handler, http.MethodGet, "/api/model-config", token, nil)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected model config success, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if _, ok := data["analysis"].(map[string]any); !ok {
			t.Fatalf("expected analysis model config in response, got %+v", data)
		}
	})
}

func lowPermissionToken(t *testing.T, handler http.Handler) string {
	t.Helper()
	adminToken := loginToken(t, handler)
	suffix := time.Now().UnixNano()
	roleCode := "low_permission"
	username := "lowperm"

	createRole := perform(handler, http.MethodPost, "/api/system/role", adminToken, map[string]any{
		"code":    roleCode,
		"menuIds": []int{200},
		"name":    "低权限角色",
		"remark":  "测试",
		"status":  1,
	})
	if createRole.Code != http.StatusOK {
		t.Fatalf("create low permission role failed: %d %s", createRole.Code, createRole.Body.String())
	}
	roleBody := decodeBody(t, createRole)
	roleData, _ := roleBody.Data.(map[string]any)
	roleID := roleData["id"]
	if roleID == nil {
		t.Fatalf("missing role id in response: %+v", roleBody.Data)
	}

	password := "123456"
	createUser := perform(handler, http.MethodPost, "/api/system/user", adminToken, map[string]any{
		"email":    "lowperm@example.com",
		"nickname": "低权限用户",
		"password": password,
		"roleIds":  []any{roleID},
		"status":   1,
		"username": fmt.Sprintf("%s_%d", username, suffix),
	})
	if createUser.Code != http.StatusOK {
		t.Fatalf("create low permission user failed: %d %s", createUser.Code, createUser.Body.String())
	}
	userBody := decodeBody(t, createUser)
	userData, _ := userBody.Data.(map[string]any)
	createdUsername, _ := userData["username"].(string)
	if createdUsername == "" {
		t.Fatalf("missing username in response: %+v", userBody.Data)
	}

	response := perform(handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"password": password,
		"username": createdUsername,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("low permission login failed: %d %s", response.Code, response.Body.String())
	}
	body := decodeBody(t, response)
	data := body.Data.(map[string]any)
	token, _ := data["accessToken"].(string)
	if token == "" {
		t.Fatal("missing low permission token")
	}
	return token
}

type vbenBody struct {
	Code int `json:"code"`
	Data any `json:"data"`
}

func newTestServer(t *testing.T) (http.Handler, string) {
	t.Helper()

	// 该测试为集成测试，需要一个可用的 PostgreSQL。
	// 设置 TEST_DATABASE_URL 后运行，例如：
	//   TEST_DATABASE_URL=postgres://nx:nx@localhost:5432/nx_admin_test?sslmode=disable go test ./...
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set TEST_DATABASE_URL to run server integration tests")
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "site-config.json")
	if err := os.WriteFile(configPath, []byte(sampleConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	database, err := db.Open(ctx, dsn, "admin", "123456")
	if err != nil {
		t.Fatalf("db open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	env := config.Env{
		AdminPassword: "123456",
		AdminUsername: "admin",
		AppEnv:        "test",
		AppVersion:    "0.0.1-test",
		JWTSecret:     "test-secret",
		Port:          5320,
		SiteConfig:    configPath,
		DatabaseURL:   dsn,
	}
	return server.New(env, database), configPath
}

func loginToken(t *testing.T, handler http.Handler) string {
	t.Helper()
	response := perform(handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"password": "123456",
		"username": "admin",
	})
	body := decodeBody(t, response)
	data := body.Data.(map[string]any)
	token, _ := data["accessToken"].(string)
	if token == "" {
		t.Fatal("missing token")
	}
	return token
}

func perform(handler http.Handler, method string, path string, token string, payload any) *httptest.ResponseRecorder {
	var body bytes.Buffer
	if payload != nil {
		_ = json.NewEncoder(&body).Encode(payload)
	}
	request := httptest.NewRequest(method, path, &body)
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) vbenBody {
	t.Helper()
	var body vbenBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

const sampleConfig = `{
  "site": {
    "brandName": "芯之力",
    "logo": "/assets/logo.svg",
    "footerTagline": "九型人格 · 性格能量 · 成长教练",
    "copyright": "© 2026 芯之力"
  },
  "navigation": {
    "main": [{ "label": "首页", "to": "/", "type": "route" }],
    "drawer": [{ "label": "首页", "to": "/", "type": "route" }],
    "tabs": [{ "label": "首页", "to": "/", "type": "route", "match": "/", "icon": "home" }]
  },
  "home": {},
  "types": [
    { "id": "1", "name": "完美型", "keywords": "原则", "description": "描述", "avatar": "/assets/avatars/1.png" }
  ]
}`
