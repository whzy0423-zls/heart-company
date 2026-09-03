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
	"unicode"

	"nine-xing/nx-backend/apps/server/internal/answerhygiene"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/uploadasset"
	"nine-xing/nx-backend/apps/server/internal/userpreference"
)

const (
	voiceChatMaxBytes      = 10 << 20
	voiceChatMinDurationMs = 800
	voiceChatMaxDurationMs = 60_000
)

type appChatVoiceStore interface {
	SaveVoicePair(ctx context.Context, sessionID, audioAssetID int64, durationMs int, transcript, answer string, sources json.RawMessage) (int64, int64, error)
	SaveVoiceMessage(ctx context.Context, sessionID, audioAssetID int64, durationMs int, transcript string) (int64, error)
	GetVoiceAudioAssetID(ctx context.Context, appUserID, messageID int64) (int64, error)
	GetVoiceTranscript(ctx context.Context, appUserID, messageID int64) (string, error)
}

type appChatVoiceKnowledgeStore interface {
	SaveVoicePairWithKnowledgeTrace(ctx context.Context, sessionID, audioAssetID int64, durationMs int, transcript, answer string, sources json.RawMessage, trace chat.KnowledgeTrace) (int64, int64, error)
}

type voiceChatResponse struct {
	UserMessage chat.Message `json:"userMessage"`
	Answer      rag.Answer   `json:"answer"`
	MessageID   int64        `json:"messageId"`
}

func (s *Server) appChatVoice(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	sessionID, ok := appChatSessionIDFromPath(r.URL.Path, "/voice")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if s.chatLimiter != nil && !s.chatLimiter.Allow(userInfo.ID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	sess, err := s.appChat.GetSession(r.Context(), userInfo.ID, sessionID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "session not found")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, voiceChatMaxBytes)
	if err := r.ParseMultipartForm(voiceChatMaxBytes); err != nil {
		httpx.Fail(w, http.StatusBadRequest, "文件过大或格式错误")
		return
	}
	tier, err := s.appChatTierForUser(r.Context(), userInfo.ID, r.FormValue("tier"))
	if err != nil {
		failAppChatTier(w, err)
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

	ctx, cancel := context.WithTimeout(r.Context(), s.chatTimeout)
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
	if !hasVoiceTranscriptContent(transcript) {
		contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		asset, createErr := s.createVoiceAsset(ctx, uploadasset.CreateInput{
			ContentType: contentType,
			Data:        audioData,
			Dir:         "app/chat/voice",
			Name:        header.Filename,
			Size:        int64(len(audioData)),
		})
		if createErr != nil {
			httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
			return
		}
		voiceStore, storeOK := s.appChat.(appChatVoiceStore)
		if !storeOK {
			httpx.Fail(w, http.StatusInternalServerError, "语音消息存储不可用")
			return
		}
		userMessageID, saveErr := voiceStore.SaveVoiceMessage(ctx, sessionID, asset.ID, durationMs, "")
		if saveErr != nil {
			httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
			return
		}
		httpx.OK(w, voiceChatResponse{
			UserMessage: chat.Message{
				ID:              userMessageID,
				SessionID:       sessionID,
				Role:            "user",
				Content:         "",
				Sources:         json.RawMessage("[]"),
				MessageType:     "voice",
				AudioDurationMs: durationMs,
				AudioURL:        fmt.Sprintf("/api/app/chat/messages/%d/audio", userMessageID),
			},
			Answer: rag.Answer{
				Answer:  "我没有听清你说了什么。请按住麦克风说话，靠近手机一些，松开后发送。",
				Sources: []rag.Source{},
			},
			MessageID: 0,
		})
		return
	}

	answer, isModelIdentity := appChatModelIdentityAnswer(transcript)
	var knowledgeTrace *chat.KnowledgeTrace
	extraction := userpreference.Extraction{}
	if !isModelIdentity {
		preferences, directives, preparedExtraction, err := s.prepareAppChatPreferencesLegacy(ctx, userInfo.ID, transcript)
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "偏好读取失败，请重试")
			return
		}
		extraction = preparedExtraction
		docs, trace := s.retrieveAppChatKnowledge(ctx, userInfo.ID, sessionID, sess.CardID, transcript)
		knowledgeTrace = trace
		profile, conversationCard := s.appChatProfilesForCard(ctx, userInfo.ID, sess.CardID)
		if memories, memoryErr := s.appChatMemoriesForPrompt(ctx, userInfo.ID, sess.CardID, 6); memoryErr == nil {
			profile.Memories = memories
		}
		generator := s.generator()
		promptContext := s.appChatContextForPrompt(ctx, sessionID, generator)
		answer, err = rag.NewService(docs, rag.WithGenerator(generator)).Ask(ctx, rag.AskInput{
			History:             promptContext.History,
			ConversationSummary: promptContext.Summary,
			Question:            transcript,
			UserProfile:         profile,
			ConversationCard:    conversationCard,
			UserPreferences:     preferences,
			CurrentDirectives:   directives,
			Tier:                tier,
		})
		if err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "回答生成失败，请重试")
			return
		}
		answer.Answer = answerhygiene.Clean(transcript, answer.Answer)
	}

	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	asset, err := s.createVoiceAsset(ctx, uploadasset.CreateInput{
		ContentType: contentType,
		Data:        audioData,
		Dir:         "app/chat/voice",
		Name:        header.Filename,
		Size:        int64(len(audioData)),
	})
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "语音保存失败，请重试")
		return
	}
	voiceStore, ok := s.appChat.(appChatVoiceStore)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "语音消息存储不可用")
		return
	}
	sourcesJSON, _ := json.Marshal(answer.Sources)
	userMessageID, assistantMessageID, err := saveAppChatVoicePair(ctx, voiceStore, sessionID, asset.ID, durationMs, transcript, answer.Answer, sourcesJSON, knowledgeTrace)
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "回答保存失败，请重试")
		return
	}
	if !isModelIdentity {
		if err := s.persistAppChatPreferences(ctx, userInfo.ID, extraction); err != nil {
			httpx.Fail(w, http.StatusInternalServerError, "偏好保存失败，请重试")
			return
		}
	}
	s.rememberChatAnswer(ctx, userInfo.ID, sess.CardID, transcript, answer.Answer)
	if assistantMessageID > 0 {
		s.recordAppProfileEvidenceAsync(userInfo.ID, sess.CardID, "voice_text", assistantMessageID, transcript)
	}

	httpx.OK(w, voiceChatResponse{
		UserMessage: chat.Message{
			ID:              userMessageID,
			SessionID:       sessionID,
			Role:            "user",
			Content:         "",
			Sources:         json.RawMessage("[]"),
			MessageType:     "voice",
			AudioDurationMs: durationMs,
			AudioURL:        fmt.Sprintf("/api/app/chat/messages/%d/audio", userMessageID),
		},
		Answer:    answer,
		MessageID: assistantMessageID,
	})
}

func saveAppChatVoicePair(ctx context.Context, store appChatVoiceStore, sessionID, audioAssetID int64, durationMs int, transcript, answer string, sources json.RawMessage, trace *chat.KnowledgeTrace) (int64, int64, error) {
	if trace == nil {
		return store.SaveVoicePair(ctx, sessionID, audioAssetID, durationMs, transcript, answer, sources)
	}
	knowledgeStore, ok := store.(appChatVoiceKnowledgeStore)
	if !ok {
		return 0, 0, errors.New("voice knowledge trace store unavailable")
	}
	return knowledgeStore.SaveVoicePairWithKnowledgeTrace(ctx, sessionID, audioAssetID, durationMs, transcript, answer, sources, *trace)
}

func hasVoiceTranscriptContent(transcript string) bool {
	for _, r := range strings.TrimSpace(transcript) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func (s *Server) appChatVoiceAudio(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	messageID, ok := messageIDFromPath(r.URL.Path, "audio")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid message id")
		return
	}
	voiceStore, ok := s.appChat.(appChatVoiceStore)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "语音消息存储不可用")
		return
	}
	assetID, err := voiceStore.GetVoiceAudioAssetID(r.Context(), userInfo.ID, messageID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "voice message not found")
		return
	}
	asset, err := s.findVoiceAsset(r.Context(), assetID)
	if err != nil {
		httpx.Fail(w, http.StatusNotFound, "voice audio not found")
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
}

func (s *Server) appChatVoiceTranscript(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	messageID, ok := messageIDFromPath(r.URL.Path, "transcript")
	if !ok {
		httpx.Fail(w, http.StatusBadRequest, "invalid message id")
		return
	}
	voiceStore, ok := s.appChat.(appChatVoiceStore)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "语音消息存储不可用")
		return
	}
	transcript, err := voiceStore.GetVoiceTranscript(r.Context(), userInfo.ID, messageID)
	if errors.Is(err, chat.ErrNotFound) {
		httpx.Fail(w, http.StatusNotFound, "voice message not found")
		return
	}
	if err != nil {
		httpx.Fail(w, http.StatusInternalServerError, "server error")
		return
	}
	httpx.OK(w, map[string]string{"text": transcript})
}

func (s *Server) createVoiceAsset(ctx context.Context, input uploadasset.CreateInput) (uploadasset.Asset, error) {
	if s.voiceAssetCreate != nil {
		return s.voiceAssetCreate(ctx, input)
	}
	if s.uploads == nil {
		return uploadasset.Asset{}, errors.New("voice asset store is unavailable")
	}
	return s.uploads.Create(ctx, input)
}

func (s *Server) findVoiceAsset(ctx context.Context, id int64) (uploadasset.Asset, error) {
	if s.voiceAssetFind != nil {
		return s.voiceAssetFind(ctx, id)
	}
	if s.uploads == nil {
		return uploadasset.Asset{}, errors.New("voice asset store is unavailable")
	}
	return s.uploads.Find(ctx, id)
}
