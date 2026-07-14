# Chat Preferences and Production SSE Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make user corrections take effect immediately and persist across conversations, default ordinary replies to 1–3 concise sentences, and prove that production delivers real SSE deltas before the model completes.

**Architecture:** Add a user-scoped communication-preference store plus a deterministic extractor for common correction instructions. Merge stored preferences with the current message before generation, with the current message taking precedence, then persist reusable corrections after a successful answer. Keep the existing MiniMax native streaming adapter for this release, add an initial SSE flush/heartbeat and byte-level App tests, then deploy the server and chat proxy together and verify first-delta timing through the production domain. The broader OpenAI/Anthropic chat-provider migration in the approved design is deliberately deferred to a separate plan so it cannot block this user-visible fix.

**Tech Stack:** Go, PostgreSQL, `net/http` SSE, existing RAG/MiniMax generator, Flutter/Dart Dio streaming, Nginx, Docker Compose, Go/Flutter tests.

**Design:** `docs/superpowers/specs/2026-07-14-compatible-chat-streaming-preferences-design.md`

---

## Chunk 1: Immediate obedience, concise defaults, and persistent preferences

### Task 1: Lock the stricter reply contract

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`

- [ ] **Step 1: Write failing prompt and token-budget tests**

Add tests requiring the default prompt to contain all of these rules:

```go
for _, want := range []string{
    "普通问题通常只回答 1-3 句",
    "只有用户明确要求展开",
    "不主动使用亲爱的、宝贝等亲昵称呼",
    "用户要求纠正时立即按新要求重答",
    "不要解释为什么要纠正",
} {
    if !strings.Contains(prompt, want) {
        t.Fatalf("default prompt missing %q: %s", want, prompt)
    }
}
```

Add table tests for a new `chatTokenBudget(question string)`:

```go
tests := []struct {
    question string
    want     int
}{
    {"不要叫我亲爱的", 220},
    {"简单说重点", 220},
    {"请详细展开分析原因和步骤", 420},
}
```

Assert both `Generate` and `GenerateStream` use that function rather than a fixed `360`.

- [ ] **Step 2: Run the focused tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/llm -run 'Test(DefaultSystemPrompt|ChatTokenBudget|MiniMaxGenerator.*Token)' -count=1
```

Expected: FAIL because the prompt still allows 2–4 sentences, does not ban unsolicited intimate names, and both paths use a fixed 360-token budget.

- [ ] **Step 3: Implement the minimal prompt and adaptive budget**

Update `defaultSystemPrompt` so that:

- ordinary confirmations, corrections, and simple questions are 1–3 sentences;
- only explicit requests such as “详细、展开、完整分析、逐步说明” permit a longer answer;
- the assistant never initiates intimate labels such as “亲爱的、宝贝”; 
- a correction request is followed directly, without restating or defending the previous answer;
- conclusions come first and unnecessary Enneagram background is omitted.

Add:

```go
func chatTokenBudget(question string) int {
    q := strings.TrimSpace(question)
    for _, marker := range []string{"详细", "展开", "完整分析", "深入分析", "逐步说明"} {
        if strings.Contains(q, marker) {
            return 420
        }
    }
    return 220
}
```

Use the budget in both synchronous and streaming request bodies.

- [ ] **Step 4: Run the package tests and verify GREEN**

```bash
go test ./internal/llm -count=1
```

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/llm/minimax.go nx-backend/apps/server/internal/llm/minimax_test.go
git commit -m "fix(chat): enforce concise correction-aware replies"
```

### Task 2: Add the user communication-preference store

**Files:**
- Create: `nx-backend/apps/server/internal/chatpreference/preference.go`
- Create: `nx-backend/apps/server/internal/chatpreference/preference_test.go`
- Create: `nx-backend/apps/server/internal/chatpreference/store.go`
- Create: `nx-backend/apps/server/internal/chatpreference/store_test.go`
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_chat_preference_test.go`

- [ ] **Step 1: Write failing deterministic extraction tests**

Cover at least:

```go
{"不要叫我亲爱的", Preference{Category: "addressing", Key: "forbidden-name:亲爱的", Instruction: "不要称呼用户为亲爱的"}},
{"以后回答短一点，只说重点", Preference{Category: "length", Key: "default-length", Instruction: "默认简短回答，先说重点"}},
{"语气直接一点，不要说教", Preference{Category: "tone", Key: "default-tone", Instruction: "语气直接，避免说教"}},
{"先给结论，不要列表", two preferences},
{"这次只回答一句", no persistent preference},
{"不用再简短了", cancellation of default-length},
```

The extractor must retain `sourceText` for audit/export but must not treat arbitrary facts as communication preferences.

- [ ] **Step 2: Run extractor tests and verify RED**

```bash
go test ./internal/chatpreference -run TestExtract -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 3: Implement focused deterministic rules**

Define:

```go
type Preference struct {
    Category    string
    Key         string
    Instruction string
    SourceText  string
    Delete      bool
}

func Extract(text string) []Preference
func EffectiveInstructions(saved []Preference, currentText string) []string
```

Keep rules limited to addressing, length, tone, format, and interaction. Current-message extracted rules override saved rules with the same key.

- [ ] **Step 4: Write failing store and schema tests**

Require this schema:

```sql
CREATE TABLE IF NOT EXISTS app_chat_preferences (
  id bigserial PRIMARY KEY,
  app_user_id bigint NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  category varchar(32) NOT NULL,
  normalized_key varchar(96) NOT NULL,
  instruction text NOT NULL,
  source_text text NOT NULL DEFAULT '',
  create_time timestamptz NOT NULL DEFAULT now(),
  update_time timestamptz NOT NULL DEFAULT now(),
  UNIQUE (app_user_id, category, normalized_key)
);
```

Store tests must prove user isolation, upsert replacement, cancellation deletion, deterministic ordering, and context cancellation.

- [ ] **Step 5: Implement store and schema**

Expose:

```go
func NewStore(db *sql.DB) *Store
func (s *Store) List(ctx context.Context, userID int64) ([]Preference, error)
func (s *Store) Apply(ctx context.Context, userID int64, changes []Preference) error
func (s *Store) DeleteAll(ctx context.Context, userID int64) (int64, error)
```

Use one transaction for a set of changes and parameterized SQL only.

- [ ] **Step 6: Run package and schema tests**

```bash
go test ./internal/chatpreference ./internal/db -run 'Test(Extract|Store|Schema.*ChatPreference)' -count=1
```

- [ ] **Step 7: Commit**

```bash
git add nx-backend/apps/server/internal/chatpreference nx-backend/apps/server/internal/db/schema.sql nx-backend/apps/server/internal/db/schema_chat_preference_test.go
git commit -m "feat(chat): persist user communication preferences"
```

### Task 3: Apply current corrections immediately and persist reusable ones

**Files:**
- Modify: `nx-backend/apps/server/internal/rag/rag.go`
- Modify: `nx-backend/apps/server/internal/rag/rag_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`

- [ ] **Step 1: Write failing prompt-precedence tests**

Extend `rag.UserProfile` with:

```go
Preferences []string `json:"preferences,omitempty"`
```

Require `buildUserPrompt` order:

```text
默认规则
已保存的交流偏好
会话前情/近期历史
当前用户消息
当前消息优先规则
```

Tests must assert that a saved “可称呼为亲爱的” plus current “不要叫我亲爱的” yields an effective prompt containing only the prohibition.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/rag ./internal/llm -run 'Test.*Preference|TestBuildUserPrompt.*Priority' -count=1
```

- [ ] **Step 3: Inject effective preferences before generation**

Add a `chatPreferences *chatpreference.Store` field to `Server`, initialize it with the existing DB, and in both `/ask` and `/ask/stream`:

1. list saved preferences;
2. merge them with `chatpreference.Extract(body.Question)`;
3. pass effective instructions through `rag.UserProfile.Preferences`;
4. generate the reply;
5. only after `SavePair` succeeds, persist reusable extracted changes.

Do not wait for a model-based preference extractor in this iteration. Deterministic extraction must be sufficient for the reported correction cases.

- [ ] **Step 4: Write handler tests for immediate and cross-session behavior**

Use two sessions for the same user and a capturing generator. Prove:

- “不要叫我亲爱的” is present in the first request prompt immediately;
- after the first complete/save succeeds, a second session also receives that preference;
- another user does not receive it;
- “这次只回答一句” affects the current prompt but is not stored;
- partial stream failure and save failure do not claim a reusable preference was saved.

- [ ] **Step 5: Implement and run server tests**

```bash
go test ./internal/server -run 'TestAppChat.*Preference|TestAppChat.*Stream' -count=1
```

- [ ] **Step 6: Commit**

```bash
git add nx-backend/apps/server/internal/rag nx-backend/apps/server/internal/llm nx-backend/apps/server/internal/server
git commit -m "feat(chat): obey and remember user corrections"
```

### Task 4: Include preferences in privacy export and deletion

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_privacy.go`
- Modify: `nx-backend/apps/server/internal/server/app_privacy_test.go`

- [ ] **Step 1: Write failing privacy tests**

Require export JSON to contain a `preferences` array without other users’ rows. Require “清空记忆” and account deletion to remove `app_chat_preferences` for the current user only.

- [ ] **Step 2: Run and verify RED**

```bash
go test ./internal/server -run 'TestAppPrivacy.*Preference' -count=1
```

- [ ] **Step 3: Implement export/delete integration**

Add a privacy DTO with category, normalized key, instruction, source text, and timestamps. Delete preferences in the existing transaction/order before disabling the account.

- [ ] **Step 4: Run and verify GREEN**

```bash
go test ./internal/server -run 'TestAppPrivacy' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/app_privacy.go nx-backend/apps/server/internal/server/app_privacy_test.go
git commit -m "feat(privacy): include chat preferences in user controls"
```

## Chunk 2: End-to-end real SSE delivery

### Task 5: Flush immediately and keep the SSE connection alive

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`

- [ ] **Step 1: Write a failing initial-flush/heartbeat timing test**

Use `httptest.Server` with a generator that blocks before its first delta. The client must receive response headers plus an SSE comment before the generator is released:

```text
: ping

```

Then release delta one, hold delta two, and assert delta one reaches the client before completion.

- [ ] **Step 2: Run and verify RED**

```bash
go test ./internal/server -run 'TestAppChatStream(InitialFlush|Heartbeat|FirstDelta)' -count=1
```

- [ ] **Step 3: Implement minimal heartbeat handling**

After setting SSE headers, write and flush an initial comment. While waiting for the generator, send a comment every 12 seconds. Stop the ticker on completion/cancellation; a heartbeat write failure must cancel generation and prevent persistence.

- [ ] **Step 4: Run stream tests repeatedly**

```bash
go test ./internal/server -run 'TestAppChat.*Stream|TestWriteAppChatSSE' -count=10
go test -race ./internal/server -run 'TestAppChat.*Stream' -count=1
```

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/app_chat.go nx-backend/apps/server/internal/server/app_chat_stream_test.go
git commit -m "fix(chat): flush and keep SSE streams alive"
```

### Task 6: Prove Flutter consumes real HTTP byte chunks immediately

**Repository:** `/Users/wohenzaiyi/Desktop/nine-xing-app`

**Files:**
- Modify: `lib/core/network/api_client.dart`
- Modify: `lib/features/chat/chat_repository.dart`
- Modify: `test/core/network/api_client_test.dart`
- Modify: `test/features/chat/chat_models_test.dart`
- Modify: `test/features/chat/chat_notifier_analytics_test.dart`

- [ ] **Step 1: Write a failing real delayed-HTTP test**

Start a local `HttpServer` that:

1. writes headers and `: ping\n\n`;
2. writes one UTF-8 `delta` event and flushes;
3. blocks until the test releases the second event;
4. writes delta two and done.

Assert `ChatRepository.askStream(...).first` completes before step 3 is released.

- [ ] **Step 2: Add timeout and cancellation tests**

Require `postStream` to use no ordinary response-body `receiveTimeout`, to decode bytes with a streaming UTF-8 decoder, and to cancel the Dio request when the Dart subscription is cancelled.

- [ ] **Step 3: Run and verify RED**

```bash
/opt/homebrew/share/flutter/bin/flutter test \
  test/core/network/api_client_test.dart \
  test/features/chat/chat_models_test.dart \
  --plain-name 'stream exposes first SSE delta before response completion'
```

- [ ] **Step 4: Implement minimal byte-stream corrections**

Use `ResponseType.stream`, `receiveTimeout: Duration.zero` (or Dio’s supported no-timeout representation), incremental UTF-8 decoding, and cancellation propagation. Keep repository framing compatible with comments, LF/CRLF, split delimiters, and multiple events per chunk.

- [ ] **Step 5: Run chat tests and verify GREEN**

```bash
/opt/homebrew/share/flutter/bin/flutter test \
  test/core/network/api_client_test.dart \
  test/features/chat/chat_models_test.dart \
  test/features/chat/chat_notifier_analytics_test.dart \
  test/features/chat/chat_screen_interaction_test.dart
```

- [ ] **Step 6: Commit App change**

```bash
git add lib/core/network/api_client.dart lib/features/chat/chat_repository.dart test/core/network/api_client_test.dart test/features/chat/chat_models_test.dart test/features/chat/chat_notifier_analytics_test.dart
git commit -m "fix(chat): consume SSE bytes as they arrive"
```

## Chunk 3: Verification, deployment, and production proof

### Task 7: Run full verification in both repositories

**Files:** No production edits expected unless verification reveals a directly related defect.

- [ ] **Step 1: Verify backend**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing/nx-backend/apps/server
go test ./... -count=1
go test -race ./internal/chatpreference ./internal/llm ./internal/rag ./internal/server -count=1
go vet ./...
```

- [ ] **Step 2: Verify App**

```bash
cd /Users/wohenzaiyi/Desktop/nine-xing-app
/opt/homebrew/share/flutter/bin/dart format --set-exit-if-changed .
/opt/homebrew/share/flutter/bin/flutter analyze
/opt/homebrew/share/flutter/bin/flutter test --exclude-tags=golden
/opt/homebrew/share/flutter/bin/flutter test \
  test/features/home/onboarding_view_golden_test.dart \
  test/features/home/star_seal_golden_test.dart
```

- [ ] **Step 3: Review diffs and preserve unrelated user files**

Confirm the four existing untracked App design/plan documents remain uncommitted and untouched.

### Task 8: Deploy server and chat proxy without touching database volumes

**Repository:** `/Users/wohenzaiyi/Desktop/nine-xing`

- [ ] **Step 1: Push the verified backend branch**

```bash
git push origin detail-tuning-video-management
```

- [ ] **Step 2: Establish authorized production access**

The current production domain resolves to `156.239.236.30`. Use an authorized SSH user/key or the cloud console. Do not guess passwords and do not move or delete database volumes.

- [ ] **Step 3: Inspect before mutation**

On the server, confirm repository path, branch/commit, `docker compose ps`, `.env` presence, database health, and whether `MINIMAX_SYSTEM_PROMPT` or stored `assist.systemPrompt` is non-empty. Report only presence/absence, never contents.

- [ ] **Step 4: Update code and rebuild only required services**

```bash
git fetch origin
git switch detail-tuning-video-management
git pull --ff-only origin detail-tuning-video-management
docker compose up -d --build server website
```

Do not run `docker compose down -v`, remove volumes, or recreate the database service unnecessarily.

- [ ] **Step 5: Verify container and proxy configuration**

```bash
docker compose ps
docker compose logs --tail=200 server
docker compose exec website nginx -t
curl -fsS http://127.0.0.1:8000/api/status
```

### Task 9: Prove production behavior with real authenticated sessions

- [ ] **Step 1: Verify immediate correction and cross-session memory**

Using a test App account:

1. session A: send `不要叫我亲爱的，以后回答短一点，只说重点`;
2. assert the immediate response does not contain `亲爱的` and is at most 3 sentences unless it must report an error;
3. session B: ask a simple question;
4. assert the response remains concise and does not use the forbidden address;
5. send `不用再简短了，请详细展开` and verify the preference changes.

- [ ] **Step 2: Verify real streaming timing through production**

Use an authenticated streaming request and record timestamps for headers, first `delta`, and `done`. Acceptance criteria:

```text
headers_at < first_delta_at < done_at
```

The first delta must be observable while the connection remains open, not reconstructed from a complete answer.

- [ ] **Step 3: Decide whether another APK is required**

If Task 6 changes App code, build the next production version through the protected GitHub Actions workflow, download it under `dist/production-v<version>`, and independently verify signature, package name, version, production API, JPush AppKey presence, channel, ZIP, and SHA-256. If Task 6 is already green without production edits, deploy backend only and do not create an unnecessary APK.

