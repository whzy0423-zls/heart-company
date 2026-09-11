# Main Chat Enneagram Type Depth Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make App main-chat enneagram type answers use canonical Chinese type names and six concrete dimensions without truncating or slowing ordinary questions.

**Architecture:** Build a deterministic response plan only in the App main-chat server package. Pass requested types, source-selection controls, output budget, and completion timeout explicitly through App knowledge and RAG inputs; shared provider adapters consume explicit overrides but never infer this main-chat behavior themselves.

**Tech Stack:** Go, `net/http`, PostgreSQL via `database/sql`/pgx, existing RAG and App knowledge packages, Go tests

**Design:** `nx-backend/docs/superpowers/specs/2026-09-11-main-chat-enneagram-type-depth-design.md`

---

## Chunk 1: Main-Chat Response Plan

### Task 1: Classify enneagram knowledge requests and build the six-dimension contract

**Files:**
- Create: `nx-backend/apps/server/internal/server/app_chat_enneagram_reply.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_enneagram_reply_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`

- [ ] **Step 1: Write failing table tests for classification**

Cover all overview, single-type, partial-type, canonical-name, numeric-range, target-sentence, emotional-support, and ordinary-domain cases from the design. Assert ordered `RequestedTypes`, empty plan for negatives, and no duplicate type.

```go
func TestBuildAppChatEnneagramReplyPlanClassifiesKnowledgeRequests(t *testing.T) {
    tests := []struct {
        question string
        wantTypes []int
    }{
        {"什么是九型人格？", []int{1,2,3,4,5,6,7,8,9}},
        {"1 2 3 4 这些型号的反馈", []int{1,2,3,4}},
        {"1号是什么样的", []int{1}},
        {"完美型和助人型", []int{1,2}},
        {"我是1号，最近关系压力很大", nil},
        {"1号和2号手机型号有什么区别", nil},
        {"1、2、3、4号", nil},
    }
    // assert plan.Enabled and plan.RequestedTypes
}
```

- [ ] **Step 2: Run the focused test and confirm RED**

Run: `go test ./internal/server -run 'TestBuildAppChatEnneagramReplyPlan' -count=1`

Expected: FAIL because `buildAppChatEnneagramReplyPlan` does not exist.

- [ ] **Step 3: Implement only the minimal deterministic classifier**

Create an unexported structure containing `RequestedTypes`, `RuntimeInstructions`, `MaxOutputTokens`, `CompletionTimeout`, `SourceLimit`, and `SourceSnippetRunes`. Implement only enough classification and ordered de-duplication to pass Steps 1-2; leave the six-dimension runtime contract unimplemented.

- [ ] **Step 4: Write failing generation-contract tests and confirm RED**

Test all nine labels, all six dimension labels, requested-only inclusion, numeric ordering, formula budgets, and ordinary empty plans. Do not assert only global substring presence; iterate `requested types × required dimensions` over the structured plan data. Parse the per-type structured contract and assert every dimension requires one concrete sentence of roughly 20-60 Chinese characters. Also assert:

- portrait context may personalize examples but cannot replace any requested type or dimension;
- missing type-library retrieval cannot remove a requested type or dimension from the generation contract;
- the contract forbids inferring an untested user's type and forbids presenting enneagram material as a clinical diagnosis.

Run: `go test ./internal/server -run 'TestBuildAppChatEnneagramReplyPlanContract' -count=1`

Expected: FAIL because canonical labels, per-type dimensions, concrete-sentence constraints, and boundary instructions are not implemented yet.

- [ ] **Step 5: Implement the six-dimension response plan**

Use the exact canonical names and all six required dimension labels. Calculate tokens with `min(3600, 600+320*len(types))`; use a 70-second minimum completion timeout. Add explicit portrait, missing-retrieval, type-inference, and clinical-diagnosis boundaries required by Step 4.

- [ ] **Step 6: Remove the old broad `appChatRuntimeInstructions` implementation**

Replace it only after the new tests pass. Keep `appChatModelIdentityAnswer` unchanged.

- [ ] **Step 7: Run focused tests and commit**

Run: `go test ./internal/server -run 'TestBuildAppChatEnneagramReplyPlan' -count=1`

Commit: `feat: plan deep enneagram type replies`

---

## Chunk 2: Explicit RAG And Provider Controls

### Task 2: Carry main-chat generation controls through RAG without changing defaults

**Files:**
- Modify: `nx-backend/apps/server/internal/rag/rag.go`
- Modify: `nx-backend/apps/server/internal/rag/rag_test.go`

- [ ] **Step 1: Add failing sync and stream propagation tests**

Extend recording generators to assert `MaxOutputTokens`, `CompletionTimeout`, `SourceLimit`, and `SourceSnippetRunes` travel from `rag.AskInput` to every `rag.GenerateInput` path, including no-match and matched-document paths.

- [ ] **Step 2: Run the focused RAG tests and confirm RED**

Run: `go test ./internal/rag -run 'ExplicitGenerationControls' -count=1`

- [ ] **Step 3: Add zero-default fields and bounded source helpers**

Add non-JSON fields to `AskInput` and `GenerateInput`. A zero value must preserve the existing search limit of four and snippet limit of 92 runes. Clamp explicit source limit and snippet length to conservative internal maxima.

- [ ] **Step 4: Propagate fields through every sync and stream generation branch**

Run: `go test ./internal/rag -count=1`

- [ ] **Step 5: Commit**

Commit: `feat: carry explicit chat generation controls`

### Task 3: Honor explicit output budgets and per-request client timeouts in all chat adapters

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`
- Modify: `nx-backend/apps/server/internal/llm/compatible_chat.go`
- Modify: `nx-backend/apps/server/internal/llm/openai_chat.go`
- Modify: `nx-backend/apps/server/internal/llm/anthropic_chat.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/llm/compatible_chat_test.go`
- Modify: `nx-backend/apps/server/internal/llm/openai_chat_test.go`
- Modify: `nx-backend/apps/server/internal/llm/anthropic_chat_test.go`

- [ ] **Step 1: Write failing request-body tests for explicit budgets**

For MiniMax, legacy compatible, native OpenAI-compatible, and native Anthropic-compatible sync/stream calls, pass `MaxOutputTokens: 1880` and assert the provider payload uses exactly 1880. Assert a zero override retains existing adaptive budgets. The tests must cover all eight concrete request paths, not only the provider-neutral input structure.

- [ ] **Step 2: Write failing small-threshold timeout tests**

Use delayed `httptest.Server` responses and injected clients. For MiniMax, legacy compatible, native OpenAI-compatible, and native Anthropic-compatible, prove both sync and stream methods actually use the shared helper. Explicit completion timeout must clone the configured client and set the requested total timeout on all eight paths. Zero override must preserve each existing ordinary policy: sync uses the configured total timeout, while stream clones the client with `Timeout = 0` and relies on the request context for body duration. Assert the helper never mutates the shared client and preserves transport and all other client fields.

- [ ] **Step 3: Run focused tests and confirm RED**

Run: `go test ./internal/llm -run 'Explicit(OutputBudget|CompletionTimeout)' -count=1`

- [ ] **Step 4: Implement provider-neutral helpers**

Add provider-neutral helpers that return the explicit output budget when positive, otherwise call the existing adaptive budget function; and return the per-request `http.Client`. The request-client helper accepts the existing sync/stream mode. When `CompletionTimeout > 0`, it clones the configured client and sets the clone's `Timeout` to exactly that value. When the override is zero, sync returns the configured client unchanged and stream preserves the existing clone plus `Timeout = 0` behavior. Preserve the guarded transport and all other client fields.

- [ ] **Step 5: Use both helpers in all sync and stream chat request paths**

MiniMax, legacy compatible, native OpenAI-compatible, and native Anthropic-compatible sync and stream methods must all call the same output-budget helper and the same request-client helper. No adapter may locally reimplement timeout selection. Streaming still uses its request context deadline for body duration; the explicit client timeout is the matching total guard for deep requests.

Run: `go test ./internal/llm -count=1`

- [ ] **Step 6: Commit**

Commit: `feat: honor explicit deep reply budgets`

---

## Chunk 3: Requested-Type Knowledge

### Task 4: Resolve and retrieve only explicitly requested type libraries

**Files:**
- Modify: `nx-backend/apps/server/internal/appknowledge/resolver.go`
- Modify: `nx-backend/apps/server/internal/appknowledge/resolver_test.go`
- Modify: `nx-backend/apps/server/internal/appknowledge/coordinator.go`
- Modify: `nx-backend/apps/server/internal/appknowledge/coordinator_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_layered_knowledge_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_voice_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_deps_test.go`

- [ ] **Step 1: Write failing resolver tests**

Extend `ConversationResolver.ResolveConversation` to accept requested types. Assert normalization to unique 1-9 values, one snapshot query for theory plus requested bindings, stable numeric ordering, and existing current-card behavior when the list is empty.

- [ ] **Step 2: Write failing coordinator and App-knowledge limit tests**

Add `RequestedTypes []int` to `appknowledge.Input`. For a card whose current type is 6 and a request for 1-4, assert release searches are exactly 1-4 plus formal theory, never type 6. Assert one failing type does not substitute another and trace keys are type-specific.

At the App main-chat knowledge boundary, add RED tests proving an empty `RequestedTypes` list preserves the existing public/theory/type/search limits `4/3/3/8000`, while an explicit list uses `2/3/9/5000`. The explicit limit path must activate only for a non-empty normalized requested-type list.

- [ ] **Step 3: Run focused tests and confirm RED**

Run: `go test ./internal/appknowledge -run 'RequestedTypes|RequestedType' -count=1`

- [ ] **Step 4: Implement requested binding resolution**

Build SQL placeholders only from normalized internal integers. Keep conversation/card validation and binding reads in the existing repeatable-read transaction. Preserve the existing single current-card binding shape for empty requests and add ordered requested bindings for explicit requests.

- [ ] **Step 5: Implement bounded concurrent type searches**

Search one best chunk per requested type concurrently; collect results and diagnostics by type, then flatten in numeric order. Cap public at two, theory at three, requested type chunks at nine, individual snippets at 360 runes, and the combined reference at 5,000 runes.

- [ ] **Step 6: Update every resolver stub before package verification**

Update all `ResolveConversation` implementations, fakes, and function adapters in `internal/appknowledge` and `internal/server`, including `app_xinzhili_voice_test.go` and `app_xinzhili_realtime_deps_test.go`, in this task so the signature change never leaves either package uncompilable. Task 5 may later add isolation assertions to the same files.

Run: `go test ./internal/appknowledge ./internal/server -run 'LayeredKnowledge|RequestedType' -count=1`

- [ ] **Step 7: Commit**

Commit: `feat: retrieve requested enneagram type knowledge`

---

## Chunk 4: Main-Chat Wiring And Verification

### Task 5: Apply one response plan to sync and streaming main chat

**Files:**
- Modify: `nx-backend/apps/server/internal/server/app_chat.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_chat_layered_knowledge_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_voice_test.go`
- Modify: `nx-backend/apps/server/internal/server/app_xinzhili_realtime_deps_test.go`
- Create: `nx-backend/apps/server/internal/server/app_chat_enneagram_performance_test.go`

- [ ] **Step 1: Write failing handler tests**

For sync and stream handlers, capture the generator input and knowledge resolver input. Assert the target question receives requested types 1-4, 1,880 tokens, 70-second completion timeout, requested-source controls, and identical runtime instructions. Assert an ordinary question receives zero-value controls and the configured timeout.

- [ ] **Step 2: Run focused tests and confirm RED**

Run: `go test ./internal/server -run 'EnneagramReplyPlan|DeepEnneagram' -count=1`

- [ ] **Step 3: Write failing completion-metadata tests and confirm RED**

Capture the existing main-chat timing logs for sync success, stream success, generation error, persistence error, and timeout. Assert they include response rune count and an explicit completion/error phase, never answer content, provider token usage, or provider finish reason.

Run: `go test ./internal/server -run 'Chat.*Timing.*(Completion|Rune|Phase)' -count=1`

Expected: FAIL because completion size and phase are not logged yet.

- [ ] **Step 4: Wire the plan before prompt loading**

Build the plan once after request validation. Pass requested types into knowledge loading, carry generation controls into `rag.AskInput`, and use the explicit timeout for both handler context and the streaming pipeline input. Keep first-delta forwarding and heartbeat code unchanged.

- [ ] **Step 5: Add completion-size timing metadata**

Extend existing main-chat timing logging with response rune count and completion/error phase without logging answer content. Do not add provider token or finish-reason fields.

- [ ] **Step 6: Prove non-main isolation**

Extend `app_xinzhili_voice_test.go`, `app_xinzhili_realtime_deps_test.go`, direct-message tests, and skill-runtime tests with the same enneagram questions and assert they receive no explicit main-chat controls.

- [ ] **Step 7: Add the opt-in live performance harness**

Create `TestMainChatEnneagramPerformanceGate` as an opt-in authenticated integration test with explicit `-mode=baseline|candidate`, `-base-url`, `-token-env`, `-session-id`, `-database-url-env`, `-baseline-file`, `-samples`, `-warmups`, `-min-request-interval`, and `-output` flags. It must skip during ordinary `go test` unless the required live-test flags are supplied. Before every measured request, reset only the dedicated test session's messages, knowledge traces, context summary, and summary cursor through the test database connection, then verify the session is empty; refuse to run unless the session belongs to the dedicated performance fixture account. Sleep as needed to enforce the configured request interval. Baseline mode only records old-version HTTP status and timing/size metrics, so the known shallow answer cannot fail the baseline run. Candidate mode issues both sync and stream cases, validates canonical headings/dimensions, reads the baseline file for the 300 ms comparison, and fails any functional or latency gate. Measure first model delta and completion independently, write only approved JSONL metrics, discard warm-ups from p95, and never retain answer text.

- [ ] **Step 8: Run server tests and commit**

Run: `go test ./internal/server ./internal/llm ./internal/rag ./internal/appknowledge -count=1`

Commit: `feat: deepen main chat enneagram type answers`

### Task 6: Quality gate and deployment

**Files:**
- No source changes expected

- [ ] **Step 1: Integrate the current remote test branch before final gates**

Run `git fetch origin test`. If `origin/test` is not already an ancestor of `HEAD`, merge it into `codex/care-system` without rewriting history and resolve any conflicts. Because this changes the candidate revision, continue through Steps 2-4 on the merged result. If `origin/test` advances again before Step 5, return to this step and repeat all final gates; never push an untested merge revision.

- [ ] **Step 2: Format and inspect**

Run: `gofmt -w` on changed Go files, `git diff --check`, and `git status --short`.

- [ ] **Step 3: Run full backend tests**

Run from `nx-backend/apps/server`: `go test ./... -count=1`

- [ ] **Step 4: Run repository quality checks**

Run `go vet ./...` and `go build ./cmd/server` from `nx-backend/apps/server`. Verify no generated or unrelated files changed with `git status --short` and `git diff --stat HEAD`.

- [ ] **Step 5: Push the feature and update the test branch**

Fetch first, record the release revision and current remote test revision, then push the feature branch and fast-forward the backend test branch only after all checks pass:

```bash
RELEASE_REV=$(git rev-parse HEAD)
OLD_TEST_REV=$(git rev-parse origin/test)
git push origin codex/care-system
git fetch origin test
git merge-base --is-ancestor origin/test "$RELEASE_REV"
git push origin "$RELEASE_REV":refs/heads/test
test "$(git ls-remote origin refs/heads/test | awk '{print $1}')" = "$RELEASE_REV"
```

If the ancestor check fails because `origin/test` advanced, return to Step 1, integrate it, rerun Steps 2-4 in full, set a new `RELEASE_REV`, then retry. Do not force-push.

- [ ] **Step 6: Capture a repeatable performance baseline**

Against the currently deployed test backend, run the authenticated streaming smoke harness for 20 iterations each of an ordinary question and `什么是九型人格`, discarding the first two warm-ups. Save only timestamp, HTTP status, TTFT, completion duration, response rune count, requested type/dimension counts, and truncation flag to `tmp/enneagram-depth-baseline.jsonl`; never save credentials or answer text. Calculate p95 from the remaining 18 samples. Baseline command:

```bash
cd nx-backend/apps/server
go test ./internal/server -run TestMainChatEnneagramPerformanceGate -count=1 -args \
  -base-url=https://xn--9iq9az5uo8fz16d.com \
  -token-env=APP_SMOKE_TOKEN -session-id="$APP_SMOKE_SESSION_ID" \
  -database-url-env=APP_SMOKE_DATABASE_URL \
  -mode=baseline -samples=20 -warmups=2 -min-request-interval=6s \
  -output=../../../tmp/enneagram-depth-baseline.jsonl
```

- [ ] **Step 7: Deploy the server with an explicit rollback point**

On the test host, record the running revision and image, fetch `origin/test`, check out the exact remote test revision, and verify equality before building:

```bash
EXPECTED_RELEASE_REV='exact RELEASE_REV printed and verified in Step 5'
test -n "$EXPECTED_RELEASE_REV"
OLD_REV=$(git rev-parse HEAD)
OLD_IMAGE=$(docker compose images -q server)
git fetch origin test
RELEASE_REV=$(git rev-parse origin/test)
test "$RELEASE_REV" = "$EXPECTED_RELEASE_REV"
git checkout --detach "$RELEASE_REV"
test "$(git rev-parse HEAD)" = "$RELEASE_REV"
docker compose build server
docker compose images server
docker compose stop server
docker compose ps server
docker ps --filter 'label=com.docker.compose.service=server' --format 'table {{.ID}}\t{{.Image}}\t{{.Status}}\t{{.Names}}'
docker compose up -d --no-deps server
docker compose ps server
docker compose logs --since=10m server
curl --fail --silent --show-error http://127.0.0.1:8080/api/app/health
curl --fail --silent --show-error https://xn--9iq9az5uo8fz16d.com/api/app/health
```

- [ ] **Step 8: Run post-deploy functional and performance gates**

Run the candidate harness:

```bash
cd nx-backend/apps/server
go test ./internal/server -run TestMainChatEnneagramPerformanceGate -count=1 -args \
  -base-url=https://xn--9iq9az5uo8fz16d.com \
  -token-env=APP_SMOKE_TOKEN -session-id="$APP_SMOKE_SESSION_ID" \
  -database-url-env=APP_SMOKE_DATABASE_URL \
  -mode=candidate -samples=20 -warmups=2 -min-request-interval=6s \
  -baseline-file=../../../tmp/enneagram-depth-baseline.jsonl \
  -output=../../../tmp/enneagram-depth-candidate.jsonl
```

The candidate mode includes sync/stream smoke cases for overview, `1 2 3 4 这些型号的反馈`, and all nine single-type questions. Require 100% canonical heading and six-dimension coverage, zero truncation, zero non-2xx responses, streaming TTFT p95 below 2.5 seconds and no more than 300 ms above baseline, and all-nine completion p95 below 45 seconds. The harness must also assert the first model delta is forwarded as received rather than held for sentence completion.

- [ ] **Step 9: Verify on the connected Android phone**

Ask `1 2 3 4 这些型号的反馈` in the main conversation and confirm the four canonical headings and six dimensions for each. Record TTFT and completion time without persisting credentials or answer content in diagnostics.

- [ ] **Step 10: Roll back on failure**

If health, completion, or latency gates fail, restore the recorded revision and rebuild only the server:

```bash
git checkout "$OLD_REV"
docker compose build server
docker compose stop server
docker compose up -d --no-deps server
curl --fail --silent --show-error http://127.0.0.1:8080/api/app/health
curl --fail --silent --show-error https://xn--9iq9az5uo8fz16d.com/api/app/health
```

Confirm the running image matches the rebuilt rollback revision. No App reinstall or database migration is required.

This rollback restores runtime only. Do not force-move the shared remote `test` branch: record the failed `RELEASE_REV` as blocked from deployment, create and verify a revert commit on `codex/care-system`, then fast-forward `test` through the same checks in Step 5 before any later deployment. `OLD_TEST_REV` remains the audit reference for the pre-release branch state, and `OLD_IMAGE` is compared against the rollback build/running image record.
