# Chat All-Types Answer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make explicit 1-9/all-types child-application questions return a complete type-by-type answer without making ordinary chat verbose.

**Architecture:** Add a deterministic all-types question detector beside the existing chat prompt and token-budget helpers. Use it to select an all-types coverage instruction and a larger token budget; leave the rest of chat routing, retrieval, streaming, and persistence unchanged.

**Tech Stack:** Go, standard library strings package, Go testing

---

## Chunk 1: Adaptive all-types prompting

### Task 1: Specify all-types behavior

**Files:**
- Modify: `apps/server/internal/llm/minimax_test.go`

- [ ] **Step 1: Write a failing prompt test**

Add a test using `1-9型号的孩子我们如何应用` that expects the generated user prompt to require ordered coverage of all nine types, child characteristics, parent communication, and a concrete application, and to omit the fixed 1-3 sentence instruction.

- [ ] **Step 2: Write a failing budget test**

Add a test expecting the all-types question to receive a substantially larger token budget than a single-type question.

- [ ] **Step 3: Run the focused tests to verify RED**

Run: `go test ./internal/llm -run 'AllTypes|ChatTokenBudget' -count=1`

Expected: FAIL because no all-types detector or instruction exists and the current budget is 220.

### Task 2: Implement adaptive prompt and budget

**Files:**
- Modify: `apps/server/internal/llm/minimax.go`

- [ ] **Step 1: Add a deterministic all-types detector**

Recognize explicit markers such as `1-9`, `1～9`, `1到9`, `九种类型`, `九个类型`, `所有型号`, `全部型号`, `所有类型`, and `逐个型号`.

- [ ] **Step 2: Add the all-types response instruction**

Require ordered `1号` through `9号` coverage. For each type require child characteristics, parent understanding/communication, and one concrete application. State that incomplete retrieved material must not cause omitted types.

- [ ] **Step 3: Expand the all-types token budget**

Return a budget sized for nine concise entries before evaluating ordinary detail markers. Preserve 220 for ordinary questions and 420 for explicit detailed single-topic questions.

- [ ] **Step 4: Run focused tests to verify GREEN**

Run: `go test ./internal/llm -run 'AllTypes|ChatTokenBudget' -count=1`

Expected: PASS.

### Task 3: Regression verification and runtime restart

**Files:**
- No additional code files

- [ ] **Step 1: Run all LLM tests**

Run: `go test ./internal/llm -count=1`

Expected: PASS.

- [ ] **Step 2: Run the full server suite**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Restart the local Go server**

Restart with `APP_ENV=dev`, port `5320`, and the existing site config path, then verify `/api/app/health` returns `status=ok`.

- [ ] **Step 4: Keep the Flutter debug session connected**

The client code does not change. Confirm the Android app remains running and can send a new chat request to the restarted backend.
