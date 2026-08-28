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

	"nine-xing/nx-backend/apps/server/internal/answerhygiene"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/httpx"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
	"nine-xing/nx-backend/apps/server/internal/theorystore"
	"nine-xing/nx-backend/apps/server/internal/voice"
)

const (
	xinzhiliMaxAudioBytes   = 10 << 20
	xinzhiliMaxRequestBytes = 11 << 20
	xinzhiliMultipartMemory = 1 << 20
	xinzhiliScene           = "xinzhili_voice"
	xinzhiliTTSWorkerCount  = 2
)

// normalizeXinzhiliVoiceTranscript keeps common ASR variants of "九型" stable
// before the transcript is displayed, searched, sent to the model, or stored.
func normalizeXinzhiliVoiceTranscript(transcript string) string {
	transcript = strings.TrimSpace(transcript)
	for _, replacement := range []struct {
		from string
		to   string
	}{
		{from: "九形", to: "九型"},
		{from: "九星", to: "九型"},
		{from: "九行", to: "九型"},
		{from: "九刑", to: "九型"},
		{from: "久行", to: "九型"},
		{from: "酒行", to: "九型"},
	} {
		transcript = strings.ReplaceAll(transcript, replacement.from, replacement.to)
	}

	// "就行" is a common phrase, so only normalize it when the surrounding
	// words indicate a 九型 request.
	for _, contextWord := range []string{"人格", "测试", "性格", "课程", "老师", "中心", "芯之力"} {
		if strings.Contains(transcript, "就行"+contextWord) {
			transcript = strings.ReplaceAll(transcript, "就行"+contextWord, "九型"+contextWord)
		}
	}
	for _, questionPattern := range []string{
		"什么是就行", "就行是什么", "了解就行", "关于就行", "学习就行", "讲讲就行", "问问就行",
	} {
		transcript = strings.ReplaceAll(transcript, questionPattern, strings.ReplaceAll(questionPattern, "就行", "九型"))
	}
	return transcript
}

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
	transcript = normalizeXinzhiliVoiceTranscript(transcript)
	if !hasVoiceTranscriptContent(transcript) {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "speech_not_understood", "message": "我没有听清你说了什么，请靠近手机再说一次。"})
		return
	}
	_ = asrStarted // reserved for structured latency logging
	if err := writeAppChatSSE(w, flusher, "transcript", map[string]string{"text": transcript}); err != nil {
		return
	}
	normalizeVoiceOutputToChinese := shouldNormalizeXinzhiliVoiceOutputToChinese(transcript)

	session, err := s.xinzhiliVoiceSession(ctx, userInfo.ID)
	if err != nil {
		_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "session_failed", "message": "会话准备失败，请重试"})
		return
	}
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "retrieving_knowledge"})
	docs, _ := s.retrieveXinzhiliDocs(ctx, transcript, 8)
	_ = writeAppChatSSE(w, flusher, "state", map[string]string{"state": "retrieving_theory"})
	theoryDocs, _ := s.retrieveXinzhiliTheoryDocs(ctx, transcript, 6, 0.2)
	docs = mergeXinzhiliRAGDocuments(docs, theoryDocs)

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
	if hooks.onTTSWorkerStart != nil {
		hooks.onTTSWorkerStart()
	}
	var ttsWorkerWG sync.WaitGroup
	ttsWorkerWG.Add(xinzhiliTTSWorkerCount)
	for i := 0; i < xinzhiliTTSWorkerCount; i++ {
		go func() {
			defer ttsWorkerWG.Done()
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
	}
	go func() {
		ttsWorkerWG.Wait()
		if hooks.onTTSWorkerExit != nil {
			hooks.onTTSWorkerExit()
		}
		close(ttsResults)
		close(ttsWorkerDone)
	}()
	defer func() {
		cancelGeneration()
		closeTTSJobs()
		<-ttsWorkerDone
	}()

	chunker := voice.NewSentenceChunker(64)
	nextSegment := 0
	nextAudioIndex := 0
	pendingAudio := make(map[int]xinzhiliTTSResult)
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
	emitOrderedAudio := func(result xinzhiliTTSResult) bool {
		if result.Err != nil {
			cancelGeneration()
			_ = writeAppChatSSE(w, flusher, "error", map[string]string{"code": "tts_failed", "message": "语音回复生成失败，请重试"})
			return false
		}
		pendingAudio[result.Job.Index] = result
		for {
			ready, ok := pendingAudio[nextAudioIndex]
			if !ok {
				return true
			}
			delete(pendingAudio, nextAudioIndex)
			if err := writeAppChatSSE(w, flusher, "audio", map[string]any{
				"index": ready.Job.Index, "text": ready.Job.Text, "contentType": ready.ContentType,
				"audioBase64": base64.StdEncoding.EncodeToString(ready.Audio),
			}); err != nil {
				cancelGeneration()
				return false
			}
			nextAudioIndex++
		}
	}
	var answerSentenceBuffer answerhygiene.SentenceBuffer
	emittedText := false
	emitCleanSentences := func(sentences []string) bool {
		for _, sentence := range sentences {
			outputDelta := answerhygiene.Clean(transcript, sentence)
			if outputDelta == answerhygiene.NeutralDirectAnswerFallback {
				continue
			}
			outputDelta = normalizeXinzhiliVoiceOutputDelta(outputDelta, normalizeVoiceOutputToChinese)
			if outputDelta == "" {
				continue
			}
			speechDelta := outputDelta
			if emittedText {
				outputDelta = "\n" + outputDelta
			}
			if err := writeAppChatSSE(w, flusher, "text_delta", map[string]string{"content": outputDelta}); err != nil {
				cancelGeneration()
				return false
			}
			if !queueChunks(chunker.Push(speechDelta)) {
				cancelGeneration()
				return false
			}
			emittedText = true
		}
		return true
	}

	for resultCh != nil || audioCh != nil {
		select {
		case delta := <-deltaCh:
			if delta == "" {
				continue
			}
			if !emitCleanSentences(answerSentenceBuffer.Push(delta)) {
				return
			}
		case completed := <-resultCh:
			answer, generationErr = completed.Answer, completed.Err
			answer.Answer = answerhygiene.Clean(transcript, answer.Answer)
			if normalizeVoiceOutputToChinese {
				answer.Answer = voice.NormalizeStrictChineseTTSInput(answer.Answer)
			}
			answer.Answer = answerhygiene.Clean(transcript, answer.Answer)
			resultCh = nil
			deltaCh = nil
			if !emitCleanSentences(answerSentenceBuffer.Flush()) {
				return
			}
			if !emittedText && strings.TrimSpace(answer.Answer) != "" {
				if err := writeAppChatSSE(w, flusher, "text_delta", map[string]string{"content": answer.Answer}); err != nil {
					cancelGeneration()
					return
				}
				if !queueChunks(chunker.Push(answer.Answer)) {
					cancelGeneration()
					return
				}
				emittedText = true
			}
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
			if !emitOrderedAudio(result) {
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

func normalizeXinzhiliVoiceOutputDelta(delta string, normalizeToChinese bool) string {
	if !normalizeToChinese {
		return delta
	}
	return voice.NormalizeStrictChineseTTSInput(delta)
}

func shouldNormalizeXinzhiliVoiceOutputToChinese(transcript string) bool {
	return !isExplicitXinzhiliEnglishRequest(transcript)
}

func isExplicitXinzhiliEnglishRequest(transcript string) bool {
	text := strings.ToLower(strings.TrimSpace(transcript))
	if text == "" {
		return false
	}
	if strings.Contains(text, "不要英文") || strings.Contains(text, "别用英文") || strings.Contains(text, "不要英语") || strings.Contains(text, "别说英文") || strings.Contains(text, "中文回答") || strings.Contains(text, "用中文") {
		return false
	}
	englishIntentPhrases := []string{
		"用英文", "说英文", "英文说", "英文回答", "英语回答", "用英语", "说英语", "英语说",
		"翻译成英文", "翻成英文", "译成英文", "翻译成英语", "英文单词", "英语单词",
		"英文怎么说", "英语怎么说", "怎么用英文", "怎么用英语", "读英文", "读英语",
	}
	for _, phrase := range englishIntentPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
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

func (s *Server) retrieveXinzhiliTheoryDocs(ctx context.Context, question string, topK int, minScore float64) ([]rag.Document, error) {
	if s.xinzhiliRetrieveTheoryDocs != nil {
		return s.xinzhiliRetrieveTheoryDocs(ctx, question, topK, minScore)
	}
	if s.db == nil {
		return nil, nil
	}
	return theorystore.NewStore(s.db).SearchActiveChunks(ctx, question, topK, minScore)
}

func mergeXinzhiliRAGDocuments(knowledgeDocs, theoryDocs []rag.Document) []rag.Document {
	documents := make([]rag.Document, 0, len(knowledgeDocs)+len(theoryDocs))
	seen := make(map[string]struct{}, len(knowledgeDocs)+len(theoryDocs))
	appendDocument := func(document rag.Document) {
		key := strings.TrimSpace(document.ID)
		if key == "" {
			key = strings.TrimSpace(document.Title) + "\x00" + strings.TrimSpace(document.Content)
		}
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		documents = append(documents, document)
	}
	for _, document := range knowledgeDocs {
		appendDocument(document)
	}
	for _, document := range theoryDocs {
		appendDocument(document)
	}
	return documents
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
