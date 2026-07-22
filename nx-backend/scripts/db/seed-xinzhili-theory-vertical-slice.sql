BEGIN;

INSERT INTO theory_libraries (key, name, description, status, default_language, current_version)
VALUES ('xinzhili', '芯之力理论库', '芯之力理论卡发布最小纵切', 'enabled', 'zh-CN', 1)
ON CONFLICT (key) DO UPDATE SET
  name = EXCLUDED.name,
  description = EXCLUDED.description,
  status = EXCLUDED.status,
  default_language = EXCLUDED.default_language,
  current_version = GREATEST(theory_libraries.current_version, EXCLUDED.current_version),
  update_time = now();

WITH library AS (
  SELECT id FROM theory_libraries WHERE key = 'xinzhili'
)
INSERT INTO theory_source_works (
  library_id, canonical_key, title, original_title, authors, editors, translators,
  publisher, edition, isbn, work_type, authority_level, epistemic_status,
  copyright_scope, metadata, status
)
SELECT id, 'han_teacher_course', '韩老师课程整理', '', '["韩老师"]'::jsonb, '[]'::jsonb,
  '[]'::jsonb, '', '', '', 'course', 5, 'course_adaptation', 'metadata_only',
  '{"seed":true,"content_policy":"original_summary_only"}'::jsonb, 'reviewed'
FROM library
ON CONFLICT (library_id, canonical_key) DO UPDATE SET
  title = EXCLUDED.title,
  authors = EXCLUDED.authors,
  work_type = EXCLUDED.work_type,
  authority_level = EXCLUDED.authority_level,
  epistemic_status = EXCLUDED.epistemic_status,
  copyright_scope = EXCLUDED.copyright_scope,
  metadata = EXCLUDED.metadata,
  status = EXCLUDED.status,
  update_time = now();

WITH work AS (
  SELECT work.id
  FROM theory_source_works work
  JOIN theory_libraries library ON library.id = work.library_id
  WHERE library.key = 'xinzhili' AND work.canonical_key = 'han_teacher_course'
)
INSERT INTO theory_source_files (
  work_id, relative_path, original_filename, file_format, mime_type, byte_size, page_count,
  sha256, title_source, extraction_class, extraction_status, extraction_quality,
  extracted_text_uri, ocr_text_uri, extractor_name, extractor_version, metadata
)
SELECT work.id, 'seed/han-teacher-course.md', 'han-teacher-course.md', 'md', 'text/markdown',
  0, NULL, '6f5c0d4c2a61b94e6287d17b51a746d44e9b7dc0225f086b30e141b4f4f7298a',
  'manual', 'text_rich', 'extracted', 0.9500, '', '', 'seed-placeholder', '1',
  '{"seed":true,"placeholder":true,"contains_copyright_text":false}'::jsonb
FROM work
WHERE NOT EXISTS (
  SELECT 1 FROM theory_source_files file
  WHERE file.work_id = work.id
    AND file.sha256 = '6f5c0d4c2a61b94e6287d17b51a746d44e9b7dc0225f086b30e141b4f4f7298a'
);

WITH library AS (
  SELECT id FROM theory_libraries WHERE key = 'xinzhili'
)
INSERT INTO theory_cards (
  library_id, canonical_key, canonical_name, aliases, domain, subdomain, card_kind,
  summary, definition, core_claim, mechanism, applicable_context, non_applicable_context,
  observable_signals, common_triggers, automatic_pattern, resource_state, shadow_or_risk,
  growth_direction, epistemic_status, evidence_level, clinical_safety, controversy_notes,
  cultural_context, authority_level, language, status, version, reviewed_at, published_at
)
SELECT id, 'inner_observer', '内在观察者', '["自我观察"]'::jsonb, 'self_awareness',
  'metacognition', 'concept', '观察当下自动反应并恢复选择空间',
  '内在观察者指注意到当下想法、情绪与身体反应，同时暂缓自动行动的能力。',
  '觉察自动反应可为更合适的回应留出空间。',
  '把注意从反应内容短暂移向反应过程，形成观察与行动之间的间隔。',
  '适用于日常压力下识别惯性反应与整理可选行动。',
  '不用于替代专业诊断、危机干预或医疗建议。',
  '["能说出此刻的情绪与身体感受","行动前出现短暂停顿"]'::jsonb,
  '["冲突","被否定","时间压力"]'::jsonb,
  '未经觉察便按熟悉模式回应。', '能同时看见反应与可选行动。',
  '可能把观察误用为压抑、回避或自我评判。', '以好奇和非评判方式观察，再选择下一步。',
  'course_adaptation', 'experiential', 'general', '', '中文课程语境下的自有提炼。',
  5, 'zh-CN', 'published', 1, now(), now()
FROM library
ON CONFLICT (library_id, canonical_key, version) DO UPDATE SET
  canonical_name = EXCLUDED.canonical_name,
  aliases = EXCLUDED.aliases,
  domain = EXCLUDED.domain,
  subdomain = EXCLUDED.subdomain,
  card_kind = EXCLUDED.card_kind,
  summary = EXCLUDED.summary,
  definition = EXCLUDED.definition,
  core_claim = EXCLUDED.core_claim,
  mechanism = EXCLUDED.mechanism,
  applicable_context = EXCLUDED.applicable_context,
  non_applicable_context = EXCLUDED.non_applicable_context,
  observable_signals = EXCLUDED.observable_signals,
  common_triggers = EXCLUDED.common_triggers,
  automatic_pattern = EXCLUDED.automatic_pattern,
  resource_state = EXCLUDED.resource_state,
  shadow_or_risk = EXCLUDED.shadow_or_risk,
  growth_direction = EXCLUDED.growth_direction,
  epistemic_status = EXCLUDED.epistemic_status,
  evidence_level = EXCLUDED.evidence_level,
  clinical_safety = EXCLUDED.clinical_safety,
  cultural_context = EXCLUDED.cultural_context,
  authority_level = EXCLUDED.authority_level,
  language = EXCLUDED.language,
  status = EXCLUDED.status,
  reviewed_at = COALESCE(theory_cards.reviewed_at, EXCLUDED.reviewed_at),
  published_at = COALESCE(theory_cards.published_at, EXCLUDED.published_at),
  update_time = now();

WITH fixture AS (
  SELECT card.id AS card_id, work.id AS work_id, file.id AS file_id
  FROM theory_libraries library
  JOIN theory_cards card ON card.library_id = library.id
    AND card.canonical_key = 'inner_observer' AND card.version = 1
  JOIN theory_source_works work ON work.library_id = library.id
    AND work.canonical_key = 'han_teacher_course'
  JOIN theory_source_files file ON file.work_id = work.id
    AND file.sha256 = '6f5c0d4c2a61b94e6287d17b51a746d44e9b7dc0225f086b30e141b4f4f7298a'
  WHERE library.key = 'xinzhili'
)
INSERT INTO theory_card_sources (
  card_id, work_id, file_id, source_role, chapter, location_label, quotation,
  interpretation_note, extraction_quality, quote_verified
)
SELECT card_id, work_id, file_id, 'primary', '', '课程主题整理', '',
  '基于课程主题的自有简短提炼，不含版权全文。', 0.9500, false
FROM fixture
WHERE NOT EXISTS (
  SELECT 1 FROM theory_card_sources source
  WHERE source.card_id = fixture.card_id
    AND source.work_id = fixture.work_id
    AND source.file_id = fixture.file_id
    AND source.source_role = 'primary'
);

WITH fixture AS (
  SELECT library.id AS library_id, card.id AS card_id
  FROM theory_libraries library
  JOIN theory_cards card ON card.library_id = library.id
    AND card.canonical_key = 'inner_observer' AND card.version = 1
  WHERE library.key = 'xinzhili'
)
INSERT INTO theory_chunks (
  library_id, card_id, chunk_key, chunk_kind, title, content, keywords, tags,
  authority_level, evidence_level, clinical_safety, token_count, content_hash, version, status
)
SELECT library_id, card_id, 'inner_observer.card', 'card', '内在观察者',
  '观察当下自动反应并恢复选择空间。适用于日常压力下暂停、觉察并选择下一步。',
  '["观察","自动反应","选择空间"]'::jsonb, '["self_awareness"]'::jsonb,
  5, 'experiential', 'general', 32,
  'a6ac85e507453ebe1109f78a1fd12e7af284687843a8fa8956a5539f15f0c3d2', 1, 'enabled'
FROM fixture
ON CONFLICT (library_id, chunk_key, version) DO UPDATE SET
  card_id = EXCLUDED.card_id,
  chunk_kind = EXCLUDED.chunk_kind,
  title = EXCLUDED.title,
  content = EXCLUDED.content,
  keywords = EXCLUDED.keywords,
  tags = EXCLUDED.tags,
  authority_level = EXCLUDED.authority_level,
  evidence_level = EXCLUDED.evidence_level,
  clinical_safety = EXCLUDED.clinical_safety,
  token_count = EXCLUDED.token_count,
  content_hash = EXCLUDED.content_hash,
  status = EXCLUDED.status,
  update_time = now();

WITH library AS (
  SELECT id FROM theory_libraries WHERE key = 'xinzhili'
)
UPDATE theory_library_releases release
SET status = 'retired', update_time = now()
FROM library
WHERE release.library_id = library.id
  AND release.status = 'active'
  AND release.version <> 1;

WITH library AS (
  SELECT id FROM theory_libraries WHERE key = 'xinzhili'
)
INSERT INTO theory_library_releases (
  library_id, version, status, embedding_model, embedding_dimensions, retrieval_mode,
  index_version, card_count, chunk_count, build_error, activated_at
)
SELECT id, 1, 'active', '', 1536, 'lexical_only', 'seed-v1', 1, 1, '', now()
FROM library
ON CONFLICT (library_id, version) DO UPDATE SET
  status = 'active',
  embedding_model = EXCLUDED.embedding_model,
  embedding_dimensions = EXCLUDED.embedding_dimensions,
  retrieval_mode = EXCLUDED.retrieval_mode,
  index_version = EXCLUDED.index_version,
  card_count = EXCLUDED.card_count,
  chunk_count = EXCLUDED.chunk_count,
  build_error = EXCLUDED.build_error,
  activated_at = COALESCE(theory_library_releases.activated_at, EXCLUDED.activated_at),
  update_time = now();

WITH fixture AS (
  SELECT release.id AS release_id, card.id AS card_id, chunk.id AS chunk_id
  FROM theory_libraries library
  JOIN theory_library_releases release ON release.library_id = library.id AND release.version = 1
  JOIN theory_cards card ON card.library_id = library.id
    AND card.canonical_key = 'inner_observer' AND card.version = 1
  JOIN theory_chunks chunk ON chunk.library_id = library.id
    AND chunk.chunk_key = 'inner_observer.card' AND chunk.version = 1
  WHERE library.key = 'xinzhili'
)
INSERT INTO theory_release_cards (release_id, card_id, chunk_id)
SELECT release_id, card_id, chunk_id FROM fixture
ON CONFLICT (release_id, card_id, chunk_id) DO NOTHING;

COMMIT;
