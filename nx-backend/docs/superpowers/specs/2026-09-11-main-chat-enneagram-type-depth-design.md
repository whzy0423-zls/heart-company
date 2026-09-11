# Main Chat Enneagram Type Depth Design

**Date:** 2026-09-11

## Goal

Make enneagram knowledge answers in the App main conversation immediately understandable and sufficiently deep. Type explanations must use canonical Chinese names such as `1号完美型` and expand each requested type through six consistent dimensions instead of returning short abstract labels.

## Scope

This behavior applies only to enneagram knowledge questions in the App main conversation. It does not change friend messages, skill conversations, ordinary general-knowledge questions, or personalized emotional-support questions that do not ask for enneagram type knowledge.

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

## Generation Contract

The App main-chat runtime instruction supplies the canonical name map and the six required dimensions. Retrieved public knowledge, the formal enneagram theory library, and the current card's type library remain reference data; when the question is about the user, the current portrait may personalize examples but must not replace the requested type coverage.

The contract forbids bare labels such as `一号：重原则` and requires concrete, contextual sentences. The answer remains mobile-readable by using one heading per type and short labeled paragraphs or bullets below it.

## Response Budgets

The current 700-token overview budget is too small for nine types with six dimensions and can truncate or compress the answer. The server will use a larger dedicated budget for all-nine overviews and a smaller dedicated deep budget for one or several requested types. Ordinary questions keep the existing low-latency budget.

## Data Flow

```text
main-chat question
  -> deterministic enneagram knowledge classifier
  -> public/theory/current-type knowledge retrieval
  -> canonical type + six-dimension runtime contract
  -> question-shape response budget
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

- all nine canonical labels and all six dimensions are present in the all-types contract;
- overview questions receive the expanded all-types budget;
- single and partial type questions receive the type-depth contract and an appropriate budget;
- partial type questions do not require unrelated types;
- ordinary questions retain the concise path and budget;
- sync and streaming App main-chat handlers pass the same runtime instruction.

Focused Go tests, the full affected-package tests, and the repository quality checks must pass before deployment.
