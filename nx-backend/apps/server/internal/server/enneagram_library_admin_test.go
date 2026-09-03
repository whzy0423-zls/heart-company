package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisterEnneagramLibraryAdminRoutesUsesIndependentPermissions(t *testing.T) {
	mux := http.NewServeMux()
	var permissions []string
	permission := func(code string, next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			permissions = append(permissions, code)
			next(w, r)
		}
	}
	server := &Server{enneagramAdmin: &fakeEnneagramLibraryAdminService{}}
	registerEnneagramLibraryAdminRoutes(mux, permission, server)

	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/api/enneagram-library/types", "App:EnneagramLibrary:View"},
		{http.MethodGet, "/api/enneagram-library/types/3", "App:EnneagramLibrary:View"},
		{http.MethodPut, "/api/enneagram-library/types/3/draft", "App:EnneagramLibrary:Edit"},
		{http.MethodPost, "/api/enneagram-library/types/3/submit", "App:EnneagramLibrary:Edit"},
		{http.MethodPost, "/api/enneagram-library/types/3/approve", "App:EnneagramLibrary:Review"},
		{http.MethodPost, "/api/enneagram-library/types/3/preview", "App:EnneagramLibrary:View"},
		{http.MethodPost, "/api/enneagram-library/types/3/publish", "App:EnneagramLibrary:Publish"},
		{http.MethodGet, "/api/enneagram-library/types/3/versions", "App:EnneagramLibrary:View"},
		{http.MethodPost, "/api/enneagram-library/types/3/rollback", "App:EnneagramLibrary:Publish"},
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

func TestParseEnneagramLibraryPathAcceptsOnlyTypesOneThroughNine(t *testing.T) {
	for number := 1; number <= 9; number++ {
		got, action, ok := parseEnneagramLibraryPath("/api/enneagram-library/types/" + string(rune('0'+number)))
		if !ok || got != number || action != "" {
			t.Fatalf("type %d parsed as %d/%q/%v", number, got, action, ok)
		}
	}
	for _, path := range []string{
		"/api/enneagram-library/types/0", "/api/enneagram-library/types/10", "/api/enneagram-library/types/x", "/api/enneagram-library/other/3",
	} {
		if _, _, ok := parseEnneagramLibraryPath(path); ok {
			t.Fatalf("invalid path accepted: %s", path)
		}
	}
}

type fakeEnneagramLibraryAdminService struct{}

func (*fakeEnneagramLibraryAdminService) Overview(context.Context) ([]enneagramTypeSummary, error) {
	return []enneagramTypeSummary{}, nil
}
func (*fakeEnneagramLibraryAdminService) Detail(context.Context, int) (enneagramTypeDetail, error) {
	return enneagramTypeDetail{}, nil
}
func (*fakeEnneagramLibraryAdminService) SaveDraft(context.Context, int, enneagramDraftInput, int64) (enneagramTypeDetail, error) {
	return enneagramTypeDetail{}, nil
}
func (*fakeEnneagramLibraryAdminService) SubmitReview(context.Context, int, int64) error { return nil }
func (*fakeEnneagramLibraryAdminService) Approve(context.Context, int, int64, string) error {
	return nil
}
func (*fakeEnneagramLibraryAdminService) Preview(context.Context, int, string) (enneagramPreview, error) {
	return enneagramPreview{}, nil
}
func (*fakeEnneagramLibraryAdminService) Publish(context.Context, int, int64) (enneagramPublishResult, error) {
	return enneagramPublishResult{}, nil
}
func (*fakeEnneagramLibraryAdminService) Versions(context.Context, int) ([]enneagramVersion, error) {
	return []enneagramVersion{}, nil
}
func (*fakeEnneagramLibraryAdminService) Rollback(context.Context, int, int, int64) (enneagramPublishResult, error) {
	return enneagramPublishResult{}, nil
}
