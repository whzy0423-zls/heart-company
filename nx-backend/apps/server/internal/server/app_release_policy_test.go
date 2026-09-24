package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/apprelease"
)

func TestAppReleasePolicyRouteRequiresAuthentication(t *testing.T) {
	server := &Server{mux: http.NewServeMux()}
	server.routes()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/app-releases/7/policy", strings.NewReader(`{"minSupportedVersionCode":100,"forceUpdate":true,"rolloutPercentage":25}`))
	server.mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("PATCH without authentication status = %d, want %d; body=%s", response.Code, http.StatusUnauthorized, response.Body.String())
	}
}

func TestAppReleasePolicyMutationUpdatesPolicy(t *testing.T) {
	publishedAt := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	service := &stubAppReleaseService{updatePolicyRelease: apprelease.Release{
		ID:                      7,
		Platform:                "android",
		VersionName:             "2.0.0",
		VersionCode:             200,
		MinSupportedVersionCode: 100,
		ForceUpdate:             true,
		RolloutPercentage:       25,
		Status:                  apprelease.StatusPublished,
		PublishedAt:             &publishedAt,
	}}
	server := &Server{appReleases: service}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPatch, "/api/app-releases/7/policy", strings.NewReader(`{"minSupportedVersionCode":100,"forceUpdate":true,"rolloutPercentage":25}`))

	server.appReleaseMutation(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	wantPolicy := apprelease.AppReleasePolicy{MinSupportedVersionCode: 100, ForceUpdate: true, RolloutPercentage: 25}
	if service.updatePolicyCalls != 1 || service.updatePolicyID != 7 || service.updatePolicy != wantPolicy {
		t.Fatalf("UpdatePolicy calls=%d id=%d policy=%+v, want one call id=7 policy=%+v", service.updatePolicyCalls, service.updatePolicyID, service.updatePolicy, wantPolicy)
	}
	data := decodeAppReleaseData(t, response)
	if data["id"] != float64(7) || data["minSupportedVersionCode"] != float64(100) || data["forceUpdate"] != true || data["rolloutPercentage"] != float64(25) {
		t.Fatalf("response data = %+v, want complete updated release", data)
	}
}

func TestAppReleasePolicyMutationRejectsWrongMethodsAndPaths(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		status int
	}{
		{name: "policy requires patch", method: http.MethodPost, path: "/api/app-releases/7/policy", status: http.StatusMethodNotAllowed},
		{name: "publish remains post", method: http.MethodPatch, path: "/api/app-releases/7/publish", status: http.StatusMethodNotAllowed},
		{name: "archive remains post", method: http.MethodPatch, path: "/api/app-releases/7/archive", status: http.StatusMethodNotAllowed},
		{name: "unknown action", method: http.MethodPatch, path: "/api/app-releases/7/unknown", status: http.StatusNotFound},
		{name: "extra path segment", method: http.MethodPatch, path: "/api/app-releases/7/policy/extra", status: http.StatusNotFound},
		{name: "invalid id", method: http.MethodPatch, path: "/api/app-releases/0/policy", status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &stubAppReleaseService{}
			server := &Server{appReleases: service}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(`{"minSupportedVersionCode":0,"forceUpdate":false,"rolloutPercentage":100}`))

			server.appReleaseMutation(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			if service.updatePolicyCalls != 0 {
				t.Fatalf("UpdatePolicy calls = %d, want 0", service.updatePolicyCalls)
			}
		})
	}
}

func TestAppReleasePolicyMutationStrictlyDecodesSmallJSONBody(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "malformed", body: `{"minSupportedVersionCode":`},
		{name: "missing minimum", body: `{"forceUpdate":false,"rolloutPercentage":100}`},
		{name: "missing force update", body: `{"minSupportedVersionCode":0,"rolloutPercentage":100}`},
		{name: "missing rollout", body: `{"minSupportedVersionCode":0,"forceUpdate":false}`},
		{name: "unknown field", body: `{"minSupportedVersionCode":0,"forceUpdate":false,"rolloutPercentage":100,"extra":true}`},
		{name: "trailing object", body: `{"minSupportedVersionCode":0,"forceUpdate":false,"rolloutPercentage":100}{}`},
		{name: "oversized", body: `{"minSupportedVersionCode":0,"forceUpdate":false,"rolloutPercentage":100,"extra":"` + strings.Repeat("x", 5<<10) + `"}`},
	}
	original := apprelease.Release{
		ID:                      7,
		MinSupportedVersionCode: 50,
		ForceUpdate:             true,
		RolloutPercentage:       25,
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &stubAppReleaseService{updatePolicyRelease: original}
			server := &Server{appReleases: service}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/app-releases/7/policy", strings.NewReader(test.body))

			server.appReleaseMutation(response, request)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body=%s", response.Code, response.Body.String())
			}
			if service.updatePolicyCalls != 0 {
				t.Fatalf("UpdatePolicy calls = %d, want 0", service.updatePolicyCalls)
			}
			if service.updatePolicyRelease != original {
				t.Fatalf("stored release = %+v, want unchanged %+v", service.updatePolicyRelease, original)
			}
		})
	}
}

func TestAppReleasePolicyMutationMapsDomainErrors(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "invalid", err: apprelease.ErrInvalidPolicy, status: http.StatusBadRequest},
		{name: "missing", err: apprelease.ErrNotFound, status: http.StatusNotFound},
		{name: "conflict", err: apprelease.ErrConflict, status: http.StatusConflict},
		{name: "internal", err: errors.New("database unavailable"), status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := &stubAppReleaseService{updatePolicyErr: test.err}
			server := &Server{appReleases: service}
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPatch, "/api/app-releases/7/policy", strings.NewReader(`{"minSupportedVersionCode":0,"forceUpdate":false,"rolloutPercentage":100}`))

			server.appReleaseMutation(response, request)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
		})
	}
}

func TestPublicAppReleaseLatestAddsPolicyWithoutChangingExistingFields(t *testing.T) {
	publishedAt := time.Date(2026, 9, 15, 8, 0, 0, 0, time.UTC)
	service := &stubAppReleaseService{latestRelease: apprelease.Release{
		ID:                      7,
		Platform:                "android",
		VersionName:             "2.0.0",
		VersionCode:             200,
		MinSupportedVersionCode: 150,
		ForceUpdate:             true,
		RolloutPercentage:       35,
		ReleaseNotes:            "stability fixes",
		FileSize:                4096,
		SHA256:                  strings.Repeat("a", 64),
		PublishedAt:             &publishedAt,
		FileAvailable:           true,
	}}
	server := &Server{appReleases: service}
	response := httptest.NewRecorder()

	server.publicAppReleaseLatest(response, httptest.NewRequest(http.MethodGet, "/api/public/app-release/latest", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	data := decodeAppReleaseData(t, response)
	gotKeys := make([]string, 0, len(data))
	for key := range data {
		gotKeys = append(gotKeys, key)
	}
	sort.Strings(gotKeys)
	wantKeys := []string{"available", "downloadUrl", "fileSize", "forceUpdate", "minSupportedVersionCode", "platform", "publishedAt", "releaseNotes", "rolloutPercentage", "sha256", "versionCode", "versionName"}
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("public latest keys = %v, want %v", gotKeys, wantKeys)
	}
	if data["available"] != true || data["versionName"] != "2.0.0" || data["versionCode"] != float64(200) || data["downloadUrl"] != "/api/public/app-releases/7/download" {
		t.Fatalf("legacy website fields changed: %+v", data)
	}
	if data["minSupportedVersionCode"] != float64(150) || data["forceUpdate"] != true || data["rolloutPercentage"] != float64(35) {
		t.Fatalf("policy fields = %+v", data)
	}
}

func TestPublicAppReleaseLatestUnavailableResponseStaysMinimal(t *testing.T) {
	server := &Server{appReleases: &stubAppReleaseService{latestErr: apprelease.ErrNotFound}}
	response := httptest.NewRecorder()

	server.publicAppReleaseLatest(response, httptest.NewRequest(http.MethodGet, "/api/public/app-release/latest", nil))

	data := decodeAppReleaseData(t, response)
	if !reflect.DeepEqual(data, map[string]any{"available": false}) {
		t.Fatalf("unavailable data = %+v, want only available=false", data)
	}
}

func decodeAppReleaseData(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var payload struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(response.Body.Bytes())).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return payload.Data
}
