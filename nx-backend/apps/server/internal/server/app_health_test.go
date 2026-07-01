package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestAppHealth(t *testing.T) {
	handler, _ := newTestServer(t)

	t.Run("returns health status without auth", func(t *testing.T) {
		response := perform(handler, http.MethodGet, "/api/app/health", "", nil)
		if response.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", response.Code)
		}

		body := decodeBody(t, response)
		if body.Code != 0 {
			t.Fatalf("expected code 0, got %d", body.Code)
		}

		data, ok := body.Data.(map[string]any)
		if !ok {
			t.Fatalf("expected data map, got %T", body.Data)
		}

		if data["service"] != "nine-xing-app" {
			t.Fatalf("expected service nine-xing-app, got %v", data["service"])
		}
		if data["status"] != "ok" {
			t.Fatalf("expected status ok, got %v", data["status"])
		}
		if data["version"] == nil || data["version"] == "" {
			t.Fatal("expected non-empty version")
		}
		if data["environment"] == nil || data["environment"] == "" {
			t.Fatal("expected non-empty environment")
		}

		timeStr, _ := data["time"].(string)
		if len(timeStr) != len("2026/06/30 14:50:02") {
			t.Fatalf("expected YYYY/MM/DD HH:mm:ss time, got %q", timeStr)
		}
	})

	t.Run("rejects non-GET method", func(t *testing.T) {
		response := perform(handler, http.MethodPost, "/api/app/health", "", nil)
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", response.Code)
		}
	})

	t.Run("app route prefix is separate from miniapp", func(t *testing.T) {
		response := perform(handler, http.MethodGet, "/api/miniapp/health", "", nil)
		if response.Code == http.StatusOK {
			var check struct {
				Code int `json:"code"`
				Data any `json:"data"`
			}
			_ = json.Unmarshal(response.Body.Bytes(), &check)
			if d, ok := check.Data.(map[string]any); ok && d["service"] == "nine-xing-app" {
				t.Fatal("/api/miniapp/health should not serve the app health endpoint")
			}
		}
	})
}
