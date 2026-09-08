---
name: myth-gods-mortals
description: Use when generating a user's “我的故事” in the 神人与凡人 style under the myth story type.
---

# 神人与凡人

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 古希腊、中国、埃及和北欧神话中的神人与凡人关系
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 神人与凡人是叙事位置的象征，不把用户或他人写成真实超自然存在。
- 重点表现限制、责任和互助，不渲染权力崇拜或宿命论。
- 神话角色的行为不能覆盖用户材料中的真实人物行为。
- 最后用现实语言解释象征对应的情感和选择。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
