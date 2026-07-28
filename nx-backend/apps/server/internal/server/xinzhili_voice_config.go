package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"nine-xing/nx-backend/apps/server/internal/auditlog"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/xinzhili"
)

type xinzhiliVoiceConfigStore interface {
	ReadActive(context.Context) (xinzhili.VoiceConfig, bool, error)
	ReadDraft(context.Context) (xinzhili.VoiceConfig, bool, error)
	SaveDraft(context.Context, xinzhili.TTSConfig, int64) (xinzhili.VoiceConfig, error)
	Activate(context.Context, int64) (xinzhili.VoiceConfig, error)
	Deactivate(context.Context, int64) error
	Restore(context.Context, int64, int64) (xinzhili.VoiceConfig, error)
	ScheduleRemoteDelete(context.Context, int64, string, string) (xinzhili.VoiceCleanupJob, error)
}

type xinzhiliVoiceConfigSecretView struct {
	APIKey       string `json:"apiKey"`
	APIKeySet    bool   `json:"apiKeySet"`
	APIKeySuffix string `json:"apiKeySuffix"`
}

type xinzhiliVoiceTTSConfigView struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	GroupID  string `json:"groupId,omitempty"`
	Model    string `json:"model"`
	Voice    string `json:"voice"`
	Format   string `json:"format"`
	xinzhiliVoiceConfigSecretView
}

type xinzhiliVoiceConfigItemView struct {
	Found      bool                       `json:"found"`
	Version    int64                      `json:"version"`
	Status     xinzhili.VoiceConfigStatus `json:"status"`
	TTS        xinzhiliVoiceTTSConfigView `json:"tts"`
	CreateTime string                     `json:"createTime,omitempty"`
	UpdateTime string                     `json:"updateTime,omitempty"`
}

type xinzhiliVoiceConfigView struct {
	Active xinzhiliVoiceConfigItemView `json:"active"`
	Draft  xinzhiliVoiceConfigItemView `json:"draft"`
}

type xinzhiliVoiceConfigUpdateRequest struct {
	ExpectedVersion *int64             `json:"expectedVersion"`
	TTS             xinzhili.TTSConfig `json:"tts"`
}

type xinzhiliVoiceConfigActionRequest struct {
	Action          string `json:"action"`
	ExpectedVersion *int64 `json:"expectedVersion"`
	Version         int64  `json:"version"`
	Provider        string `json:"provider"`
	RemoteVoiceID   string `json:"remoteVoiceId"`
}

func (s *Server) xinzhiliVoiceConfigHandler(w http.ResponseWriter, r *http.Request) {
	if s.xinzhiliVoiceConfig == nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "芯之力音色配置服务暂不可用")
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.xinzhiliVoiceConfigGet(w, r)
	case http.MethodPut:
		s.xinzhiliVoiceConfigSaveDraft(w, r)
	case http.MethodPost:
		s.xinzhiliVoiceConfigAction(w, r)
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "Method Not Allowed")
	}
}

func (s *Server) xinzhiliVoiceConfigGet(w http.ResponseWriter, r *http.Request) {
	view, err := s.buildXinzhiliVoiceConfigView(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpx.OK(w, view)
}

func (s *Server) xinzhiliVoiceConfigSaveDraft(w http.ResponseWriter, r *http.Request) {
	var input xinzhiliVoiceConfigUpdateRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	if input.ExpectedVersion == nil {
		httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
		return
	}
	before, _ := s.buildXinzhiliVoiceConfigView(r.Context())
	saved, err := s.xinzhiliVoiceConfig.SaveDraft(r.Context(), input.TTS, *input.ExpectedVersion)
	if errors.Is(err, xinzhili.ErrConfigConflict) {
		httpx.Fail(w, http.StatusConflict, "config_version_conflict")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "xinzhili_voice_config.save_draft", TargetType: "xinzhili_voice_config", TargetID: "global", Before: before, After: buildXinzhiliVoiceConfigItemView(saved, true), Summary: "保存芯之力音色配置草稿"})
	view, err := s.buildXinzhiliVoiceConfigView(r.Context())
	if err != nil {
		view = xinzhiliVoiceConfigView{Draft: buildXinzhiliVoiceConfigItemView(saved, true)}
	}
	if !view.Draft.Found {
		view.Draft = buildXinzhiliVoiceConfigItemView(saved, true)
	}
	httpx.OK(w, view)
}

func (s *Server) xinzhiliVoiceConfigAction(w http.ResponseWriter, r *http.Request) {
	var input xinzhiliVoiceConfigActionRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}
	input.Action = strings.TrimSpace(input.Action)
	before, _ := s.buildXinzhiliVoiceConfigView(r.Context())
	switch input.Action {
	case "activate":
		if input.ExpectedVersion == nil {
			httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
			return
		}
		if _, err := s.xinzhiliVoiceConfig.Activate(r.Context(), *input.ExpectedVersion); err != nil {
			s.xinzhiliVoiceConfigActionError(w, err)
			return
		}
	case "deactivate":
		if input.ExpectedVersion == nil {
			httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
			return
		}
		if err := s.xinzhiliVoiceConfig.Deactivate(r.Context(), *input.ExpectedVersion); err != nil {
			s.xinzhiliVoiceConfigActionError(w, err)
			return
		}
	case "restore":
		if input.ExpectedVersion == nil {
			httpx.Fail(w, http.StatusBadRequest, "expectedVersion is required")
			return
		}
		if _, err := s.xinzhiliVoiceConfig.Restore(r.Context(), input.Version, *input.ExpectedVersion); err != nil {
			s.xinzhiliVoiceConfigActionError(w, err)
			return
		}
	case "schedule_remote_delete":
		if _, err := s.xinzhiliVoiceConfig.ScheduleRemoteDelete(r.Context(), input.Version, input.Provider, input.RemoteVoiceID); err != nil {
			s.xinzhiliVoiceConfigActionError(w, err)
			return
		}
	default:
		httpx.Fail(w, http.StatusBadRequest, "unknown action")
		return
	}
	view, err := s.buildXinzhiliVoiceConfigView(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.recordAdminAudit(r, auditlog.Entry{Action: "xinzhili_voice_config." + input.Action, TargetType: "xinzhili_voice_config", TargetID: "global", Before: before, After: view, Summary: "更新芯之力音色配置状态"})
	httpx.OK(w, view)
}

func (s *Server) xinzhiliVoiceConfigActionError(w http.ResponseWriter, err error) {
	if errors.Is(err, xinzhili.ErrConfigConflict) {
		httpx.Fail(w, http.StatusConflict, "config_version_conflict")
		return
	}
	httpx.Fail(w, http.StatusBadRequest, err.Error())
}

func (s *Server) buildXinzhiliVoiceConfigView(ctx context.Context) (xinzhiliVoiceConfigView, error) {
	active, activeFound, err := s.xinzhiliVoiceConfig.ReadActive(ctx)
	if err != nil {
		return xinzhiliVoiceConfigView{}, err
	}
	draft, draftFound, err := s.xinzhiliVoiceConfig.ReadDraft(ctx)
	if err != nil {
		return xinzhiliVoiceConfigView{}, err
	}
	return xinzhiliVoiceConfigView{Active: buildXinzhiliVoiceConfigItemView(active, activeFound), Draft: buildXinzhiliVoiceConfigItemView(draft, draftFound)}, nil
}

func buildXinzhiliVoiceConfigItemView(cfg xinzhili.VoiceConfig, found bool) xinzhiliVoiceConfigItemView {
	view := xinzhiliVoiceConfigItemView{Found: found}
	if !found {
		return view
	}
	view.Version, view.Status = cfg.Version, cfg.Status
	view.CreateTime = cfg.CreateTime.Format(timeRFC3339NanoOrEmpty)
	view.UpdateTime = cfg.UpdateTime.Format(timeRFC3339NanoOrEmpty)
	view.TTS = xinzhiliVoiceTTSConfigView{
		Provider: cfg.TTS.Provider,
		Endpoint: cfg.TTS.Endpoint,
		GroupID:  cfg.TTS.GroupID,
		Model:    cfg.TTS.Model,
		Voice:    cfg.TTS.Voice,
		Format:   cfg.TTS.Format,
		xinzhiliVoiceConfigSecretView: xinzhiliVoiceConfigSecretView{
			APIKeySet:    cfg.APIKeySet || strings.TrimSpace(cfg.TTS.APIKey) != "",
			APIKeySuffix: cfg.APIKeySuffix,
		},
	}
	return view
}

const timeRFC3339NanoOrEmpty = "2006-01-02T15:04:05.999999999Z07:00"
