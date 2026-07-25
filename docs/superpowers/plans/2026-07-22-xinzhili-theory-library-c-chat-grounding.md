# 芯之力理论库 C：共享会话 Grounding 与引用事务 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 App 文本、SSE 流式和语音会话真正执行“站点/知识库 → 理论库 → AI”，并原子保存消息、引用和检索运行。

**Architecture:** 保留现有 `rag.Service` 供小程序、支付问答和 `off` 模式使用，新增 App 专用 generation-only `rag.GroundedService`。三个 handler 共用 `appChatTurnCoordinator`，统一 Ground → 生成计划 → 来源快照 → Finalize；SSE 只替换生成传输。SSE 在持久化候选后发 `meta`，只在最终事务提交后发 `done`。

**Tech Stack:** Go 1.22, PostgreSQL transactions, existing MiniMax/OpenAI-compatible generators, SSE, Go tests.

**Depends on:** Milestones A and B complete.

---

## Chunk 1: Shared chat grounding

### Task 1: Make RAG generation-only with typed prompt sections

**Files:**
- Modify: `nx-backend/apps/server/internal/rag/rag.go`
- Modify: `nx-backend/apps/server/internal/rag/rag_test.go`
- Create: `nx-backend/apps/server/internal/rag/grounded.go`
- Create: `nx-backend/apps/server/internal/rag/grounded_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/llm/compatible_chat.go`
- Modify: `nx-backend/apps/server/internal/llm/compatible_chat_test.go`

- [ ] **Step 1: Write failing generation tests**

Keep all legacy retrieval tests. Add `GroundedGenerateInput` tests with typed `SiteSources`, `KnowledgeSources`, `TheorySources`, `SafetyNotes`, `ResponseMode`, `DeterministicAnswer` and `FallbackAnswer`. Assert prompt order and evidence labels.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/rag ./internal/llm -run 'Grounded|Grounding|Prompt' -count=1`

- [ ] **Step 3: Add a separate generation-only API**

Add `NewGroundedService(generator)`, `Generate(ctx, GroundedGenerateInput)` and `GenerateStream(ctx, GroundedGenerateInput, emit)`. It never accepts raw `[]Document`, never calls `search`, and never decides intent/risk/fallback. It either returns the orchestrator-provided `DeterministicAnswer`, calls the model, or uses the already-provided `FallbackAnswer` only for a pre-output model error. Leave legacy `NewService(docs...).Ask/AskStream` untouched.

- [ ] **Step 4: Build separated prompt sections**

Use exact ordered headings `[用户和会话上下文]`, `[当前问题]`, `[知识库事实与经验]`, `[理论库解释框架]`, `[理论边界、争议和安全提醒]`, `[回答规则]`. Put site facts first inside the knowledge/facts section with explicit corpus labels. Preserve the existing concise instruction at the end of `[回答规则]`.

- [ ] **Step 5: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/rag ./internal/llm -count=1`

```bash
git add nx-backend/apps/server/internal/rag nx-backend/apps/server/internal/llm
git commit -m "refactor(rag): generate from typed grounding sources"
```

### Task 2: Add atomic answer finalization and source records

**Files:**
- Modify: `nx-backend/apps/server/internal/db/schema.sql`
- Modify: `nx-backend/apps/server/internal/db/schema_theory_library_test.go`
- Create: `nx-backend/apps/server/internal/chat/finalize.go`
- Create: `nx-backend/apps/server/internal/chat/finalize_test.go`
- Modify: `nx-backend/apps/server/internal/chat/store.go`
- Modify: `nx-backend/apps/server/internal/userpreference/store.go`
- Modify: `nx-backend/apps/server/internal/userpreference/store_test.go`

- [ ] **Step 1: Write failing finalization tests**

Cover text and voice inputs. Assert one transaction inserts user message, assistant message, `app_chat_message_sources`, compatible sources JSON, preference mutations, session update, run message IDs, and completed status. Cover invalid corpus/locator combinations, run/session ownership mismatch, cancellation, `persist_failed`, and a forced source insert failure with full rollback.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/chat ./internal/db ./internal/userpreference -run 'Finalize|MessageSources|ApplyTx' -count=1`

- [ ] **Step 3: Add source DDL**

Create `app_chat_message_sources(message_id, run_id, corpus_type, site_source_key, rag_document_id, theory_chunk_id, source_ref_key, title_snapshot, snippet_snapshot, version_snapshot, sort, create_time)`. Add FKs to message/run/knowledge/theory chunk, `ON DELETE RESTRICT`, `num_nonnulls(...)=1`, corpus-to-locator `CHECK`s, unique `(message_id, sort)`, and indexes on run/message/all locators.

- [ ] **Step 4: Implement `FinalizeChatAnswer`**

Use a single `sql.Tx`. Add a tx-aware `userpreference.ApplyTx`; pass preference mutations in `FinalizeInput`. Preserve voice asset fields through `FinalizeInput.UserMessage`. Mark the run `completed` only after messages, compatible JSON, normalized sources, preferences and session timestamp succeed. After commit, memory/profile-evidence jobs are best-effort and must never turn a completed response into HTTP/SSE failure.

- [ ] **Step 5: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/chat ./internal/db ./internal/userpreference -count=1`

```bash
git add nx-backend/apps/server/internal/chat nx-backend/apps/server/internal/db nx-backend/apps/server/internal/userpreference
git commit -m "feat(chat): atomically save grounded answers"
```

### Task 3: Migrate synchronous text chat to shared grounding

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding_test.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounded_turn.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounded_turn_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`

- [ ] **Step 1: Write failing handler tests**

Assert the sync handler delegates one turn to the coordinator, which calls grounding once, creates the generation plan, maps normalized and compatible sources once, generates once and finalizes once. Define `sourceSet` enum as `none|site|knowledge|theory|site_knowledge|site_theory|knowledge_theory|site_knowledge_theory`; define separate `responseMode=grounded|general|clarification|safety`; include `degraded bool`.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'AppChat.*Grounding|AppChatAsk' -count=1`

- [ ] **Step 3: Add a shared request builder**

Move profile, card, memory, summary, history, preferences, directives and tier assembly into `buildAppChatGroundingInput`. Implement `appChatTurnCoordinator.Prepare`, `Generate`, `GenerateStream` and `Finalize`; centralize fallback decisions, typed source conversion, snapshot conversion and compatible JSON here.

- [ ] **Step 4: Replace live retrieval and SavePair**

For live typed modes, remove `retrieveAppDocsForQuery` and `rag.NewService(docs...)` from sync App chat. Use the coordinator and `rag.GroundedService`. `off` still calls the untouched legacy path; `shadow` still answers through that exact legacy path.

- [ ] **Step 5: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/server -run AppChat -count=1`

```bash
git add nx-backend/apps/server/internal/server/app_chat.go nx-backend/apps/server/internal/server/app_chat_grounding.go nx-backend/apps/server/internal/server/app_chat_grounding_test.go nx-backend/apps/server/internal/server/app_chat_grounded_turn.go nx-backend/apps/server/internal/server/app_chat_grounded_turn_test.go nx-backend/apps/server/internal/server/app_chat_test.go
git commit -m "feat(chat): ground synchronous answers through theory"
```

### Task 4: Migrate SSE with a compatible `meta` contract

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_sse_contract_test.go`

- [ ] **Step 1: Write failing event-order and compatibility tests**

Wire events are only `meta`, `delta`, `done`, `error`. Internal order is `meta → delta... → FinalizeChatAnswer commit → done`; prove finalization with an internal recorder hook, never emit a `finalize` event. On partial model or finalization failure: `meta, delta..., error`, no done.

Use this payload schema:

```json
{"runId":"123","sourceSet":"knowledge_theory","responseMode":"grounded","siteCount":0,"knowledgeCount":3,"theoryCount":2,"riskLevel":"low","degraded":false}
```

The compatibility test must parse only `delta/done/error`, ignore unknown `meta`, and still reconstruct the same answer and message ID as the pre-meta protocol.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'SSE.*Meta|Stream.*Grounding|SSEContract' -count=1`

- [ ] **Step 3: Implement the event sequence**

Persist retrieval candidates before `meta`. Send `done` only after `FinalizeChatAnswer` commits. Mark cancellation and errors on the run.

- [ ] **Step 4: Run existing timing tests**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'AppChatAskStream|SSE' -count=1`

Expected: PASS, including first delta before generation completion.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/server/app_chat.go nx-backend/apps/server/internal/server/app_chat_stream_test.go nx-backend/apps/server/internal/server/app_chat_sse_contract_test.go
git commit -m "feat(chat): stream grounded answers with auditable completion"
```

### Task 5: Migrate voice chat to the same pipeline

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat_voice.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_voice_test.go`

- [ ] **Step 1: Write failing voice grounding tests**

Assert recognized transcript uses the same grounding input and finalizer, voice asset fields are preserved, and no-speech voice messages continue using the existing standalone save path without a retrieval run.

- [ ] **Step 2: Verify failure**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'Voice.*Grounding|AppChatVoice' -count=1`

- [ ] **Step 3: Replace duplicated RAG and SaveVoicePair code**

Use `appChatTurnCoordinator` and `FinalizeChatAnswer` with a voice `UserMessageInput`; do not duplicate retrieval, fallback or source mapping.

- [ ] **Step 4: Run and commit**

Run: `cd nx-backend/apps/server && go test ./internal/server -run 'AppChatVoice|AppChatAsk|AppChatAskStream' -count=1`

```bash
git add nx-backend/apps/server/internal/server/app_chat_voice.go nx-backend/apps/server/internal/server/app_chat_voice_test.go
git commit -m "feat(chat): ground voice answers through theory"
```

### Task 6: Staff rollout switch and full regression

**Files:**
- Modify: `nx-backend/apps/server/internal/grounding/orchestrator.go`
- Modify: `nx-backend/apps/server/internal/config/env.go`
- Modify: `nx-backend/apps/server/internal/config/env_test.go`
- Modify: `nx-backend/apps/server/internal/server/server.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding_rollout_test.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_grounding_parity_test.go`

- [ ] **Step 1: Write failing rollout tests**

Extend `THEORY_GROUNDING_MODE` from B's `off|shadow` to `off|shadow|staff|enabled`. Parse `THEORY_GROUNDING_STAFF_USER_IDS` as comma-separated positive App user IDs; invalid IDs fail startup. Prove `off` uses legacy, `shadow` records but answers through the exact legacy path, `staff` enables typed grounding only for listed IDs, and `enabled` enables all App chat requests.

- [ ] **Step 2: Implement rollout selection**

Add `THEORY_GROUNDING_THEORY_ENABLED=false` as the emergency switch: live modes keep typed site/knowledge generation and tracing but skip theory retrieval. Apply identical selection to text/SSE/voice.

- [ ] **Step 3: Add cross-entry fallback and safety parity tests**

For text/SSE/voice, run the shared table cases: knowledge failure with non-growth intent, current-fact miss, both stores empty, unknown clarification, high/crisis safety response, model failure before output, and trace-start persistence failure before any response is committed. Assert identical sourceSet/responseMode/fallback and compatible sources. Add SSE-only cases for model failure after deltas and retrieval-hit persistence failure before `meta`; add sync/voice equivalents that fail before the HTTP success body.

- [ ] **Step 4: Run full checks**

Run:

```bash
cd nx-backend/apps/server
gofmt -w internal/rag/*.go internal/chat/*.go internal/grounding/*.go internal/server/app_chat*.go internal/config/*.go internal/userpreference/*.go
go test ./...
go vet ./...
git diff --check
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add nx-backend/apps/server/internal/grounding nx-backend/apps/server/internal/config nx-backend/apps/server/internal/server
git commit -m "feat(chat): add staff theory grounding rollout"
```

### Task 7: Reproducible protocol smoke checkpoint

**Files:**
- Create: `nx-backend/scripts/theory/smoke-grounded-chat.sh`
- Create: `nx-backend/scripts/theory/fixtures/sse-with-meta.txt`
- Create: `nx-backend/scripts/theory/fixtures/voice-smoke.wav`
- Create: `docs/theory-grounding-smoke-record.md`

- [ ] **Step 1: Run authenticated local smoke tests**

Implement `smoke-grounded-chat.sh` with required `BASE_URL`, auth token and session ID. It must call sync, SSE and voice endpoints using the committed short speech fixture, then query a read-only admin trace endpoint or PostgreSQL test database to record run ID, all three source counts, answer message ID and source rows for every entry.

Run: `BASE_URL=http://127.0.0.1:8080 APP_AUTH_TOKEN=... SESSION_ID=... bash nx-backend/scripts/theory/smoke-grounded-chat.sh`

- [ ] **Step 2: Verify an old-client parser**

Check `fixtures/sse-with-meta.txt` with the repository compatibility parser test from Task 4. If the production Flutter repository is outside this workspace, record its exact commit and parser-test command in `docs/theory-grounding-smoke-record.md`; staff enablement remains blocked until that command passes. This record is required, not optional.

- [ ] **Step 3: Record the checkpoint**

Write timestamp, server commit, App/parser commit, commands, outputs and active release ID to `docs/theory-grounding-smoke-record.md`, then commit the script, fixture and record. Do not claim enabled rollout yet.

- [ ] **Step 4: Commit the reproducible smoke artifacts**

```bash
git add nx-backend/scripts/theory/smoke-grounded-chat.sh nx-backend/scripts/theory/fixtures/sse-with-meta.txt nx-backend/scripts/theory/fixtures/voice-smoke.wav docs/theory-grounding-smoke-record.md
git commit -m "test(chat): record grounded protocol smoke evidence"
```
