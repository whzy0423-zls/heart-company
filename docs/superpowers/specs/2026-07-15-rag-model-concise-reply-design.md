# RAG Model Concise Reply Design

## Goal

App chat must answer through one authoritative pipeline: use the user's original question to retrieve knowledge, combine the retrieved sources with conversation context, append a fixed concise-answer instruction, and send that prompt to the chat model configured in the admin backend. The App displays the model's SSE output without replacing greetings or truncating the answer.

## Data flow

1. The App sends the original question to `/api/app/chat/sessions/{id}/ask/stream`.
2. The backend uses only the original question for knowledge retrieval.
3. The backend loads conversation summary/history, user profile, memories, and saved communication preferences.
4. The backend builds the model user prompt in this order:
   - user profile and memories;
   - saved preferences and conversation context;
   - original user question;
   - current-message directives;
   - retrieved knowledge sources;
   - the fixed final instruction: `请基于检索资料直接回答当前问题。请精简一些告诉我，优先使用 1～3 句话，不要长篇展开，不使用“亲爱的”等亲昵称呼。`
5. The runtime chat generator uses the model/API key/model name currently configured in the admin model settings.
6. Model deltas are forwarded immediately as SSE `delta` events; the completed model answer is persisted unchanged and returned in the `done` event.
7. The App renders those deltas and the final answer without canned greeting replacement, sentence clipping, or rune clipping.

## Boundaries

- Retrieval must never include the fixed instruction in its query, so search relevance is based on the user's actual question.
- The admin-configured system prompt remains supported. The fixed concise instruction is appended to the end of the per-request user prompt so it is present even when the admin overrides the system prompt.
- Saved user preferences and current-message corrections remain part of the prompt. The fixed instruction establishes the default product style for concise, neutral replies.
- Source metadata remains attached to the generated answer and is persisted as before.

## Failure behavior

- If streaming starts and the upstream model fails, return the existing SSE error behavior; do not replace partial model output with a locally composed knowledge paragraph.
- If the model is unavailable before any delta, preserve the existing safe fallback behavior for service availability. This fallback is not the normal answer path.
- Knowledge retrieval failure may yield no sources, but the configured model still receives the original question and the fixed concise instruction.

## App behavior

- Remove `ChatReplyGuard` transformations that replace presence greetings or shorten ordinary answers.
- Keep transport/content sanitation unrelated to reply length or wording.
- Apply no different transformation to streamed deltas, final responses, partial responses, or loaded history; all display the server-persisted model text.

## Verification

- Backend prompt tests prove the original question and retrieved sources appear before the exact fixed instruction.
- Backend request tests prove the configured chat model receives the prompt and SSE deltas are forwarded.
- RAG tests prove retrieval uses the original question and source metadata reaches generation.
- App notifier tests prove streamed, completed, partial, and historical model text is not shortened or replaced.
- Existing SSE timing, preference memory, chat persistence, static analysis, and release tests remain green.
