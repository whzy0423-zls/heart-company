# Main Chat Enneagram Type Depth Design

**Date:** 2026-09-11

## Goal

Make enneagram knowledge answers in the App main conversation immediately understandable and sufficiently deep. Type explanations must use canonical Chinese names such as `1号完美型` and expand each requested type through six consistent dimensions instead of returning short abstract labels.

## Scope

This behavior applies only to enneagram knowledge questions in the App main conversation. It does not change friend messages, skill conversations, ordinary general-knowledge questions, or personalized emotional-support questions that do not ask for enneagram type knowledge.

The classification and response plan are created in the App main-chat handlers, not inferred inside the shared LLM adapters. The handlers pass explicit runtime instructions, requested knowledge types, output-token budget, and completion timeout through `rag.AskInput` / `rag.GenerateInput`. Other callers receive no explicit plan and keep their current behavior.

## Canonical Type Names

The response contract uses these exact labels:

1. `1号完美型`
2. `2号助人型`
3. `3号成就型`
4. `4号自我型`
5. `5号思考型`
6. `6号忠诚型`
7. `7号活跃型`
8. `8号领袖型`
9. `9号和平型`

Each requested type must cover:

- 核心欲望
- 核心恐惧
- 防御机制
- 压力表现
- 关系模式
- 成长方向

## Question Classification

The server classifies three enneagram knowledge shapes without an extra LLM call:

- Overview or all-types questions: explain the shared framework, then cover all nine canonical types.
- Partial type questions: cover only the explicitly requested types, preserving numeric order.
- Single type questions: cover the requested type through all six dimensions and keep the explanation relevant to the user's wording.

Questions that merely mention a relationship, emotion, or current situation continue through the existing fast or companion path unless they explicitly request type knowledge.

Classification is deterministic. A normal knowledge request requires a strong enneagram anchor plus a knowledge intent:

- Strong anchors: `九型`, `九型人格`, one of the nine canonical Chinese type names, a `1号` through `9号` reference next to `人格` / `性格`, or multiple valid type numbers followed by `这些型号` / `这些类型`.
- Knowledge intents: an interrogative mechanism phrase such as `是什么`, `为什么`, `如何`, `怎么`, `有什么区别`, `如何表现`, or an explicit knowledge noun such as `核心欲望`, `核心恐惧`, `防御机制`, `压力表现`, `关系模式`, `成长方向`, `特点`, `解释`, `对比`, or `反馈`.
- `类型`, `型号`, `关系`, `压力`, and `成长` alone are weak words and never establish the classification.
- Overview phrases such as `什么是九型` select all nine directly.

There are two numeric knowledge sentence forms: a single `1号` through `9号` followed by an explicit question / knowledge phrase such as `是什么样的`, `为什么`, `特点`, or `反馈`; and multiple valid type numbers followed by `这些型号` / `这些类型`, with an optional suffix such as `的反馈`, `解释`, `区别`, or `分别`. Ordinary-domain markers such as `手机`, `产品`, `文件`, `题`, `房间`, `楼`, and `日期` exclude these numeric forms.

A pure numeric list such as `1、2、3、4号` has no sufficient enneagram context and remains unclassified in this version. This deliberately avoids silently treating ambiguous numbers as personality types.

Accepted type references include Arabic digits, Chinese digits, `号`, whitespace, commas, Chinese list punctuation, ranges, and canonical names. Required positive examples include the complete target sentence `1 2 3 4 这些型号的反馈`, `1号是什么样的`, `完美型和助人型`, and `1号性格为什么害怕犯错`. Required negative examples include a context-free `1、2、3、4号`, `第1到9题`, `所有类型的文件`, `这个文件类型是什么`, `1号和2号手机型号有什么区别`, and `我是1号，最近关系压力很大`. If a message has both emotional-support and an explicit mechanism question such as `我是1号，为什么压力下总挑错`, the explicit current request wins; otherwise companion behavior wins.

## Generation Contract

The App main-chat runtime instruction supplies the canonical name map and the six required dimensions. Retrieved public knowledge, the formal enneagram theory library, and the explicitly requested type libraries remain reference data; when the question is about the user, the current portrait may personalize examples but must not replace the requested type coverage.

The contract forbids bare labels such as `一号：重原则` and requires concrete, contextual sentences. The answer remains mobile-readable by using one heading per type and short labeled paragraphs or bullets below it.

Each dimension receives one concrete sentence of roughly 20-60 Chinese characters. An overview may add a short shared introduction and boundary note, but it must not duplicate a generic paragraph under every type.

## Requested-Type Knowledge Retrieval

The main-chat response plan passes an ordered, deduplicated list of requested type numbers to the App knowledge coordinator. The coordinator validates the authenticated conversation and card first, then resolves the formal theory binding and every requested type binding in one repeatable-read snapshot.

- Explicit type questions query only the requested type libraries; the current card type is not silently injected when it was not requested.
- Questions without an explicit type plan retain the existing current-card type behavior.
- Requested type searches run concurrently with a maximum of one selected chunk per type, then return in numeric type order.
- Public knowledge contributes at most two chunks, formal theory at most three chunks, and requested type libraries at most nine total chunks.
- Each selected type snippet is capped at 360 runes and the combined generation reference is capped at 5,000 runes.
- Trace keys distinguish requested libraries as `enneagram_type_01` through `enneagram_type_09`; a failed type library records its own diagnostic and does not substitute another type.

The RAG call receives an explicit main-chat source limit sufficient for the formal theory plus requested types. Other RAG callers keep the existing source count and snippet limits.

## Response Budgets

The current 700-token overview budget is too small for nine types with six dimensions and can truncate or compress the answer. The explicit main-chat type-depth budget is:

```text
min(3600, 600 + 320 * requested_type_count)
```

This yields 920 tokens for one type, 1,880 for four types, and 3,480 for all nine. The value is carried on `GenerateInput` and overrides provider defaults only when it is non-zero. MiniMax, OpenAI-compatible, and Anthropic sync/stream transports all use the same explicit value. Ordinary questions and non-main-chat callers keep their existing budgets.

For type-depth requests, the explicit response plan carries `CompletionTimeout = max(configured_chat_timeout, 70 seconds)` through `AskInput` and `GenerateInput`. The sync/stream handlers use it as their request context deadline. Each provider adapter clones its HTTP client for that request and raises only the cloned client's total timeout to the same value; connection, TLS handshake, and response-header timeouts remain bounded by the existing transport. Ordinary requests keep the configured handler and client total timeout. Existing 15-second SSE heartbeats and immediate first-increment forwarding remain unchanged. No additional classifier model call or generation pass is added.

Provider tests inject small ordinary and extended thresholds to prove that a type-depth response may continue past the ordinary client timeout but is still stopped by its explicit extended deadline. The same tests prove ordinary requests retain the old timeout.

## Data Flow

```text
main-chat question
  -> deterministic main-chat response plan
  -> public/theory/requested-type knowledge retrieval
  -> canonical type + six-dimension runtime contract
  -> explicit type-count response budget and timeout
  -> existing streaming model generation
  -> unchanged Flutter rendering
```

No JSON post-processing or second model pass is added, so the first streamed increment remains immediate.

## Error Handling

- Missing type-specific retrieval does not remove a requested type; the model may use conservative common enneagram knowledge.
- Invalid or ambiguous numeric references do not silently select a type.
- A failed non-critical knowledge layer continues with remaining layers under the existing diagnostics behavior.
- The answer must not infer an untested user's type or present enneagram material as a clinical diagnosis.

## Verification

Automated tests will prove that:

- every requested type is mapped to its canonical label and its own six required dimensions in the generation contract;
- overview questions receive all nine requested types and the 3,480-token budget;
- single and partial type questions receive only their requested types, preserve numeric order, and receive 920 / formula-derived budgets;
- partial type questions do not require unrelated types;
- syntax variants, ranges, canonical names, bare-list shorthand, ambiguous numbers, unrelated file/phone terminology, and emotional-support negative examples classify correctly;
- ordinary questions retain the concise path, source limit, timeout, and budget;
- sync and streaming App main-chat handlers pass the same runtime instruction.
- MiniMax, OpenAI-compatible, and Anthropic sync/stream requests honor explicit budgets, while non-main-chat inputs do not receive the new budget;
- requested type retrieval queries the correct release bindings concurrently, excludes an unrequested current-card type, preserves stable numeric order, enforces snippet/total limits, and isolates diagnostics;
- non-main-chat text, voice, realtime, direct-message, and skill paths do not receive the main-chat response plan.

Focused Go tests, the full affected-package tests, and the repository quality checks must pass before deployment. A fixed smoke set covering overview, four-type, and each single type must have 100% requested-type/dimension completion and zero truncation. On the test network, main-chat streaming TTFT p95 must stay below 2.5 seconds and no more than 300 ms above the pre-change baseline; all-nine completion p95 must stay below 45 seconds.

## Deployment And Rollback

Deploy the backend through the repository's existing test/release workflow, restart the server, and verify health before directing traffic. Smoke-test both sync and streaming main-chat endpoints, then verify the all-nine and four-type answers in the Android App. Record TTFT, completion time, generated response rune count, completion/error phase, and timeout/truncation failures in the existing chat timing logs. Provider token usage and finish reason are not added to the generator contract in this change; response runes are the explicitly labeled local size metric.

Rollback re-deploys the previous backend commit; the Flutter contract and database schema remain compatible. No App reinstall or data migration is required for this server-side behavior change.
