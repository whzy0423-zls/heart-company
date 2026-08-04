package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/voice"
)

const (
	xinzhiliMaxAudioBytes   = 10 << 20
	xinzhiliMaxRequestBytes = 11 << 20
	xinzhiliMultipartMemory = 1 << 20
	xinzhiliScene           = "xinzhili_voice"
)

type appXinzhiliSceneStore interface {
	GetOrCreateSceneSession(ctx context.Context, appUserID, cardID int64, scene string) (chat.Session, error)
}

type xinzhiliTTSJob struct {
	Index int
	Text  string
}

type xinzhiliTTSResult struct {
	Job         xinzhiliTTSJob
	Audio       []byte
	ContentType string
	Err         error
}

type xinzhiliVoiceRuntimeHooks struct {
	onTTSWorkerStart func()
	onTTSWorkerExit  func()
}

// appXinzhiliVoiceTurnStream 完成一轮“录音→ASR→检索→LLM→分句TTS”的低延迟对话。
func (s *Server) appXinzhiliVoiceTurnStream(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, xinzhiliMaxRequestBytes)
	s.appXinzhiliVoiceTurnStreamWithMultipartMemory(w, r, xinzhiliMultipartMemory)
}

func (s *Server) appXinzhiliVoiceTurnStreamWithMultipartMemory(w http.ResponseWriter, r *http.Request, maxMemory int64) {
	s.appXinzhiliVoiceTurnStreamWithRuntimeHooks(w, r, maxMemory, xinzhiliVoiceRuntimeHooks{})
}

func (s *Server) appXinzhiliVoiceTurnStreamWithRuntimeHooks(w http.ResponseWriter, r *http.Request, maxMemory int64, hooks xinzhiliVoiceRuntimeHooks) {
	userInfo, ok := appUserFromContext(r)
	if !ok {
		httpx.Fail(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if !s.chatLimiter.Allow(userInfo.ID, time.Now()) {
		httpx.Fail(w, http.StatusTooManyRequests, "请求过于频繁，请稍后再试")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		httpx.Fail(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if err := s.ensureXinzhiliMember(r.Context(), userInfo.ID); err != nil {
		httpx.Fail(w, http.StatusForbidden, "芯之力为会员专属功能")
		return
	}
	cfg, err := s.loadXinzhiliVoiceConfig(r.Context())
	if err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, "请先在后台配置好芯之力语音模型后再重试")
		return
	}
	if err := cfg.ValidateReady(); err != nil {
		httpx.Fail(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	cleanupMultipart, err := parseXinzhiliMultipartForm(r, maxMemory)
	defer cleanupMultipart()
	if err != nil {
		if isTooLarge(err) {
			httpx.Fail(w, http.StatusRequestEntityTooLarge, "音频文件无效或过大")
			return
		}
		httpx.Fail(w, http.StatusBadRequest, "音频上传格式不正确")
		return
	}
	durationMs, err := strconv.Atoi(strings.TrimSpace(r.FormValue("durationMs")))
	if err != nil || durationMs < 300 || durationMs > 60000 {
		httpx.Fail(w, http.StatusBadRequest, "语音时长需在 0.3 到 60 秒之间")
		return
	}
	file, header, err := r.FormFile("audio")
	if err != nil {
		httpx.Fail(w, http.StatusBadRequest, "未找到音频文件")
		return
	}
	defer file.Close()
	audio, err := io.ReadAll(io.LimitReader(file, xinzhiliMaxAudioBytes+1))
	if err != nil || len(audio) == 0 || len(audio) > xinzhiliMaxAudioBytes {
		httpx.Fail(w, http.StatusBadRequest, "音频文件无效或过大")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := writeAppChatSSE(w, flusher, "ready", map[string]any{"durationMs": durationMs}); err != nil {
		return
	}

	timeout := s.chatTimeout
	if timeout < time.Second {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "transcribing"})
	asrStarted := time.Now()
	transcript, err := s.transcribeXinzhili(ctx, cfg, audio, header.Filename)
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "asr_failed", "message": "语音识别失败，请再试一次"})
		return
	}
	transcript = strings.TrimSpace(transcript)
	if !hasVoiceTranscriptContent(transcript) {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "speech_not_understood", "message": "我没有听清你说了什么，请靠近手机再说一次。"})
		return
	}
	_ = asrStarted // reserved for structured latency logging
	if err := writeAppChatSSE(w, flusher, "transcript", map[string]string{"text": transcript}); err != nil {
		return
	}

	session, err := s.xinzhiliVoiceSession(ctx, userInfo.ID)
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "session_failed", "message": "会话准备失败，请重试"})
		return
	}
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "retrieving_knowledge"})
	docs, _ := s.retrieveXinzhiliDocs(ctx, transcript, 8)
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "retrieving_theory"})

	preferences, directives, extraction, err := s.prepareAppChatPreferencesLegacy(ctx, userInfo.ID, transcript)
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "context_failed", "message": "上下文读取失败，请重试"})
		return
	}
	if prompt := strings.TrimSpace(cfg.SystemPrompt); prompt != "" {
		directives = append([]string{prompt}, directives...)
	}
	profile, conversationCard := s.appChatProfilesForCard(ctx, userInfo.ID, session.CardID)
	if s.db != nil {
		if memories, memoryErr := s.appChatMemoriesForPrompt(ctx, userInfo.ID, session.CardID, 6); memoryErr == nil {
			profile.Memories = memories
		}
	}
	promptContext := appChatPromptContext{}
	if s.appChat != nil {
		promptContext = s.appChatContextForPrompt(ctx, session.ID, s.generator())
	}
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "thinking"})

	generationCtx, cancelGeneration := context.WithCancel(ctx)
	deltas := make(chan string)
	type generationResult struct {
		Answer rag.Answer
		Err    error
	}
	generationDone := make(chan generationResult, 1)
	go func() {
		answer, generationErr := rag.NewService(docs, rag.WithGenerator(s.generator())).AskStream(generationCtx, rag.AskInput{
			History: promptContext.History, ConversationSummary: promptContext.Summary,
			Question: transcript, UserProfile: profile, ConversationCard: conversationCard,
			UserPreferences: preferences, CurrentDirectives: directives, Tier: "companion",
		}, func(delta string) error {
			select {
			case deltas <- delta:
				return nil
			case <-generationCtx.Done():
				return generationCtx.Err()
			}
		})
		generationDone <- generationResult{Answer: answer, Err: generationErr}
	}()

	ttsJobs := make(chan xinzhiliTTSJob, 8)
	ttsResults := make(chan xinzhiliTTSResult, 8)
	ttsWorkerDone := make(chan struct{})
	var closeTTSJobsOnce sync.Once
	closeTTSJobs := func() {
		closeTTSJobsOnce.Do(func() { close(ttsJobs) })
	}
	go func() {
		defer close(ttsWorkerDone)
		if hooks.onTTSWorkerStart != nil {
			hooks.onTTSWorkerStart()
		}
		if hooks.onTTSWorkerExit != nil {
			defer hooks.onTTSWorkerExit()
		}
		defer close(ttsResults)
		for {
			var job xinzhiliTTSJob
			select {
			case <-generationCtx.Done():
				return
			case received, open := <-ttsJobs:
				if !open {
					return
				}
				job = received
			}
			audioBytes, contentType, synthErr := s.synthesizeXinzhili(generationCtx, cfg, job.Text)
			select {
			case ttsResults <- xinzhiliTTSResult{Job: job, Audio: audioBytes, ContentType: contentType, Err: synthErr}:
			case <-generationCtx.Done():
				return
			}
			if synthErr != nil {
				return
			}
		}
	}()
	defer func() {
		cancelGeneration()
		closeTTSJobs()
		<-ttsWorkerDone
	}()

	chunker := voice.NewSentenceChunker(64)
	nextSegment := 0
	speakingStateSent := false
	var answer rag.Answer
	var generationErr error
	deltaCh := (<-chan string)(deltas)
	resultCh := (<-chan generationResult)(generationDone)
	audioCh := (<-chan xinzhiliTTSResult)(ttsResults)

	queueChunks := func(chunks []string) bool {
		for _, chunk := range chunks {
			if !speakingStateSent {
				if err := writeAppChatSSE(w, flusher, "state", map[string]string{"state": "speaking"}); err != nil {
					return false
				}
				speakingStateSent = true
			}
			job := xinzhiliTTSJob{Index: nextSegment, Text: chunk}
			nextSegment++
			select {
			case ttsJobs <- job:
			case <-generationCtx.Done():
				return false
			}
		}
		return true
	}

	for resultCh != nil || audioCh != nil {
		select {
		case delta := <-deltaCh:
			if err := writeAppChatSSE(w, flusher, "text_delta", map[string]string{"content": delta}); err != nil {
				cancelGeneration()
				return
			}
			if !queueChunks(chunker.Push(delta)) {
				cancelGeneration()
				return
			}
		case completed := <-resultCh:
			answer, generationErr = completed.Answer, completed.Err
			resultCh = nil
			deltaCh = nil
			if !queueChunks(chunker.Flush()) {
				cancelGeneration()
				return
			}
			closeTTSJobs()
		case result, open := <-audioCh:
			if !open {
				audioCh = nil
				continue
			}
			if result.Err != nil {
				cancelGeneration()
				_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "tts_failed", "message": "语音回复生成失败，请重试"})
				return
			}
			if err := writeAppChatSSE(w, flusher, "audio", map[string]any{
				"index": result.Job.Index, "text": result.Job.Text, "contentType": result.ContentType,
				"audioBase64": base64.StdEncoding.EncodeToString(result.Audio),
			}); err != nil {
				cancelGeneration()
				return
			}
		case <-ctx.Done():
			cancelGeneration()
			return
		}
	}
	closeTTSJobs()
	if generationErr != nil || strings.TrimSpace(answer.Answer) == "" {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "generation_failed", "message": "回答生成失败，请重试"})
		return
	}

	sourcesJSON, _ := json.Marshal(answer.Sources)
	messageID, err := s.saveXinzhiliPair(ctx, session.ID, transcript, answer.Answer, sourcesJSON)
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "save_failed", "message": "回答保存失败，请重试"})
		return
	}
	if err := s.persistAppChatPreferences(ctx, userInfo.ID, extraction); err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "preference_failed", "message": "偏好保存失败，请重试"})
		return
	}
	_ = writeAppChatSSE(w, flusher, "done", map[string]any{"answer": answer.Answer, "sources": answer.Sources, "messageId": messageID})
}

func parseXinzhiliMultipartForm(r *http.Request, maxMemory int64) (func(), error) {
	err := r.ParseMultipartForm(maxMemory)
	return func() {
		cleanupXinzhiliMultipartForm(r.MultipartForm, func(form *multipart.Form) error {
			return form.RemoveAll()
		}, log.Printf)
	}, err
}

func cleanupXinzhiliMultipartForm(form *multipart.Form, removeAll func(*multipart.Form) error, warnf func(string, ...any)) {
	if form == nil {
		return
	}
	if err := removeAll(form); err != nil {
		warnf("xinzhili multipart cleanup failed: %v", err)
	}
}

func (s *Server) ensureXinzhiliMember(ctx context.Context, userID int64) error {
	if s.xinzhiliMemberCheck != nil {
		return s.xinzhiliMemberCheck(ctx, userID)
	}
	_, err := s.appChatTierForUser(ctx, userID, "companion")
	return err
}

func (s *Server) loadXinzhiliVoiceConfig(ctx context.Context) (modelconfig.XinzhiliVoiceConfig, error) {
	if s.xinzhiliConfigLoader != nil {
		return s.xinzhiliConfigLoader(ctx)
	}
	stored, found, err := modelconfig.ReadStore(ctx, s.db)
	if err != nil || !found {
		return modelconfig.XinzhiliVoiceConfig{}, errors.New("xinzhili voice config missing")
	}
	return stored.ApplyXinzhiliVoice(), nil
}

func (s *Server) transcribeXinzhili(ctx context.Context, cfg modelconfig.XinzhiliVoiceConfig, audio []byte, filename string) (string, error) {
	if s.xinzhiliTranscribe != nil {
		return s.xinzhiliTranscribe(ctx, audio, filename)
	}
	return voice.NewCompatibleSpeechClient(cfg).Transcribe(ctx, audio, filename)
}

func (s *Server) synthesizeXinzhili(ctx context.Context, cfg modelconfig.XinzhiliVoiceConfig, text string) ([]byte, string, error) {
	if s.xinzhiliSynthesize != nil {
		return s.xinzhiliSynthesize(ctx, text)
	}
	return voice.NewCompatibleSpeechClient(cfg).Synthesize(ctx, text)
}

func (s *Server) xinzhiliVoiceSession(ctx context.Context, userID int64) (chat.Session, error) {
	if s.xinzhiliSession != nil {
		return s.xinzhiliSession(ctx, userID)
	}
	if s.quiz == nil {
		return chat.Session{}, errors.New("primary card store unavailable")
	}
	primary, err := s.quiz.PrimaryCard(ctx, userID)
	if err != nil {
		return chat.Session{}, err
	}
	store, ok := s.appChat.(appXinzhiliSceneStore)
	if !ok {
		return chat.Session{}, errors.New("xinzhili scene store unavailable")
	}
	return store.GetOrCreateSceneSession(ctx, userID, primary.ID, xinzhiliScene)
}

func (s *Server) retrieveXinzhiliDocs(ctx context.Context, question string, topK int) ([]rag.Document, error) {
	if s.xinzhiliRetrieveDocs != nil {
		return s.xinzhiliRetrieveDocs(ctx, question, topK)
	}
	return s.retrieveAppDocsForQuery(ctx, question, topK)
}

func (s *Server) saveXinzhiliPair(ctx context.Context, sessionID int64, question, answer string, sources json.RawMessage) (int64, error) {
	if s.xinzhiliSavePair != nil {
		return s.xinzhiliSavePair(ctx, sessionID, question, answer, sources)
	}
	if s.appChat == nil {
		return 0, fmt.Errorf("chat store unavailable")
	}
	return s.appChat.SavePair(ctx, sessionID, question, answer, sources)
}
