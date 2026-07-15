# RAG Model Concise Reply Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make App chat retrieve knowledge with the original question, send the question/context/sources plus a fixed concise instruction to the admin-configured chat model, and display its SSE answer without App-side rewriting.

**Architecture:** Keep retrieval and model generation authoritative in the Go backend. Append one product-owned instruction at the end of every model user prompt, while preserving the admin-configured system prompt and model connection. Remove the Flutter reply guard so streamed, completed, partial, and historical messages show the server/model text unchanged apart from the existing technical content sanitizer.

**Tech Stack:** Go HTTP/SSE backend, existing `rag` and `llm` packages, PostgreSQL-backed model configuration, Flutter/Dart chat notifier, Go tests and Flutter tests.

---

## Chunk 1: Backend model prompt

### Task 1: Lock the fixed instruction and prompt order with tests

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/minimax_test.go`
- Modify: `nx-backend/apps/server/internal/llm/minimax.go`

- [ ] **Step 1: Write a failing prompt-order test**

Add a test that builds a prompt with an original question and one retrieved source. Assert that the prompt contains the unchanged question, contains the source, and ends exactly with:

```text
请基于检索资料直接回答当前问题。请精简一些告诉我，优先使用 1～3 句话，不要长篇展开，不使用“亲爱的”等亲昵称呼。
```

Assert ordering is `用户问题` before `检索资料` before the fixed instruction.

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/llm -run 'TestBuildUserPromptEndsWithFixedConciseInstruction' -count=1
```

Expected: FAIL because the current prompt ends with the older generic instruction.

- [ ] **Step 3: Add a named fixed instruction constant and append it last**

In `minimax.go`, add a package constant for the exact instruction and make `buildUserPrompt` append it after the retrieval section. Do not alter `input.Question` and do not add the instruction to RAG search input.

- [ ] **Step 4: Add a configured-model request test**

Extend the local HTTP model test to configure a custom system prompt/model, call `Generate`, and assert:

- the configured model remains in the request body;
- the configured system prompt remains the system message;
- the user message includes retrieved knowledge and ends with the fixed instruction.

- [ ] **Step 5: Run backend model tests and verify GREEN**

Run:

```bash
cd nx-backend/apps/server
go test ./internal/llm -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit backend prompt change**

```bash
git add nx-backend/apps/server/internal/llm/minimax.go nx-backend/apps/server/internal/llm/minimax_test.go
git commit -m "fix(chat): send fixed concise instruction to model"
```

### Task 2: Verify RAG and SSE preserve the authoritative model answer

**Files:**
- Modify if coverage is missing: `nx-backend/apps/server/internal/rag/rag_test.go`
- Modify if coverage is missing: `nx-backend/apps/server/internal/server/app_chat_stream_test.go`

- [ ] **Step 1: Add or strengthen a RAG test**

Assert that the generator receives the original question unchanged, receives retrieved source metadata, and its returned answer is returned unchanged by `AskStream`.

- [ ] **Step 2: Run the focused RAG test**

```bash
cd nx-backend/apps/server
go test ./internal/rag -run 'TestAskStream.*OriginalQuestion.*Sources' -count=1
```

Expected: PASS after coverage is in place; no production RAG rewrite should be necessary.

- [ ] **Step 3: Run chat/RAG/SSE backend verification**

```bash
cd nx-backend/apps/server
go test ./internal/rag ./internal/llm ./internal/server -count=1
```

Expected: PASS, including real delta-before-completion and preference tests.

- [ ] **Step 4: Commit any test-only coverage changes**

```bash
git add nx-backend/apps/server/internal/rag/rag_test.go nx-backend/apps/server/internal/server/app_chat_stream_test.go
git commit -m "test(chat): verify RAG model streaming pipeline"
```

Skip this commit if no files changed because existing coverage already proves the requirement.

## Chunk 2: Flutter displays model output unchanged

### Task 3: Remove App-side canned and length transformations

**App repository:** `/Users/wohenzaiyi/Desktop/nine-xing-app`

**Files:**
- Delete: `lib/features/chat/chat_reply_guard.dart`
- Delete: `test/features/chat/chat_reply_guard_test.dart`
- Modify: `lib/features/chat/chat_notifier.dart`
- Modify: `test/features/chat/chat_notifier_analytics_test.dart`

- [ ] **Step 1: Create an isolated App worktree from current `main`**

Use branch `feature/display-model-replies-unchanged` under the existing global worktree directory.

- [ ] **Step 2: Change notifier tests first**

Replace guard-oriented expectations with failing tests that prove:

- a streamed long model answer remains complete;
- a model answer to `在吗` is not replaced with canned text;
- loaded history remains the server-persisted model text;
- partial stream text remains unchanged after failure.

- [ ] **Step 3: Run the focused notifier tests and verify RED**

```bash
flutter test test/features/chat/chat_notifier_analytics_test.dart
```

Expected: FAIL because `ChatReplyGuard` currently replaces or truncates content.

- [ ] **Step 4: Remove reply guard usage**

Delete `ChatReplyGuard`, remove its import, and use only `ContentSanitizer.sanitize` at the same presentation boundaries. Do not add greeting replacement, sentence limits, or rune limits elsewhere.

- [ ] **Step 5: Delete obsolete guard tests and run chat tests**

```bash
flutter test \
  test/features/chat/chat_notifier_analytics_test.dart \
  test/features/chat/chat_models_test.dart \
  test/features/chat/chat_screen_interaction_test.dart
```

Expected: PASS.

- [ ] **Step 6: Commit App change**

```bash
git add lib/features/chat/chat_notifier.dart test/features/chat/chat_notifier_analytics_test.dart
git rm lib/features/chat/chat_reply_guard.dart test/features/chat/chat_reply_guard_test.dart
git commit -m "fix(chat): display backend model replies unchanged"
```

## Chunk 3: Full verification and integration

### Task 4: Verify both repositories

- [ ] **Step 1: Format and test backend**

```bash
cd nx-backend/apps/server
gofmt -w internal/llm/minimax.go internal/llm/minimax_test.go internal/rag/rag_test.go
go test ./internal/rag ./internal/llm ./internal/server -count=1
go test ./... -count=1
```

Expected: all Go tests PASS.

- [ ] **Step 2: Format, analyze, and test App**

```bash
dart format --output=none --set-exit-if-changed .
flutter analyze
flutter test --exclude-tags=golden
flutter test test/features/home/onboarding_view_golden_test.dart test/features/home/star_seal_golden_test.dart
```

Expected: formatting unchanged, analyze has no issues, all tests PASS.

- [ ] **Step 3: Review diffs against the approved design**

Confirm the fixed instruction is appended after retrieval context, admin model configuration is preserved, the original question remains the retrieval query, SSE model deltas are unchanged, and App-side rewriting is absent.

- [ ] **Step 4: Merge backend and App feature branches**

Fast-forward each feature branch into its existing base branch only after fresh verification. Preserve unrelated untracked files in both primary workspaces.

- [ ] **Step 5: Deploy backend before producing the next APK**

The App change depends on the production backend prompt fix. Deploy the backend commit through the existing production deployment path, verify the live stream endpoint emits model-generated concise deltas, then build the next production App version.
