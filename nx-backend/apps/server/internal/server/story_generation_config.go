package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
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
		result := generator.Ping(probeCtx)
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
	result := generator.Ping(probeCtx)
	cancel()
	httpx.OK(w, result)
}
