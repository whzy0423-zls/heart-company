package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
)

type skillVoiceResponse struct {
	UserMessage chat.Message `json:"userMessage"`
	Answer      rag.Answer   `json:"answer"`
	MessageID   int64        `json:"messageId"`
}

const skillVoiceCleanupTimeout = 5 * time.Second

func (s *Server) appSkillSessionVoice(w http.ResponseWriter, r *http.Request, appUserID int64) {
	sessionID, ok := appPathID(r.URL.Path, "/api/app/skill-sessions/", "/voice")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "会话编号无效")
		return
	}
	if s.chatLimiter != nil && !s.chatLimiter.Allow(appUserID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	session, err := s.skillChat.GetSession(r.Context(), appUserID, sessionID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "技能会话不存在或已停用")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, voiceChatMaxBytes)
	if err := r.ParseMultipartForm(voiceChatMaxBytes); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "文件过大或格式错误")
		return
	}
	durationMs, err := strconv.Atoi(strings.TrimSpace(r.FormValue("durationMs")))
	if err != nil || durationMs < voiceChatMinDurationMs || durationMs > voiceChatMaxDurationMs {
		httpx.Fail(w, http.StatusBadRequest, "语音时长需在 0.8 到 60 秒之间")
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "未找到音频文件")
		return
	}
	defer file.Close()
	if !isAllowedASRAudioUpload(header) {
		httpx.Fail(w, http.StatusBadRequest, "仅支持上传音频文件")
		return
	}
	audioData, err := io.ReadAll(io.LimitReader(file, voiceChatMaxBytes+1))
	if err != nil || len(audioData) == 0 || len(audioData) > voiceChatMaxBytes {
		httpx.Fail(w, http.StatusBadRequest, "音频文件无效或过大")
		return
	}
	_, timeout := s.chatRuntime()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	transcript, err := s.recognizeSpeech(ctx, audioData, header.Filename)
	if err != nil {
		if errors.Is(err, errASRNotConfigured) {
			httpx.Fail(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		httpx.Fail(w, http.StatusInternalServerError, "语音识别失败，请重试")
		return
	}
	transcript = strings.TrimSpace(transcript)
	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	if !hasVoiceTranscriptContent(transcript) {
		asset, err := s.createSkillVoiceAsset(ctx, audioData, header.Filename, contentType)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
			return
		}
		messageID, err := s.skillChat.SaveVoiceMessage(ctx, appUserID, sessionID, session.GenerationRevision, asset.ID, durationMs, "")
		if err != nil {
			s.deleteOrphanVoiceAsset(ctx, asset.ID)
			httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
			return
		}
		httpx.OK(w, skillVoiceResponse{
			UserMessage: skillVoiceUserMessage(sessionID, messageID, durationMs),
			Answer:      rag.Answer{Answer: "我没有听清你说了什么。请靠近手机重新说一次。", Sources: []rag.Source{}, Suggestions: []string{}},
		})
		return
	}
	result, err := s.skillChatRuntime.Generate(ctx, appUserID, sessionID, transcript, nil)
	if err != nil {
		failSkillChat(w, err)
		return
	}
	asset, err := s.createSkillVoiceAsset(ctx, audioData, header.Filename, contentType)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
		return
	}
	sourcesJSON, _ := json.Marshal(result.Sources)
	userMessageID, assistantMessageID, err := s.skillChat.SaveVoicePair(ctx, appUserID, sessionID, result.Trace, asset.ID, durationMs, transcript, result.Answer, sourcesJSON)
	if err != nil {
		s.deleteOrphanVoiceAsset(ctx, asset.ID)
		httpx.Fail(w, http.StatusInternalServerError, "回答保存失败，请重试")
		return
	}
	httpx.OK(w, skillVoiceResponse{
		UserMessage: skillVoiceUserMessage(sessionID, userMessageID, durationMs),
		Answer:      rag.Answer{Answer: result.Answer, Sources: result.Sources, Suggestions: result.Suggestions},
		MessageID:   assistantMessageID,
	})
}

func (s *Server) createSkillVoiceAsset(ctx context.Context, data []byte, filename, contentType string) (uploadasset.Asset, error) {
	return s.createVoiceAsset(ctx, uploadasset.CreateInput{
		ContentType: contentType, Data: data, Dir: "app/skill-chat/voice",
		Name: filename, Size: int64(len(data)),
	})
}

func (s *Server) deleteOrphanVoiceAsset(ctx context.Context, assetID int64) {
	if s.db != nil && assetID > 0 {
		cleanupCtx, cancel := skillVoiceCleanupContext(ctx)
		defer cancel()
		_, _ = s.db.ExecContext(cleanupCtx, `DELETE FROM upload_assets WHERE id=$1`, assetID)
	}
}

func skillVoiceCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), skillVoiceCleanupTimeout)
}

func skillVoiceUserMessage(sessionID, messageID int64, durationMs int) chat.Message {
	return chat.Message{
		ID: messageID, SessionID: sessionID, Role: "user", Content: "", Sources: json.RawMessage("[]"),
		MessageType: "voice", AudioDurationMs: durationMs,
		AudioURL: fmt.Sprintf("/api/app/skill-messages/%d/audio", messageID),
	}
}

func (s *Server) appSkillMessagesRouter(w http.ResponseWriter, r *http.Request) {
	user, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/audio") && r.Method == http.MethodGet:
		messageID, valid := appPathID(path, "/api/app/skill-messages/", "/audio")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "消息编号无效")
			return
		}
		assetID, err := s.skillChat.GetVoiceAudioAssetID(r.Context(), user.ID, messageID)
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "语音消息不存在")
			return
		}
		asset, err := s.findVoiceAsset(r.Context(), assetID)
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "语音文件不存在")
			return
		}
		contentType := strings.TrimSpace(asset.ContentType)
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(asset.Data)))
		w.Header().Set("Cache-Control", "private, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(asset.Data)
	case strings.HasSuffix(path, "/transcript") && r.Method == http.MethodGet:
		messageID, valid := appPathID(path, "/api/app/skill-messages/", "/transcript")
		if !valid {
			httpx.Fail(w, http.StatusBadRequest, "消息编号无效")
			return
		}
		transcript, err := s.skillChat.GetVoiceTranscript(r.Context(), user.ID, messageID)
		if err != nil {
			httpx.Fail(w, http.StatusNotFound, "语音消息不存在")
			return
		}
		httpx.OK(w, map[string]string{"text": transcript})
	default:
		httpx.Fail(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
