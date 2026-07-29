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
	"reflect"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/db"
	"nine-xing/nx-backend/apps/server/internal/server"
	"nine-xing/nx-backend/apps/server/internal/testutil"
)

func TestVbenCompatibleAPI(t *testing.T) {
	for run := 1; run <= 2; run++ {
		t.Run(fmt.Sprintf("run_%d", run), testVbenCompatibleAPI)
	}
}

func testVbenCompatibleAPI(t *testing.T) {
	handler, configPath := newTestServer(t)
	var adminToken string

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
		adminToken, _ = data["accessToken"].(string)
	})

	t.Run("returns user, codes, menus, and site config with token", func(t *testing.T) {
		for _, path := range []string{"/api/user/info", "/api/auth/codes", "/api/menu/all", "/api/site-config"} {
			response := perform(handler, http.MethodGet, path, adminToken, nil)
			body := decodeBody(t, response)
			if response.Code != http.StatusOK || body.Code != 0 {
				t.Fatalf("%s expected success, got status=%d body=%+v", path, response.Code, body)
			}
		}
	})

	t.Run("updates current user profile", func(t *testing.T) {
		originalResponse := perform(handler, http.MethodGet, "/api/user/info", adminToken, nil)
		originalBody := decodeBody(t, originalResponse)
		original, ok := originalBody.Data.(map[string]any)
		if originalResponse.Code != http.StatusOK || originalBody.Code != 0 || !ok {
			t.Fatalf("read original admin profile failed: status=%d body=%s", originalResponse.Code, originalResponse.Body.String())
		}

		payload := map[string]any{
			"avatar":   "https://cdn.example.com/avatar.png",
			"email":    "admin@example.com",
			"phone":    "18800000000",
			"realName": "新的管理员",
			"remark":   "个人简介",
			"username": "admin-new",
		}

		defer func() {
			restore := make(map[string]any, 6)
			for _, field := range []string{"avatar", "email", "phone", "realName", "remark", "username"} {
				restore[field] = original[field]
			}
			response := perform(handler, http.MethodPut, "/api/user/profile", adminToken, restore)
			if response.Code != http.StatusOK {
				t.Errorf("restore admin profile failed: status=%d body=%s", response.Code, response.Body.String())
				return
			}
			restored := decodeBody(t, response).Data.(map[string]any)
			for _, field := range []string{"avatar", "email", "phone", "realName", "remark", "username"} {
				if restored[field] != original[field] {
					t.Errorf("admin profile field %s was not restored: got=%v want=%v", field, restored[field], original[field])
				}
			}
		}()

		response := perform(handler, http.MethodPut, "/api/user/profile", adminToken, payload)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected profile update success, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if data["username"] != "admin-new" || data["realName"] != "新的管理员" || data["avatar"] != "https://cdn.example.com/avatar.png" {
			t.Fatalf("unexpected profile payload: %+v", data)
		}

		infoResponse := perform(handler, http.MethodGet, "/api/user/info", adminToken, nil)
		infoBody := decodeBody(t, infoResponse)
		info := infoBody.Data.(map[string]any)
		if info["username"] != "admin-new" || info["realName"] != "新的管理员" || info["avatar"] != "https://cdn.example.com/avatar.png" {
			t.Fatalf("expected user info to reflect profile changes, got %+v", info)
		}
	})

	t.Run("updates site config", func(t *testing.T) {
		var config map[string]any
		raw, _ := os.ReadFile(configPath)
		if err := json.Unmarshal(raw, &config); err != nil {
			t.Fatal(err)
		}
		config["site"].(map[string]any)["brandName"] = "九型芯之力"
		home := config["home"].(map[string]any)
		carousel := map[string]any{
			"autoplay": true,
			"interval": float64(4000),
			"items": []any{map[string]any{
				"enabled": true,
				"image":   "https://nine-xing.oss-cn-hangzhou.aliyuncs.com/miniapp/carousel-1.webp",
			}},
		}
		miniappHome := map[string]any{
			"brand": map[string]any{"enabled": true, "name": "九型芯之力", "tagline": "看见动机，找到成长方向"},
			"hero": map[string]any{
				"enabled": true, "kicker": "老师导学", "title": "读懂自己", "description": "从核心动机出发", "buttonText": "开始人格测试",
			},
			"entriesSection": map[string]any{
				"enabled": true, "title": "探索你的九型能量", "description": "选择此刻最需要的一步",
				"items": []any{map[string]any{
					"key": "test", "enabled": true, "title": "人格测试", "description": "找到核心动机", "icon": "compass", "theme": "blue",
				}},
			},
			"growth": map[string]any{
				"enabled": true, "eyebrow": "老师陪伴", "title": "把测试发现带进课程练习", "description": "让理解沉淀为行动",
			},
		}
		home["miniappCarousel"] = carousel
		home["miniappHome"] = miniappHome

		response := perform(handler, http.MethodPut, "/api/site-config", adminToken, config)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected save success, got status=%d body=%+v", response.Code, body)
		}

		nextRaw, _ := os.ReadFile(configPath)
		if !bytes.Contains(nextRaw, []byte("九型芯之力")) {
			t.Fatal("expected file to be updated")
		}

		publicResponse := perform(handler, http.MethodGet, "/api/public/site-config", "", nil)
		publicBody := decodeBody(t, publicResponse)
		if publicResponse.Code != http.StatusOK || publicBody.Code != 0 {
			t.Fatalf("expected public site config success, got status=%d body=%+v", publicResponse.Code, publicBody)
		}
		publicConfig := publicBody.Data.(map[string]any)
		publicHome := publicConfig["home"].(map[string]any)
		if !reflect.DeepEqual(publicHome["miniappHome"], miniappHome) {
			t.Fatalf("expected miniappHome to survive authenticated update and public read: got=%#v want=%#v", publicHome["miniappHome"], miniappHome)
		}
		if !reflect.DeepEqual(publicHome["miniappCarousel"], carousel) {
			t.Fatalf("expected miniappCarousel to remain unchanged: got=%#v want=%#v", publicHome["miniappCarousel"], carousel)
		}
		publicCarousel := publicHome["miniappCarousel"].(map[string]any)
		publicItems := publicCarousel["items"].([]any)
		publicImage := publicItems[0].(map[string]any)["image"]
		if publicImage != "https://nine-xing.oss-cn-hangzhou.aliyuncs.com/miniapp/carousel-1.webp" {
			t.Fatalf("expected OSS carousel URL to remain unchanged, got %v", publicImage)
		}
	})

	t.Run("provides system management apis", func(t *testing.T) {
		suffix := time.Now().UnixNano()
		username := fmt.Sprintf("tester_%d", suffix)
		roleCode := fmt.Sprintf("tester_%d", suffix)
		for _, path := range []string{"/api/system/user/list", "/api/system/role/list", "/api/system/menu/list"} {
			response := perform(handler, http.MethodGet, path, adminToken, nil)
			body := decodeBody(t, response)
			if response.Code != http.StatusOK || body.Code != 0 {
				t.Fatalf("%s expected success, got status=%d body=%+v", path, response.Code, body)
			}
		}

		createUser := perform(handler, http.MethodPost, "/api/system/user", adminToken, map[string]any{
			"email":    fmt.Sprintf("tester_%d@example.com", suffix),
			"nickname": "测试用户",
			"password": "123456",
			"roleIds":  []string{},
			"status":   1,
			"username": username,
		})
		userBody := decodeBody(t, createUser)
		if createUser.Code != http.StatusOK || userBody.Code != 0 {
			t.Fatalf("expected create user success, got status=%d body=%+v", createUser.Code, userBody)
		}

		createRole := perform(handler, http.MethodPost, "/api/system/role", adminToken, map[string]any{
			"code":    roleCode,
			"menuIds": []int{201},
			"name":    "测试角色",
			"remark":  "测试",
			"status":  1,
		})
		roleBody := decodeBody(t, createRole)
		if createRole.Code != http.StatusOK || roleBody.Code != 0 {
			t.Fatalf("expected create role success, got status=%d body=%+v", createRole.Code, roleBody)
		}
	})

	t.Run("rejects query token for signup event stream", func(t *testing.T) {
		response := perform(handler, http.MethodGet, "/api/signups/events?token="+adminToken, "", nil)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("expected query token to be rejected for signup event stream, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("accepts authorization header for signup event stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		request := httptest.NewRequest(http.MethodGet, "/api/signups/events", nil).WithContext(ctx)
		request.Header.Set("Authorization", "Bearer "+adminToken)
		response := &cancelOnFlushRecorder{ResponseRecorder: httptest.NewRecorder(), cancel: cancel}
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !response.Flushed {
			t.Fatalf("expected authorized event stream to flush, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("forbids backend api without matching permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler, adminToken)
		response := perform(handler, http.MethodGet, "/api/system/user/list", token, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing System:User:List permission, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("forbids site config read without website permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler, adminToken)
		response := perform(handler, http.MethodGet, "/api/site-config", token, nil)
		if response.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing Website:Read permission, got %d body=%s", response.Code, response.Body.String())
		}
	})

	t.Run("forbids site config update without website write permission", func(t *testing.T) {
		token := lowPermissionToken(t, handler, adminToken)
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

		list := perform(handler, http.MethodGet, "/api/signups/list?keyword=王同学", adminToken, nil)
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
		response := perform(handler, http.MethodGet, "/api/model-config", adminToken, nil)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected model config success, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if _, ok := data["analysis"].(map[string]any); !ok {
			t.Fatalf("expected analysis model config in response, got %+v", data)
		}
	})

	t.Run("lists app user insights for admins only", func(t *testing.T) {
		response := perform(handler, http.MethodGet, "/api/app-users/insights", adminToken, nil)
		body := decodeBody(t, response)
		if response.Code != http.StatusOK || body.Code != 0 {
			t.Fatalf("expected app user insights success, got status=%d body=%+v", response.Code, body)
		}
		data := body.Data.(map[string]any)
		if _, ok := data["items"].([]any); !ok {
			t.Fatalf("expected insights items array, got %+v", data)
		}

		lowToken := lowPermissionToken(t, handler, adminToken)
		forbidden := perform(handler, http.MethodGet, "/api/app-users/insights", lowToken, nil)
		if forbidden.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for missing Customer:UserInsights:List permission, got %d body=%s", forbidden.Code, forbidden.Body.String())
		}
	})

	t.Run("allows page scoped helper APIs with minimum page permissions", func(t *testing.T) {
		cases := []struct {
			menuIDs []int
			name    string
			paths   []string
		}{
			{
				menuIDs: []int{901},
				name:    "reading",
				paths:   []string{"/api/reading/settings", "/api/voice/options"},
			},
			{
				menuIDs: []int{1103},
				name:    "xinzhili_model",
				paths:   []string{"/api/xinzhili-model-config", "/api/voice/options"},
			},
			{
				menuIDs: []int{702},
				name:    "voice_test",
				paths:   []string{"/api/voice/generations/list?page=1&pageSize=1", "/api/voice/profiles/list?page=1&pageSize=1"},
			},
			{
				menuIDs: []int{1004},
				name:    "storyboard",
				paths:   []string{"/api/video/storyboards/list?page=1&pageSize=1", "/api/video/analysis/list?page=1&pageSize=1"},
			},
			{
				menuIDs: []int{1001},
				name:    "video_generate",
				paths:   []string{"/api/video/generations/list?page=1&pageSize=1", "/api/video/assets/polish-prompt"},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				token := tokenWithMenus(t, handler, adminToken, tc.name, tc.menuIDs)
				for _, path := range tc.paths {
					method := http.MethodGet
					var payload any
					if path == "/api/video/assets/polish-prompt" {
						method = http.MethodPost
						payload = map[string]any{"kind": "video", "prompt": "测试提示词"}
					}
					response := perform(handler, method, path, token, payload)
					if response.Code == http.StatusForbidden || response.Code == http.StatusUnauthorized {
						t.Fatalf("%s should be authorized for %s, got %d body=%s", tc.name, path, response.Code, response.Body.String())
					}
				}
			})
		}
	})

	t.Run("system update permission can edit without create permission", func(t *testing.T) {
		targetUsername := fmt.Sprintf("target_%d", time.Now().UnixNano())
		createTarget := perform(handler, http.MethodPost, "/api/system/user", adminToken, map[string]any{
			"nickname": "待编辑用户",
			"password": "123456",
			"roleIds":  []any{},
			"status":   1,
			"username": targetUsername,
		})
		if createTarget.Code != http.StatusOK {
			t.Fatalf("create target user failed: %d %s", createTarget.Code, createTarget.Body.String())
		}
		targetBody := decodeBody(t, createTarget)
		targetData := targetBody.Data.(map[string]any)
		targetID, _ := targetData["id"].(string)
		if targetID == "" {
			t.Fatalf("missing target id: %+v", targetBody.Data)
		}

		token := tokenWithMenus(t, handler, adminToken, "system_update_only", []int{401, 406})
		createForbidden := perform(handler, http.MethodPost, "/api/system/user", token, map[string]any{
			"nickname": "不应创建",
			"password": "123456",
			"roleIds":  []any{},
			"status":   1,
			"username": fmt.Sprintf("forbidden_%d", time.Now().UnixNano()),
		})
		if createForbidden.Code != http.StatusForbidden {
			t.Fatalf("expected create to be forbidden without System:User:Create, got %d body=%s", createForbidden.Code, createForbidden.Body.String())
		}

		update := perform(handler, http.MethodPut, "/api/system/user/"+targetID, token, map[string]any{
			"id":       targetID,
			"nickname": "已编辑用户",
			"roleIds":  []any{},
			"status":   1,
			"username": targetUsername,
		})
		if update.Code != http.StatusOK {
			t.Fatalf("expected update to pass with System:User:Update, got %d body=%s", update.Code, update.Body.String())
		}
	})
}

func lowPermissionToken(t *testing.T, handler http.Handler, adminToken string) string {
	t.Helper()
	suffix := time.Now().UnixNano()
	roleCode := fmt.Sprintf("low_permission_%d", suffix)
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
		"email":    fmt.Sprintf("lowperm_%d@example.com", suffix),
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

func tokenWithMenus(t *testing.T, handler http.Handler, adminToken string, name string, menuIDs []int) string {
	t.Helper()
	suffix := time.Now().UnixNano()
	roleCode := fmt.Sprintf("probe_%s_%d", name, suffix)
	username := fmt.Sprintf("probe_%s_%d", name, suffix)

	createRole := perform(handler, http.MethodPost, "/api/system/role", adminToken, map[string]any{
		"code":    roleCode,
		"menuIds": menuIDs,
		"name":    roleCode,
		"remark":  "最小权限流程测试",
		"status":  1,
	})
	if createRole.Code != http.StatusOK {
		t.Fatalf("create role %s failed: %d %s", roleCode, createRole.Code, createRole.Body.String())
	}
	roleBody := decodeBody(t, createRole)
	roleData, _ := roleBody.Data.(map[string]any)
	roleID := roleData["id"]
	if roleID == nil {
		t.Fatalf("missing role id in response: %+v", roleBody.Data)
	}

	password := "123456"
	createUser := perform(handler, http.MethodPost, "/api/system/user", adminToken, map[string]any{
		"nickname": username,
		"password": password,
		"roleIds":  []any{roleID},
		"status":   1,
		"username": username,
	})
	if createUser.Code != http.StatusOK {
		t.Fatalf("create user %s failed: %d %s", username, createUser.Code, createUser.Body.String())
	}

	response := perform(handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"password": password,
		"username": username,
	})
	if response.Code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", username, response.Code, response.Body.String())
	}
	body := decodeBody(t, response)
	data := body.Data.(map[string]any)
	token, _ := data["accessToken"].(string)
	if token == "" {
		t.Fatalf("missing token for %s", username)
	}
	return token
}

type vbenBody struct {
	Code int `json:"code"`
	Data any `json:"data"`
}

type cancelOnFlushRecorder struct {
	*httptest.ResponseRecorder
	cancel context.CancelFunc
}

func (r *cancelOnFlushRecorder) Flush() {
	r.ResponseRecorder.Flush()
	r.cancel()
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
	if err := testutil.ValidateIsolatedPostgresDSN(dsn); err != nil {
		t.Fatal(err)
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
	if _, err := database.ExecContext(ctx, `DELETE FROM request_rate_limits WHERE scope='admin_login'`); err != nil {
		t.Fatalf("reset admin login rate limits: %v", err)
	}

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
	return loginTokenForUsername(t, handler, "admin")
}

func loginTokenForUsername(t *testing.T, handler http.Handler, username string) string {
	t.Helper()
	response := perform(handler, http.MethodPost, "/api/auth/login", "", map[string]string{
		"password": "123456",
		"username": username,
	})
	body := decodeBody(t, response)
	data, ok := body.Data.(map[string]any)
	if !ok {
		t.Fatalf("login %s failed: status=%d body=%s", username, response.Code, response.Body.String())
	}
	token, _ := data["accessToken"].(string)
	if token == "" {
		t.Fatalf("missing token for %s", username)
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
    { "id": "1", "name": "完美型", "keywords": "原则", "description": "描述", "avatar": "/assets/avatars/1.webp" }
  ]
}`
