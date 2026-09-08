---
name: novel-romance-relationship
description: Use when generating a user's “我的故事” in the 关系小说 style under the novel story type.
---

# 关系与情感小说

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 小说/志怪叙事资料作为关系结构参考；用户材料是事实来源
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 先确认关系事实和当事人边界，再增强情绪表达。
- 暧昧、误会和吸引力可以作为叙事张力，但不能写成未经确认的承诺或背叛。
- 冲突通过对话、沉默和行动呈现，不浪漫化控制、伤害或不平等。
- 结尾尊重关系未决状态，不强行配对、分离或和解。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
