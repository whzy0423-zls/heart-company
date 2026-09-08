---
name: novel-plot-structure
description: Use when generating a user's “我的故事” in the 情节推进 style under the novel story type.
---

# 小说情节结构

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 中国神话鬼怪合集/[中国古代小说的原型与母题].吴光正.扫描版.pdf; 中国神话鬼怪合集/[中国的神话传说与古小说].小南一武著孙昌武译.扫描版.pdf
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 使用起因、升级、转折、后果和回望五段结构，不为制造高潮虚构新事件。
- 把材料中的重复叙述合并成一个清晰场景，保留时间和因果的不确定标记。
- 章节结尾留下未解决的问题或情绪余波，但下一章必须回到已知材料。
- 冲突强度服从真实经历，不用灾难、阴谋或反转替代事实空缺。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
