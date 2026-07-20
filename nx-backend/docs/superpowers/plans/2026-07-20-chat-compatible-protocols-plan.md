# Chat Compatible Protocols Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the MiniMax-only mobile chat transport with selectable OpenAI-compatible and Anthropic-compatible gateway protocols.

**Architecture:** Extend stored chat configuration with a provider field and introduce a protocol-aware chat generator implementing the existing RAG generator and streaming interfaces. Keep prompt construction and tier behavior shared, while isolating provider-specific HTTP payloads, headers, response parsing, and SSE parsing. Update the Vue form and API types to expose only provider, base URL, model, and key.

**Tech Stack:** Go, HTTP/SSE, Vue 3, Ant Design Vue, TypeScript, Vitest, Go testing.

---

### Task 1: Chat configuration compatibility

- [ ] Add failing modelconfig tests for provider normalization, legacy default, merge, and removal of Group ID usage.
- [ ] Add `provider` to chat config and normalize missing values to OpenAI compatible.
- [ ] Propagate provider through server view/save/test endpoints.

### Task 2: Protocol-aware chat generator

- [ ] Add failing tests for OpenAI ordinary/stream requests and responses.
- [ ] Add failing tests for Anthropic ordinary/stream requests and responses.
- [ ] Implement shared prompt/tier logic with provider-specific HTTP adapters.
- [ ] Implement provider-aware connectivity testing.

### Task 3: Runtime integration

- [ ] Add failing tests proving server runtime creates the compatible generator from stored/env config.
- [ ] Replace MiniMax chat generator construction without changing video analysis MiniMax usage.
- [ ] Verify text, stream, and voice requests all use the new generator.

### Task 4: Admin UI

- [ ] Add failing Vitest coverage for protocol selector, default API Base, removed Group ID, and payload provider.
- [ ] Update API types and model form.
- [ ] Show protocol-specific endpoint helper text and errors.

### Task 5: Verification and deployment

- [ ] Run Go internal tests and web model configuration tests.
- [ ] Merge into the active backend branch without touching video page changes.
- [ ] Restart Go/Vite services, open the desktop admin page, and test `https://coding-play.codes`.
