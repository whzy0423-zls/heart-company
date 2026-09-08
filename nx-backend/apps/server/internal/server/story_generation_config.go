package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/llm"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
)

type storyGenerationConfigView struct {
	Enabled        bool    `json:"enabled"`
	Provider       string  `json:"provider"`
	APIBase        string  `json:"apiBase"`
	Model          string  `json:"model"`
	Temperature    float64 `json:"temperature"`
	MaxTokens      int     `json:"maxTokens"`
	TimeoutSeconds int     `json:"timeoutSeconds"`
	SystemPrompt   string  `json:"systemPrompt"`
	APIKeySet      bool    `json:"apiKeySet"`
}

func buildStoryGenerationConfigView(cfg modelconfig.StoryGenerationConfig) storyGenerationConfigView {
	cfg = cfg.Normalized()
	return storyGenerationConfigView{
		Enabled: cfg.Enabled, Provider: cfg.Provider, APIBase: cfg.APIBase, Model: cfg.Model,
		Temperature: cfg.Temperature, MaxTokens: cfg.MaxTokens, TimeoutSeconds: cfg.TimeoutSeconds,
		SystemPrompt: cfg.SystemPrompt, APIKeySet: strings.TrimSpace(cfg.APIKey) != "",
	}
}

func probeStoryGenerationModel(ctx context.Context, completer llm.JSONCompleter, apiBase, model string) llm.PingResult {
	result := llm.PingResult{APIBase: strings.TrimSpace(apiBase), Model: strings.TrimSpace(model)}
	if completer == nil {
		result.Message = "故事模型不支持结构化输出"
		return result
	}
	started := time.Now()
	raw, err := completer.CompleteJSON(ctx, "只输出一个 JSON 对象，不要 Markdown。", `返回 {"ok":true}。`, 64)
	result.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		result.Message = "结构化输出测试失败：" + err.Error()
		return result
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		result.Message = "结构化输出测试失败：模型没有返回有效 JSON"
		return result
	}
	if ok, _ := payload["ok"].(bool); !ok {
		result.Message = "结构化输出测试失败：模型返回的 JSON 内容不符合要求"
		return result
	}
	result.OK = true
	result.Message = fmt.Sprintf("连通正常，故事模型 %s 已通过结构化输出校验", result.Model)
	return result
}

func (s *Server) storyGenerationConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	if r.Method == http.MethodGet {
		stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		httpx.OK(w, buildStoryGenerationConfigView(stored.ApplyStoryGeneration()))
		return
	}

	s.modelConfigUpdateMu.Lock()
	defer s.modelConfigUpdateMu.Unlock()
	var incoming modelconfig.Config
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	merged := stored.MergeIncoming(incoming)
	story := merged.ApplyStoryGeneration()
	if story.Enabled {
		if err := story.Validate(); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := validateExternalAPIBase("storyGeneration.apiBase", story.APIBase); err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		generator, err := s.buildStoryGenerator(merged)
		if err != nil {
			httpx.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		probeCtx, cancel := context.WithTimeout(r.Context(), s.modelConfigProbeDeadline(time.Duration(story.TimeoutSeconds)*time.Second))
		result := probeStoryGenerationModel(probeCtx, generator, story.APIBase, story.Model)
		cancel()
		if !result.OK {
			httpx.Fail(w, http.StatusBadRequest, result.Message)
			return
		}
		if err := modelconfig.UpsertStore(r.Context(), s.db, merged); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.modelMu.Lock()
		s.storyGen = generator
		s.storyConfig = story
		s.modelMu.Unlock()
	} else {
		if err := modelconfig.UpsertStore(r.Context(), s.db, merged); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		s.modelMu.Lock()
		s.storyGen = nil
		s.storyConfig = story
		s.modelMu.Unlock()
	}
	s.recordAdminAudit(r, auditlog.Entry{
		Action: "story_generation_config.update", TargetType: "story_generation_config", TargetID: "global",
		Before: modelConfigAuditSnapshot(stored), After: modelConfigAuditSnapshot(merged), Summary: "更新故事生成模型配置",
	})
	httpx.OK(w, buildStoryGenerationConfigView(story))
}

func (s *Server) testStoryGenerationConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
		return
	}
	var incoming modelconfig.Config
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil && !errors.Is(err, io.EOF) {
			httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
	}
	stored, _, err := modelconfig.ReadStore(r.Context(), s.db)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	merged := stored.MergeIncoming(incoming)
	story := merged.ApplyStoryGeneration()
	if !story.Enabled {
		httpx.OK(w, map[string]any{"ok": true, "message": "故事专用模型未启用，当前将回退聊天模型"})
		return
	}
	if err := story.Validate(); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateExternalAPIBase("storyGeneration.apiBase", story.APIBase); err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	generator, err := s.buildStoryGenerator(merged)
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	probeCtx, cancel := context.WithTimeout(r.Context(), s.modelConfigProbeDeadline(time.Duration(story.TimeoutSeconds)*time.Second))
	result := probeStoryGenerationModel(probeCtx, generator, story.APIBase, story.Model)
	cancel()
	httpx.OK(w, result)
}
