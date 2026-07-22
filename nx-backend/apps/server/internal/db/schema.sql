-- PostgreSQL schema for nine-xing admin (RBAC).
-- 幂等：用 IF NOT EXISTS，可重复执行。

CREATE TABLE IF NOT EXISTS users (
  id            BIGSERIAL PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  avatar        TEXT NOT NULL DEFAULT '',
  nickname      TEXT NOT NULL DEFAULT '',
  email         TEXT NOT NULL DEFAULT '',
  phone         TEXT NOT NULL DEFAULT '',
  remark        TEXT NOT NULL DEFAULT '',
  status        INT  NOT NULL DEFAULT 1,
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS token_version INT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS roles (
  id          BIGSERIAL PRIMARY KEY,
  code        TEXT NOT NULL UNIQUE,
  name        TEXT NOT NULL,
  remark      TEXT NOT NULL DEFAULT '',
  status      INT  NOT NULL DEFAULT 1,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS menus (
  id         BIGSERIAL PRIMARY KEY,
  pid        BIGINT NOT NULL DEFAULT 0,
  name       TEXT NOT NULL,
  path       TEXT NOT NULL DEFAULT '',
  component  TEXT NOT NULL DEFAULT '',
  auth_code  TEXT NOT NULL DEFAULT '',
  type       TEXT NOT NULL DEFAULT 'menu',
  status     INT  NOT NULL DEFAULT 1,
  sort       INT  NOT NULL DEFAULT 0,
  meta       JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS user_roles (
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, role_id)
);

CREATE TABLE IF NOT EXISTS role_menus (
  role_id BIGINT NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
  menu_id BIGINT NOT NULL REFERENCES menus(id) ON DELETE CASCADE,
  PRIMARY KEY (role_id, menu_id)
);

CREATE TABLE IF NOT EXISTS site_configs (
  key         TEXT PRIMARY KEY,
  config      JSONB NOT NULL,
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_releases (
  id BIGSERIAL PRIMARY KEY,
  platform TEXT NOT NULL CHECK (platform IN ('android')),
  app_name TEXT NOT NULL DEFAULT '',
  package_name TEXT NOT NULL DEFAULT '',
  icon_path TEXT NOT NULL DEFAULT '',
  version_name TEXT NOT NULL,
  version_code BIGINT NOT NULL CHECK (version_code > 0),
  release_notes TEXT NOT NULL DEFAULT '',
  file_name TEXT NOT NULL,
  file_path TEXT NOT NULL,
  file_size BIGINT NOT NULL CHECK (file_size > 0),
  sha256 TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  UNIQUE(platform, version_code)
);

ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS app_name TEXT NOT NULL DEFAULT '';
ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS package_name TEXT NOT NULL DEFAULT '';
ALTER TABLE app_releases ADD COLUMN IF NOT EXISTS icon_path TEXT NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_releases_one_published_per_platform
  ON app_releases(platform)
  WHERE status = 'published';

CREATE TABLE IF NOT EXISTS signups (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT NOT NULL DEFAULT '',
  contact_type TEXT NOT NULL DEFAULT 'phone',
  contact     TEXT NOT NULL DEFAULT '',
  interest    TEXT NOT NULL DEFAULT '',
  message     TEXT NOT NULL DEFAULT '',
  follow_status TEXT NOT NULL DEFAULT 'pending',
  owner       TEXT NOT NULL DEFAULT '',
  next_follow_time TIMESTAMPTZ,
  follow_note TEXT NOT NULL DEFAULT '',
  visitor_id   TEXT NOT NULL DEFAULT '',
  source_path  TEXT NOT NULL DEFAULT '',
  landing_page TEXT NOT NULL DEFAULT '',
  referrer     TEXT NOT NULL DEFAULT '',
  utm_source   TEXT NOT NULL DEFAULT '',
  utm_medium   TEXT NOT NULL DEFAULT '',
  utm_campaign TEXT NOT NULL DEFAULT '',
  utm_content  TEXT NOT NULL DEFAULT '',
  utm_term     TEXT NOT NULL DEFAULT '',
  ip          TEXT NOT NULL DEFAULT '',
  user_agent  TEXT NOT NULL DEFAULT '',
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS signup_followups (
  id BIGSERIAL PRIMARY KEY,
  signup_id BIGINT NOT NULL REFERENCES signups(id) ON DELETE CASCADE,
  status TEXT NOT NULL DEFAULT '',
  owner TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  next_follow_time TIMESTAMPTZ,
  operator TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS messages (
  id BIGSERIAL PRIMARY KEY,
  type TEXT NOT NULL DEFAULT 'signup',
  title TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL DEFAULT '',
  business_id TEXT NOT NULL DEFAULT '',
  business_type TEXT NOT NULL DEFAULT '',
  target_path TEXT NOT NULL DEFAULT '',
  is_read BOOLEAN NOT NULL DEFAULT false,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS game_results (
  id BIGSERIAL PRIMARY KEY,
  visitor_id TEXT NOT NULL DEFAULT '',
  gender TEXT NOT NULL DEFAULT '',
  result_type INT NOT NULL DEFAULT 0,
  second_type INT NOT NULL DEFAULT 0,
  score JSONB NOT NULL DEFAULT '{}'::jsonb,
  centers JSONB NOT NULL DEFAULT '[]'::jsonb,
  ip TEXT NOT NULL DEFAULT '',
  user_agent TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS upload_assets (
  id           BIGSERIAL PRIMARY KEY,
  key          TEXT NOT NULL UNIQUE,
  name         TEXT NOT NULL DEFAULT '',
  dir          TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT 'application/octet-stream',
  size         BIGINT NOT NULL DEFAULT 0,
  data         BYTEA NOT NULL,
  object_key   TEXT NOT NULL DEFAULT '',
  object_url   TEXT NOT NULL DEFAULT '',
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS site_visits (
  id          BIGSERIAL PRIMARY KEY,
  visitor_id  TEXT NOT NULL DEFAULT '',
  path        TEXT NOT NULL DEFAULT '/',
  title       TEXT NOT NULL DEFAULT '',
  referrer    TEXT NOT NULL DEFAULT '',
  ip          TEXT NOT NULL DEFAULT '',
  user_agent  TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS voice_profiles (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL DEFAULT '',
  provider        TEXT NOT NULL DEFAULT 'minimax',
  voice_id        TEXT NOT NULL DEFAULT '',
  sample_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  sample_url      TEXT NOT NULL DEFAULT '',
  sample_name     TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'draft',
  remark          TEXT NOT NULL DEFAULT '',
  last_error      TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS voice_generations (
  id              BIGSERIAL PRIMARY KEY,
  profile_id      BIGINT REFERENCES voice_profiles(id) ON DELETE SET NULL,
  provider        TEXT NOT NULL DEFAULT 'minimax',
  voice_id        TEXT NOT NULL DEFAULT '',
  text            TEXT NOT NULL DEFAULT '',
  model           TEXT NOT NULL DEFAULT '',
  audio_asset_id  BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  audio_url       TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'success',
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS voice_content_jobs (
  id              BIGSERIAL PRIMARY KEY,
  title           TEXT NOT NULL DEFAULT '',
  source_type     TEXT NOT NULL DEFAULT 'manual',
  source_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  source_name     TEXT NOT NULL DEFAULT '',
  source_url      TEXT NOT NULL DEFAULT '',
  voice_source    TEXT NOT NULL DEFAULT 'official',
  profile_id      BIGINT REFERENCES voice_profiles(id) ON DELETE SET NULL,
  voice_id        TEXT NOT NULL DEFAULT '',
  voice_name      TEXT NOT NULL DEFAULT '',
  model           TEXT NOT NULL DEFAULT '',
  text            TEXT NOT NULL DEFAULT '',
  audio_asset_id  BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  audio_url       TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'success',
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 视频生成（异步）。创建任务后写入 'queued' 行并记录网关 task_id，
-- 轮询完成后下载视频字节经 upload_assets 落库并回填资产/元数据。
CREATE TABLE IF NOT EXISTS video_generations (
  id              BIGSERIAL PRIMARY KEY,
  provider        TEXT NOT NULL DEFAULT 'newapi',
  model           TEXT NOT NULL DEFAULT '',
  prompt          TEXT NOT NULL DEFAULT '',
  image_url       TEXT NOT NULL DEFAULT '',
  used_images     JSONB NOT NULL DEFAULT '[]'::jsonb,
  used_videos     JSONB NOT NULL DEFAULT '[]'::jsonb,
  used_audios     JSONB NOT NULL DEFAULT '[]'::jsonb,
  task_id         TEXT NOT NULL DEFAULT '',
  seconds         INT NOT NULL DEFAULT 15,
  aspect_ratio    TEXT NOT NULL DEFAULT '16:9',
  video_asset_id  BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  video_url       TEXT NOT NULL DEFAULT '',
  duration        DOUBLE PRECISION NOT NULL DEFAULT 0,
  fps             DOUBLE PRECISION NOT NULL DEFAULT 0,
  width           INT NOT NULL DEFAULT 0,
  height          INT NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'queued',
  error_message   TEXT NOT NULL DEFAULT '',
  viewed_flag     BOOLEAN NOT NULL DEFAULT false,
  backup_flag     BOOLEAN NOT NULL DEFAULT false,
  subtitle_remove TEXT NOT NULL DEFAULT '',
  upscaled_flag   BOOLEAN NOT NULL DEFAULT false,
  upscaled_resolution TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS seconds INT NOT NULL DEFAULT 15;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS aspect_ratio TEXT NOT NULL DEFAULT '16:9';
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS used_images JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS used_videos JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS used_audios JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS viewed_flag BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS backup_flag BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS subtitle_remove TEXT NOT NULL DEFAULT '';
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS upscaled_flag BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS upscaled_resolution TEXT NOT NULL DEFAULT '';

-- 资产库:按类型保存可复用的视频生成素材(场景/人物/物品/服装/风格/音频/视频)
CREATE TABLE IF NOT EXISTS video_assets (
  id              BIGSERIAL PRIMARY KEY,
  type            TEXT NOT NULL DEFAULT 'scene',   -- scene/character/prop/outfit/style/audio/video
  name            TEXT NOT NULL DEFAULT '',
  asset_id        BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  url             TEXT NOT NULL DEFAULT '',
  cover_url       TEXT NOT NULL DEFAULT '',
  remark          TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'active',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 视频分析:上传参考视频后异步调用多模态/对话模型，提取场景、人物、资产并生成 seedance2.0 参考提示词。
CREATE TABLE IF NOT EXISTS video_analysis_jobs (
  id              BIGSERIAL PRIMARY KEY,
  video_asset_id  BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  video_url       TEXT NOT NULL DEFAULT '',
  video_name      TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'queued',
  scenes          JSONB NOT NULL DEFAULT '[]'::jsonb,
  characters      JSONB NOT NULL DEFAULT '[]'::jsonb,
  assets          JSONB NOT NULL DEFAULT '[]'::jsonb,
  has_speech      BOOLEAN NOT NULL DEFAULT false,
  audio_summary   TEXT NOT NULL DEFAULT '',
  speech_topics   JSONB NOT NULL DEFAULT '[]'::jsonb,
  speech_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
  speech_outline  JSONB NOT NULL DEFAULT '[]'::jsonb,
  seedance_prompt TEXT NOT NULL DEFAULT '',
  raw_result      TEXT NOT NULL DEFAULT '',
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_analysis_jobs ADD COLUMN IF NOT EXISTS has_speech BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_analysis_jobs ADD COLUMN IF NOT EXISTS audio_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE video_analysis_jobs ADD COLUMN IF NOT EXISTS speech_topics JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_analysis_jobs ADD COLUMN IF NOT EXISTS speech_keywords JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE video_analysis_jobs ADD COLUMN IF NOT EXISTS speech_outline JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 分镜设计:基于已完成的视频分析和给定主题，异步生成可编辑的 Seedance 2.0 分镜方案。
CREATE TABLE IF NOT EXISTS video_storyboards (
  id              BIGSERIAL PRIMARY KEY,
  analysis_job_id BIGINT REFERENCES video_analysis_jobs(id) ON DELETE SET NULL,
  title           TEXT NOT NULL DEFAULT '',
  theme           TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'queued',
  style_guide     JSONB NOT NULL DEFAULT '[]'::jsonb,
  global_prompt   TEXT NOT NULL DEFAULT '',
  shots           JSONB NOT NULL DEFAULT '[]'::jsonb,
  raw_result      TEXT NOT NULL DEFAULT '',
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 视频项目工作台（降低抽卡率的项目制工作流） ============

-- 视频项目：一个项目 = 一部成片，包含角色/场景/分镜与全局风格。
CREATE TABLE IF NOT EXISTS video_projects (
  id              BIGSERIAL PRIMARY KEY,
  name            TEXT NOT NULL DEFAULT '',
  description     TEXT NOT NULL DEFAULT '',
  theme           TEXT NOT NULL DEFAULT '',          -- 创作主题（AI 剧本生成的输入）
  style_guide     TEXT NOT NULL DEFAULT '',          -- 全局风格英文描述，注入每个分镜提示词
  status          TEXT NOT NULL DEFAULT 'active',    -- active/archived
  compose_status  TEXT NOT NULL DEFAULT 'pending',   -- pending/composing/completed/failed
  final_video_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  final_video_url TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 项目角色：引用全局资产库（video_assets），可被项目级配置覆盖。
-- description 为详细英文外貌描述（提示词素材），reference_image_url 为角色标准照（一致性关键）。
CREATE TABLE IF NOT EXISTS video_project_characters (
  id                  BIGSERIAL PRIMARY KEY,
  project_id          BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  asset_id            BIGINT REFERENCES video_assets(id) ON DELETE SET NULL,
  name                TEXT NOT NULL DEFAULT '',
  description         TEXT NOT NULL DEFAULT '',
  reference_image_url TEXT NOT NULL DEFAULT '',
  is_main             BOOLEAN NOT NULL DEFAULT false,
  create_time         TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 项目场景：引用全局资产库，reference_video_url 为运镜参考视频（可选）。
CREATE TABLE IF NOT EXISTS video_project_scenes (
  id                  BIGSERIAL PRIMARY KEY,
  project_id          BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  asset_id            BIGINT REFERENCES video_assets(id) ON DELETE SET NULL,
  name                TEXT NOT NULL DEFAULT '',
  description         TEXT NOT NULL DEFAULT '',
  reference_image_url TEXT NOT NULL DEFAULT '',
  reference_video_url TEXT NOT NULL DEFAULT '',
  create_time         TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time         TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 分镜：核心表。生成时由 PromptBuilder 自动组装提示词与参考素材，
-- image_reference_modes/video_reference_mode 控制参考素材策略（降低抽卡率），
-- end_frame_url 在生成完成后自动提取，供下一分镜作首帧继承。
CREATE TABLE IF NOT EXISTS video_shots (
  id                    BIGSERIAL PRIMARY KEY,
  project_id            BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  order_num             INT NOT NULL DEFAULT 1,
  name                  TEXT NOT NULL DEFAULT '',
  script_original_content TEXT NOT NULL DEFAULT '', -- 分镜剧本原文（liuguang 工作台兼容）
  action_description    TEXT NOT NULL DEFAULT '',    -- 动作描述（中文即可）
  dynamic_description   TEXT NOT NULL DEFAULT '',    -- 视频生成动态描述/提示词草稿
  grid_storyboard_prompt TEXT NOT NULL DEFAULT '',   -- 多格分镜图提示词占位
  storyboard_url        TEXT NOT NULL DEFAULT '',    -- 多格分镜图地址占位
  video_model           TEXT NOT NULL DEFAULT '',    -- 分镜生视频模型
  video_resolution      TEXT NOT NULL DEFAULT '',    -- 分镜生视频分辨率
  sound_and_picture_together TEXT NOT NULL DEFAULT '', -- 音画同出开关/模式
  duration              INT NOT NULL DEFAULT 15,
  aspect_ratio          TEXT NOT NULL DEFAULT '16:9',
  character_ids         JSONB NOT NULL DEFAULT '[]'::jsonb, -- 出场角色 id 数组
  scene_id              BIGINT REFERENCES video_project_scenes(id) ON DELETE SET NULL,
  image_reference_modes JSONB NOT NULL DEFAULT '["prev_frame","character_ref"]'::jsonb, -- prev_frame/character_ref/scene_ref
  video_reference_mode  TEXT NOT NULL DEFAULT 'none', -- none/prev_video/scene_demo
  camera_movement       TEXT NOT NULL DEFAULT '',
  generation_id         BIGINT REFERENCES video_generations(id) ON DELETE SET NULL,
  generated_prompt      TEXT NOT NULL DEFAULT '',
  used_images           JSONB NOT NULL DEFAULT '[]'::jsonb,
  used_videos           JSONB NOT NULL DEFAULT '[]'::jsonb,
  used_audios           JSONB NOT NULL DEFAULT '[]'::jsonb,
  end_frame_url         TEXT NOT NULL DEFAULT '',
  status                TEXT NOT NULL DEFAULT 'draft', -- draft/generating/completed/failed
  error_message         TEXT NOT NULL DEFAULT '',
  create_time           TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time           TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS script_original_content TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS dynamic_description TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS grid_storyboard_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS storyboard_url TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS video_model TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS video_resolution TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS sound_and_picture_together TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS used_audios JSONB NOT NULL DEFAULT '[]'::jsonb;

-- 分镜级参考素材：图片/视频/音频统一上传到 OSS 后关联到具体分镜。
CREATE TABLE IF NOT EXISTS video_shot_assets (
  id            BIGSERIAL PRIMARY KEY,
  shot_id       BIGINT NOT NULL REFERENCES video_shots(id) ON DELETE CASCADE,
  asset_type    TEXT NOT NULL DEFAULT 'image', -- image/video/audio
  object_url    TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL DEFAULT '',
  mime_type     TEXT NOT NULL DEFAULT '',
  size_bytes    BIGINT NOT NULL DEFAULT 0,
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS asset_type TEXT NOT NULL DEFAULT 'image';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS object_url TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS create_time TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT now();
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
      FROM information_schema.columns
     WHERE table_name = 'video_shot_assets'
       AND column_name = 'project_asset_id'
       AND is_nullable = 'NO'
  ) THEN
    ALTER TABLE video_shot_assets ALTER COLUMN project_asset_id DROP NOT NULL;
  END IF;
END $$;

-- 关联生成记录到项目/分镜，便于统计资产成功率。
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES video_projects(id) ON DELETE SET NULL;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS shot_id BIGINT REFERENCES video_shots(id) ON DELETE SET NULL;

-- 资产质量追踪：跨项目统计使用次数与成功率，用于推荐高质量资产。
ALTER TABLE video_assets ADD COLUMN IF NOT EXISTS usage_count INT NOT NULL DEFAULT 0;
ALTER TABLE video_assets ADD COLUMN IF NOT EXISTS success_count INT NOT NULL DEFAULT 0;

-- 成片合成任务：FFmpeg 拼接项目内全部已完成分镜。
CREATE TABLE IF NOT EXISTS video_compose_jobs (
  id              BIGSERIAL PRIMARY KEY,
  project_id      BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  status          TEXT NOT NULL DEFAULT 'queued', -- queued/processing/completed/failed
  transition_type TEXT NOT NULL DEFAULT 'none',   -- none/fade
  music_url       TEXT NOT NULL DEFAULT '',
  final_video_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  final_video_url TEXT NOT NULL DEFAULT '',
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS rag_documents (
  id          BIGSERIAL PRIMARY KEY,
  title       TEXT NOT NULL DEFAULT '',
  content     TEXT NOT NULL DEFAULT '',
  tags        JSONB NOT NULL DEFAULT '[]'::jsonb,
  status      TEXT NOT NULL DEFAULT 'enabled',
  source      TEXT NOT NULL DEFAULT 'manual',
  sort        INT  NOT NULL DEFAULT 0,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============ 阅读管理（H5 文章）============
-- 后台维护、H5 读书页展示的文章。正文为 Markdown 文本。
CREATE TABLE IF NOT EXISTS articles (
  id           BIGSERIAL PRIMARY KEY,
  title        TEXT NOT NULL DEFAULT '',
  summary      TEXT NOT NULL DEFAULT '',
  cover        TEXT NOT NULL DEFAULT '',
  author       TEXT NOT NULL DEFAULT '',
  category     TEXT NOT NULL DEFAULT '',
  content      TEXT NOT NULL DEFAULT '',
  tags         JSONB NOT NULL DEFAULT '[]'::jsonb,
  status       TEXT NOT NULL DEFAULT 'published',
  sort         INT  NOT NULL DEFAULT 0,
  view_count   BIGINT NOT NULL DEFAULT 0,
  publish_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_articles_status_sort ON articles(status, sort ASC, publish_time DESC);
CREATE INDEX IF NOT EXISTS idx_articles_update_time ON articles(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_articles_category ON articles(category);

-- 听书：缓存的音频与音色配置。
ALTER TABLE articles ADD COLUMN IF NOT EXISTS voice_key       TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_asset_id  BIGINT;
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_url       TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_voice_key TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_status    TEXT NOT NULL DEFAULT 'none';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_error     TEXT NOT NULL DEFAULT '';
ALTER TABLE articles ADD COLUMN IF NOT EXISTS audio_time      TIMESTAMPTZ;

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS contact_type TEXT NOT NULL DEFAULT 'phone';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS follow_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS next_follow_time TIMESTAMPTZ;
ALTER TABLE signups ADD COLUMN IF NOT EXISTS follow_note TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE signups ADD COLUMN IF NOT EXISTS visitor_id TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS source_path TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS landing_page TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS referrer TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS utm_source TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS utm_medium TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS utm_campaign TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS utm_content TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS utm_term TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS game_result_id BIGINT REFERENCES game_results(id) ON DELETE SET NULL;
ALTER TABLE site_visits ADD COLUMN IF NOT EXISTS visitor_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_role_menus_role ON role_menus(role_id);
CREATE INDEX IF NOT EXISTS idx_menus_pid ON menus(pid);
CREATE INDEX IF NOT EXISTS idx_signups_create_time ON signups(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_signups_follow_status ON signups(follow_status);
CREATE INDEX IF NOT EXISTS idx_signup_followups_signup ON signup_followups(signup_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_signups_visitor_id ON signups(visitor_id);
CREATE INDEX IF NOT EXISTS idx_signups_next_follow ON signups(next_follow_time) WHERE next_follow_time IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_type_read ON messages(type, is_read, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_game_results_create_time ON game_results(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_game_results_type ON game_results(result_type);
CREATE INDEX IF NOT EXISTS idx_game_results_visitor_time ON game_results(visitor_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_upload_assets_create_time ON upload_assets(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_site_visits_create_time ON site_visits(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_site_visits_visitor_id ON site_visits(visitor_id);
CREATE INDEX IF NOT EXISTS idx_voice_profiles_create_time ON voice_profiles(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_voice_generations_create_time ON voice_generations(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_voice_content_jobs_create_time ON voice_content_jobs(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_assets_create_time ON video_assets(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_assets_type ON video_assets(type);
CREATE INDEX IF NOT EXISTS idx_video_analysis_jobs_create_time ON video_analysis_jobs(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_analysis_jobs_status ON video_analysis_jobs(status);
CREATE INDEX IF NOT EXISTS idx_video_storyboards_create_time ON video_storyboards(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_storyboards_status ON video_storyboards(status);
CREATE INDEX IF NOT EXISTS idx_video_projects_create_time ON video_projects(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_project_characters_project ON video_project_characters(project_id);
CREATE INDEX IF NOT EXISTS idx_video_project_scenes_project ON video_project_scenes(project_id);
CREATE INDEX IF NOT EXISTS idx_video_shots_project_order ON video_shots(project_id, order_num);
CREATE INDEX IF NOT EXISTS idx_video_shots_status ON video_shots(status);
CREATE INDEX IF NOT EXISTS idx_video_shot_assets_shot ON video_shot_assets(shot_id, asset_type);
CREATE INDEX IF NOT EXISTS idx_video_generations_shot ON video_generations(shot_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_video_compose_jobs_project ON video_compose_jobs(project_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_rag_documents_status_sort ON rag_documents(status, sort ASC, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_rag_documents_update_time ON rag_documents(update_time DESC);

-- ============ 小程序（微信）相关表 ============
CREATE TABLE IF NOT EXISTS wx_users (
  id            BIGSERIAL PRIMARY KEY,
  openid        TEXT NOT NULL UNIQUE,
  unionid       TEXT NOT NULL DEFAULT '',
  nickname      TEXT NOT NULL DEFAULT '',
  avatar        TEXT NOT NULL DEFAULT '',
  phone         TEXT NOT NULL DEFAULT '',
  gender        TEXT NOT NULL DEFAULT '',
  main_type     INT  NOT NULL DEFAULT 0,
  member_level  INT  NOT NULL DEFAULT 0,
  channel       TEXT NOT NULL DEFAULT '',
  scene         TEXT NOT NULL DEFAULT '',
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS test_records (
  id          BIGSERIAL PRIMARY KEY,
  wx_user_id  BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  gender      TEXT NOT NULL DEFAULT '',
  result_type INT  NOT NULL,
  second_type INT  NOT NULL DEFAULT 0,
  scores      JSONB NOT NULL DEFAULT '{}'::jsonb,
  centers     JSONB NOT NULL DEFAULT '[]'::jsonb,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bookings (
  id            BIGSERIAL PRIMARY KEY,
  wx_user_id    BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  kind          TEXT NOT NULL DEFAULT 'consult',
  contact_name  TEXT NOT NULL DEFAULT '',
  phone         TEXT NOT NULL DEFAULT '',
  intent        TEXT NOT NULL DEFAULT '',
  preferred_time TEXT NOT NULL DEFAULT '',
  message       TEXT NOT NULL DEFAULT '',
  status        TEXT NOT NULL DEFAULT 'pending',
  signup_id     BIGINT,
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_test_records_user ON test_records(wx_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_bookings_user ON bookings(wx_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_wx_users_create_time ON wx_users(create_time DESC);

-- ============ 支付 / 付费解锁 ============
-- 订单：覆盖「深度报告单次解锁」等付费项。amount 单位为分。
CREATE TABLE IF NOT EXISTS orders (
  id           BIGSERIAL PRIMARY KEY,
  out_trade_no TEXT NOT NULL UNIQUE,                 -- 商户订单号（我方生成）
  wx_user_id   BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  product      TEXT NOT NULL DEFAULT 'report',        -- report | member | ...
  ref_id       BIGINT NOT NULL DEFAULT 0,             -- 关联对象（如 test_records.id）
  title        TEXT NOT NULL DEFAULT '',
  amount       INT  NOT NULL DEFAULT 0,               -- 金额（分）
  status       TEXT NOT NULL DEFAULT 'pending',       -- pending | paid | closed | refunded
  transaction_id TEXT NOT NULL DEFAULT '',            -- 微信支付单号
  prepay_id    TEXT NOT NULL DEFAULT '',
  paid_at      TIMESTAMPTZ,
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 报告解锁：一次成功支付解锁一份测试记录的深度报告。
CREATE TABLE IF NOT EXISTS report_unlocks (
  id            BIGSERIAL PRIMARY KEY,
  wx_user_id    BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  test_record_id BIGINT NOT NULL REFERENCES test_records(id) ON DELETE CASCADE,
  order_id      BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (wx_user_id, test_record_id)
);

CREATE INDEX IF NOT EXISTS idx_orders_user ON orders(wx_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_report_unlocks_user ON report_unlocks(wx_user_id);

-- ============ 芯之力理论库（规范来源、理论卡、检索块与发布快照）============
CREATE TABLE IF NOT EXISTS theory_libraries (
  id BIGSERIAL PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','enabled','disabled')),
  default_language TEXT NOT NULL DEFAULT 'zh-CN',
  current_version INTEGER NOT NULL DEFAULT 0 CHECK (current_version >= 0),
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS theory_library_releases (
  id BIGSERIAL PRIMARY KEY,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE CASCADE,
  version INTEGER NOT NULL CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','building','ready','active','retired','failed')),
  embedding_model TEXT NOT NULL DEFAULT '',
  embedding_dimensions INTEGER NOT NULL DEFAULT 1536 CHECK (embedding_dimensions = 1536),
  retrieval_mode TEXT NOT NULL DEFAULT 'lexical_only' CHECK (retrieval_mode IN ('lexical_only','hybrid')),
  index_version TEXT NOT NULL DEFAULT '',
  card_count INTEGER NOT NULL DEFAULT 0 CHECK (card_count >= 0),
  chunk_count INTEGER NOT NULL DEFAULT 0 CHECK (chunk_count >= 0),
  build_error TEXT NOT NULL DEFAULT '',
  activated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  activated_at TIMESTAMPTZ,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (library_id, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_active_release
  ON theory_library_releases(library_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS theory_source_works (
  id BIGSERIAL PRIMARY KEY,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE CASCADE,
  canonical_key TEXT NOT NULL,
  title TEXT NOT NULL,
  original_title TEXT NOT NULL DEFAULT '',
  authors JSONB NOT NULL DEFAULT '[]'::jsonb,
  editors JSONB NOT NULL DEFAULT '[]'::jsonb,
  translators JSONB NOT NULL DEFAULT '[]'::jsonb,
  publisher TEXT NOT NULL DEFAULT '',
  published_year INTEGER,
  edition TEXT NOT NULL DEFAULT '',
  isbn TEXT NOT NULL DEFAULT '',
  work_type TEXT NOT NULL CHECK (work_type IN ('book','course','handout','article','original_text','research','other')),
  authority_level SMALLINT NOT NULL CHECK (authority_level BETWEEN 1 AND 5),
  epistemic_status TEXT NOT NULL CHECK (epistemic_status IN ('source_text','author_interpretation','course_adaptation','traditional_symbolism','hypothesis','evidence_informed')),
  copyright_scope TEXT NOT NULL CHECK (copyright_scope IN ('metadata_only','internal_excerpt','licensed','full_internal')),
  canonical_work_id BIGINT REFERENCES theory_source_works(id) ON DELETE SET NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  status TEXT NOT NULL DEFAULT 'registered' CHECK (status IN ('registered','extracting','reviewed','archived')),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (library_id, canonical_key)
);

CREATE TABLE IF NOT EXISTS theory_source_files (
  id BIGSERIAL PRIMARY KEY,
  work_id BIGINT NOT NULL REFERENCES theory_source_works(id) ON DELETE CASCADE,
  relative_path TEXT NOT NULL,
  original_filename TEXT NOT NULL,
  file_format TEXT NOT NULL,
  mime_type TEXT NOT NULL DEFAULT '',
  byte_size BIGINT NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
  page_count INTEGER CHECK (page_count IS NULL OR page_count > 0),
  sha256 TEXT NOT NULL,
  duplicate_of_file_id BIGINT REFERENCES theory_source_files(id) ON DELETE SET NULL,
  title_source TEXT NOT NULL CHECK (title_source IN ('filename','metadata','cover','manual')),
  extraction_class TEXT NOT NULL CHECK (extraction_class IN ('text_rich','mixed','image_dominant','cover_only')),
  extraction_status TEXT NOT NULL DEFAULT 'pending' CHECK (extraction_status IN ('pending','extracted','needs_ocr','ocr_running','review_required','failed')),
  extraction_quality NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (extraction_quality BETWEEN 0 AND 1),
  extracted_text_uri TEXT NOT NULL DEFAULT '',
  ocr_text_uri TEXT NOT NULL DEFAULT '',
  extractor_name TEXT NOT NULL DEFAULT '',
  extractor_version TEXT NOT NULL DEFAULT '',
  error_code TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS theory_cards (
  id BIGSERIAL PRIMARY KEY,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE CASCADE,
  canonical_key TEXT NOT NULL,
  canonical_name TEXT NOT NULL,
  aliases JSONB NOT NULL DEFAULT '[]'::jsonb,
  domain TEXT NOT NULL DEFAULT '',
  subdomain TEXT NOT NULL DEFAULT '',
  card_kind TEXT NOT NULL CHECK (card_kind IN ('concept','claim','axis','stage','relation','profile','practice','warning')),
  summary TEXT NOT NULL DEFAULT '',
  definition TEXT NOT NULL DEFAULT '',
  core_claim TEXT NOT NULL DEFAULT '',
  mechanism TEXT NOT NULL DEFAULT '',
  applicable_context TEXT NOT NULL DEFAULT '',
  non_applicable_context TEXT NOT NULL DEFAULT '',
  observable_signals JSONB NOT NULL DEFAULT '[]'::jsonb,
  common_triggers JSONB NOT NULL DEFAULT '[]'::jsonb,
  automatic_pattern TEXT NOT NULL DEFAULT '',
  resource_state TEXT NOT NULL DEFAULT '',
  shadow_or_risk TEXT NOT NULL DEFAULT '',
  growth_direction TEXT NOT NULL DEFAULT '',
  epistemic_status TEXT NOT NULL CHECK (epistemic_status IN ('source_text','author_interpretation','course_adaptation','traditional_symbolism','hypothesis','evidence_informed')),
  evidence_level TEXT NOT NULL CHECK (evidence_level IN ('strong','moderate','limited','traditional','experiential','unknown')),
  clinical_safety TEXT NOT NULL CHECK (clinical_safety IN ('general','caution','restricted','escalate')),
  controversy_notes TEXT NOT NULL DEFAULT '',
  cultural_context TEXT NOT NULL DEFAULT '',
  authority_level SMALLINT NOT NULL CHECK (authority_level BETWEEN 1 AND 5),
  language TEXT NOT NULL DEFAULT 'zh-CN',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','in_review','published','superseded','retired')),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  reviewed_at TIMESTAMPTZ,
  published_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (library_id, canonical_key, version)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_published_card_key
  ON theory_cards(library_id, canonical_key) WHERE status = 'published';

CREATE TABLE IF NOT EXISTS theory_practices (
  id BIGSERIAL PRIMARY KEY,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  goal TEXT NOT NULL,
  estimated_minutes INTEGER NOT NULL DEFAULT 0 CHECK (estimated_minutes >= 0),
  steps JSONB NOT NULL DEFAULT '[]'::jsonb,
  reflection_prompts JSONB NOT NULL DEFAULT '[]'::jsonb,
  expected_feedback JSONB NOT NULL DEFAULT '[]'::jsonb,
  stop_conditions JSONB NOT NULL DEFAULT '[]'::jsonb,
  professional_escalation JSONB NOT NULL DEFAULT '[]'::jsonb,
  contraindications TEXT NOT NULL DEFAULT '',
  practice_schema_version TEXT NOT NULL DEFAULT 'xinzhili.practice.v1',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','in_review','published','superseded','retired')),
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (card_id, version)
);

CREATE TABLE IF NOT EXISTS theory_card_relations (
  id BIGSERIAL PRIMARY KEY,
  from_card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  to_card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  relation_type TEXT NOT NULL CHECK (relation_type IN ('belongs_to','prerequisite','next_stage','supports','extends','contrasts','conflicts','risks','practices')),
  note TEXT NOT NULL DEFAULT '',
  confidence NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (confidence BETWEEN 0 AND 1),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','disabled')),
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  reviewed_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (from_card_id <> to_card_id),
  UNIQUE (from_card_id, to_card_id, relation_type)
);

CREATE TABLE IF NOT EXISTS theory_card_sources (
  id BIGSERIAL PRIMARY KEY,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  work_id BIGINT NOT NULL REFERENCES theory_source_works(id) ON DELETE RESTRICT,
  file_id BIGINT REFERENCES theory_source_files(id) ON DELETE RESTRICT,
  source_role TEXT NOT NULL CHECK (source_role IN ('primary','supporting','extension','counterpoint','controversy')),
  chapter TEXT NOT NULL DEFAULT '',
  page_start INTEGER CHECK (page_start IS NULL OR page_start > 0),
  page_end INTEGER CHECK (page_end IS NULL OR page_end > 0),
  location_label TEXT NOT NULL DEFAULT '',
  quotation TEXT NOT NULL DEFAULT '',
  interpretation_note TEXT NOT NULL DEFAULT '',
  extraction_quality NUMERIC(5,4) NOT NULL DEFAULT 0 CHECK (extraction_quality BETWEEN 0 AND 1),
  quote_verified BOOLEAN NOT NULL DEFAULT false,
  verified_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  verified_at TIMESTAMPTZ,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (page_end IS NULL OR page_start IS NULL OR page_end >= page_start)
);

CREATE OR REPLACE FUNCTION validate_theory_card_source_file_work()
RETURNS TRIGGER AS $$
DECLARE
  linked_work_id BIGINT;
BEGIN
  IF TG_TABLE_NAME = 'theory_card_sources' THEN
    IF NEW.file_id IS NULL THEN
      RETURN NEW;
    END IF;
    SELECT work_id INTO linked_work_id FROM theory_source_files WHERE id = NEW.file_id;
    IF linked_work_id IS DISTINCT FROM NEW.work_id THEN
      RAISE EXCEPTION 'theory card source work_id % does not match file % work_id %',
        NEW.work_id, NEW.file_id, linked_work_id;
    END IF;
  ELSIF EXISTS (
    SELECT 1
    FROM theory_card_sources
    WHERE file_id = NEW.id AND work_id IS DISTINCT FROM NEW.work_id
  ) THEN
    RAISE EXCEPTION 'theory source file % work_id % conflicts with linked card sources',
      NEW.id, NEW.work_id;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'theory_card_sources_file_work_match'
  ) THEN
    CREATE CONSTRAINT TRIGGER theory_card_sources_file_work_match
      AFTER INSERT OR UPDATE ON theory_card_sources
      DEFERRABLE INITIALLY DEFERRED
      FOR EACH ROW EXECUTE FUNCTION validate_theory_card_source_file_work();
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_trigger WHERE tgname = 'theory_source_files_card_source_work_match'
  ) THEN
    CREATE CONSTRAINT TRIGGER theory_source_files_card_source_work_match
      AFTER UPDATE ON theory_source_files
      DEFERRABLE INITIALLY DEFERRED
      FOR EACH ROW EXECUTE FUNCTION validate_theory_card_source_file_work();
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS theory_chunks (
  id BIGSERIAL PRIMARY KEY,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  practice_id BIGINT REFERENCES theory_practices(id) ON DELETE SET NULL,
  chunk_key TEXT NOT NULL,
  chunk_kind TEXT NOT NULL CHECK (chunk_kind IN ('card','practice')),
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
  tags JSONB NOT NULL DEFAULT '[]'::jsonb,
  authority_level SMALLINT NOT NULL CHECK (authority_level BETWEEN 1 AND 5),
  evidence_level TEXT NOT NULL CHECK (evidence_level IN ('strong','moderate','limited','traditional','experiential','unknown')),
  clinical_safety TEXT NOT NULL CHECK (clinical_safety IN ('general','caution','restricted','escalate')),
  token_count INTEGER NOT NULL DEFAULT 0 CHECK (token_count >= 0),
  content_hash TEXT NOT NULL,
  version INTEGER NOT NULL DEFAULT 1 CHECK (version > 0),
  status TEXT NOT NULL DEFAULT 'enabled' CHECK (status IN ('enabled','disabled','retired')),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (library_id, chunk_key, version)
);

-- embedding 列由下方可选 pgvector 初始化块添加；普通 PostgreSQL 只保留元数据。
CREATE TABLE IF NOT EXISTS theory_chunk_embeddings (
  id BIGSERIAL PRIMARY KEY,
  chunk_id BIGINT NOT NULL REFERENCES theory_chunks(id) ON DELETE CASCADE,
  embedding_model TEXT NOT NULL,
  dimensions INTEGER NOT NULL DEFAULT 1536 CHECK (dimensions = 1536),
  content_hash TEXT NOT NULL,
  embedded_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','ready','failed','stale')),
  error_message TEXT NOT NULL DEFAULT '',
  UNIQUE (chunk_id, embedding_model, content_hash)
);

CREATE TABLE IF NOT EXISTS theory_release_cards (
  id BIGSERIAL PRIMARY KEY,
  release_id BIGINT NOT NULL REFERENCES theory_library_releases(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE RESTRICT,
  chunk_id BIGINT NOT NULL REFERENCES theory_chunks(id) ON DELETE RESTRICT,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (release_id, card_id, chunk_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_theory_release_chunk
  ON theory_release_cards(release_id, chunk_id);

CREATE INDEX IF NOT EXISTS idx_theory_libraries_status ON theory_libraries(status);
CREATE INDEX IF NOT EXISTS idx_theory_libraries_update_time ON theory_libraries(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_libraries_created_by ON theory_libraries(created_by);
CREATE INDEX IF NOT EXISTS idx_theory_libraries_updated_by ON theory_libraries(updated_by);
CREATE INDEX IF NOT EXISTS idx_theory_releases_status ON theory_library_releases(status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_releases_activated_by ON theory_library_releases(activated_by);
CREATE INDEX IF NOT EXISTS idx_theory_releases_update_time ON theory_library_releases(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_source_works_library ON theory_source_works(library_id);
CREATE INDEX IF NOT EXISTS idx_theory_source_works_canonical_key ON theory_source_works(canonical_key);
CREATE INDEX IF NOT EXISTS idx_theory_source_works_canonical_work ON theory_source_works(canonical_work_id);
CREATE INDEX IF NOT EXISTS idx_theory_source_works_status ON theory_source_works(status);
CREATE INDEX IF NOT EXISTS idx_theory_source_works_update_time ON theory_source_works(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_source_files_work ON theory_source_files(work_id);
CREATE INDEX IF NOT EXISTS idx_theory_source_files_duplicate ON theory_source_files(duplicate_of_file_id);
CREATE INDEX IF NOT EXISTS idx_theory_source_files_sha256 ON theory_source_files(sha256);
CREATE INDEX IF NOT EXISTS idx_theory_source_files_status ON theory_source_files(extraction_status);
CREATE INDEX IF NOT EXISTS idx_theory_source_files_update_time ON theory_source_files(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_cards_library ON theory_cards(library_id);
CREATE INDEX IF NOT EXISTS idx_theory_cards_canonical_key ON theory_cards(canonical_key);
CREATE INDEX IF NOT EXISTS idx_theory_cards_status ON theory_cards(status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_cards_reviewed_by ON theory_cards(reviewed_by);
CREATE INDEX IF NOT EXISTS idx_theory_cards_created_by ON theory_cards(created_by);
CREATE INDEX IF NOT EXISTS idx_theory_cards_updated_by ON theory_cards(updated_by);
CREATE INDEX IF NOT EXISTS idx_theory_practices_card ON theory_practices(card_id);
CREATE INDEX IF NOT EXISTS idx_theory_practices_status ON theory_practices(status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_relations_from_card ON theory_card_relations(from_card_id);
CREATE INDEX IF NOT EXISTS idx_theory_relations_to_card ON theory_card_relations(to_card_id);
CREATE INDEX IF NOT EXISTS idx_theory_relations_status ON theory_card_relations(status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_relations_created_by ON theory_card_relations(created_by);
CREATE INDEX IF NOT EXISTS idx_theory_relations_reviewed_by ON theory_card_relations(reviewed_by);
CREATE INDEX IF NOT EXISTS idx_theory_card_sources_card ON theory_card_sources(card_id);
CREATE INDEX IF NOT EXISTS idx_theory_card_sources_work ON theory_card_sources(work_id);
CREATE INDEX IF NOT EXISTS idx_theory_card_sources_file ON theory_card_sources(file_id);
CREATE INDEX IF NOT EXISTS idx_theory_card_sources_verified_by ON theory_card_sources(verified_by);
CREATE INDEX IF NOT EXISTS idx_theory_card_sources_update_time ON theory_card_sources(update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_library ON theory_chunks(library_id);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_card ON theory_chunks(card_id);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_practice ON theory_chunks(practice_id);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_key ON theory_chunks(chunk_key);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_status ON theory_chunks(status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_theory_chunks_content_hash ON theory_chunks(content_hash);
CREATE INDEX IF NOT EXISTS idx_theory_embeddings_chunk ON theory_chunk_embeddings(chunk_id);
CREATE INDEX IF NOT EXISTS idx_theory_embeddings_status ON theory_chunk_embeddings(status);
CREATE INDEX IF NOT EXISTS idx_theory_embeddings_content_hash ON theory_chunk_embeddings(content_hash);
CREATE INDEX IF NOT EXISTS idx_theory_release_cards_release ON theory_release_cards(release_id);
CREATE INDEX IF NOT EXISTS idx_theory_release_cards_card ON theory_release_cards(card_id);
CREATE INDEX IF NOT EXISTS idx_theory_release_cards_chunk ON theory_release_cards(chunk_id);

-- 中文词法检索优先使用 pg_trgm；扩展不可用时由应用回退到受限 ILIKE。
DO $$
BEGIN
  BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
  EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm 不可用，跳过理论库 trigram 索引初始化：%', SQLERRM;
    RETURN;
  END;

  IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm') THEN
    EXECUTE $trgm$
      CREATE INDEX IF NOT EXISTS idx_theory_chunks_lexical_trgm
        ON theory_chunks USING gin ((title || ' ' || content || ' ' || keywords::text || ' ' || tags::text) gin_trgm_ops)
    $trgm$;
    EXECUTE $trgm$
      CREATE INDEX IF NOT EXISTS idx_theory_cards_lexical_trgm
        ON theory_cards USING gin ((canonical_name || ' ' || aliases::text) gin_trgm_ops)
    $trgm$;
  END IF;
END $$;

-- ============ pgvector 向量检索（可选，按扩展可用性自动启用）============
-- 使用 pgvector/pgvector:pg16 镜像时自动建扩展、加 embedding 列与近邻索引；
-- 普通 postgres 镜像下扩展文件不存在，整段静默跳过，关键词检索仍可用。
DO $$
BEGIN
  BEGIN
    CREATE EXTENSION IF NOT EXISTS vector;
  EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pgvector 不可用，跳过向量检索初始化：%', SQLERRM;
    RETURN;
  END;

  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'vector') THEN
    -- 1536 维对应多数 text-embedding 模型；换模型时一并调整。
    EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedding vector(1536)';
    EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedding_model TEXT NOT NULL DEFAULT ''''';
    EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedded_at TIMESTAMPTZ';
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_rag_documents_embedding ON rag_documents USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)';
    EXECUTE 'ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)';
    EXECUTE 'CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw ON theory_chunk_embeddings USING hnsw (embedding vector_cosine_ops)';
  END IF;
END $$;

-- ============ 成长心语（分组 + 心语）============
CREATE TABLE IF NOT EXISTS mind_groups (
  id          BIGSERIAL PRIMARY KEY,
  name        TEXT NOT NULL DEFAULT '',
  intro       TEXT NOT NULL DEFAULT '',
  sort        INT  NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'enabled',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mind_quotes (
  id          BIGSERIAL PRIMARY KEY,
  group_id    BIGINT REFERENCES mind_groups(id) ON DELETE SET NULL,
  title       TEXT NOT NULL DEFAULT '',
  content     TEXT NOT NULL DEFAULT '',
  prompt      TEXT NOT NULL DEFAULT '',
  sort        INT  NOT NULL DEFAULT 0,
  status      TEXT NOT NULL DEFAULT 'enabled',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_mind_groups_sort ON mind_groups(status, sort ASC, id ASC);
CREATE INDEX IF NOT EXISTS idx_mind_quotes_group ON mind_quotes(group_id, sort ASC, id ASC);

-- ===== App 用户体系 =====

CREATE TABLE IF NOT EXISTS app_users (
  id              BIGSERIAL PRIMARY KEY,
  phone           TEXT NOT NULL UNIQUE,
  nickname        TEXT NOT NULL DEFAULT '',
  avatar          TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'active',
  member_level    TEXT NOT NULL DEFAULT 'free',
  member_started_at TIMESTAMPTZ,
  member_expires_at TIMESTAMPTZ,
  register_source TEXT NOT NULL DEFAULT 'sms',
  last_login_at   TIMESTAMPTZ,
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_user_preferences (
  id            BIGSERIAL PRIMARY KEY,
  app_user_id   BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  category      TEXT NOT NULL CHECK (category IN ('addressing', 'length', 'tone', 'format', 'interaction', 'custom')),
  slot          TEXT NOT NULL CHECK (slot IN ('addressing.preferred_name', 'addressing.avoid_dear', 'length.detail_level', 'tone.direct', 'tone.formality', 'tone.warmth', 'format.no_lists', 'format.conclusion_first', 'interaction.no_followup', 'custom.communication_style')),
  instruction   TEXT NOT NULL CHECK (char_length(instruction) BETWEEN 1 AND 512),
  source_text   TEXT NOT NULL DEFAULT '' CHECK (char_length(source_text) <= 1024),
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (split_part(slot, '.', 1) = category),
  UNIQUE (app_user_id, slot)
);

CREATE INDEX IF NOT EXISTS idx_app_user_preferences_user_order
  ON app_user_preferences(app_user_id, category, slot);
CREATE INDEX IF NOT EXISTS idx_app_user_preferences_category
  ON app_user_preferences(category, app_user_id);

CREATE TABLE IF NOT EXISTS app_sms_codes (
  id          BIGSERIAL PRIMARY KEY,
  phone       TEXT NOT NULL,
  code_hash   TEXT NOT NULL,
  expires_at  TIMESTAMPTZ NOT NULL,
  used        BOOLEAN NOT NULL DEFAULT false,
  send_ip     TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_sms_codes_phone ON app_sms_codes(phone, used, expires_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_users_insights_order
  ON app_users(create_time DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_app_users_status_member_order
  ON app_users(status, member_level, create_time DESC, id DESC);

CREATE TABLE IF NOT EXISTS app_refresh_tokens (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  token_hash  TEXT NOT NULL UNIQUE,
  device_info TEXT NOT NULL DEFAULT '',
  expires_at  TIMESTAMPTZ NOT NULL,
  revoked     BOOLEAN NOT NULL DEFAULT false,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_refresh_tokens_user ON app_refresh_tokens(app_user_id, revoked);

-- ===== 九型测试与卡片 =====

CREATE TABLE IF NOT EXISTS app_quiz_questions (
  id          BIGSERIAL PRIMARY KEY,
  sort        INT  NOT NULL DEFAULT 0,
  body        TEXT NOT NULL,
  options     JSONB NOT NULL DEFAULT '[]'::jsonb,
  dimension   TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'enabled',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_quiz_submissions (
  id           BIGSERIAL PRIMARY KEY,
  app_user_id  BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  answers      JSONB NOT NULL DEFAULT '[]'::jsonb,
  result       JSONB NOT NULL DEFAULT '{}'::jsonb,
  primary_type INT NOT NULL DEFAULT 0,
  wing_type    INT NOT NULL DEFAULT 0,
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_quiz_submissions_user ON app_quiz_submissions(app_user_id, create_time DESC);

CREATE TABLE IF NOT EXISTS app_user_cards (
  id           BIGSERIAL PRIMARY KEY,
  app_user_id  BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_type    TEXT NOT NULL DEFAULT 'primary',
  name         TEXT NOT NULL DEFAULT '',
  relation     TEXT NOT NULL DEFAULT '',
  enneagram    INT NOT NULL DEFAULT 0,
  wing         INT NOT NULL DEFAULT 0,
  profile      JSONB NOT NULL DEFAULT '{}'::jsonb,
  status       TEXT NOT NULL DEFAULT 'active',
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_user_cards_user ON app_user_cards(app_user_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_user_cards_primary ON app_user_cards(app_user_id) WHERE card_type = 'primary' AND status = 'active';

-- ----- 九型测试与卡片：增量迁移（幂等，老库补列）-----
ALTER TABLE app_quiz_questions   ADD COLUMN IF NOT EXISTS quiz_version  TEXT        NOT NULL DEFAULT 'v1';
ALTER TABLE app_quiz_questions   ADD COLUMN IF NOT EXISTS update_time   TIMESTAMPTZ NOT NULL DEFAULT now();

ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS quiz_version   TEXT  NOT NULL DEFAULT 'v1';
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS gender         TEXT  NOT NULL DEFAULT '';
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS score          JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS adjusted_score JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS centers        JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS wing_type      INT   NOT NULL DEFAULT 0;
ALTER TABLE app_quiz_submissions ADD COLUMN IF NOT EXISTS second_type    INT   NOT NULL DEFAULT 0;

ALTER TABLE app_user_cards       ADD COLUMN IF NOT EXISTS submission_id BIGINT REFERENCES app_quiz_submissions(id) ON DELETE SET NULL;

-- ----- App 关系合盘：缓存两张用户卡片的本地确定性合盘结果 -----
CREATE TABLE IF NOT EXISTS app_compatibility_reports (
  id              BIGSERIAL PRIMARY KEY,
  app_user_id     BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_a_id       BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  card_b_id       BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  card_a_name     TEXT NOT NULL DEFAULT '',
  card_b_name     TEXT NOT NULL DEFAULT '',
  card_a_type     INT NOT NULL DEFAULT 0,
  card_b_type     INT NOT NULL DEFAULT 0,
  summary         TEXT NOT NULL DEFAULT '',
  highlights      JSONB NOT NULL DEFAULT '[]'::jsonb,
  conflict_points JSONB NOT NULL DEFAULT '[]'::jsonb,
  suggestions     JSONB NOT NULL DEFAULT '[]'::jsonb,
  is_full         BOOLEAN NOT NULL DEFAULT true,
  algorithm_version TEXT NOT NULL DEFAULT 'v1',
  relation_level    TEXT NOT NULL DEFAULT '',
  scores            JSONB NOT NULL DEFAULT '{}'::jsonb,
  explain_tags      JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence          JSONB NOT NULL DEFAULT '[]'::jsonb,
  generated_detail  TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS algorithm_version TEXT  NOT NULL DEFAULT 'v1';
ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS relation_level    TEXT  NOT NULL DEFAULT '';
ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS scores            JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS explain_tags      JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS evidence          JSONB NOT NULL DEFAULT '[]'::jsonb;
ALTER TABLE app_compatibility_reports ADD COLUMN IF NOT EXISTS generated_detail  TEXT  NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_app_compatibility_reports_user
  ON app_compatibility_reports(app_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_app_compatibility_reports_cards
  ON app_compatibility_reports(app_user_id, card_a_id, card_b_id);

-- ----- App 问答会话：存储每张卡的对话历史 -----
CREATE TABLE IF NOT EXISTS app_chat_sessions (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id     BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  context_summary TEXT NOT NULL DEFAULT '',
  context_summary_through_message_id BIGINT NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary_through_message_id BIGINT NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_user ON app_chat_sessions(app_user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_card ON app_chat_sessions(card_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS app_chat_messages (
  id         BIGSERIAL PRIMARY KEY,
  session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,
  role       TEXT NOT NULL,           -- 'user' | 'assistant'
  content    TEXT NOT NULL DEFAULT '',
  sources    JSONB NOT NULL DEFAULT '[]'::jsonb,
  favorite   BOOLEAN NOT NULL DEFAULT false,  -- 是否被用户收藏
  feedback   TEXT NOT NULL DEFAULT '',        -- 'helpful' | 'inaccurate' | 'continue' | ''
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS feedback TEXT NOT NULL DEFAULT '';
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS message_type TEXT NOT NULL DEFAULT 'text';
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_duration_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS transcript TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_app_chat_messages_session ON app_chat_messages(session_id, create_time);
CREATE INDEX IF NOT EXISTS idx_app_chat_messages_favorite ON app_chat_messages(favorite) WHERE favorite = true;

-- ----- App 专属记忆：用户可见、可删除/停用的卡片记忆 -----
CREATE TABLE IF NOT EXISTS app_memories (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id     BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  content     TEXT NOT NULL DEFAULT '',
  status      TEXT NOT NULL DEFAULT 'active',
  source_time TIMESTAMPTZ,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_memories_card ON app_memories(app_user_id, card_id, status, update_time DESC);
CREATE INDEX IF NOT EXISTS idx_app_memories_user_status_update
  ON app_memories(app_user_id, status, update_time DESC, id DESC);

-- ----- App 埋点事件：Flutter App 鉴权后上报的最小事件流 -----
CREATE TABLE IF NOT EXISTS app_analytics_events (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
  event       TEXT NOT NULL DEFAULT '',
  params      JSONB NOT NULL DEFAULT '{}'::jsonb,
  client_ts   TIMESTAMPTZ,
  ip          TEXT NOT NULL DEFAULT '',
  user_agent  TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_analytics_events_user_time
  ON app_analytics_events(app_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_app_analytics_events_event_time
  ON app_analytics_events(event, create_time DESC);

-- ----- App 权益订单：App 用户独立订单，真实支付回调接入后发放权益 -----
CREATE TABLE IF NOT EXISTS app_orders (
  id              BIGSERIAL PRIMARY KEY,
  out_trade_no    TEXT NOT NULL UNIQUE,
  app_user_id     BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  product_id      TEXT NOT NULL DEFAULT '',
  title           TEXT NOT NULL DEFAULT '',
  amount          INT NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'pending',
  transaction_id  TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at         TIMESTAMPTZ,
  activation_at TIMESTAMPTZ,
  membership_expires_at TIMESTAMPTZ
);

ALTER TABLE app_users ADD COLUMN IF NOT EXISTS member_started_at TIMESTAMPTZ;
ALTER TABLE app_users ADD COLUMN IF NOT EXISTS member_expires_at TIMESTAMPTZ;
ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS activation_at TIMESTAMPTZ;
ALTER TABLE app_orders ADD COLUMN IF NOT EXISTS membership_expires_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_app_orders_user ON app_orders(app_user_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_app_orders_status ON app_orders(status, create_time DESC);

-- ----- App 每日成长打卡：记录用户每天完成的成长练习 -----
CREATE TABLE IF NOT EXISTS app_daily_checkins (
  id           BIGSERIAL PRIMARY KEY,
  app_user_id  BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  checkin_date DATE NOT NULL,           -- 打卡日期（Asia/Shanghai）
  main_type    INT NOT NULL DEFAULT 0,  -- 打卡时的主型，用于回顾
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_checkins_user_date
  ON app_daily_checkins(app_user_id, checkin_date);

-- ----- App 每日画像校准：每天 5 道题，累计 100 题触发复评 -----
CREATE TABLE IF NOT EXISTS app_daily_quiz_questions (
  id           BIGSERIAL PRIMARY KEY,
  sort         INT NOT NULL DEFAULT 0,
  body         TEXT NOT NULL DEFAULT '',
  options      JSONB NOT NULL DEFAULT '[]'::jsonb,
  dimension    TEXT NOT NULL DEFAULT '',
  type_weights JSONB NOT NULL DEFAULT '{}'::jsonb,
  status       TEXT NOT NULL DEFAULT 'active',
  create_time  TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_daily_quiz_questions_active_sort
  ON app_daily_quiz_questions(status, sort, id);

CREATE TABLE IF NOT EXISTS app_daily_quiz_sets (
  id             BIGSERIAL PRIMARY KEY,
  quiz_date      DATE NOT NULL UNIQUE,
  status         TEXT NOT NULL DEFAULT 'pending',
  source         TEXT NOT NULL DEFAULT '',
  model_provider TEXT NOT NULL DEFAULT '',
  model_name     TEXT NOT NULL DEFAULT '',
  prompt         TEXT NOT NULL DEFAULT '',
  raw_response   TEXT NOT NULL DEFAULT '',
  question_ids   JSONB NOT NULL DEFAULT '[]'::jsonb,
  error_message  TEXT NOT NULL DEFAULT '',
  generated_at   TIMESTAMPTZ,
  published_at   TIMESTAMPTZ,
  pushed_at      TIMESTAMPTZ,
  create_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_quiz_sets_date
  ON app_daily_quiz_sets(quiz_date);

CREATE TABLE IF NOT EXISTS app_daily_quiz_question_versions (
  id             BIGSERIAL PRIMARY KEY,
  set_id         BIGINT NOT NULL REFERENCES app_daily_quiz_sets(id) ON DELETE CASCADE,
  question_id    BIGINT NOT NULL REFERENCES app_daily_quiz_questions(id) ON DELETE RESTRICT,
  slot_no        INT NOT NULL DEFAULT 1,
  version_no     INT NOT NULL DEFAULT 1,
  is_active      BOOLEAN NOT NULL DEFAULT true,
  body           TEXT NOT NULL DEFAULT '',
  options        JSONB NOT NULL DEFAULT '[]'::jsonb,
  dimension      TEXT NOT NULL DEFAULT '',
  type_weights   JSONB NOT NULL DEFAULT '{}'::jsonb,
  source         TEXT NOT NULL DEFAULT '',
  model_provider TEXT NOT NULL DEFAULT '',
  model_name     TEXT NOT NULL DEFAULT '',
  prompt         TEXT NOT NULL DEFAULT '',
  raw_response   TEXT NOT NULL DEFAULT '',
  operator       TEXT NOT NULL DEFAULT '',
  replace_reason TEXT NOT NULL DEFAULT '',
  create_time    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_quiz_question_versions_slot_version
  ON app_daily_quiz_question_versions(set_id, slot_no, version_no);
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_quiz_question_versions_active
  ON app_daily_quiz_question_versions(set_id, slot_no)
  WHERE is_active = true;

CREATE TABLE IF NOT EXISTS app_daily_quiz_batches (
  id             BIGSERIAL PRIMARY KEY,
  app_user_id    BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id        BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  quiz_date      DATE NOT NULL,
  round_no       INT NOT NULL DEFAULT 1,
  question_ids   JSONB NOT NULL DEFAULT '[]'::jsonb,
  answered_count INT NOT NULL DEFAULT 0,
  completed      BOOLEAN NOT NULL DEFAULT false,
  completed_at   TIMESTAMPTZ,
  push_claimed_at TIMESTAMPTZ,
  push_sent_at   TIMESTAMPTZ,
  create_time    TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_quiz_batches_card_date_round
  ON app_daily_quiz_batches(card_id, quiz_date, round_no);
CREATE INDEX IF NOT EXISTS idx_app_daily_quiz_batches_user_date
  ON app_daily_quiz_batches(app_user_id, quiz_date DESC, id DESC);

CREATE TABLE IF NOT EXISTS app_daily_quiz_answers (
  id          BIGSERIAL PRIMARY KEY,
  batch_id    BIGINT NOT NULL REFERENCES app_daily_quiz_batches(id) ON DELETE CASCADE,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id     BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  round_no    INT NOT NULL DEFAULT 1,
  question_id BIGINT NOT NULL,
  option_id   TEXT NOT NULL DEFAULT '',
  type_delta  JSONB NOT NULL DEFAULT '{}'::jsonb,
  answered_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_daily_quiz_answers_batch_question
  ON app_daily_quiz_answers(batch_id, question_id);
CREATE INDEX IF NOT EXISTS idx_app_daily_quiz_answers_card_round
  ON app_daily_quiz_answers(card_id, round_no, answered_at, id);

CREATE TABLE IF NOT EXISTS app_profile_evidence (
  id              BIGSERIAL PRIMARY KEY,
  app_user_id     BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id         BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  round_no        INT NOT NULL DEFAULT 1,
  source_type     TEXT NOT NULL DEFAULT '',
  source_id       BIGINT,
  evidence_text   TEXT NOT NULL DEFAULT '',
  trait_scores    JSONB NOT NULL DEFAULT '{}'::jsonb,
  type_scores     JSONB NOT NULL DEFAULT '{}'::jsonb,
  emotion_scores  JSONB NOT NULL DEFAULT '{}'::jsonb,
  behavior_scores JSONB NOT NULL DEFAULT '{}'::jsonb,
  confidence      NUMERIC NOT NULL DEFAULT 0,
  status          TEXT NOT NULL DEFAULT 'active',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_app_profile_evidence_card_round
  ON app_profile_evidence(card_id, round_no, status, create_time DESC);

CREATE TABLE IF NOT EXISTS app_reassessment_jobs (
  id                   BIGSERIAL PRIMARY KEY,
  app_user_id          BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id              BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  round_no             INT NOT NULL DEFAULT 1,
  trigger_reason       TEXT NOT NULL DEFAULT '',
  evidence_window_start TIMESTAMPTZ,
  evidence_window_end   TIMESTAMPTZ,
  daily_answer_count   INT NOT NULL DEFAULT 0,
  chat_evidence_count  INT NOT NULL DEFAULT 0,
  voice_evidence_count INT NOT NULL DEFAULT 0,
  behavior_evidence_count INT NOT NULL DEFAULT 0,
  old_main_type        INT NOT NULL DEFAULT 0,
  suggested_main_type  INT NOT NULL DEFAULT 0,
  confidence           NUMERIC NOT NULL DEFAULT 0,
  status               TEXT NOT NULL DEFAULT 'pending',
  report_json          JSONB NOT NULL DEFAULT '{}'::jsonb,
  push_claimed_at      TIMESTAMPTZ,
  push_sent_at         TIMESTAMPTZ,
  create_time          TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time          TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_daily_quiz_batches ADD COLUMN IF NOT EXISTS push_claimed_at TIMESTAMPTZ;
ALTER TABLE app_daily_quiz_batches ADD COLUMN IF NOT EXISTS push_sent_at TIMESTAMPTZ;
ALTER TABLE app_reassessment_jobs ADD COLUMN IF NOT EXISTS push_claimed_at TIMESTAMPTZ;
ALTER TABLE app_reassessment_jobs ADD COLUMN IF NOT EXISTS push_sent_at TIMESTAMPTZ;

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_reassessment_jobs_open
  ON app_reassessment_jobs(card_id, round_no)
  WHERE status IN ('pending','generating','generated');
CREATE INDEX IF NOT EXISTS idx_app_reassessment_jobs_card_time
  ON app_reassessment_jobs(card_id, create_time DESC, id DESC);

CREATE TABLE IF NOT EXISTS app_profile_versions (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id     BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  version     INT NOT NULL DEFAULT 1,
  main_type   INT NOT NULL DEFAULT 0,
  wing_type   INT NOT NULL DEFAULT 0,
  profile_json JSONB NOT NULL DEFAULT '{}'::jsonb,
  source      TEXT NOT NULL DEFAULT 'initial_quiz',
  confidence  NUMERIC NOT NULL DEFAULT 0,
  is_active   BOOLEAN NOT NULL DEFAULT true,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_app_profile_versions_active
  ON app_profile_versions(card_id)
  WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_app_profile_versions_card_version
  ON app_profile_versions(card_id, version DESC);

-- ----- App 推送设备令牌：存储用户的 JPush Registration ID -----
CREATE TABLE IF NOT EXISTS app_device_tokens (
  id              BIGSERIAL PRIMARY KEY,
  app_user_id     BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  registration_id TEXT NOT NULL,
  platform        TEXT NOT NULL DEFAULT 'android',
  device_info     TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

DELETE FROM app_device_tokens a
USING app_device_tokens b
WHERE a.registration_id = b.registration_id
  AND (a.update_time, a.id) < (b.update_time, b.id);
DROP INDEX IF EXISTS idx_app_device_tokens_user_regid;
CREATE UNIQUE INDEX IF NOT EXISTS idx_app_device_tokens_registration_id
  ON app_device_tokens(registration_id);
CREATE INDEX IF NOT EXISTS idx_app_device_tokens_user
  ON app_device_tokens(app_user_id);

-- ----- 推送通知记录：后台发送的推送历史 -----
CREATE TABLE IF NOT EXISTS push_notifications (
  id            BIGSERIAL PRIMARY KEY,
  title         TEXT NOT NULL DEFAULT '',
  content       TEXT NOT NULL DEFAULT '',
  target_type   TEXT NOT NULL DEFAULT 'all',
  target_value  TEXT NOT NULL DEFAULT '',
  deep_link     TEXT NOT NULL DEFAULT '',
  sent_count    INT  NOT NULL DEFAULT 0,
  status        TEXT NOT NULL DEFAULT 'pending',
  error_message TEXT NOT NULL DEFAULT '',
  operator      TEXT NOT NULL DEFAULT '',
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_push_notifications_time ON push_notifications(create_time DESC);


-- ----- 后台高风险操作审计：记录关键管理动作，便于追溯 -----
CREATE TABLE IF NOT EXISTS admin_operation_logs (
  id            BIGSERIAL PRIMARY KEY,
  operator_id   BIGINT,
  operator_name TEXT NOT NULL DEFAULT '',
  action        TEXT NOT NULL DEFAULT '',
  target_type   TEXT NOT NULL DEFAULT '',
  target_id     TEXT NOT NULL DEFAULT '',
  ip            TEXT NOT NULL DEFAULT '',
  user_agent    TEXT NOT NULL DEFAULT '',
  before_data   JSONB NOT NULL DEFAULT '{}'::jsonb,
  after_data    JSONB NOT NULL DEFAULT '{}'::jsonb,
  summary       TEXT NOT NULL DEFAULT '',
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_time
  ON admin_operation_logs(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_target
  ON admin_operation_logs(target_type, target_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_admin_operation_logs_operator
  ON admin_operation_logs(operator_id, create_time DESC);


-- ----- 分布式固定窗口限流：跨实例共享登录/短信/公开写入限流状态 -----
CREATE TABLE IF NOT EXISTS request_rate_limits (
  scope      TEXT NOT NULL,
  key        TEXT NOT NULL,
  count      INT  NOT NULL DEFAULT 0,
  expires_at TIMESTAMPTZ NOT NULL,
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (scope, key)
);
CREATE INDEX IF NOT EXISTS idx_request_rate_limits_expires ON request_rate_limits(expires_at);
