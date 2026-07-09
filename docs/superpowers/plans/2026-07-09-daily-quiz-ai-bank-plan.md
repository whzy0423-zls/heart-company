# 每日题 AI 题库管理 Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现每日题 AI 生成、提前生成、12 点推送、后台题库管理、单题换题版本留存。

**Architecture:** 后端新增每日题集/版本表与 profilecalibration store 能力，server 暴露 admin API 并在推送 loop 中加入 11:30/11:40/11:50 预生成和 12:00 推送兜底。前端新增 `profile-calibration/daily-quiz-bank.vue` 页面和 API 封装；菜单迁移到「画像校准」。

**Tech Stack:** Go `net/http` + PostgreSQL schema + Vue 3/Ant Design Vue + Vben 动态菜单。

---

## Chunk 1: 后端数据和接口
- [x] 写 schema/menu/server route/profilecalibration store 的失败测试。
- [x] 新增 `app_daily_quiz_sets` 与 `app_daily_quiz_question_versions` schema。
- [x] 新增 `DailyQuizSet`/`DailyQuizQuestionVersion` 类型和 store 方法。
- [x] 修改 `TodayBatchForDate` 优先使用每日题集。
- [x] 新增 admin handlers 与路由。

## Chunk 2: 定时生成和推送
- [x] 修改 push loop：11:30/11:40/11:50 生成；12:00 推送前兜底；每日只推送一次。
- [x] 给 daily quiz reminder service 增加 `EnsureDailyQuizSet`/`MarkDailyQuizSetPushed`。
- [x] targeted 测试覆盖 12 点逻辑。

## Chunk 3: 管理端大模型和 AI 生成
- [x] 扩展 modelconfig 管理端模型字段。
- [x] 新增结构化文本生成接口（优先 OpenAI-compatible，Anthropic 可作为配置协议字段留接口）。
- [x] 从 RAG 公共库取文档上下文构建 prompt。
- [x] 生成失败 fallback 默认题并记录 raw/error。

## Chunk 4: 前端后台
- [x] 新增 `daily-quiz` API 封装。
- [x] 新增每日题库管理页面。
- [x] 更新模型配置页面展示管理端大模型配置与超时时间。
- [x] 更新菜单测试/种子：画像校准目录、每日题库管理、每日题推送记录。

## Chunk 5: 验证
- [x] `cd nx-backend/apps/server && go test ./...`
- [x] `cd nx-backend && pnpm exec vitest run --dom apps/web-antd/src/views/profile-calibration/daily-quiz-bank.test.ts`
- [x] `cd nx-backend && pnpm -F @vben/web-antd run typecheck`

## Completion Notes

- 已完成每日 5 题持续画像校准主链路：App 端每日题、累计 100 题触发画像校准报告、报告确认/拒绝、聊天/语音/行为证据参与评估。
- 已完成后台「画像校准」菜单下的每日题库管理和每日题推送记录。
- 已完成后台管理端大模型配置与每日题生成模型覆盖配置，密钥仅显示是否已配置，不回显。
- 已完成 12 点前预生成、12 点推送和 12:00-12:10 补偿窗口。
- 已完成单题更换版本记录；当天已有任意用户答题后整套题锁定，避免题目数据错乱。
