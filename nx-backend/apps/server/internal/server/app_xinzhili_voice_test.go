package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/auth"
	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/modelconfig"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestAppXinzhiliVoiceTurnStreamsTranscriptTextAudioAndPersists(t *testing.T) {
	var savedQuestion, savedAnswer string
	s := &Server{
		chatTimeout: 30,
		ragGen:      xinzhiliTestGenerator{},
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) {
			return readyXinzhiliVoiceConfig(), nil
		},
		xinzhiliMemberCheck: func(context.Context, int64) error { return nil },
		xinzhiliTranscribe:  func(context.Context, []byte, string) (string, error) { return "我最近总是着急", nil },
		xinzhiliSynthesize: func(_ context.Context, text string) ([]byte, string, error) {
			return []byte("audio:" + text), "audio/mpeg", nil
		},
		xinzhiliSession: func(context.Context, int64) (chat.Session, error) {
			return chat.Session{ID: 91, CardID: 5}, nil
		},
		xinzhiliRetrieveDocs: func(context.Context, string, int) ([]rag.Document, error) {
			return []rag.Document{{ID: "kb-1", Title: "知识", Content: "着急时先觉察身体"}, {ID: "theory-1", Title: "九型理论", Content: "不同型号有不同防御机制"}}, nil
		},
		xinzhiliSavePair: func(_ context.Context, sessionID int64, question, answer string, _ json.RawMessage) (int64, error) {
			if sessionID != 91 {
				t.Fatalf("session id = %d", sessionID)
			}
			savedQuestion, savedAnswer = question, answer
			return 101, nil
		},
	}
	req := newXinzhiliMultipartRequest(t, []byte("wav"), 1300)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	body := res.Body.String()
	for _, want := range []string{
		"event: ready",
		`"state":"transcribing"`,
		`event: transcript`,
		`"text":"我最近总是着急"`,
		`"state":"retrieving_knowledge"`,
		`"state":"retrieving_theory"`,
		`event: text_delta`,
		`event: audio`,
		base64.StdEncoding.EncodeToString([]byte("audio:先停一下。")),
		`event: done`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SSE body missing %q:\n%s", want, body)
		}
	}
	if savedQuestion != "我最近总是着急" || savedAnswer != "先停一下。再感受身体。" {
		t.Fatalf("saved pair = %q / %q", savedQuestion, savedAnswer)
	}
}

func TestAppXinzhiliVoiceTurnDoesNotGeneratePsychologyForBlankTranscript(t *testing.T) {
	generated := false
	s := &Server{
		ragGen:               generatorFunc(func(context.Context, rag.GenerateInput) (string, error) { generated = true; return "不应生成", nil }),
		xinzhiliConfigLoader: func(context.Context) (modelconfig.XinzhiliVoiceConfig, error) { return readyXinzhiliVoiceConfig(), nil },
		xinzhiliMemberCheck:  func(context.Context, int64) error { return nil },
		xinzhiliTranscribe:   func(context.Context, []byte, string) (string, error) { return "……", nil },
	}
	req := newXinzhiliMultipartRequest(t, []byte("silence"), 900)
	req = req.WithContext(contextWithAppUser(req.Context(), auth.UserInfo{ID: 7}))
	res := httptest.NewRecorder()

	s.appXinzhiliVoiceTurnStream(res, req)

	if generated {
		t.Fatal("blank transcript must not call the conversation model")
	}
	if body := res.Body.String(); !strings.Contains(body, `"code":"speech_not_understood"`) || !strings.Contains(body, "没有听清") {
		t.Fatalf("unexpected SSE body: %s", body)
	}
}

func readyXinzhiliVoiceConfig() modelconfig.XinzhiliVoiceConfig {
	return modelconfig.XinzhiliVoiceConfig{
		Enabled: true,
		ASR:     modelconfig.SpeechModelConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://speech.example/v1", APIKey: "asr", Model: "whisper-1"},
		TTS:     modelconfig.SpeechModelConfig{Provider: modelconfig.ProviderOpenAICompatible, APIBase: "https://speech.example/v1", APIKey: "tts", Model: "tts-1", Voice: "nova"},
	}
}

func newXinzhiliMultipartRequest(t *testing.T, audio []byte, durationMs int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("audio", "turn.wav")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(audio); err != nil {
		t.Fatal(err)
	}
	_ = writer.WriteField("durationMs", "1300")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/app/xinzhili/turns/stream", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

type xinzhiliTestGenerator struct{}

func (xinzhiliTestGenerator) Generate(context.Context, rag.GenerateInput) (string, error) {
	return "先停一下。再感受身体。", nil
}
func (xinzhiliTestGenerator) GenerateStream(_ context.Context, _ rag.GenerateInput, emit rag.StreamEmitter) (string, error) {
	if err := emit("先停一下。"); err != nil {
		return "", err
	}
	if err := emit("再感受身体。"); err != nil {
		return "", err
	}
	return "先停一下。再感受身体。", nil
}

type generatorFunc func(context.Context, rag.GenerateInput) (string, error)

func (f generatorFunc) Generate(ctx context.Context, input rag.GenerateInput) (string, error) {
	return f(ctx, input)
}
