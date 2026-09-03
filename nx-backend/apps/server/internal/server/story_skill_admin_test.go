package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStorySkillDraftMetadataStagesAppFacingFields(t *testing.T) {
	input := storySkillDraftInput{
		Category: "folk",
		Key:      "folk-compact-v2",
		Name:     "民间故事紧凑叙事",
		Summary:  "压缩支线并保留关键转折",
	}
	metadata := storySkillDraftMetadata(input, 42)
	for key, want := range map[string]any{
		"storyStyle":   "folk",
		"draftKey":     "folk-compact-v2",
		"draftName":    "民间故事紧凑叙事",
		"draftSummary": "压缩支线并保留关键转折",
		"uploadedBy":   int64(42),
	} {
		if metadata[key] != want {
			t.Errorf("metadata[%s]=%v want=%v", key, metadata[key], want)
		}
	}
}

func TestPublishedStorySkillCannotBeEdited(t *testing.T) {
	if err := validateStorySkillEditState(false); err != nil {
		t.Fatalf("unpublished story skill rejected: %v", err)
	}
	if err := validateStorySkillEditState(true); !errors.Is(err, errPublishedStorySkillReadOnly) {
		t.Fatalf("published story skill edit error=%v", err)
	}
}

func TestRegisterStorySkillAdminRoutesUsesCRUDPermissions(t *testing.T) {
	mux := http.NewServeMux()
	var permissions []string
	permission := func(code string, next http.HandlerFunc) http.HandlerFunc {
		return func(_ http.ResponseWriter, _ *http.Request) {
			permissions = append(permissions, code)
			_ = next
		}
	}
	registerStorySkillAdminRoutes(mux, permission, &Server{})

	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/story-skills", "App:StoryManagement:View"},
		{http.MethodPost, "/api/story-skills/upload", "App:StoryManagement:Edit"},
		{http.MethodGet, "/api/story-skills/7", "App:StoryManagement:View"},
		{http.MethodPatch, "/api/story-skills/7", "App:StoryManagement:Edit"},
		{http.MethodDelete, "/api/story-skills/7", "App:StoryManagement:Delete"},
		{http.MethodPost, "/api/story-skills/7/publish", "App:StoryManagement:Publish"},
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

func TestParseStorySkillAdminPath(t *testing.T) {
	tests := []struct {
		path   string
		id     int64
		action string
		ok     bool
	}{
		{"/api/story-skills/7", 7, "", true},
		{"/api/story-skills/7/publish", 7, "publish", true},
		{"/api/story-skills/0", 0, "", false},
		{"/api/story-skills/nope", 0, "", false},
		{"/api/story-skills/7/unknown", 0, "", false},
	}
	for _, test := range tests {
		id, action, ok := parseStorySkillAdminPath(test.path)
		if id != test.id || action != test.action || ok != test.ok {
			t.Errorf("parse %q = %d/%q/%v want %d/%q/%v", test.path, id, action, ok, test.id, test.action, test.ok)
		}
	}
}

func TestStorySkillCategoriesMatchAppStoryStyles(t *testing.T) {
	want := []string{"myth", "folk", "fairy_tale", "novel", "realistic"}
	if len(storySkillCategories) != len(want) {
		t.Fatalf("categories=%d want=%d", len(storySkillCategories), len(want))
	}
	for _, category := range want {
		if !validStorySkillCategory(category) {
			t.Errorf("published story style %q is not accepted", category)
		}
	}
	for _, category := range []string{"", "all", "folk_story", "MYTH"} {
		if validStorySkillCategory(category) {
			t.Errorf("invalid story style %q was accepted", category)
		}
	}
}

func TestStorySkillKeyValidation(t *testing.T) {
	for _, key := range []string{"folk-concise", "myth_v2", "a1"} {
		if !validStorySkillKey(key) {
			t.Errorf("valid key %q was rejected", key)
		}
	}
	for _, key := range []string{"", "a", "Folk", "民间故事", "has space"} {
		if validStorySkillKey(key) {
			t.Errorf("invalid key %q was accepted", key)
		}
	}
}

func TestStorySkillUploadRejectsUnsupportedAndOversizedFiles(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		message  string
	}{
		{name: "unsupported extension", filename: "skill.pdf", content: "rules", message: "SKILL.md"},
		{name: "oversized", filename: "SKILL.md", content: strings.Repeat("a", (1<<20)+1), message: "1MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, contentType := storySkillMultipartBody(t, tt.filename, tt.content)
			request := httptest.NewRequest(http.MethodPost, "/api/story-skills/upload", body)
			request.Header.Set("Content-Type", contentType)
			response := httptest.NewRecorder()

			(&Server{}).storySkillAdminRouter(response, request)

			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), tt.message) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestStorySkillAppEndpointRejectsUnknownStyleWithoutFallingBackToAll(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/app/skill-libraries/story-skills?storyStyle=unknown", nil)
	response := httptest.NewRecorder()

	(&Server{}).appSkillLibrariesRouter(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		Data []any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 0 {
		t.Fatalf("unknown style returned skills: %s", response.Body.String())
	}
}

func storySkillMultipartBody(t *testing.T, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range map[string]string{
		"category": "folk",
		"key":      "folk-concise",
		"name":     "民间精简写作",
		"summary":  "只保留关键事件",
		"version":  "1.0.0",
	} {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body, writer.FormDataContentType()
}
