package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"nine-xing/nx-backend/apps/server/internal/chat"
	"nine-xing/nx-backend/apps/server/internal/quiz"
	"nine-xing/nx-backend/apps/server/internal/rag"
)

func TestBuildAppChatConversationCardPreservesSecondaryProfile(t *testing.T) {
	got := buildAppChatConversationCard(quiz.Card{
		CardType: "secondary",
		Name:     "妈妈",
		Relation: "家人",
		MainType: 2,
		WingType: 1,
		Profile:  []byte(`{"primaryMotivation":"希望被需要"}`),
	})

	if got.CardType != "secondary" || got.Name != "妈妈" || got.Relation != "家人" || got.MainType != 2 || got.WingType != 1 {
		t.Fatalf("unexpected conversation card: %+v", got)
	}
	if !strings.Contains(got.Profile, "希望被需要") {
		t.Fatalf("expected bounded profile JSON, got %q", got.Profile)
	}
}

func TestAppChatHistoryFromMessagesFiltersInvalidEntries(t *testing.T) {
	history := appChatHistoryFromMessages([]chat.Message{
		{Role: "user", Content: "  我女儿今年八岁  "},
		{Role: "user", MessageType: "voice", Transcript: "  她最近不愿意沟通  "},
		{Role: "tool", Content: "不应注入"},
		{Role: "assistant", Content: "  我记住了  "},
		{Role: "user", Content: "   "},
	})

	if len(history) != 3 {
		t.Fatalf("expected 3 valid history messages, got %+v", history)
	}
	if history[0].Role != "user" || history[0].Content != "我女儿今年八岁" {
		t.Fatalf("unexpected first history message: %+v", history[0])
	}
	if history[1].Role != "user" || history[1].Content != "她最近不愿意沟通" {
		t.Fatalf("unexpected second history message: %+v", history[1])
	}
	if history[2].Role != "assistant" || history[2].Content != "我记住了" {
		t.Fatalf("unexpected third history message: %+v", history[2])
	}
}

func TestCompactAppChatContextSummarizesOldMessagesAndKeepsRecentTwelve(t *testing.T) {
	messages := make([]chat.Message, 0, 25)
	for i := 1; i <= 25; i++ {
		role := "user"
		if i%2 == 0 {
			role = "assistant"
		}
		messages = append(messages, chat.Message{ID: int64(i), Role: role, Content: fmt.Sprintf("消息%d", i)})
	}
	summarizer := &fakeConversationSummarizer{summary: "早期对话摘要"}

	context := compactAppChatContext(context.Background(), "旧摘要", messages, summarizer)

	if context.Summary != "早期对话摘要" {
		t.Fatalf("unexpected summary: %q", context.Summary)
	}
	if context.SummaryThroughMessageID != 13 {
		t.Fatalf("expected summary through message 13, got %d", context.SummaryThroughMessageID)
	}
	if len(context.History) != 12 || context.History[0].Content != "消息14" || context.History[11].Content != "消息25" {
		t.Fatalf("expected the latest 12 messages verbatim, got %+v", context.History)
	}
	if summarizer.previous != "旧摘要" || len(summarizer.messages) != 13 {
		t.Fatalf("summarizer did not receive prior summary and old messages: %+v", summarizer)
	}
}

func TestCompactAppChatContextAdvancesWatermarkByValidMessages(t *testing.T) {
	messages := make([]chat.Message, 0, 26)
	for i := 1; i <= 25; i++ {
		messages = append(messages, chat.Message{ID: int64(i), Role: "user", Content: fmt.Sprintf("消息%d", i)})
	}
	messages = append(messages, chat.Message{ID: 26, Role: "tool", Content: "忽略"})

	context := compactAppChatContext(context.Background(), "", messages, &fakeConversationSummarizer{summary: "摘要"})

	if context.SummaryThroughMessageID != 13 {
		t.Fatalf("expected watermark at the 13th valid message id 13, got %d", context.SummaryThroughMessageID)
	}
	if context.History[0].Content != "消息14" {
		t.Fatalf("expected recent history to start after summarized valid messages, got %+v", context.History)
	}
}

func TestCompactAppChatContextSummarizesAllOldMessagesOnce(t *testing.T) {
	messages := make([]chat.Message, 0, 100)
	for i := 1; i <= 100; i++ {
		messages = append(messages, chat.Message{ID: int64(i), Role: "user", Content: fmt.Sprintf("消息%d", i)})
	}
	summarizer := &fakeConversationSummarizer{summary: "滚动摘要"}

	context := compactAppChatContext(context.Background(), "", messages, summarizer)

	if summarizer.calls != 1 || len(summarizer.messages) != 88 {
		t.Fatalf("expected one summary call with all 88 old messages, got calls=%d messages=%d", summarizer.calls, len(summarizer.messages))
	}
	if context.SummaryThroughMessageID != 88 || len(context.History) != 12 || context.History[0].Content != "消息89" {
		t.Fatalf("unexpected compacted context: %+v", context)
	}
}

func TestBuildAppChatPromptContextKeepsCurrentContextWhenSummaryCASIsRejected(t *testing.T) {
	messages := make([]chat.Message, 0, 25)
	for i := 9; i <= 33; i++ {
		messages = append(messages, chat.Message{ID: int64(i), Role: "user", Content: fmt.Sprintf("消息%d", i)})
	}
	store := &fakeAppChatContextStore{
		state:        chat.ConversationState{Summary: "旧摘要", SummaryThroughMessageID: 8},
		messages:     messages,
		rejectUpdate: true,
	}
	summarizer := &fakeConversationSummarizer{summary: "仅供当前请求的新摘要"}

	got := buildAppChatPromptContext(context.Background(), 42, store, summarizer)

	if got.Summary != "仅供当前请求的新摘要" || got.SummaryThroughMessageID != 21 {
		t.Fatalf("expected current request to use compacted context, got %+v", got)
	}
	if len(got.History) != 12 || got.History[0].Content != "消息22" {
		t.Fatalf("expected current request to retain the latest 12 messages, got %+v", got.History)
	}
	if store.state.Summary != "旧摘要" || store.state.SummaryThroughMessageID != 8 {
		t.Fatalf("rejected CAS must not overwrite persisted state, got %+v", store.state)
	}
}

func TestCompactAppChatContextFallsBackToRecentTwentyWhenSummaryFails(t *testing.T) {
	messages := make([]chat.Message, 0, 25)
	for i := 1; i <= 25; i++ {
		messages = append(messages, chat.Message{ID: int64(i), Role: "user", Content: fmt.Sprintf("消息%d", i)})
	}
	summarizer := &fakeConversationSummarizer{err: errors.New("summary unavailable")}

	context := compactAppChatContext(context.Background(), "原摘要", messages, summarizer)

	if context.Summary != "原摘要" || len(context.History) != 20 || context.History[0].Content != "消息6" {
		t.Fatalf("expected original summary plus recent 20 messages, got %+v", context)
	}
	if context.ShouldPersistUpdatedSummary {
		t.Fatal("failed summary must not advance the persisted watermark")
	}
}

func TestBuildAppChatPromptContextPersistsCompactedSummary(t *testing.T) {
	messages := make([]chat.Message, 0, 25)
	for i := 9; i <= 33; i++ {
		messages = append(messages, chat.Message{ID: int64(i), Role: "user", Content: fmt.Sprintf("消息%d", i)})
	}
	store := &fakeAppChatContextStore{
		state:    chat.ConversationState{Summary: "旧摘要", SummaryThroughMessageID: 8},
		messages: messages,
	}
	summarizer := &fakeConversationSummarizer{summary: "更新后的摘要"}

	got := buildAppChatPromptContext(context.Background(), 42, store, summarizer)

	if got.Summary != "更新后的摘要" || len(got.History) != 12 || got.History[0].Content != "消息22" {
		t.Fatalf("unexpected prompt context: %+v", got)
	}
	if store.updatedSessionID != 42 || store.expectedThrough != 8 || store.newThrough != 21 || store.updatedSummary != "更新后的摘要" {
		t.Fatalf("summary was not conditionally persisted: %+v", store)
	}
}

type fakeAppChatContextStore struct {
	state            chat.ConversationState
	messages         []chat.Message
	updatedSessionID int64
	expectedThrough  int64
	newThrough       int64
	updatedSummary   string
	rejectUpdate     bool
}

func (f *fakeAppChatContextStore) GetConversationState(context.Context, int64) (chat.ConversationState, error) {
	return f.state, nil
}

func (f *fakeAppChatContextStore) ListMessagesAfter(_ context.Context, _ int64, afterMessageID int64) ([]chat.Message, error) {
	messages := make([]chat.Message, 0, len(f.messages))
	for _, message := range f.messages {
		if message.ID > afterMessageID {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (f *fakeAppChatContextStore) ListRecentMessages(context.Context, int64, int) ([]chat.Message, error) {
	return f.messages, nil
}

func (f *fakeAppChatContextStore) UpdateConversationSummary(_ context.Context, sessionID, expectedThrough int64, summary string, newThrough int64) (bool, error) {
	f.updatedSessionID = sessionID
	f.expectedThrough = expectedThrough
	f.updatedSummary = summary
	f.newThrough = newThrough
	if f.rejectUpdate {
		return false, nil
	}
	f.state.Summary = summary
	f.state.SummaryThroughMessageID = newThrough
	return true, nil
}

type fakeConversationSummarizer struct {
	summary  string
	previous string
	messages []rag.Message
	calls    int
	err      error
}

func (f *fakeConversationSummarizer) SummarizeConversation(_ context.Context, previous string, messages []rag.Message) (string, error) {
	f.calls++
	f.previous = previous
	f.messages = append([]rag.Message(nil), messages...)
	if f.err != nil {
		return "", f.err
	}
	return f.summary, nil
}

func TestChatMemoryContentOnlyKeepsExplicitStableFacts(t *testing.T) {
	tests := []struct {
		name     string
		question string
		want     string
	}{
		{name: "explicit remember", question: "请记住，我女儿今年八岁", want: "请记住，我女儿今年八岁"},
		{name: "personal identity", question: "我是小学老师", want: "我是小学老师"},
		{name: "family fact", question: "我的儿子是4号性格", want: "我的儿子是4号性格"},
		{name: "ordinary question", question: "1到9号孩子应该怎么沟通？", want: ""},
		{name: "temporary feeling", question: "我今天心情很不好", want: ""},
		{name: "question about identity", question: "我是几号人格？", want: ""},
		{name: "privacy opt out", question: "不要记住这件事", want: ""},
		{name: "forget request", question: "请忘掉我刚才说的", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chatMemoryContent(tt.question); got != tt.want {
				t.Fatalf("chatMemoryContent(%q) = %q, want %q", tt.question, got, tt.want)
			}
		})
	}
}

func TestAppChatMemoriesForPromptLoadsRecentActiveMemoriesScopedToCard(t *testing.T) {
	registerAppChatMemoryTestDriver()
	database, err := sql.Open(appChatMemoryTestDriverName, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	s := &Server{db: database}
	memories, err := s.appChatMemoriesForPrompt(context.Background(), 7, 11, 6)
	if err != nil {
		t.Fatalf("appChatMemoriesForPrompt returned error: %v", err)
	}

	want := []string{"用户曾问：如何处理职场压力？", "用户曾问：如何改善亲密关系？"}
	if strings.Join(memories, "\n") != strings.Join(want, "\n") {
		t.Fatalf("unexpected memories: %#v", memories)
	}
}

const appChatMemoryTestDriverName = "app_chat_memory_test"

var registerAppChatMemoryTestDriverOnce sync.Once

func registerAppChatMemoryTestDriver() {
	registerAppChatMemoryTestDriverOnce.Do(func() {
		sql.Register(appChatMemoryTestDriverName, appChatMemoryTestDriver{})
	})
}

type appChatMemoryTestDriver struct{}

func (appChatMemoryTestDriver) Open(string) (driver.Conn, error) {
	return appChatMemoryTestConn{}, nil
}

type appChatMemoryTestConn struct{}

func (appChatMemoryTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (appChatMemoryTestConn) Close() error                        { return nil }
func (appChatMemoryTestConn) Begin() (driver.Tx, error)           { return appQuizTestTx{}, nil }

func (appChatMemoryTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if !strings.Contains(query, "FROM app_memories") ||
		!strings.Contains(query, "app_user_id = $1") ||
		!strings.Contains(query, "card_id = $2") ||
		!strings.Contains(query, "status = 'active'") ||
		!strings.Contains(query, "ORDER BY update_time DESC, id DESC") ||
		!strings.Contains(query, "LIMIT $3") {
		return nil, errors.New("memory query is not scoped to active user/card memories")
	}
	if len(args) != 3 ||
		asInt64(args[0].Value) != 7 ||
		asInt64(args[1].Value) != 11 ||
		asInt64(args[2].Value) != 6 {
		return nil, errors.New("unexpected memory query arguments")
	}
	return &appChatMemoryRows{
		values: [][]driver.Value{
			{"用户曾问：如何处理职场压力？"},
			{"  "},
			{"用户曾问：如何改善亲密关系？"},
		},
	}, nil
}

func asInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	default:
		return 0
	}
}

type appChatMemoryRows struct {
	values [][]driver.Value
	index  int
}

func (r *appChatMemoryRows) Columns() []string {
	return []string{"content"}
}

func (r *appChatMemoryRows) Close() error {
	return nil
}

func (r *appChatMemoryRows) Next(dest []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(dest, r.values[r.index])
	r.index++
	return nil
}
