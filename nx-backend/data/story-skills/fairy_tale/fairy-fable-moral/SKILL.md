---
name: fairy-fable-moral
description: Use when generating a user's “我的故事” in the 寓言冲突 style under the fairy_tale story type.
---

# 寓言训诫

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 01-童话故事类-321本/其他童话故事; 目录中含寓言、拟人和生活习惯类绘本
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 一篇只保留一个主要命题，例如诚实、边界、合作或承担后果。
- 让角色通过选择暴露问题，再用后果和修复呈现道理，不直接训话。
- 允许结尾留下讨论问题，不把复杂现实压缩成绝对善恶。
- 动物和物件是象征角色，不得把用户真实人物贬损为某种动物或负面标签。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
