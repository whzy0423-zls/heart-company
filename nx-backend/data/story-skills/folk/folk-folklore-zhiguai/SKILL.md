---
name: folk-folklore-zhiguai
description: Use when generating a user's “我的故事” in the 民俗与志怪 style under the folk story type.
---

# 民俗志怪

## Purpose

This is a writing behavior skill for the `我的故事` flow. It changes narrative form, not the user's facts. The source collection is a reference for motifs and structure, never a license to copy a book or invent events.

## Source boundary

- Candidate sources: 2 中国社会民俗史丛书（全30册）; 8 宗教与民俗典（全10册）; 中国神话鬼怪合集/志怪小说/《猫苑》.pdf
- The corpus contains duplicate files, scanned files and OCR-incomplete files. Treat any source not explicitly marked as readable as a candidate for motif discovery only.
- Do not quote long passages or imitate a named author. Keep the user's confirmed facts as the evidence boundary.

## Required workflow

1. Extract the confirmed people, places, time order, actions, feelings and outcome from the fact cards and outline.
2. Separate facts, user interpretation and the selected symbolic device before drafting.
3. Apply the rules below to the narrative surface while preserving the fact sequence and result.
4. Draft 4-5 chapters. Each chapter contains a short title, a summary and a focused body with concrete actions and emotional movement.
5. Put interpretation, growth and uncertainty in `reflection`; never disguise inference as an event.

## Style rules

    - 民俗资料用于环境、物件和仪式细节，不用于证明超自然事件真实发生。
- 志怪元素只承担隐喻或悬念功能，不渲染恐怖、迷信或对特定群体的偏见。
- 把研究资料、传说口述和用户亲历明确区分，必要时使用“传说中”或“故事里”。
- 故事仍以人物选择和现实后果收束，不让鬼怪替代真实冲突的解决。

## Output contract

Return the existing `我的故事` JSON shape only: `perspective`, `tone`, `chapters` and `reflection`. Do not return Markdown, source notes or a second story. Keep the requested perspective and tone. Do not add characters, locations, dates, diagnoses, legal conclusions or guaranteed outcomes.

## Final checks

- Every major event is traceable to a confirmed material or is clearly framed as metaphor.
- The selected story type is visible in the language and structure without becoming a copy of a known work.
- Symbolic rewriting is clearly separated from reality, especially for `myth`, `folk` and `fairy_tale`.
- Sensitive experiences are handled without glamorizing harm or providing harmful instructions.
