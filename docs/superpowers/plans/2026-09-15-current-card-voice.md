# Current Card Voice Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every ordinary chat response use the currently selected card's Enneagram voice instead of the user's primary card voice.

**Architecture:** Keep card selection in the existing session-to-`rag.ConversationCard` path. Add one provider-neutral system-prompt resolver for the trusted current-card voice and extend compatible-provider user context with the selected card details; all sync and streaming adapters then inherit the same behavior.

**Tech Stack:** Go, `net/http`, existing `rag` and `llm` packages, Go `testing`

---

## Chunk 1: Provider-neutral current-card voice

### Task 1: Specify the nine voice mappings and precedence

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/chat_generator_prompt_test.go`
- Modify: `nx-backend/apps/server/internal/llm/skill_runtime_prompt_test.go`

- [x] Add a table-driven failing test that passes card types one through nine and asserts each resolved system prompt contains the selected type and its unique voice markers.
- [x] Add failing tests that an invalid/missing type adds no card voice, a selected type overrides profile context, and skill runtime omits card voice.
- [x] Run `go test ./internal/llm -run 'CurrentCardVoice|SkillRuntime'` and verify the new tests fail.

### Task 2: Implement the shared voice resolver

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`

- [x] Add a bounded one-to-nine style table and a helper that emits the trusted current-card instruction.
- [x] Update `resolveRuntimeSystemPrompt` so ordinary chat appends that instruction after the fixed/default prompt while skill runtime remains isolated.
- [x] Run `gofmt` and the focused tests; verify they pass.

## Chunk 2: Compatible-provider card context

### Task 3: Add selected-card context to OpenAI and Anthropic prompts

**Files:**
- Modify: `nx-backend/apps/server/internal/llm/chat_generator.go`
- Modify: `nx-backend/apps/server/internal/llm/chat_generator_test.go`

- [x] Add a failing test asserting `buildCompatibleChatUserMessage` includes name, relationship, main type, wing type, profile, and the secondary-card boundary.
- [x] Extract/reuse a bounded card-reference builder and add it to compatible chat reference data.
- [x] Run `go test ./internal/llm` and verify it passes.

## Chunk 3: Regression verification

### Task 4: Verify HTTP and server paths

**Files:**
- Test: `nx-backend/apps/server/internal/llm/*_test.go`
- Test: `nx-backend/apps/server/internal/server/*_test.go`

- [x] Run `go test ./internal/llm ./internal/server`.
- [x] Run `git diff --check` and inspect the scoped diff.
- [x] Commit only the design, plan, prompt implementation, and focused tests.
