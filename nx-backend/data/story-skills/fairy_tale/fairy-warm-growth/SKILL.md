---
name: fairy-warm-growth
description: Use when generating a user's “我的故事” in the 温和成长 style under the fairy_tale story type.
---

# 温暖成长

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 宫泽贤治童话集/已恢复编码文本; 01-童话故事类-321本/成长主题绘本
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 把成长写成一次理解、求助、告别或重新连接，而不是单纯战胜敌人。
- 保留悲伤和困难，但不美化伤害、不把创伤当作成长的必经道路。
- 使用自然、季节、动物或旅途作低强度象征，避免堆叠奇观。
- 结尾给出可感知的小变化，保持希望但不承诺现实一定圆满。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
