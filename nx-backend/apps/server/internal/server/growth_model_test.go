package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

func TestGrowthModelClientHonorsConfiguredResponseTimeout(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
		want    time.Duration
	}{
		{"long_completion", 180, 180 * time.Second},
		{"zero_default", 0, 30 * time.Second},
		{"negative_default", -1, 30 * time.Second},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := newGrowthModelHTTPClient(modelconfig.AdminModelConfig{TimeoutSeconds: tc.seconds})
			transport, ok := client.Transport.(*http.Transport)
			if !ok || client.Timeout != tc.want || transport.ResponseHeaderTimeout != tc.want {
				t.Fatal("growth completion timeout was shortened by the transport")
			}
			if transport.DialContext == nil || client.CheckRedirect == nil || !transport.DisableKeepAlives {
				t.Fatal("growth transport lost guarded connection settings")
			}
		})
	}
}

func TestGrowthModelProviderOutputLimits(t *testing.T) {
	for _, tc := range []struct {
		provider, path, limitKey, response string
	}{
		{"openai-compatible", "/v1/chat/completions", "max_tokens", `{"choices":[{"message":{"content":"{}"}}]}`},
		{"openai", "/v1/chat/completions", "max_tokens", `{"choices":[{"message":{"content":"{}"}}]}`},
		{"newapi", "/v1/chat/completions", "max_tokens", `{"choices":[{"message":{"content":"{}"}}]}`},
		{"anthropic-compatible", "/v1/messages", "max_tokens", `{"content":[{"type":"text","text":"{}"}]}`},
		{"anthropic", "/v1/messages", "max_tokens", `{"content":[{"type":"text","text":"{}"}]}`},
		{"minimax", "/v1/text/chatcompletion_v2", "max_tokens", `{"reply":"{}"}`},
		{"", "/v1/text/chatcompletion_v2", "max_tokens", `{"reply":"{}"}`},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			calls := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error("invalid request JSON")
				}
				if r.URL.Path != tc.path || body[tc.limitKey] != float64(1234) || body["model"] != "fixture-model" {
					t.Error("provider path, model, or output limit changed")
				}
				if tc.provider == "anthropic" || tc.provider == "anthropic-compatible" {
					if r.Header.Get("x-api-key") != "fixture-key" || body["system"] != "fixture-system" {
						t.Error("Anthropic credentials or system prompt changed")
					}
				} else if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("provider credentials changed")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			cfg := modelconfig.AdminModelConfig{Provider: tc.provider, APIBase: upstream.URL, APIKey: "fixture-key", Model: "fixture-model"}
			result, err := callGrowthModelJSONWithClient(context.Background(), upstream.Client(), cfg, "fixture-system", "fixture-user", 1234)
			if err != nil || result != "{}" || calls != 1 {
				t.Fatal("bounded provider completion failed")
			}
		})
	}
}

func TestGrowthModelRejectsNonpositiveLimitBeforeRequest(t *testing.T) {
	for _, limit := range []int{0, -1} {
		if _, err := callGrowthModelJSON(context.Background(), modelconfig.AdminModelConfig{}, "", "", limit); err == nil {
			t.Fatal("growth model accepted a nonpositive output limit")
		}
	}
}

func TestAdminModelOutputLimitDefaultsRemainUnchanged(t *testing.T) {
	for _, tc := range []struct {
		name, limitKey, response string
		limit                    float64
		call                     func(context.Context, *http.Client, modelconfig.AdminModelConfig, string, string) (string, error)
	}{
		{"openai", "max_tokens", `{"choices":[{"message":{"content":"{}"}}]}`, 0, callOpenAICompatibleJSON},
		{"anthropic", "max_tokens", `{"content":[{"type":"text","text":"{}"}]}`, 2600, callAnthropicJSON},
		{"minimax", "tokens_to_generate", `{"reply":"{}"}`, 1800, callMiniMaxJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if json.NewDecoder(r.Body).Decode(&body) != nil {
					t.Error("invalid request JSON")
				}
				value, exists := body[tc.limitKey]
				if (tc.limit == 0 && exists) || (tc.limit != 0 && value != tc.limit) {
					t.Error("existing admin generation limit changed")
				}
				_, _ = w.Write([]byte(tc.response))
			}))
			defer upstream.Close()
			if _, err := tc.call(context.Background(), upstream.Client(), modelconfig.AdminModelConfig{APIBase: upstream.URL}, "", ""); err != nil {
				t.Fatal("existing admin completion failed")
			}
		})
	}
}
