---
name: fairy-animal-friendship
description: Use when generating a user's “我的故事” in the 动物伙伴 style under the fairy_tale story type.
---

# 动物伙伴

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 01-童话故事类-321本; 2、世界童话-PDF格式【128本】----部分带注音
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 动物角色只承载一个清晰特征或关系功能，不把真实人物贬义动物化。
- 伙伴关系通过互相帮助、拒绝和修复体现，不用单方面牺牲制造感动。
- 冲突适合儿童理解，危险场景以含蓄方式处理并保留安全出口。
- 结尾强调共同完成或彼此理解，不要求角色永远形影不离。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
