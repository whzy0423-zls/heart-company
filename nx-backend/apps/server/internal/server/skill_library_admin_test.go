package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterSkillLibraryAdminRoutesUsesViewAndEditPermissions(t *testing.T) {
	mux := http.NewServeMux()
	var permissions []string
	permission := func(code string, next http.HandlerFunc) http.HandlerFunc {
		return func(_ http.ResponseWriter, _ *http.Request) {
			permissions = append(permissions, code)
			_ = next
		}
	}
	registerSkillLibraryAdminRoutes(mux, permission, &Server{})

	tests := []struct{ method, path, want string }{
		{http.MethodGet, "/api/skill-library-management", "App:SkillLibrary:View"},
		{http.MethodPatch, "/api/skill-library-management/library/3", "App:SkillLibrary:Edit"},
		{http.MethodPatch, "/api/skill-library-management/categories/7", "App:SkillLibrary:Edit"},
		{http.MethodPatch, "/api/skill-library-management/skills/9", "App:SkillLibrary:Edit"},
	}
	for _, test := range tests {
		permissions = nil
		response := httptest.NewRecorder()
		request := httptest.NewRequest(test.method, test.path, nil)
		mux.ServeHTTP(response, request)
		if len(permissions) != 1 || permissions[0] != test.want {
			t.Errorf("%s %s permissions=%v want=%s", test.method, test.path, permissions, test.want)
		}
	}
}

func TestParseSkillLibraryAdminPath(t *testing.T) {
	tests := []struct {
		path, resource string
		id             int64
		ok             bool
	}{
		{"/api/skill-library-management/library/3", "library", 3, true},
		{"/api/skill-library-management/categories/7", "categories", 7, true},
		{"/api/skill-library-management/skills/9", "skills", 9, true},
		{"/api/skill-library-management/skills/0", "", 0, false},
		{"/api/skill-library-management/skills/nope", "", 0, false},
		{"/api/skill-library-management/unknown/1", "", 0, false},
	}
	for _, test := range tests {
		resource, id, ok := parseSkillLibraryAdminPath(test.path)
		if resource != test.resource || id != test.id || ok != test.ok {
			t.Errorf("parse %q = %q/%d/%v want %q/%d/%v", test.path, resource, id, ok, test.resource, test.id, test.ok)
		}
	}
}

func TestDecodeSkillAdminUpdateValidatesEditableFields(t *testing.T) {
	validBody, _ := json.Marshal(map[string]any{
		"name": "论语心得", "summary": "从经典文本中整理处世与关系问题",
		"description": "用于自我反思和关系沟通", "categoryId": 7,
		"iconKey": "book-open", "colorToken": "green", "sortOrder": 12, "status": "enabled",
	})
	request := httptest.NewRequest(http.MethodPatch, "/api/skill-library-management/skills/9", bytes.NewReader(validBody))
	input, err := decodeSkillAdminUpdate(request)
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "论语心得" || input.CategoryID != 7 || input.Status != "enabled" {
		t.Fatalf("unexpected input: %+v", input)
	}
	for _, body := range []string{
		`{"name":"","summary":"摘要","categoryId":7,"status":"enabled"}`,
		`{"name":"名称","summary":"","categoryId":7,"status":"enabled"}`,
		`{"name":"名称","summary":"摘要","categoryId":0,"status":"enabled"}`,
		`{"name":"名称","summary":"摘要","categoryId":7,"status":"draft"}`,
	} {
		request := httptest.NewRequest(http.MethodPatch, "/api/skill-library-management/skills/9", bytes.NewBufferString(body))
		if _, err := decodeSkillAdminUpdate(request); err == nil {
			t.Fatalf("invalid update accepted: %s", body)
		}
	}
}
