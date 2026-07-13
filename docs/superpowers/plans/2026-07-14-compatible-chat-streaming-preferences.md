# Compatible Chat Streaming and User Preferences Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the MiniMax-specific chat path with native OpenAI-compatible and Anthropic-compatible adapters, make Flutter SSE visibly incremental, and persist user-wide communication preferences with concise human-like defaults.

**Architecture:** The backend will expose one provider-neutral streaming generator interface backed by two native protocol adapters and a single factory used by startup, save, runtime reload, and connection testing. A dedicated user-preference store and extractor will inject global preferences into every chat while current-message instructions retain highest non-safety priority. The App keeps the normalized `delta/done/error` contract and proves byte-level incremental delivery with a delayed local HTTP server.

**Tech Stack:** Go `net/http`, PostgreSQL, SSE, Vue 3/TypeScript, Vitest, Flutter/Dart, Dio, `dart:io` local test servers.

**Design:** `docs/superpowers/specs/2026-07-14-compatible-chat-streaming-preferences-design.md`

---

## Chunk 1: Chat configuration and provider abstraction

### Task 1: Replace MiniMax chat configuration with an explicit compatible-provider contract

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config.go`
- Modify: `nx-backend/apps/server/internal/modelconfig/model_config_test.go`
- Create: `nx-backend/apps/server/internal/server/model_config_security_test.go`
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`

- [ ] **Step 1: Write failing configuration tests**

Add tests asserting that the public/saved chat configuration contains `provider`, `apiBase`, `apiKey`, `model`, and `timeoutSeconds`, but no `groupId`. Cover trimming, only accepting `openai-compatible` and `anthropic-compatible`, preserving a blank provider for legacy JSON, and rejecting the value `minimax`.

Add an application test using a legacy JSON object such as:

```go
raw := `{"chat":{"apiBase":"https://api.minimax.chat/v1","apiKey":"old","groupId":"legacy","model":"MiniMax-M2.7"}}`
```

Assert that loading it yields an unconfigured chat provider instead of inferring OpenAI or MiniMax.

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/modelconfig ./internal/server -run 'Test.*Chat.*Config|Test.*ModelConfig.*Chat' -count=1
```

Expected: FAIL because `ChatConfig` has no provider/timeout and still exposes `groupId`/`ApplyChat(config.MiniMaxConfig)`.

- [ ] **Step 3: Implement the minimal compatible chat config**

Introduce a focused type:

```go
type ChatConfig struct {
    Provider       string `json:"provider"`
    APIBase        string `json:"apiBase"`
    APIKey         string `json:"apiKey"`
    Model          string `json:"model"`
    TimeoutSeconds int    `json:"timeoutSeconds"`
}

func (c ChatConfig) Normalized() ChatConfig
func (c ChatConfig) Validate() error
func (c Config) EffectiveChat() ChatConfig
```

`EffectiveChat` may merge the existing `Assist.SystemPrompt` later at generator construction time, but it must not inherit `env.MiniMax` credentials. A blank or unknown provider remains invalid/unconfigured.

To keep this intermediate commit compiling, retain `ApplyChat` and any MiniMax-only struct member only as deprecated internal compatibility shims with `json:"-"`; legacy JSON may be detected through custom unmarshal/raw inspection but must not be re-emitted. Remove those shims only in Task 6 after both new adapters and the runtime factory exist.

- [ ] **Step 4: Run model configuration tests and verify GREEN**

```bash
go test ./internal/modelconfig ./internal/server -run 'Test.*Chat.*Config|Test.*ModelConfig.*Chat' -count=1
```

- [ ] **Step 5: Commit the configuration contract**

```bash
git add nx-backend/apps/server/internal/modelconfig nx-backend/apps/server/internal/server/model_config_security_test.go nx-backend/apps/server/internal/server/server_unit_test.go
git commit -m "refactor(chat): require compatible provider config"
```

### Task 2: Update the management UI to configure only OpenAI or Anthropic chat

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/web-antd/src/views/settings/model.vue`
- Modify: `nx-backend/apps/web-antd/src/api/core/model-config.ts`
- Test: `nx-backend/apps/web-antd/src/views/settings/model.test.ts`

- [ ] **Step 1: Write failing UI contract tests**

Add `data-testid="chat-model-section"`, mount the view with a legacy chat payload, and scope assertions to that section:

```ts
const chat = wrapper.get('[data-testid="chat-model-section"]')
expect(chat.text()).toContain('请选择协议')
expect(chat.text()).toContain('OpenAI 协议')
expect(chat.text()).toContain('Anthropic 协议')
expect(chat.text()).not.toContain('Group ID')
expect(chat.text()).not.toContain('MiniMax')
```

Assert the save/test API payload contains `provider` and `timeoutSeconds`, and never contains `groupId`.

- [ ] **Step 2: Run the view test and verify RED**

```bash
cd nx-backend
pnpm exec vitest run --dom apps/web-antd/src/views/settings/model.test.ts
```

Expected: FAIL because the chat form is MiniMax-specific.

- [ ] **Step 3: Implement the provider selector and migration state**

Split options explicitly: `chatProviderOptions` contains only OpenAI and Anthropic, while the existing Admin/Daily Quiz options retain MiniMax where still supported. Do not assign a default provider when the loaded chat payload omits it; show a required unconfigured state. Remove Group ID inputs and MiniMax placeholders only from the chat section. Add timeout validation consistent with other compatible model forms.

- [ ] **Step 4: Run focused tests, type checking, and verify GREEN**

```bash
pnpm exec vitest run --dom apps/web-antd/src/views/settings/model.test.ts
pnpm --filter @vben/web-antd run typecheck
```

- [ ] **Step 5: Commit the management UI**

```bash
git add nx-backend/apps/web-antd/src/views/settings/model.vue nx-backend/apps/web-antd/src/views/settings/model.test.ts nx-backend/apps/web-antd/src/api/core/model-config.ts
git commit -m "feat(settings): configure compatible chat providers"
```

### Task 3: Add provider-neutral chat contracts and a bounded SSE reader

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Create: `nx-backend/apps/server/internal/llm/chat_generator.go`
- Create: `nx-backend/apps/server/internal/llm/chat_generator_test.go`
- Create: `nx-backend/apps/server/internal/llm/sse_reader.go`
- Create: `nx-backend/apps/server/internal/llm/sse_reader_test.go`

- [ ] **Step 1: Write failing contract and framing tests**

Define the desired provider-neutral interface and configuration without constructing concrete adapters yet. Add table tests for LF/CRLF frames, delimiters split across reads, multiple events per read, comment lines, multi-line `data`, a one-megabyte size bound, cancellation, and emitter errors. Factory provider-switch tests belong to Task 6 after both adapters exist.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd nx-backend/apps/server
go test ./internal/llm -run 'TestChatGeneratorContract|TestSSEReader' -count=1
```

- [ ] **Step 3: Implement shared contracts only**

Create:

```go
type ChatGeneratorConfig struct {
    Provider, APIBase, APIKey, Model, SystemPrompt string
    Timeout time.Duration
    Client  *http.Client
}

type ChatGenerator interface {
    rag.Generator
    rag.StreamingGenerator
    rag.ConversationSummarizer
    JSONCompleter
    Ping(context.Context) PingResult
    PolishPrompt(context.Context, string, string) (string, error)
}

type JSONCompleter interface {
    CompleteJSON(context.Context, string, string, int) (string, error)
}
```

The shared SSE reader returns parsed event name/data without interpreting provider JSON. It must read incrementally, never `io.ReadAll` successful streams, and stop immediately on context cancellation or emitter failure. Define the injected-client seam used by later adapter tests, but defer production URL validation/factory construction to Task 6.

- [ ] **Step 4: Run focused tests and verify GREEN**

```bash
go test ./internal/llm -run 'TestChatGeneratorContract|TestSSEReader' -count=1
```

- [ ] **Step 5: Commit the provider-neutral foundation**

```bash
git add nx-backend/apps/server/internal/llm/chat_generator* nx-backend/apps/server/internal/llm/sse_reader*
git commit -m "refactor(chat): add provider generator factory"
```

## Chunk 2: Native OpenAI and Anthropic adapters

### Task 4: Implement the OpenAI-compatible synchronous and streaming adapter

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Create: `nx-backend/apps/server/internal/llm/openai_chat.go`
- Create: `nx-backend/apps/server/internal/llm/openai_chat_test.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`

- [ ] **Step 1: Write failing native protocol tests**

Use `httptest.Server` to assert POST path `/v1/chat/completions` when the configured API base is the versioned root ending in `/v1`, `Authorization: Bearer`, model/messages/system/history mapping, `stream: true`, and the concise output budget. Assert the adapter only appends `/chat/completions` and never invents or duplicates a version segment.

Add a gated stream test: upstream flushes `第一段`, blocks, then sends `第二段` and `[DONE]`. The emitter must receive `第一段` before the gate is released. Cover empty/usage-only events, finish reasons, non-2xx error objects, malformed JSON, cancellation, and full answer accumulation without duplicate final snapshots.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/llm -run 'TestOpenAIChat' -count=1
```

- [ ] **Step 3: Implement the minimal OpenAI adapter**

Build native Chat Completions requests and parse only `choices[].message.content` for sync and `choices[].delta.content` for stream. Implement provider-neutral conversation summarization, prompt polishing, and a narrow `CompleteJSON(system, user, maxTokens)` path with native request-structure tests and no RAG/persona prompt. Reuse prompt/history assembly from the old chat generator only after the tests lock its behavior. Keep MiniMax-only fields and endpoints out of the new file.

- [ ] **Step 4: Run adapter and RAG tests and verify GREEN**

```bash
go test ./internal/llm ./internal/rag -run 'TestOpenAIChat|TestServiceAskStream' -count=1
```

- [ ] **Step 5: Commit the OpenAI adapter**

```bash
git add nx-backend/apps/server/internal/llm/openai_chat.go nx-backend/apps/server/internal/llm/openai_chat_test.go nx-backend/apps/server/internal/llm/chat_generator.go
git commit -m "feat(chat): add native openai streaming adapter"
```

### Task 5: Implement the Anthropic-compatible synchronous and streaming adapter

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Create: `nx-backend/apps/server/internal/llm/anthropic_chat.go`
- Create: `nx-backend/apps/server/internal/llm/anthropic_chat_test.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`

- [ ] **Step 1: Write failing native protocol tests**

Assert POST `/v1/messages` from the versioned API base, `x-api-key`, `anthropic-version`, top-level `system`, alternating role messages, required `max_tokens`, and `stream: true`. Add a gated stream using native events:

```text
event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"第一段"}}
```

Block before the second delta and `message_stop`; assert the first delta already reached the emitter. Cover `message_start`, block start/stop, `message_delta`, error events, non-text blocks, non-2xx Anthropic errors, malformed JSON, cancellation, and full answer accumulation.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/llm -run 'TestAnthropicChat' -count=1
```

- [ ] **Step 3: Implement the minimal Anthropic adapter**

Use Anthropic Messages structures directly. Do not convert native SSE into OpenAI choices. Extract sync text only from `type=text` content blocks and stream text only from `content_block_delta/text_delta`. Implement conversation summarization, prompt polishing, and `CompleteJSON(system, user, maxTokens)` through native Messages requests with focused structure tests.

- [ ] **Step 4: Run adapter and RAG tests and verify GREEN**

```bash
go test ./internal/llm ./internal/rag -run 'TestAnthropicChat|TestServiceAskStream' -count=1
```

- [ ] **Step 5: Commit the Anthropic adapter**

```bash
git add nx-backend/apps/server/internal/llm/anthropic_chat.go nx-backend/apps/server/internal/llm/anthropic_chat_test.go nx-backend/apps/server/internal/llm/chat_generator.go
git commit -m "feat(chat): add native anthropic streaming adapter"
```

### Task 6: Route startup, save, reload, and connection tests through the same factory

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/server/server_test.go`
- Modify: `nx-backend/apps/server/internal/server/server_unit_test.go`
- Modify: `nx-backend/apps/server/internal/server/model_config_security_test.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator_test.go`
- Modify: `nx-backend/apps/server/internal/netguard/netguard.go`
- Modify: `nx-backend/apps/server/internal/netguard/netguard_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`

- [ ] **Step 1: Write failing runtime wiring tests**

Implement the factory RED tests now: exactly OpenAI and Anthropic construct the matching adapters; blank, MiniMax, and unknown providers fail. Test that no stored provider produces `nil` chat generator/unconfigured state, connection testing uses the same factory, and a failed new config changes neither the stored JSON nor the previous live generator. Cover no DB config, legacy config without provider, and an environment containing only MiniMax credentials.

Add regressions proving long-conversation summarization preserves the existing `(previousSummary, messages)` semantics and the prompt-polishing endpoint preserves its `(draft, kind)` image/video distinction through both compatible providers. When provider changes, saving without a new API key must fail instead of inheriting the old provider's secret.

Add timeout tests separating provider request limit, handler total business limit, and SSE idle limit; the existing `MINIAPP_CHAT_TIMEOUT_SECONDS` must have an explicit documented role instead of silently overriding the configured provider timeout.

Add deterministic SSRF tests proving production construction rejects localhost/private IP API bases, redirects to private networks, and hostnames resolving to private addresses via the protected transport. Introduce a test-only/injected resolver and dialer construction seam in `netguard`; production still uses the default secure resolver and never relaxes validation. Only explicitly injected test clients may reach `httptest.Server`.

Assert `NewMiniMaxGenerator` remains usable only for analysis/other legacy features and is no longer referenced by `ragGen`, `ApplyChat`, or `testChatModel`.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/server ./internal/llm ./internal/netguard -count=1
```

- [ ] **Step 3: Implement atomic provider wiring**

Implement `NewChatGenerator` with the provider switch and production `netguard` validation. Add one server helper that validates config and returns a ready generator. Use it in startup load, config save/reload, `/api/model-config/test-chat`, long-summary selection, and the prompt-polishing handler; replace the latter's concrete `*MiniMaxGenerator` assertion while preserving `(draft, kind)`. Save in this order: merge and validate, build and minimally probe the candidate, persist, then acquire the model write lock for a no-fail swap. Verify database atomicity with the repository's real PostgreSQL test fixture or a reliable SQL driver fake. Because the endpoint saves the whole settings page, only probe chat when chat fields changed or `assist.enabled` changed from false to true; unrelated video/image/admin changes must not consume chat quota or be blocked by an unconfigured chat section. Do not fall back to `env.MiniMax` for chat.

Define timeout ownership here: `chat.timeoutSeconds` is the configured end-to-end chat deadline and replaces `MINIAPP_CHAT_TIMEOUT_SECONDS` for configured compatible chat; the environment value is legacy fallback only. Extend the guarded transport with explicit connect, TLS handshake, and response-header timeouts and tests. Add a provider-idle timer in Task 11, reset only by valid model deltas, with a documented default shorter than the total deadline.

Remove chat-specific prompt/request/stream code from `minimax.go` only after the new adapter tests are green; retain MiniMax functionality still used by analysis or other features.

- [ ] **Step 4: Run server, model config, LLM, and RAG tests**

```bash
go test ./internal/server ./internal/modelconfig ./internal/llm ./internal/rag ./internal/netguard -count=1
```

- [ ] **Step 5: Commit runtime wiring**

```bash
git add nx-backend/apps/server/internal/server nx-backend/apps/server/internal/modelconfig nx-backend/apps/server/internal/llm nx-backend/apps/server/internal/netguard
git commit -m "refactor(chat): route compatible providers at runtime"
```

## Chunk 3: Global communication preferences and concise prompt behavior

### Task 7: Add user-global preference schema and transactional store

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Create: `nx-backend/apps/server/internal/db/schema_user_preferences_test.go`
- Create: `nx-backend/apps/server/internal/userpreference/store.go`
- Create: `nx-backend/apps/server/internal/userpreference/store_test.go`

- [ ] **Step 1: Write failing schema and store tests**

Require `app_user_preferences` with user cascade deletion, category check, a conflict `slot`, instruction/source text, timestamps, and a unique key on `(app_user_id, slot)`.

Store tests must cover user isolation, stable ordering, upsert replacement, deleting one slot, and applying conflicting mutations in one transaction. Use the repository's real PostgreSQL test helper for transaction semantics; unit-level validation may use the existing custom driver pattern.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
cd nx-backend/apps/server
go test ./internal/db ./internal/userpreference -run 'Test.*Preference' -count=1
```

- [ ] **Step 3: Implement the minimal schema and store**

Create a package with:

```go
type Preference struct { Category, Slot, Instruction, SourceText string }
type Mutation struct { Upsert *Preference; DeleteSlot string }
func (s *Store) List(ctx context.Context, userID int64) ([]Preference, error)
func (s *Store) Apply(ctx context.Context, userID int64, mutations []Mutation) error
```

Validate categories and allowed communication-style slots in Go as well as SQL. Bound instruction/source lengths, cap preference count and total injected size per user, and never accept a zero user ID.

- [ ] **Step 4: Run focused and DB tests and verify GREEN**

```bash
go test ./internal/db ./internal/userpreference -count=1
TEST_DATABASE_URL='postgres://nx:nx@localhost:5432/nx_admin_test?sslmode=disable' go test ./internal/userpreference -count=1
```

- [ ] **Step 5: Commit preference persistence**

```bash
git add nx-backend/apps/server/internal/db nx-backend/apps/server/internal/userpreference
git commit -m "feat(chat): persist global user preferences"
```

### Task 8: Extract, replace, and cancel common communication instructions

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Create: `nx-backend/apps/server/internal/userpreference/extractor.go`
- Create: `nx-backend/apps/server/internal/userpreference/extractor_test.go`
- Create: `nx-backend/apps/server/internal/userpreference/llm_extractor.go`
- Create: `nx-backend/apps/server/internal/userpreference/llm_extractor_test.go`

- [ ] **Step 1: Write failing deterministic-rule tests**

Table-test at least:

```text
不要叫我亲爱的 -> addressing/avoid_dear
以后叫我小林 -> addressing/preferred_name
回答短一点、不要长篇大论 -> length/concise
以后详细一点 -> length/detailed (replaces concise)
少说教，直接一点 -> tone/direct
不要列表 -> format/no_lists
先给结论 -> format/conclusion_first
不要反问我 -> interaction/no_followup
取消之前回答简短的要求 -> delete length
这次只给结论 -> current-only, not persisted
```

Also cover false positives such as quoted text, questions about another person's preference, and ordinary facts.

- [ ] **Step 2: Run extractor tests and verify RED**

```bash
go test ./internal/userpreference -run 'TestExtract' -count=1
```

- [ ] **Step 3: Implement deterministic extraction and optional LLM fallback**

Return both current-turn directives and durable mutations. Deterministic rules handle common explicit Chinese instructions. Avoid an import cycle by defining a narrow injected function inside `userpreference`, for example `type CompleteJSON func(context.Context, string, string, int) (string, error)`; server wiring passes the active generator's native `llm.JSONCompleter.CompleteJSON` method. The optional LLM extractor receives only a strict extraction system prompt and the user message, never the RAG/persona prompt; invalid JSON, timeout, provider errors, or a full concurrency slot produce no mutations and never block the chat.

Conflict handling must be slot-based: concise and detailed both write `length.detail_level`; preferred names write `addressing.preferred_name`; avoid-address rules use separate slots and may coexist with a preferred name. Reject safety bypasses, factual claims, arbitrary task instructions, and unbounded `custom` content.

- [ ] **Step 4: Run extractor tests and verify GREEN**

```bash
go test ./internal/userpreference -count=1
```

- [ ] **Step 5: Commit preference extraction**

```bash
git add nx-backend/apps/server/internal/userpreference
git commit -m "feat(chat): learn communication preferences"
```

### Task 9: Inject preferences and current instructions into concise prompts

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Modify: `nx-backend/apps/server/internal/rag/rag.go`
- Modify: `nx-backend/apps/server/internal/rag/rag_test.go`

- [ ] **Step 1: Write failing prompt-priority tests**

Assert the assembled prompt orders safety/defaults, saved preferences, history/summary, and current message. Verify current “这次详细说” overrides saved concise preference for the current turn; “以后都详细” also schedules durable replacement. Verify the default includes ordinary 1–3 sentence replies, short structured complex answers, expansion only on explicit request, no default intimate nickname, no forced summary/advice, and at most one useful follow-up.

Add handler tests proving saved preferences from user A never reach user B and the same user's preferences apply to a different chat session/card. Define synchronous mutation failure as fail-closed for durable preference commands: return an explicit error before model generation and never answer “已经记住”. One-time current directives that produce no durable mutation continue normally.

Add tests proving durable mutations are committed before a success `done`, model generation is not called after a durable write failure, an async extractor slot at capacity skips immediately, and async extraction uses its own bounded context after the request context is canceled.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/llm ./internal/rag ./internal/server ./internal/userpreference -run 'Test.*Prompt|Test.*Preference.*Chat|TestAppChat.*Preference' -count=1
```

- [ ] **Step 3: Implement preference-aware chat input**

Extend both `rag.AskInput` and `rag.GenerateInput` with focused fields such as `UserPreferences []string` and `CurrentDirectives []string` instead of concatenating unstructured text in the HTTP handler. Load global preferences before calling RAG. Pass the original current question last so its explicit instruction remains most specific.

Run deterministic extraction before generation so the current response obeys the instruction immediately. Persist explicit durable add/cancel/forget mutations synchronously as an independent user setting operation; generation failure does not undo them, and a successful `done` must never claim a preference was remembered if its write failed. Only unresolved but clearly style-related model extraction may run asynchronously with an independent bounded context and concurrency slot; it must not affect or make promises in the current reply.

- [ ] **Step 4: Run LLM, RAG, and server tests and verify GREEN**

```bash
go test ./internal/llm ./internal/rag ./internal/server ./internal/userpreference -count=1
```

- [ ] **Step 5: Commit preference-aware prompts**

```bash
git add nx-backend/apps/server/internal/llm nx-backend/apps/server/internal/rag nx-backend/apps/server/internal/server nx-backend/apps/server/internal/userpreference
git commit -m "feat(chat): obey global and current user instructions"
```

### Task 10: Include preferences in privacy export, memory clearing, and account deletion

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_privacy.go`
- Modify: `nx-backend/apps/server/internal/server/app_privacy_test.go`

- [ ] **Step 1: Write failing privacy lifecycle tests**

Insert preferences for the test user. Assert `/privacy/export` includes a `preferences` collection without other users' data. Assert `/privacy/memories` deletes both card memories and global communication preferences in one transaction. Assert `/privacy/account` explicitly deletes preferences because the existing account flow anonymizes rather than physically deleting `app_users`. Add transaction-failure rollback tests and a policy version/text assertion.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/server -run 'TestAppPrivacy.*Preference|TestAppPrivacy' -count=1
```

- [ ] **Step 3: Implement privacy lifecycle coverage**

Read export data through the preference store. Perform clear and account-anonymization cleanup transactionally so the API cannot report success after deleting only one memory class. Update the privacy policy version/text to name learned communication preferences.

- [ ] **Step 4: Run privacy tests and verify GREEN**

```bash
go test ./internal/server -run 'TestAppPrivacy' -count=1
TEST_DATABASE_URL='postgres://nx:nx@localhost:5432/nx_admin_test?sslmode=disable' go test ./internal/server -run 'TestAppPrivacy' -count=1
```

- [ ] **Step 5: Commit privacy integration**

```bash
git add nx-backend/apps/server/internal/server/app_privacy.go nx-backend/apps/server/internal/server/app_privacy_test.go
git commit -m "feat(privacy): cover communication preferences"
```

## Chunk 4: Backend SSE delivery guarantees

### Task 11: Flush immediately and keep the normalized App SSE stream alive

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences`

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`

- [ ] **Step 1: Write failing real HTTP timing and heartbeat tests**

Use `httptest.Server`, a blocking context/profile/summary dependency, and a controlled generator. After auth/body/session validation, assert headers and `: connected\n\n` arrive before releasing blocked retrieval or summarization. Then assert the first `event: delta` arrives before the provider releases the second delta. Advance an injected ticker/clock to assert `: ping\n\n` is written during a long gap and ignored by business parsing.

Cover initial `: connected` write failure, client cancellation while waiting, heartbeat write failure, provider failure after a partial delta, save failure, and done ordering. Inject the blocked pre-generation dependency through the existing `ragDocs` fake or a focused test seam.

Add provider-idle tests: heartbeat does not reset idle time, each valid delta resets it, and idle timeout emits exactly one error, cancels the provider, leaks no worker goroutine, and never saves the pair. Add a race/leak test where the writer exits while the worker is trying to send a delta/result.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
go test ./internal/server -run 'TestAppChat.*Stream|TestWriteAppChatSSE' -count=1
```

- [ ] **Step 3: Implement immediate flush and bounded heartbeat**

Write headers, write an initial comment, and flush before profile/retrieval/summary/provider work. Run the slow pipeline in a worker goroutine and send typed events through buffered channels to one handler-owned writer pump; every worker send selects on `ctx.Done()` so writer exit cannot strand a goroutine. Only that pump may write heartbeat, delta, done, or error frames. Write failure cancels the child context. Enforce exactly one terminal event and stop every ticker/timer on exit.

Implement a provider-idle timer shorter than the configured total deadline. Reset it only for valid model deltas, never for outbound heartbeat comments. Record structured timings for connected, first delta, completion, cancellation, provider idle timeout, and error without logging message text or secrets.

- [ ] **Step 4: Run focused and package tests and verify GREEN**

```bash
go test ./internal/server ./internal/rag -count=1
go test -race ./internal/server ./internal/rag -run 'TestAppChat.*Stream|TestWriteAppChatSSE' -count=1
```

- [ ] **Step 5: Commit SSE delivery guarantees**

```bash
git add nx-backend/apps/server/internal/server/app_chat.go nx-backend/apps/server/internal/server/app_chat_stream_test.go
git commit -m "fix(chat): flush and keep sse streams alive"
```

## Chunk 5: Flutter byte streaming and visible incremental rendering

### Task 12: Make Dio SSE requests byte-incremental, timeout-safe, and cancellable

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing-app/compatible-chat-preferences`

**Files:**
- Modify: `lib/core/network/api_client.dart`
- Modify: `test/core/network/api_client_test.dart`
- Create: `test/core/network/api_client_stream_test.dart`

- [ ] **Step 1: Write a failing delayed local HTTP test**

Start `HttpServer.bind(InternetAddress.loopbackIPv4, 0)` and construct `ApiClient` through an injected `baseUrl`. In this test file, temporarily clear/install a real-network `HttpOverrides`, restore it in teardown, and run the file with `--concurrency=1` so Flutter's test network replacement cannot return a synthetic 400. The handler sends SSE headers and the first UTF-8 frame in deliberately split byte chunks, flushes, then blocks on a completer before sending the second event. Call the real `ApiClient.postStream` and assert the accumulated raw string already contains the first complete SSE frame before releasing the second event; semantic `ChatStreamDelta('第一段')` is asserted in Task 13.

The incremental first-frame assertion may already pass on current code, so the mandatory RED assertions are that the SSE request disables the inherited 20-second `receiveTimeout`, a `CancelToken` reaches the adapter, and canceling the subscription/token closes the actual server request. Also prove split multi-byte Chinese UTF-8 remains valid and 401 refresh retries only before any successful stream content is exposed.

- [ ] **Step 2: Run focused tests and verify RED**

```bash
flutter test --concurrency=1 test/core/network/api_client_stream_test.dart test/core/network/api_client_test.dart
```

- [ ] **Step 3: Implement stream-specific request options**

Extend the constructor precisely as `ApiClient(this._tokens, {HttpClientAdapter? adapter, String? baseUrl})`, with production still defaulting to `Env.apiBase`; do not inject a fully configured Dio that could bypass production options/interceptors. Use `Options(responseType: ResponseType.stream, receiveTimeout: Duration.zero)` or the Dio-supported no-timeout equivalent, accept a dedicated `CancelToken`, and keep one incremental UTF-8 decoder over raw bytes. Do not collect the body before yielding. Ensure stream cleanup and explicit notifier cancellation terminate the HTTP request. Before a pre-content 401 retry, cancel or drain only the old response-body subscription without calling the shared `CancelToken.cancel()`, then retry with that same still-active token; assert the old body closed, the token remains active before retry, and external cancellation stops refresh waiting plus the retry. After any delta/content has been exposed, never retry authentication.

- [ ] **Step 4: Run focused tests and verify GREEN**

```bash
flutter test --concurrency=1 test/core/network/api_client_stream_test.dart test/core/network/api_client_test.dart
```

- [ ] **Step 5: Commit byte streaming**

```bash
git add lib/core/network/api_client.dart test/core/network/api_client_test.dart test/core/network/api_client_stream_test.dart
git commit -m "fix(chat): stream sse bytes without receive timeout"
```

### Task 13: Prove Repository and Notifier expose the first delta before completion

**Repository:** `/Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing-app/compatible-chat-preferences`

**Files:**
- Modify: `lib/features/chat/chat_repository.dart`
- Modify: `lib/features/chat/chat_notifier.dart`
- Modify: `lib/features/chat/screens/chat_screen.dart`
- Modify: `test/features/chat/chat_models_test.dart`
- Modify: `test/features/chat/chat_notifier_analytics_test.dart`
- Modify: `test/features/chat/chat_screen_interaction_test.dart`
- Create: `test/features/chat/chat_real_streaming_test.dart`

- [ ] **Step 1: Write a failing end-to-end local streaming test**

Wire the real `ApiClient`, `ChatRepository`, and `ChatNotifier` to a local delayed HTTP server with real-network `HttpOverrides` restored in teardown. The server must implement both the notifier's initial messages GET and the streaming POST. Send the first `delta`, block the server, and assert a listener sees one pending assistant bubble containing only `第一段` while `notifier.isSending` remains true. Release the second delta and `done`; assert final content is not duplicated and the message ID is set. Run this file with `--concurrency=1`.

Add repository cases for CRLF, delimiter split across bytes, one chunk with multiple events, heartbeat comments, unknown events, malformed error data, oversized unterminated buffers, partial failure, and stream close without done. Define EOF without `done` as failure: preserve partial text with `id == 0`, enter `error`, and expose reload; an empty EOF also enters error.

Add notifier tests proving `_activeStreamCancelToken` is canceled on `loadSession`, reload, session switch, and dispose; every send creates a fresh token. In `finally`, clear it only when `identical(_activeStreamCancelToken, localToken)` so an old request cannot clear a newer token. Cancellation from an older generation must not write error state or notify again. Normal done, protocol error, and synchronous exception must all clean up the matching token.

In `chat_screen_interaction_test.dart`, add widget tests proving content growth auto-scrolls only when the reader was already near the bottom; it must not pull a user away from older history, and rapid deltas must not create overlapping `animateTo()` calls. Record `wasNearBottom` before layout growth and track pending content length/version rather than only message count.

- [ ] **Step 2: Run chat tests and verify RED**

```bash
flutter test --concurrency=1 test/features/chat/chat_real_streaming_test.dart
flutter test test/features/chat/chat_models_test.dart test/features/chat/chat_notifier_analytics_test.dart test/features/chat/chat_screen_interaction_test.dart
```

- [ ] **Step 3: Implement only changes required by the real test**

Keep one pending assistant bubble and call `notifyListeners()` for every non-empty delta. Thread a fresh `CancelToken` from notifier through repository to `ApiClient` and implement the identity-safe lifecycle above. Cap SSE events at 1 MiB: parse all complete frames first, reject any complete oversized event, then apply the same bound to the remaining incomplete frame. Empty `data:`, malformed/non-object JSON, invalid UTF-8, and overflow produce stable friendly protocol errors without deleting deltas already shown. Ensure done metadata updates the pending bubble instead of appending a second answer; EOF without done and partial failures retain text with ID zero but enter error. Near-bottom content growth schedules one safe post-frame scroll without stealing position from a user reading history.

- [ ] **Step 4: Run focused and all chat tests and verify GREEN**

```bash
flutter test --concurrency=1 test/features/chat
```

- [ ] **Step 5: Commit visible incremental rendering**

```bash
git add lib/features/chat/chat_repository.dart lib/features/chat/chat_notifier.dart lib/features/chat/screens/chat_screen.dart test/features/chat/chat_models_test.dart test/features/chat/chat_notifier_analytics_test.dart test/features/chat/chat_screen_interaction_test.dart test/features/chat/chat_real_streaming_test.dart
git commit -m "fix(chat): render each streamed delta immediately"
```

## Chunk 6: Cross-repository verification and review

### Task 14: Run full backend, admin, Flutter, and build verification

**Repositories:** Both isolated worktrees.

**Files:**
- Modify only if a regression test exposes an in-scope defect.

- [ ] **Step 1: Run backend formatting, static checks, and full tests**

```bash
cd /Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences/nx-backend/apps/server
test -z "$(find internal/llm internal/modelconfig internal/userpreference internal/server internal/rag -name '*.go' -type f -exec gofmt -l {} +)"
go vet ./...
go test ./... -count=1
go test -race ./internal/llm ./internal/userpreference ./internal/rag ./internal/server -count=1
TEST_DATABASE_URL='postgres://nx:nx@localhost:5432/nx_admin_test?sslmode=disable' go test ./... -count=1
```

- [ ] **Step 2: Run management frontend tests and production build**

```bash
cd /Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing/compatible-chat-preferences/nx-backend
pnpm run test:unit
pnpm --filter @vben/web-antd run typecheck
pnpm --filter @vben/web-antd run build
```

- [ ] **Step 3: Run Flutter analysis, full tests, and Android build**

```bash
cd /Users/wohenzaiyi/.config/superpowers/worktrees/nine-xing-app/compatible-chat-preferences
dart format --output=none --set-exit-if-changed lib test
flutter analyze
flutter test
flutter build apk --debug
```

- [ ] **Step 4: Run focused acceptance probes**

Run the OpenAI and Anthropic gated stream tests, both providers' private-base/redirect/DNS SSRF tests, provider/handler/idle timeout tests, backend real HTTP SSE test, durable preference-write failure test, preference cross-session/override/cancel tests, privacy transaction lifecycle tests, Flutter real HTTP/cancel test, and EOF-without-done test individually with `-count=1` or Flutter's focused test paths. Record first-delta-before-completion and cancellation evidence.

- [ ] **Step 5: Inspect diffs and repository hygiene**

```bash
git diff --check
git status --short
git log --oneline --decorate -15
```

Confirm the original App worktree's four untracked docs are untouched and the original backend `artifacts/` tree is untouched.

- [ ] **Step 6: Request final spec-compliance and code-quality reviews**

Provide reviewers the design, this plan, both branch base/head SHAs, full verification output, and a requirement checklist. Fix every Critical/Important issue, re-run relevant tests, and re-request review until approved.

- [ ] **Step 7: Commit review fixes separately only when review produced changes**

```bash
git commit -m "fix(chat): address final review findings"
```
