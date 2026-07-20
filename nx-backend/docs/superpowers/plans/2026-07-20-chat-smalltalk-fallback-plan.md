# Chat Smalltalk and Fallback Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prevent short conversational messages from retrieving unrelated enneagram documents and return a natural response when the chat model is unavailable.

**Architecture:** Add deterministic smalltalk classification and fallback generation inside the RAG service, before document search. Filter generic Chinese terms from retrieval evidence, and replace the sourced deterministic fallback after generator failure with a concise retry response.

**Tech Stack:** Go, existing RAG service, Go `testing`.

---

### Task 1: Reproduce short-message false retrieval

- [ ] Add failing tests for “你在干嘛”, “在吗”, “你好”, and “你是谁”.
- [ ] Verify the current service returns unrelated sources.
- [ ] Add a smalltalk classifier and bypass document search.
- [ ] Verify model and local-fallback paths.

### Task 2: Remove generic retrieval evidence

- [ ] Add a failing test using the real “拖绳效应” phrases.
- [ ] Filter generic n-grams such as “你在” and “干嘛”.
- [ ] Verify substantive enneagram queries still retrieve documents.

### Task 3: Replace generator-failure sourced fallback

- [ ] Add a failing test for generator failure after a valid document match.
- [ ] Return a concise retry response without presenting a synthesized knowledge answer.
- [ ] Run RAG, LLM, server, and full internal tests.
- [ ] Merge, restart backend, and verify on device.
