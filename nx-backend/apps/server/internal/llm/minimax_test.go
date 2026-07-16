package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nine-xing/nx-backend/apps/server/internal/config"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func newLocalMiniMaxGenerator(upstream *httptest.Server, cfg config.MiniMaxConfig) *MiniMaxGenerator {
	cfg.APIBase = upstream.URL
	generator := NewMiniMaxGenerator(cfg)
	generator.client = upstream.Client()
	return generator
}

func TestMiniMaxGeneratorGenerateStreamEmitsBeforeUpstreamCompletes(t *testing.T) {
	firstFlushed := make(chan struct{})
	releaseSecond := make(chan struct{})
	defer func() {
		select {
		case <-releaseSecond:
		default:
			close(releaseSecond)
		}
	}()
	requestErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/text/chatcompletion_v2" {
			requestErr <- fmt.Errorf("unexpected endpoint: %s", r.URL.Path)
			return
		}
		if r.URL.Query().Get("GroupId") != "test-group" {
			requestErr <- fmt.Errorf("unexpected GroupId: %s", r.URL.RawQuery)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			requestErr <- fmt.Errorf("unexpected authorization: %s", r.Header.Get("Authorization"))
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			requestErr <- err
			return
		}
		encoded, _ := json.Marshal(body)
		for _, want := range []string{"MiniMax-M3", "测试系统提示", "现在怎么办？", "先稳住情绪"} {
			if !strings.Contains(string(encoded), want) {
				requestErr <- fmt.Errorf("request missing %q: %s", want, encoded)
				return
			}
		}
		if body["stream"] != true {
			requestErr <- fmt.Errorf("expected stream=true, got %+v", body["stream"])
			return
		}
		if body["tokens_to_generate"] != float64(220) {
			requestErr <- fmt.Errorf("unexpected token budget: %+v", body["tokens_to_generate"])
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"先听你说。\"}}]}\n\n")
		w.(http.Flusher).Flush()
		close(firstFlushed)
		<-releaseSecond
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"我们慢慢来。\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey:       "test-key",
		GroupID:      "test-group",
		Model:        "MiniMax-M3",
		SystemPrompt: "测试系统提示",
	})
	emitted := make(chan string, 2)
	result := make(chan struct {
		answer string
		err    error
	}, 1)
	go func() {
		answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{
			Question: "现在怎么办？",
			Sources:  []rag.Source{{Title: "资料", Snippet: "先稳住情绪"}},
		}, func(delta string) error {
			emitted <- delta
			return nil
		})
		result <- struct {
			answer string
			err    error
		}{answer: answer, err: err}
	}()

	select {
	case <-firstFlushed:
	case err := <-requestErr:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush the first event")
	}
	select {
	case got := <-emitted:
		if got != "先听你说。" {
			t.Fatalf("unexpected first delta: %q", got)
		}
	case got := <-result:
		t.Fatalf("GenerateStream completed before the upstream second event: %+v", got)
	case <-time.After(time.Second):
		t.Fatal("first delta was not emitted while upstream was still blocked")
	}
	select {
	case got := <-result:
		t.Fatalf("GenerateStream completed before release: %+v", got)
	default:
	}
	close(releaseSecond)

	select {
	case got := <-emitted:
		if got != "我们慢慢来。" {
			t.Fatalf("unexpected second delta: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second delta was not emitted")
	}
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatalf("GenerateStream returned error: %v", got.err)
		}
		if got.answer != "先听你说。我们慢慢来。" {
			t.Fatalf("unexpected complete answer: %q", got.answer)
		}
	case <-time.After(time.Second):
		t.Fatal("GenerateStream did not complete")
	}
}

func TestMiniMaxGeneratorGenerateStreamParsesSSEVariants(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "LF OpenAI delta content",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"甲\"}}]}\n\n" +
				"data: [DONE]\n\n",
			want: "甲",
		},
		{
			name: "CRLF MiniMax string delta",
			body: "data: {\"choices\":[{\"delta\":\"乙\"}]}\r\n\r\n" +
				"data: [DONE]\r\n\r\n",
			want: "乙",
		},
		{
			name: "multiple events and messages text",
			body: ": keepalive\n\n" +
				"data: {\"choices\":[{\"messages\":[{\"text\":\"丙\"}]}]}\n\n" +
				"data: {\"choices\":[{\"text\":\"丁\"}]}\n\n" +
				"data: {\"usage\":{\"total_tokens\":2}}\n\n" +
				"data: {\"choices\":[{\"finish_reason\":\"stop\"}]}\n\n" +
				"data: [DONE]\n\n",
			want: "丙丁",
		},
		{
			name: "message content compatibility",
			body: "event: message\n" +
				"data: {\"choices\":[{\"message\":{\"content\":\"戊\"}}]}\n\n",
			want: "戊",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
			var emitted strings.Builder
			answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(delta string) error {
				emitted.WriteString(delta)
				return nil
			})
			if err != nil {
				t.Fatalf("GenerateStream returned error: %v", err)
			}
			if answer != tt.want || emitted.String() != tt.want {
				t.Fatalf("answer/emitted = %q/%q, want %q", answer, emitted.String(), tt.want)
			}
		})
	}
}

func TestMiniMaxGeneratorGenerateStreamDoesNotDuplicateFinalSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":\"你\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"message\":{\"content\":\"你好\"},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	var emitted strings.Builder
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(delta string) error {
		emitted.WriteString(delta)
		return nil
	})
	if err != nil {
		t.Fatalf("GenerateStream returned error: %v", err)
	}
	if answer != "你好" || emitted.String() != "你好" {
		t.Fatalf("answer/emitted = %q/%q, want deduplicated snapshot %q", answer, emitted.String(), "你好")
	}
}

func TestMiniMaxGeneratorGenerateStreamDoesNotUseClientTotalTimeout(t *testing.T) {
	firstFlushed := make(chan struct{})
	releaseSecond := make(chan struct{})
	defer func() {
		select {
		case <-releaseSecond:
		default:
			close(releaseSecond)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":\"长\"}]}\n\n")
		w.(http.Flusher).Flush()
		close(firstFlushed)
		<-releaseSecond
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":\"答\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	generator.client.Timeout = 20 * time.Millisecond
	result := make(chan error, 1)
	go func() {
		answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(string) error { return nil })
		if err == nil && answer != "长答" {
			err = fmt.Errorf("unexpected answer: %q", answer)
		}
		result <- err
	}()
	select {
	case <-firstFlushed:
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush")
	}
	time.Sleep(60 * time.Millisecond)
	close(releaseSecond)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("stream must outlive the client's total timeout: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GenerateStream did not complete")
	}
}

func TestMiniMaxGeneratorGenerateStreamHandlesDelimiterAcrossReads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, part := range []string{
			"data: {\"choices\":[{\"delta\":\"分片\"}]}\r",
			"\n\r",
			"\n",
			"data: [DONE]\n",
			"\n",
		} {
			_, _ = fmt.Fprint(w, part)
			flusher.Flush()
		}
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(string) error { return nil })
	if err != nil {
		t.Fatalf("GenerateStream returned error: %v", err)
	}
	if answer != "分片" {
		t.Fatalf("unexpected answer: %q", answer)
	}
}

func TestMiniMaxGeneratorGenerateStreamRejectsProtocolErrors(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantError  string
	}{
		{name: "non 2xx", statusCode: http.StatusBadGateway, body: "upstream unavailable", wantError: "请求失败(502)"},
		{name: "SSE error event", body: "event: error\ndata: {\"message\":\"限流了\"}\n\n", wantError: "限流了"},
		{name: "plain text SSE error event", body: "event: error\ndata: rate limited\n\n", wantError: "rate limited"},
		{name: "error JSON", body: "data: {\"error\":{\"message\":\"鉴权失败\"}}\n\n", wantError: "鉴权失败"},
		{name: "base response error", body: "data: {\"base_resp\":{\"status_code\":1001,\"status_msg\":\"参数错误\"}}\n\n", wantError: "参数错误"},
		{name: "malformed JSON", body: "data: {not-json}\n\n", wantError: "响应解析失败"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				statusCode := tt.statusCode
				if statusCode == 0 {
					statusCode = http.StatusOK
				}
				w.WriteHeader(statusCode)
				_, _ = fmt.Fprint(w, tt.body)
			}))
			defer server.Close()

			generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
			answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(string) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("answer/error = %q/%v, want error containing %q", answer, err, tt.wantError)
			}
			if answer != "" {
				t.Fatalf("error body must not become an answer: %q", answer)
			}
		})
	}
}

func TestMiniMaxGeneratorGenerateStreamRejectsOversizedEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":%q}]}\n\n", strings.Repeat("x", miniMaxMaxStreamEventBytes))
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	_, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "事件过大") {
		t.Fatalf("expected oversized event error, got %v", err)
	}
}

func TestMiniMaxGeneratorGenerateStreamAcceptsEventAtSizeLimit(t *testing.T) {
	prefix := "data: {\"choices\":[{\"delta\":\""
	suffix := "\"}]}\n"
	delta := strings.Repeat("x", miniMaxMaxStreamEventBytes-len(prefix)-len(suffix))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, prefix+delta+suffix+"\n")
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	answer, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, nil)
	if err != nil {
		t.Fatalf("event at size limit must be accepted: %v", err)
	}
	if answer != delta {
		t.Fatalf("unexpected answer length: got %d, want %d", len(answer), len(delta))
	}
}

func TestMiniMaxGeneratorGenerateStreamPropagatesContextCancellation(t *testing.T) {
	firstFlushed := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":\"第一段\"}]}\n\n")
		w.(http.Flusher).Flush()
		close(firstFlushed)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	emitted := make(chan string, 1)
	result := make(chan error, 1)
	go func() {
		_, err := generator.GenerateStream(ctx, rag.GenerateInput{Question: "test"}, func(delta string) error {
			emitted <- delta
			return nil
		})
		result <- err
	}()

	select {
	case <-firstFlushed:
	case <-time.After(time.Second):
		t.Fatal("upstream did not flush")
	}
	select {
	case got := <-emitted:
		if got != "第一段" {
			t.Fatalf("unexpected delta: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("first delta was not emitted")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("GenerateStream did not stop after cancellation")
	}
}

func TestMiniMaxGeneratorGenerateStreamPropagatesEmitterError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":\"内容\"}]}\n\n")
	}))
	defer server.Close()

	wantErr := errors.New("client disconnected")
	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	_, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "test"}, func(string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected emitter error, got %v", err)
	}
}

func TestMiniMaxGeneratorGenerateStreamRequiresAPIKey(t *testing.T) {
	_, err := NewMiniMaxGenerator(config.MiniMaxConfig{}).GenerateStream(context.Background(), rag.GenerateInput{Question: "hi"}, nil)
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestMiniMaxGeneratorSendsRAGContext(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header: %s", r.Header.Get("Authorization"))
		}
		if !strings.Contains(r.URL.RawQuery, "GroupId=test-group") {
			t.Fatalf("missing group id in query: %s", r.URL.RawQuery)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "模型回答"},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey:  "test-key",
		GroupID: "test-group",
	})
	answer, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question: "完美型怎么成长？",
		UserProfile: rag.UserProfile{
			Nickname: "小九",
			MainType: 1,
		},
		Sources: []rag.Source{{Title: "1号 完美型", Snippet: "允许不完美"}},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}
	if answer != "模型回答" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	messages, _ := requestBody["messages"].([]any)
	if len(messages) < 2 {
		t.Fatalf("expected messages in request: %+v", requestBody)
	}
	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "允许不完美") || !strings.Contains(string(body), "完美型怎么成长") {
		t.Fatalf("request did not include rag context/question: %s", string(body))
	}
}

func TestMiniMaxGeneratorUsesConciseChatTokenBudget(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "简短回答"}}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	if _, err := generator.Generate(context.Background(), rag.GenerateInput{Question: "我该怎么办？"}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if requestBody["tokens_to_generate"] != float64(220) {
		t.Fatalf("expected chat token budget 220, got %+v", requestBody["tokens_to_generate"])
	}
}

func TestChatTokenBudget(t *testing.T) {
	tests := []struct {
		question string
		want     int
	}{
		{question: "不要叫我亲爱的", want: 220},
		{question: "简单说重点", want: 220},
		{question: "请详细展开分析原因和步骤", want: 420},
		{question: "请完整分析一下", want: 420},
		{question: "1-9型号的孩子我们如何应用", want: 1200},
	}

	for _, tt := range tests {
		t.Run(tt.question, func(t *testing.T) {
			if got := chatTokenBudget(tt.question); got != tt.want {
				t.Fatalf("chatTokenBudget(%q) = %d, want %d", tt.question, got, tt.want)
			}
		})
	}
}

func TestMiniMaxGeneratorGenerateStreamUsesAdaptiveChatTokenBudget(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"回答\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	if _, err := generator.GenerateStream(context.Background(), rag.GenerateInput{Question: "请详细展开分析原因"}, nil); err != nil {
		t.Fatalf("GenerateStream returned error: %v", err)
	}

	if requestBody["tokens_to_generate"] != float64(420) {
		t.Fatalf("expected stream chat token budget 420, got %+v", requestBody["tokens_to_generate"])
	}
}

func TestDefaultSystemPromptUsesWarmConciseAdaptiveStyle(t *testing.T) {
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{})
	prompt := generator.resolveSystemPrompt()

	for _, want := range []string{
		"像一个懂用户的朋友",
		"自然、有温度、少说教",
		"普通问题通常只回答 1-3 句",
		"只有用户明确要求展开",
		"不主动使用亲爱的、宝贝等亲昵称呼",
		"用户要求纠正时立即按新要求重答",
		"不要解释为什么要纠正",
		"不要机械复述用户的话",
		"不要固定总结",
		"不要固定给建议",
		"最多追问一个真正有用的问题",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("default system prompt missing %q: %s", want, prompt)
		}
	}
}

func TestBuildUserPromptDoesNotForceFixedAnswerStructure(t *testing.T) {
	prompt := buildUserPrompt(rag.GenerateInput{Question: "完美型怎么成长？"})

	for _, forbidden := range []string{
		"给出 2-4 段回答",
		"最后给一个可执行的小建议",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("user prompt must not force %q: %s", forbidden, prompt)
		}
	}
}

func TestBuildUserPromptEndsWithFixedConciseInstruction(t *testing.T) {
	prompt := buildUserPrompt(rag.GenerateInput{
		Question: "观察型怎么沟通？",
		Sources: []rag.Source{{
			Title:   "5号 观察型",
			Snippet: "观察型重视边界和独处空间。",
		}},
	})

	questionIndex := strings.Index(prompt, "用户问题：观察型怎么沟通？")
	sourceIndex := strings.Index(prompt, "1. 5号 观察型：观察型重视边界和独处空间。")
	instructionIndex := strings.LastIndex(prompt, fixedConciseReplyInstruction)
	if questionIndex < 0 || sourceIndex < 0 || instructionIndex < 0 || !(questionIndex < sourceIndex && sourceIndex < instructionIndex) {
		t.Fatalf("prompt order is wrong: %s", prompt)
	}
	if !strings.HasSuffix(prompt, fixedConciseReplyInstruction) {
		t.Fatalf("prompt must end with fixed concise instruction: %s", prompt)
	}
}

func TestBuildUserPromptUsesAllTypesCoverageInstruction(t *testing.T) {
	prompt := buildUserPrompt(rag.GenerateInput{
		Question: "1-9型号的孩子我们如何应用",
		UserProfile: rag.UserProfile{
			MainType: 6,
		},
		Sources: []rag.Source{{
			Title:   "6号 忠诚型",
			Snippet: "忠诚型重视安全感。",
		}},
	})

	for _, want := range []string{
		"按1号到9号的顺序逐一回答",
		"孩子的典型特点",
		"家长如何理解和沟通",
		"一个具体应用方法",
		"不能因为检索资料不完整而遗漏任何型号",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("all-types prompt missing %q: %s", want, prompt)
		}
	}
	if strings.Contains(prompt, fixedConciseReplyInstruction) {
		t.Fatalf("all-types prompt must not force the fixed concise instruction: %s", prompt)
	}
}

func TestMiniMaxGeneratorKeepsConfiguredSystemPromptAndAddsFixedConciseInstruction(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "简短回答"}}},
		})
	}))
	defer server.Close()

	const configuredSystemPrompt = "这是后台配置的系统提示词。"
	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey:       "test-key",
		Model:        "configured-chat-model",
		SystemPrompt: configuredSystemPrompt,
	})
	_, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question: "完美型怎么成长？",
		Sources: []rag.Source{{
			Title:   "1号 完美型",
			Snippet: "允许不完美。",
		}},
	})
	if err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	if requestBody["model"] != "configured-chat-model" {
		t.Fatalf("configured model was not used: %+v", requestBody["model"])
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("unexpected messages: %+v", requestBody["messages"])
	}
	system, _ := messages[0].(map[string]any)
	user, _ := messages[1].(map[string]any)
	if system["content"] != configuredSystemPrompt {
		t.Fatalf("configured system prompt was not preserved: %+v", system)
	}
	userPrompt, _ := user["content"].(string)
	if !strings.Contains(userPrompt, "允许不完美。") || !strings.HasSuffix(userPrompt, fixedConciseReplyInstruction) {
		t.Fatalf("user prompt missing RAG context or fixed instruction: %s", userPrompt)
	}
}

func TestBuildUserPromptPutsCurrentDirectivesAfterSavedPreferencesAndQuestion(t *testing.T) {
	prompt := buildUserPrompt(rag.GenerateInput{
		Question:          "不要叫我亲爱的",
		UserPreferences:   []string{"称呼用户为亲爱的", "回答更详细"},
		CurrentDirectives: []string{"不要使用“亲爱的”等亲昵称呼", "回答简短，避免长篇大论"},
	})

	savedIndex := strings.Index(prompt, "已保存的交流偏好")
	questionIndex := strings.Index(prompt, "用户问题：不要叫我亲爱的")
	currentIndex := strings.Index(prompt, "当前消息优先规则")
	if savedIndex < 0 || questionIndex < 0 || currentIndex < 0 || !(savedIndex < questionIndex && questionIndex < currentIndex) {
		t.Fatalf("prompt precedence order is wrong: %s", prompt)
	}
	if !strings.Contains(prompt, "与旧偏好或历史冲突时，以当前规则为准") {
		t.Fatalf("prompt missing explicit current-message precedence: %s", prompt)
	}
}

func TestResolveSystemPromptKeepsConfiguredOverride(t *testing.T) {
	const customPrompt = "你是用户自定义的专属陪伴者。"
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{SystemPrompt: customPrompt})

	if got := generator.resolveSystemPrompt(); got != customPrompt {
		t.Fatalf("expected configured system prompt %q, got %q", customPrompt, got)
	}
}

func TestMiniMaxGeneratorSendsUserMemories(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "结合记忆的回答"},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey: "test-key",
	})
	if _, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question: "我最近压力大怎么办？",
		UserProfile: rag.UserProfile{
			Memories: []string{"用户曾问：如何处理职场压力？"},
		},
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "近期记忆") ||
		!strings.Contains(string(body), "如何处理职场压力") {
		t.Fatalf("request did not include user memories: %s", string(body))
	}
}

func TestMiniMaxGeneratorSendsConversationSummary(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "继续回答"}}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	if _, err := generator.Generate(context.Background(), rag.GenerateInput{
		Question:            "那我接下来怎么说？",
		ConversationSummary: "用户和女儿因为作业发生争执，之前建议先暂停十分钟。",
	}); err != nil {
		t.Fatalf("Generate returned error: %v", err)
	}

	body, _ := json.Marshal(requestBody)
	if !strings.Contains(string(body), "会话前情") || !strings.Contains(string(body), "先暂停十分钟") {
		t.Fatalf("request did not include conversation summary: %s", string(body))
	}
}

func TestMiniMaxGeneratorSummarizesConversationWithPreviousContext(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{"message": map[string]any{"content": "用户与女儿讨论作业冲突，仍需确认沟通方式。"}}},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key"})
	summary, err := generator.SummarizeConversation(context.Background(), "用户女儿八岁。", []rag.Message{
		{Role: "user", Content: "她今天因为作业哭了"},
		{Role: "assistant", Content: "先接住情绪，再讨论作业"},
	})
	if err != nil {
		t.Fatalf("SummarizeConversation returned error: %v", err)
	}
	if summary != "用户与女儿讨论作业冲突，仍需确认沟通方式。" {
		t.Fatalf("unexpected summary: %q", summary)
	}

	body, _ := json.Marshal(requestBody)
	text := string(body)
	for _, want := range []string{"会话摘要", "用户女儿八岁", "她今天因为作业哭了", "尚未解决"} {
		if !strings.Contains(text, want) {
			t.Fatalf("summary request missing %q: %s", want, text)
		}
	}
}

func TestMiniMaxGeneratorRequiresAPIKey(t *testing.T) {
	_, err := NewMiniMaxGenerator(config.MiniMaxConfig{}).Generate(context.Background(), rag.GenerateInput{Question: "hi"})
	if err == nil {
		t.Fatal("expected missing api key error")
	}
}

func TestMiniMaxGeneratorRejectsLocalAPIBaseBeforeDial(t *testing.T) {
	sawRequest := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		sawRequest = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "local response"},
				},
			},
		})
	}))
	defer server.Close()

	generator := NewMiniMaxGenerator(config.MiniMaxConfig{
		APIBase: server.URL,
		APIKey:  "test-key",
	})

	_, err := generator.Generate(context.Background(), rag.GenerateInput{Question: "hi"})
	if err == nil {
		t.Fatal("expected local API base to be rejected")
	}
	if !strings.Contains(err.Error(), "private or local") {
		t.Fatalf("expected private/local address error, got %v", err)
	}
	if sawRequest {
		t.Fatal("local API base must be rejected before sending the request")
	}
}

func TestMiniMaxGeneratorUsesConfiguredTimeout(t *testing.T) {
	generator := NewMiniMaxGenerator(config.MiniMaxConfig{TimeoutSeconds: 12})
	if generator.client.Timeout.String() != "12s" {
		t.Fatalf("expected configured timeout, got %s", generator.client.Timeout)
	}

	defaultGenerator := NewMiniMaxGenerator(config.MiniMaxConfig{})
	if defaultGenerator.client.Timeout.String() != "25s" {
		t.Fatalf("expected 25s default timeout, got %s", defaultGenerator.client.Timeout)
	}
}

func TestAnalyzeVideoUsesOpenAICompatibleEndpointForProxyBase(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"scenes":["室内"],"characters":["女性"],"assets":["近景"],"seedancePrompt":"室内女性近景，柔和光线"}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey: "test-key",
		Model:  "gpt-5.5",
	})
	result, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err != nil {
		t.Fatalf("AnalyzeVideo returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected OpenAI-compatible endpoint, got %s", gotPath)
	}
	if requestBody["max_tokens"] == nil {
		t.Fatalf("expected max_tokens in OpenAI-compatible request: %+v", requestBody)
	}
	if result.SeedancePrompt == "" {
		t.Fatalf("expected parsed seedance prompt: %+v", result)
	}
}

func TestAnalyzeVideoSendsVideoURLContentForMiniMaxM3(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"scenes":["室外"],"characters":["少年"],"assets":["航拍"],"seedancePrompt":"室外少年奔跑，航拍镜头"}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey:  "test-key",
		GroupID: "voice-group",
		Model:   "MiniMax-M3",
	})
	result, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err != nil {
		t.Fatalf("AnalyzeVideo returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected MiniMax-M3 to use chat completions endpoint, got %s", gotPath)
	}
	if requestBody["model"] != "MiniMax-M3" {
		t.Fatalf("expected MiniMax-M3 model in request, got %+v", requestBody["model"])
	}
	if requestBody["max_completion_tokens"] != float64(1200) {
		t.Fatalf("expected max_completion_tokens for MiniMax-M3 request, got %+v", requestBody)
	}
	if _, ok := requestBody["max_tokens"]; ok {
		t.Fatalf("expected MiniMax-M3 request to avoid deprecated max_tokens, got %+v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("expected MiniMax-M3 thinking to be disabled for JSON analysis, got %+v", requestBody["thinking"])
	}
	messages, ok := requestBody["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("expected two messages, got %+v", requestBody["messages"])
	}
	userMessage, ok := messages[1].(map[string]any)
	if !ok {
		t.Fatalf("expected user message object, got %+v", messages[1])
	}
	content, ok := userMessage["content"].([]any)
	if !ok || len(content) < 2 {
		t.Fatalf("expected multimodal user content, got %+v", userMessage["content"])
	}
	var foundVideoURL bool
	for _, part := range content {
		partMap, ok := part.(map[string]any)
		if !ok || partMap["type"] != "video_url" {
			continue
		}
		videoURL, _ := partMap["video_url"].(map[string]any)
		if videoURL["url"] == "https://example.com/video.mp4" {
			foundVideoURL = true
		}
	}
	if !foundVideoURL {
		body, _ := json.Marshal(requestBody)
		t.Fatalf("expected request to contain video_url content part, got %s", string(body))
	}
	if result.SeedancePrompt == "" {
		t.Fatalf("expected parsed seedance prompt: %+v", result)
	}
}

func TestAnalyzeVideoReturnsHelpfulErrorForNonJSONModelAnswer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": "<html><body>403 Forbidden</body></html>"},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey: "test-key",
		Model:  "MiniMax-M3",
	})
	_, err := generator.AnalyzeVideo(context.Background(), "https://example.com/video.mp4", "demo.mp4")
	if err == nil {
		t.Fatal("expected AnalyzeVideo to reject non-JSON model answer")
	}
	if !strings.Contains(err.Error(), "视频分析模型未返回有效 JSON") {
		t.Fatalf("expected helpful non-JSON error, got %v", err)
	}
	if strings.Contains(err.Error(), "invalid character '<'") {
		t.Fatalf("expected implementation detail to be hidden, got %v", err)
	}
}

func TestParseVideoAnalysisIgnoresMiniMaxThinkingBlock(t *testing.T) {
	result, err := parseVideoAnalysis(`<think>
我需要按 JSON 输出，草稿结构类似 {"scenes": []}。
</think>
` + "```json" + `
{
  "scenes": ["室外街道"],
  "characters": ["行人"],
  "assets": ["手持镜头"],
  "seedancePrompt": "室外街道里行人经过，手持跟拍，真实自然光"
}
` + "```")
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if result.SeedancePrompt == "" || len(result.Scenes) != 1 {
		t.Fatalf("expected parsed JSON after thinking block, got %+v", result)
	}
}

func TestParseVideoAnalysisIncludesSpeechInsights(t *testing.T) {
	result, err := parseVideoAnalysis(`{
  "scenes": ["访谈室"],
  "characters": ["主持人"],
  "assets": ["固定机位"],
  "hasSpeech": true,
  "audioSummary": "视频中主要讨论自我认知和行动计划。",
  "speechTopics": ["自我认知", "行动计划"],
  "speechKeywords": ["目标", "复盘"],
  "speechOutline": ["介绍问题背景", "提出行动建议"],
  "seedancePrompt": "访谈室里主持人讲述自我认知主题，固定机位，柔和光线"
}`)
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if !result.HasSpeech {
		t.Fatal("expected hasSpeech to be true")
	}
	if result.AudioSummary != "视频中主要讨论自我认知和行动计划。" {
		t.Fatalf("unexpected audio summary: %q", result.AudioSummary)
	}
	if len(result.SpeechTopics) != 2 || result.SpeechTopics[0] != "自我认知" {
		t.Fatalf("unexpected speech topics: %+v", result.SpeechTopics)
	}
	if len(result.SpeechKeywords) != 2 || result.SpeechKeywords[1] != "复盘" {
		t.Fatalf("unexpected speech keywords: %+v", result.SpeechKeywords)
	}
	if len(result.SpeechOutline) != 2 || result.SpeechOutline[0] != "介绍问题背景" {
		t.Fatalf("unexpected speech outline: %+v", result.SpeechOutline)
	}
}

func TestParseVideoAnalysisCoercesCommonLLMTypeDrift(t *testing.T) {
	result, err := parseVideoAnalysis(`{
  "scenes": "室内窗边\n书桌特写",
  "characters": "女性主角、旁白",
  "assets": "窗光",
  "hasSpeech": "true",
  "audioSummary": "一段关于自我接纳的旁白。",
  "speechTopics": "自我接纳, 行动计划",
  "speechKeywords": "觉察",
  "speechOutline": "提出困惑\n给出建议",
  "seedancePrompt": "室内窗边女性沉思，柔和窗光，近景推入"
}`)
	if err != nil {
		t.Fatalf("parseVideoAnalysis returned error: %v", err)
	}
	if !result.HasSpeech {
		t.Fatal("expected string true to parse as bool")
	}
	if len(result.Scenes) != 2 || result.Scenes[0] != "室内窗边" {
		t.Fatalf("expected string scenes to become list, got %+v", result.Scenes)
	}
	if len(result.Characters) != 2 || result.Characters[1] != "旁白" {
		t.Fatalf("expected comma-separated characters to become list, got %+v", result.Characters)
	}
	if len(result.SpeechTopics) != 2 || result.SpeechTopics[1] != "行动计划" {
		t.Fatalf("expected string speech topics to become list, got %+v", result.SpeechTopics)
	}
}

func TestParseVideoStoryboardDesignIgnoresThinkingBlock(t *testing.T) {
	result, err := parseVideoStoryboardDesign(`<think>
先根据视频解析和主题规划三段式节奏。
</think>
` + "```json" + `
{
  "title": "九型课程开场分镜",
  "styleGuide": ["温暖自然光", "真实纪实质感"],
  "globalPrompt": "围绕自我认知主题的 Seedance 2.0 短片，节奏舒缓",
  "shots": [
    {
      "index": 1,
      "duration": 3,
      "scene": "清晨教室",
      "characters": ["讲师"],
      "assets": ["白板", "柔和窗光"],
      "action": "讲师看向镜头微笑",
      "camera": "中景缓慢推进",
      "lighting": "暖色自然光",
      "audio": "轻柔环境音乐",
      "seedancePrompt": "清晨教室中讲师看向镜头微笑，中景缓慢推进，暖色自然光，真实纪实质感"
    }
  ]
}
` + "```")
	if err != nil {
		t.Fatalf("parseVideoStoryboardDesign returned error: %v", err)
	}
	if result.Title != "九型课程开场分镜" || len(result.Shots) != 1 {
		t.Fatalf("unexpected storyboard result: %+v", result)
	}
	if result.Shots[0].SeedancePrompt == "" || result.Shots[0].Characters[0] != "讲师" {
		t.Fatalf("expected parsed shot prompt and characters, got %+v", result.Shots[0])
	}
}

func TestParseVideoStoryboardDesignCoercesCommonLLMTypeDrift(t *testing.T) {
	result, err := parseVideoStoryboardDesign(`{
  "title": "疗愈主题分镜",
  "styleGuide": "柔和光\n真实纪实",
  "globalPrompt": "疗愈短片",
  "shots": [
    {
      "index": "1",
      "title": "开场",
      "duration": "3秒",
      "scene": "室内",
      "characters": "女性、书本",
      "assets": "窗光, 绿植",
      "action": "低头翻书",
      "camera": "近景推入",
      "composition": "人物在画面右侧",
      "lighting": "柔和光",
      "audio": "轻音乐",
      "dialogue": false,
      "seedancePrompt": "室内女性低头翻书，近景推入，柔和光"
    },
    {
      "index": 2,
      "duration": "4s",
      "scene": "书桌",
      "characters": ["女性"],
      "assets": ["书本"],
      "action": "抬头微笑",
      "camera": "固定中景",
      "lighting": "自然光",
      "audio": true,
      "dialogue": "今天开始接纳自己",
      "seedancePrompt": "书桌旁女性抬头微笑，固定中景，自然光"
    }
  ]
}`)
	if err != nil {
		t.Fatalf("parseVideoStoryboardDesign returned error: %v", err)
	}
	if len(result.StyleGuide) != 2 || result.StyleGuide[1] != "真实纪实" {
		t.Fatalf("expected string styleGuide to become list, got %+v", result.StyleGuide)
	}
	if len(result.Shots) != 2 {
		t.Fatalf("expected two shots, got %+v", result.Shots)
	}
	if result.Shots[0].Duration != 3 || result.Shots[1].Duration != 4 {
		t.Fatalf("expected string durations to parse, got %+v and %+v", result.Shots[0].Duration, result.Shots[1].Duration)
	}
	if len(result.Shots[0].Characters) != 2 || result.Shots[0].Characters[1] != "书本" {
		t.Fatalf("expected string characters to become list, got %+v", result.Shots[0].Characters)
	}
	if result.Shots[0].Dialogue != "false" || result.Shots[1].Audio != "true" {
		t.Fatalf("expected bool string fields to be preserved textually, got shot1 dialogue=%q shot2 audio=%q", result.Shots[0].Dialogue, result.Shots[1].Audio)
	}
}

func TestGenerateVideoStoryboardEmptyShotsErrorIncludesModelPreview(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"title":"空分镜","styleGuide":["柔和光"],"globalPrompt":"测试短片","shots":[]}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey: "test-key",
		Model:  "MiniMax-M3",
	})
	_, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{
		Theme: "测试主题",
	})
	if err == nil {
		t.Fatal("expected empty shots to return an error")
	}
	message := err.Error()
	if !strings.Contains(message, "分镜设计模型未返回分镜明细") || !strings.Contains(message, "返回片段") || !strings.Contains(message, "空分镜") {
		t.Fatalf("expected error to include model preview for diagnosis, got %q", message)
	}
}

func TestGenerateVideoStoryboardUsesOpenAICompatibleJSONRequest(t *testing.T) {
	var gotPath string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"title":"疗愈主题分镜","styleGuide":["柔和光"],"globalPrompt":"疗愈短片","shots":[{"index":1,"duration":4,"scene":"室内","characters":["女性"],"assets":["窗光"],"action":"低头翻书","camera":"近景推入","lighting":"柔和光","audio":"轻音乐","seedancePrompt":"室内女性低头翻书，近景推入，柔和光"}]}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{
		APIKey: "test-key",
		Model:  "MiniMax-M3",
	})
	result, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{
		Assets:         []string{"窗光"},
		Characters:     []string{"女性"},
		Scenes:         []string{"室内"},
		SeedancePrompt: "室内女性近景，柔和光线",
		Theme:          "疗愈感品牌宣传",
		VideoName:      "demo.mp4",
	})
	if err != nil {
		t.Fatalf("GenerateVideoStoryboard returned error: %v", err)
	}
	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected OpenAI-compatible endpoint, got %s", gotPath)
	}
	if requestBody["max_completion_tokens"] != float64(1800) {
		t.Fatalf("expected max_completion_tokens for storyboard request, got %+v", requestBody)
	}
	thinking, ok := requestBody["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("expected thinking disabled, got %+v", requestBody["thinking"])
	}
	if result.Title == "" || len(result.Shots) != 1 {
		t.Fatalf("expected parsed storyboard result, got %+v", result)
	}
}

func TestGenerateVideoStoryboardEmptyShotsErrorExposesRawResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{
				map[string]any{
					"message": map[string]any{"content": `{"title":"空分镜","styleGuide":["柔和光"],"globalPrompt":"测试短片","shots":[]}`},
				},
			},
		})
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key", Model: "MiniMax-M3"})
	_, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{Theme: "测试主题"})
	if err == nil {
		t.Fatal("expected empty shots to return an error")
	}
	raw := StoryboardRawResultFromError(err)
	if !strings.Contains(raw, `"title":"空分镜"`) || !strings.Contains(raw, `"shots":[]`) {
		t.Fatalf("expected raw model answer from error, got %q", raw)
	}
}

func TestGenerateVideoStoryboardHTTPErrorExposesRawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"upstream overloaded","request_id":"story-500"}`))
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key", Model: "MiniMax-M3"})
	_, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{Theme: "测试主题"})
	if err == nil {
		t.Fatal("expected HTTP error")
	}
	if raw := StoryboardRawResultFromError(err); !strings.Contains(raw, "story-500") {
		t.Fatalf("expected raw HTTP response from error, got %q", raw)
	}
}

func TestGenerateVideoStoryboardInvalidJSONExposesRawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json-response`))
	}))
	defer server.Close()

	generator := newLocalMiniMaxGenerator(server, config.MiniMaxConfig{APIKey: "test-key", Model: "MiniMax-M3"})
	_, err := generator.GenerateVideoStoryboard(context.Background(), VideoStoryboardInput{Theme: "测试主题"})
	if err == nil {
		t.Fatal("expected JSON parse error")
	}
	if raw := StoryboardRawResultFromError(err); raw != "not-json-response" {
		t.Fatalf("expected raw invalid JSON response from error, got %q", raw)
	}
}
