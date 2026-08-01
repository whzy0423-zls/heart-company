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
  model           TEXT NOT NULL DEFAULT '',
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

ALTER TABLE voice_profiles ADD COLUMN IF NOT EXISTS model TEXT NOT NULL DEFAULT '';
UPDATE voice_profiles
   SET model = CASE
     WHEN provider = 'bailian' THEN 'MiniMax/speech-2.8-turbo'
     WHEN provider = 'minimax' THEN 'speech-02-hd'
     ELSE model
   END
 WHERE model = '';

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
  shot_revision   INT NOT NULL DEFAULT 0,
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
  script_content  TEXT NOT NULL DEFAULT '',
  script_revision INT NOT NULL DEFAULT 0,
  style_guide     TEXT NOT NULL DEFAULT '',          -- 全局风格英文描述，注入每个分镜提示词
  status          TEXT NOT NULL DEFAULT 'active',    -- active/archived
  compose_status  TEXT NOT NULL DEFAULT 'pending',   -- pending/composing/completed/failed
  final_video_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  final_video_url TEXT NOT NULL DEFAULT '',
  final_video_input_hash TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_content TEXT NOT NULL DEFAULT '';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS final_video_input_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS confirmed_script_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_step TEXT NOT NULL DEFAULT 'script';
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_mode TEXT NOT NULL DEFAULT 'guided' CHECK (workflow_mode IN ('guided','autopilot'));
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS workflow_settings_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS asset_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS script_confirmed_at TIMESTAMPTZ;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS breakdown_confirmed_at TIMESTAMPTZ;
ALTER TABLE video_projects ADD COLUMN IF NOT EXISTS storyboard_confirmed_at TIMESTAMPTZ;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_projects_workflow_mode_check'
  ) THEN
    ALTER TABLE video_projects
      ADD CONSTRAINT video_projects_workflow_mode_check
      CHECK (workflow_mode IN ('guided','autopilot'));
  END IF;
END $$;

-- 剧本拆解版本：AI 结果先保存为可编辑草稿，确认后再物化项目资产。
CREATE TABLE IF NOT EXISTS video_project_breakdowns (
  id                     BIGSERIAL PRIMARY KEY,
  project_id             BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  version                INT NOT NULL CHECK (version > 0),
  revision               INT NOT NULL DEFAULT 1 CHECK (revision > 0),
  status                 TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','superseded','failed')),
  source_script_revision INT NOT NULL DEFAULT 0,
  script_snapshot        TEXT NOT NULL DEFAULT '',
  characters             JSONB NOT NULL DEFAULT '[]'::jsonb,
  scenes                 JSONB NOT NULL DEFAULT '[]'::jsonb,
  props                  JSONB NOT NULL DEFAULT '[]'::jsonb,
  outfits                JSONB NOT NULL DEFAULT '[]'::jsonb,
  styles                 JSONB NOT NULL DEFAULT '[]'::jsonb,
  story_beats            JSONB NOT NULL DEFAULT '[]'::jsonb,
  raw_result             TEXT NOT NULL DEFAULT '',
  error_message          TEXT NOT NULL DEFAULT '',
  create_time            TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_breakdowns_version
  ON video_project_breakdowns(project_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_breakdowns_confirmed
  ON video_project_breakdowns(project_id) WHERE status='confirmed';

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

ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS visual_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'legacy' CHECK (source IN ('ai','manual','library','legacy'));
ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'));
ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS required BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS breakdown_item_key TEXT NOT NULL DEFAULT '';
ALTER TABLE video_project_characters ADD COLUMN IF NOT EXISTS source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_project_characters_source_check'
  ) THEN
    ALTER TABLE video_project_characters
      ADD CONSTRAINT video_project_characters_source_check
      CHECK (source IN ('ai','manual','library','legacy'));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_project_characters_status_check'
  ) THEN
    ALTER TABLE video_project_characters
      ADD CONSTRAINT video_project_characters_status_check
      CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'));
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_characters_breakdown_key
  ON video_project_characters(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';

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

ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS visual_prompt TEXT NOT NULL DEFAULT '';
ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'legacy' CHECK (source IN ('ai','manual','library','legacy'));
ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'));
ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS required BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS breakdown_item_key TEXT NOT NULL DEFAULT '';
ALTER TABLE video_project_scenes ADD COLUMN IF NOT EXISTS source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_project_scenes_source_check'
  ) THEN
    ALTER TABLE video_project_scenes
      ADD CONSTRAINT video_project_scenes_source_check
      CHECK (source IN ('ai','manual','library','legacy'));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_project_scenes_status_check'
  ) THEN
    ALTER TABLE video_project_scenes
      ADD CONSTRAINT video_project_scenes_status_check
      CHECK (status IN ('draft','confirmed','generating','ready','failed','detached'));
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_scenes_breakdown_key
  ON video_project_scenes(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';

-- 物品、服饰和风格资产；人物与场景继续使用原表保持兼容。
CREATE TABLE IF NOT EXISTS video_project_assets (
  id                  BIGSERIAL PRIMARY KEY,
  project_id          BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  type                TEXT NOT NULL CHECK (type IN ('prop','outfit','style')),
  breakdown_item_key  TEXT NOT NULL DEFAULT '',
  source_breakdown_id BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL,
  name                TEXT NOT NULL DEFAULT '',
  description         TEXT NOT NULL DEFAULT '',
  visual_prompt       TEXT NOT NULL DEFAULT '',
  usage_note          TEXT NOT NULL DEFAULT '',
  required            BOOLEAN NOT NULL DEFAULT false,
  global_asset_id     BIGINT REFERENCES video_assets(id) ON DELETE SET NULL,
  reference_image_url TEXT NOT NULL DEFAULT '',
  source              TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('ai','manual','library','legacy')),
  status              TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','generating','ready','failed','detached')),
  metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
  create_time         TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_assets_breakdown_key
  ON video_project_assets(project_id, breakdown_item_key) WHERE breakdown_item_key<>'';

-- 五类项目资产共用候选图记录；target_type + target_id 定位具体资产表。
CREATE TABLE IF NOT EXISTS video_project_asset_candidates (
  id                    BIGSERIAL PRIMARY KEY,
  project_id            BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  target_type           TEXT NOT NULL CHECK (target_type IN ('character','scene','prop','outfit','style')),
  target_id             BIGINT NOT NULL,
  prompt                TEXT NOT NULL DEFAULT '',
  image_asset_id        BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  image_url             TEXT NOT NULL DEFAULT '',
  source                TEXT NOT NULL DEFAULT 'generated' CHECK (source IN ('generated','upload','library','legacy')),
  generation_request_id TEXT NOT NULL DEFAULT '',
  status                TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','generating','ready','failed')),
  error_message         TEXT NOT NULL DEFAULT '',
  selected              BOOLEAN NOT NULL DEFAULT false,
  create_time           TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_asset_candidates_selected
  ON video_project_asset_candidates(target_type, target_id) WHERE selected=true;
CREATE INDEX IF NOT EXISTS idx_video_project_asset_candidates_project
  ON video_project_asset_candidates(project_id, target_type, target_id, create_time DESC);

-- AI 分镜草稿版本；确认时按稳定 source_key 物化到 video_shots。
CREATE TABLE IF NOT EXISTS video_project_storyboard_versions (
  id                        BIGSERIAL PRIMARY KEY,
  project_id                BIGINT NOT NULL REFERENCES video_projects(id) ON DELETE CASCADE,
  version                   INT NOT NULL CHECK (version > 0),
  revision                  INT NOT NULL DEFAULT 1 CHECK (revision > 0),
  status                    TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','confirmed','superseded','failed')),
  source_script_revision    INT NOT NULL DEFAULT 0,
  source_breakdown_id       BIGINT REFERENCES video_project_breakdowns(id) ON DELETE SET NULL,
  source_asset_revision     INT NOT NULL DEFAULT 0,
  source_capability_version TEXT NOT NULL DEFAULT '',
  baseline_storyboard_id    BIGINT REFERENCES video_project_storyboard_versions(id) ON DELETE SET NULL,
  shots                     JSONB NOT NULL DEFAULT '[]'::jsonb,
  raw_result                TEXT NOT NULL DEFAULT '',
  error_message             TEXT NOT NULL DEFAULT '',
  create_time               TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_storyboard_versions_version
  ON video_project_storyboard_versions(project_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS uq_video_project_storyboard_versions_confirmed
  ON video_project_storyboard_versions(project_id) WHERE status='confirmed';

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
  generation_revision   INT NOT NULL DEFAULT 0,
  selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL,
  source_key            TEXT NOT NULL DEFAULT '',
  source_script_revision INT NOT NULL DEFAULT 0,
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
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS generation_revision INT NOT NULL DEFAULT 0;
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_key TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_script_revision INT NOT NULL DEFAULT 0;

-- Existing projects select only a successful, usable terminal generation.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
      FROM information_schema.columns
     WHERE table_name = 'video_shots'
       AND column_name = 'selected_generation_id'
  ) THEN
    ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL;

    UPDATE video_shots AS shots
       SET selected_generation_id = generations.id
      FROM video_generations AS generations
     WHERE generations.id = shots.generation_id
       AND generations.status IN ('completed','succeeded')
       AND generations.video_url <> '';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_shots_project_source_key
ON video_shots(project_id, source_key) WHERE source_key <> '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS generation_mode TEXT NOT NULL DEFAULT 'reference' CHECK (generation_mode IN ('reference','edit','extend'));
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_override TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_version TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS audio_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS prompt_diagnostics JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS source_key TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS archived_at TIMESTAMPTZ;
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL;
ALTER TABLE video_shots ADD COLUMN IF NOT EXISTS selected_generation_ack_hash TEXT NOT NULL DEFAULT '';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'video_shots_generation_mode_check'
  ) THEN
    ALTER TABLE video_shots
      ADD CONSTRAINT video_shots_generation_mode_check
      CHECK (generation_mode IN ('reference','edit','extend'));
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_shots_source_key
  ON video_shots(project_id, source_key) WHERE source_key<>'' AND archived_at IS NULL;

-- 分镜级参考素材：图片/视频/音频统一上传到 OSS 后关联到具体分镜。
CREATE TABLE IF NOT EXISTS video_shot_assets (
  id            BIGSERIAL PRIMARY KEY,
  shot_id       BIGINT NOT NULL REFERENCES video_shots(id) ON DELETE CASCADE,
  asset_type    TEXT NOT NULL DEFAULT 'image', -- image/video/audio
  object_url    TEXT NOT NULL DEFAULT '',
  name          TEXT NOT NULL DEFAULT '',
  mime_type     TEXT NOT NULL DEFAULT '',
  size_bytes    BIGINT NOT NULL DEFAULT 0,
  reference_role TEXT NOT NULL DEFAULT '',
  sort_order    INT NOT NULL DEFAULT 0,
  source_type   TEXT NOT NULL DEFAULT '',
  source_id     TEXT NOT NULL DEFAULT '',
  usage_note    TEXT NOT NULL DEFAULT '',
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS asset_type TEXT NOT NULL DEFAULT 'image';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS object_url TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS name TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS size_bytes BIGINT NOT NULL DEFAULT 0;
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS reference_role TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS source_type TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS source_id TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS usage_note TEXT NOT NULL DEFAULT '';
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS create_time TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT now();
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
      FROM information_schema.columns
     WHERE table_name = 'video_shot_assets'
       AND column_name = 'sort_order'
  ) THEN
    ALTER TABLE video_shot_assets ADD COLUMN IF NOT EXISTS sort_order INT NOT NULL DEFAULT 0;

    WITH ranked AS (
      SELECT id, ROW_NUMBER() OVER (PARTITION BY shot_id ORDER BY create_time, id) - 1 AS sort_order
        FROM video_shot_assets
    )
    UPDATE video_shot_assets AS assets
       SET sort_order = ranked.sort_order
      FROM ranked
     WHERE ranked.id = assets.id;
  END IF;
END $$;
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
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS shot_revision INT NOT NULL DEFAULT 0;

-- 视频生成提交：在创建中转站任务前保存一次明确生成意图。
-- request_key 防止浏览器重发；活动分镜索引防止同一镜头并发创建多个任务。
CREATE TABLE IF NOT EXISTS video_generation_submissions (
  id                 BIGSERIAL PRIMARY KEY,
  request_key        UUID NOT NULL,
  project_id         BIGINT REFERENCES video_projects(id) ON DELETE SET NULL,
  shot_id            BIGINT REFERENCES video_shots(id) ON DELETE SET NULL,
  request_hash       TEXT NOT NULL DEFAULT '',
  prompt_hash        TEXT NOT NULL DEFAULT '',
  capability_version TEXT NOT NULL DEFAULT '',
  request_snapshot   JSONB NOT NULL DEFAULT '{}'::jsonb,
  status             TEXT NOT NULL DEFAULT 'prepared'
                     CHECK (status IN ('prepared','submitting','accepted','unknown_outcome','completed','failed','cancelled','reconciled')),
  upstream_task_id   TEXT NOT NULL DEFAULT '',
  generation_id      BIGINT REFERENCES video_generations(id) ON DELETE SET NULL,
  error_message      TEXT NOT NULL DEFAULT '',
  create_time        TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_generation_submissions_request_key
  ON video_generation_submissions(request_key);
CREATE UNIQUE INDEX IF NOT EXISTS uq_video_generation_submissions_active_shot
  ON video_generation_submissions(shot_id)
  WHERE shot_id IS NOT NULL AND status IN ('prepared','submitting','accepted','unknown_outcome');
CREATE UNIQUE INDEX IF NOT EXISTS uq_video_generation_submissions_upstream_task
  ON video_generation_submissions(upstream_task_id)
  WHERE upstream_task_id <> '';
CREATE INDEX IF NOT EXISTS idx_video_generation_submissions_generation
  ON video_generation_submissions(generation_id)
  WHERE generation_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_video_generations_task_id
  ON video_generations(task_id)
  WHERE task_id <> '';

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
  compose_input_hash TEXT NOT NULL DEFAULT '',
  compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  progress        INT NOT NULL DEFAULT 0,
  error_message   TEXT NOT NULL DEFAULT '',
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS compose_input_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS compose_input_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE video_compose_jobs ADD COLUMN IF NOT EXISTS progress INT NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS video_generation_submissions (
  id BIGSERIAL PRIMARY KEY,
  request_key UUID NOT NULL UNIQUE,
  shot_id BIGINT NOT NULL REFERENCES video_shots(id) ON DELETE CASCADE,
  generation_id BIGINT REFERENCES video_generations(id) ON DELETE SET NULL,
  task_id TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'prepared',
  request_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_message TEXT NOT NULL DEFAULT '',
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (status IN ('prepared','submitting','accepted','unknown_outcome','reconciled','completed','failed','cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_generation_submissions_active_shot
ON video_generation_submissions(shot_id)
WHERE status IN ('prepared','submitting','accepted','unknown_outcome','reconciled');

CREATE UNIQUE INDEX IF NOT EXISTS idx_video_compose_jobs_active_project
ON video_compose_jobs(project_id) WHERE status IN ('queued','processing');

-- 老项目无损迁移：只补空字段，不覆盖用户已经选择的资产或视频版本。
CREATE OR REPLACE FUNCTION migrate_legacy_video_project_workflows()
RETURNS void
LANGUAGE plpgsql
AS $$
BEGIN
  UPDATE video_project_characters c
     SET breakdown_item_key = 'legacy:character:' || c.id::text,
         visual_prompt = CASE WHEN c.visual_prompt = '' THEN c.description ELSE c.visual_prompt END,
         source = 'legacy',
         status = CASE WHEN c.reference_image_url <> '' AND c.status = 'draft' THEN 'ready' ELSE c.status END,
         required = c.required OR EXISTS (
           SELECT 1
             FROM video_shots s
            WHERE s.project_id = c.project_id
              AND s.archived_at IS NULL
              AND s.character_ids ? c.id::text
         )
   WHERE c.breakdown_item_key = '';

  UPDATE video_project_scenes sc
     SET breakdown_item_key = 'legacy:scene:' || sc.id::text,
         visual_prompt = CASE WHEN sc.visual_prompt = '' THEN sc.description ELSE sc.visual_prompt END,
         source = 'legacy',
         status = CASE WHEN sc.reference_image_url <> '' AND sc.status = 'draft' THEN 'ready' ELSE sc.status END,
         required = sc.required OR EXISTS (
           SELECT 1
             FROM video_shots s
            WHERE s.project_id = sc.project_id
              AND s.archived_at IS NULL
              AND s.scene_id = sc.id
         )
   WHERE sc.breakdown_item_key = '';

  INSERT INTO video_project_asset_candidates (
    project_id, target_type, target_id, prompt, image_asset_id, image_url,
    source, status, selected
  )
  SELECT c.project_id, 'character', c.id,
         COALESCE(NULLIF(c.visual_prompt,''), c.description),
         va.asset_id, c.reference_image_url, 'legacy', 'ready', true
    FROM video_project_characters c
    LEFT JOIN video_assets va ON va.id = c.asset_id
   WHERE c.reference_image_url <> ''
     AND NOT EXISTS (
       SELECT 1
         FROM video_project_asset_candidates candidate
        WHERE candidate.target_type = 'character'
          AND candidate.target_id = c.id
          AND candidate.source = 'legacy'
     )
  ON CONFLICT (target_type, target_id) WHERE selected=true DO NOTHING;

  INSERT INTO video_project_asset_candidates (
    project_id, target_type, target_id, prompt, image_asset_id, image_url,
    source, status, selected
  )
  SELECT sc.project_id, 'scene', sc.id,
         COALESCE(NULLIF(sc.visual_prompt,''), sc.description),
         va.asset_id, sc.reference_image_url, 'legacy', 'ready', true
    FROM video_project_scenes sc
    LEFT JOIN video_assets va ON va.id = sc.asset_id
   WHERE sc.reference_image_url <> ''
     AND NOT EXISTS (
       SELECT 1
         FROM video_project_asset_candidates candidate
        WHERE candidate.target_type = 'scene'
          AND candidate.target_id = sc.id
          AND candidate.source = 'legacy'
     )
  ON CONFLICT (target_type, target_id) WHERE selected=true DO NOTHING;

  WITH ranked AS (
    SELECT id,
           row_number() OVER (PARTITION BY shot_id ORDER BY create_time, id) AS ordinal
      FROM video_shot_assets
     WHERE reference_role = '' OR source_type = '' OR source_id = ''
  )
  UPDATE video_shot_assets asset
     SET reference_role = CASE
           WHEN asset.reference_role <> '' THEN asset.reference_role
           WHEN asset.asset_type = 'video' THEN 'reference_video'
           WHEN asset.asset_type = 'audio' THEN 'reference_audio'
           ELSE 'reference_image'
         END,
         sort_order = ranked.ordinal,
         source_type = CASE WHEN asset.source_type = '' THEN 'legacy_shot_asset' ELSE asset.source_type END,
         source_id = CASE WHEN asset.source_id = '' THEN ranked.id::text ELSE asset.source_id END
    FROM ranked
   WHERE asset.id = ranked.id;

  UPDATE video_shots s
     SET selected_generation_id = s.generation_id
    FROM video_generations g
   WHERE s.selected_generation_id IS NULL
     AND s.generation_id = g.id
     AND g.status IN ('completed','succeeded');
END;
$$;

SELECT migrate_legacy_video_project_workflows();

CREATE UNIQUE INDEX IF NOT EXISTS uq_video_shot_assets_exact_reference
  ON video_shot_assets(
    shot_id,
    asset_type,
    reference_role,
    COALESCE(source_type,''),
    COALESCE(source_id,''),
    object_url
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
CREATE INDEX IF NOT EXISTS idx_video_shot_assets_order ON video_shot_assets(shot_id, sort_order, id);
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
  member_started_at TIMESTAMPTZ,
  member_expires_at TIMESTAMPTZ,
  channel       TEXT NOT NULL DEFAULT '',
  scene         TEXT NOT NULL DEFAULT '',
  create_time   TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_login_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE wx_users ADD COLUMN IF NOT EXISTS member_started_at TIMESTAMPTZ;
ALTER TABLE wx_users ADD COLUMN IF NOT EXISTS member_expires_at TIMESTAMPTZ;

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

-- ============ 平台来源与统一业务消息升级 ============
-- 迁移刻意放在 bookings/test_records/wx_users 均已创建之后，兼容旧数据库升级。
CREATE TABLE IF NOT EXISTS migration_logs (
  key TEXT PRIMARY KEY,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 第一阶段：只增加可空字段，确保历史数据能够先被安全回填。
ALTER TABLE signups ADD COLUMN IF NOT EXISTS source_platform TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS platform TEXT;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS event_key TEXT;

-- 第二阶段：回填报名来源。历史报名默认来自官网，有预约关联的报名来自小程序。
UPDATE signups SET source_platform = 'website'
WHERE source_platform IS NULL OR source_platform = '';

UPDATE signups s
SET source_platform = 'miniapp'
FROM bookings b
WHERE b.signup_id = s.id
  AND s.source_platform IS DISTINCT FROM 'miniapp';

-- 第三阶段：按可验证的业务关系回填历史消息，不根据模糊标题猜测归属。
UPDATE messages m
SET platform = CASE
      WHEN m.platform IS NULL OR btrim(m.platform) = '' THEN
        CASE WHEN s.source_platform = 'miniapp' THEN 'miniapp' ELSE 'website' END
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy'
        AND m.target_path IN ('', '/message/management?type=signup') THEN
        CASE WHEN s.source_platform = 'miniapp' THEN 'miniapp' ELSE 'website' END
      ELSE m.platform
    END,
    event_key = CASE
      WHEN m.event_key IS NULL OR btrim(m.event_key) = '' THEN
        CASE WHEN s.source_platform = 'miniapp' THEN 'miniapp.booking.created' ELSE 'signup.created' END
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy'
        AND m.target_path IN ('', '/message/management?type=signup') THEN
        CASE WHEN s.source_platform = 'miniapp' THEN 'miniapp.booking.created' ELSE 'signup.created' END
      ELSE m.event_key
    END,
    target_path = CASE
      WHEN btrim(m.target_path) = '' OR m.target_path = '/message/management?type=signup' THEN
        '/customer/signups?leadId=' || s.id::text || '&open=detail'
      ELSE m.target_path
    END,
    business_id = s.id::text
FROM signups s
WHERE m.business_type = 'signup'
  AND m.business_id = s.id::text
  AND (
    m.platform IS NULL OR btrim(m.platform) = ''
    OR m.event_key IS NULL OR btrim(m.event_key) = ''
    OR btrim(m.target_path) = '' OR m.target_path = '/message/management?type=signup'
  );

UPDATE messages m
SET platform = CASE
      WHEN m.platform IS NULL OR btrim(m.platform) = '' THEN 'miniapp'
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy' AND btrim(m.target_path) = '' THEN 'miniapp'
      ELSE m.platform
    END,
    event_key = CASE
      WHEN m.event_key IS NULL OR btrim(m.event_key) = '' THEN 'miniapp.user.created'
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy' AND btrim(m.target_path) = '' THEN 'miniapp.user.created'
      ELSE m.event_key
    END,
    target_path = CASE
      WHEN btrim(m.target_path) = '' THEN '/customer/miniapp-users?userId=' || u.id::text || '&open=detail'
      ELSE m.target_path
    END,
    business_id = u.id::text
FROM wx_users u
WHERE m.business_type = 'miniapp-user'
  AND m.business_id = u.id::text
  AND (
    m.platform IS NULL OR btrim(m.platform) = ''
    OR m.event_key IS NULL OR btrim(m.event_key) = ''
    OR btrim(m.target_path) = ''
  );

UPDATE messages m
SET platform = CASE
      WHEN m.platform IS NULL OR btrim(m.platform) = '' THEN 'miniapp'
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy' AND btrim(m.target_path) = '' THEN 'miniapp'
      ELSE m.platform
    END,
    event_key = CASE
      WHEN m.event_key IS NULL OR btrim(m.event_key) = '' THEN 'miniapp.quiz.submitted'
      WHEN m.platform = 'system' AND m.event_key = 'system.legacy' AND btrim(m.target_path) = '' THEN 'miniapp.quiz.submitted'
      ELSE m.event_key
    END,
    target_path = CASE
      WHEN btrim(m.target_path) = '' THEN '/customer/miniapp-users?userId=' || r.wx_user_id::text
        || '&testRecordId=' || r.id::text || '&open=test'
      ELSE m.target_path
    END,
    business_id = r.id::text
FROM test_records r
WHERE m.business_type = 'miniapp-test-record'
  AND m.business_id = r.id::text
  AND (
    m.platform IS NULL OR btrim(m.platform) = ''
    OR m.event_key IS NULL OR btrim(m.event_key) = ''
    OR btrim(m.target_path) = ''
  );

UPDATE messages
SET platform = CASE WHEN platform IS NULL OR btrim(platform) = '' THEN 'system' ELSE platform END,
    event_key = CASE
      WHEN event_key IS NULL OR btrim(event_key) = '' OR event_key = 'system.legacy'
        THEN 'system.legacy.' || id::text
      ELSE event_key
    END,
    business_type = CASE WHEN btrim(business_type) = '' THEN 'message' ELSE business_type END,
    business_id = CASE WHEN btrim(business_id) = '' THEN id::text ELSE business_id END
WHERE platform IS NULL OR btrim(platform) = ''
   OR event_key IS NULL OR btrim(event_key) = '' OR event_key = 'system.legacy';

-- 第四阶段：先记录历史异常摘要，再清理，之后才能安全增加约束。
WITH orphaned AS (
  SELECT b.id
  FROM bookings b
  LEFT JOIN signups s ON s.id = b.signup_id
  WHERE b.signup_id IS NOT NULL AND s.id IS NULL
), orphan_summary AS (
  SELECT count(*) AS count,
         COALESCE((SELECT jsonb_agg(id ORDER BY id) FROM (SELECT id FROM orphaned ORDER BY id LIMIT 20) sample), '[]'::jsonb) AS ids
  FROM orphaned
)
INSERT INTO migration_logs (key, detail)
SELECT 'platform_notification.orphan_bookings', jsonb_build_object('count', count, 'ids', ids)
FROM orphan_summary
ON CONFLICT (key) DO NOTHING;

UPDATE bookings b
SET signup_id = NULL
WHERE signup_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM signups s WHERE s.id = b.signup_id);

WITH ranked AS (
  SELECT id,
         row_number() OVER (PARTITION BY event_key, business_type, business_id ORDER BY id) AS duplicate_number
  FROM messages
), duplicate_summary AS (
  SELECT count(*) AS count,
         COALESCE((SELECT jsonb_agg(id ORDER BY id) FROM (SELECT id FROM ranked WHERE duplicate_number > 1 ORDER BY id LIMIT 20) sample), '[]'::jsonb) AS ids
  FROM ranked
  WHERE duplicate_number > 1
)
INSERT INTO migration_logs (key, detail)
SELECT 'platform_notification.duplicate_messages', jsonb_build_object('count', count, 'ids', ids)
FROM duplicate_summary
ON CONFLICT (key) DO NOTHING;

WITH ranked AS (
  SELECT id,
         row_number() OVER (PARTITION BY event_key, business_type, business_id ORDER BY id) AS duplicate_number
  FROM messages
)
DELETE FROM messages duplicate
USING ranked
WHERE duplicate.id = ranked.id
  AND ranked.duplicate_number > 1;

-- 第五阶段：历史数据干净后增加取值、引用和幂等约束。
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'chk_signups_source_platform' AND conrelid = 'signups'::regclass
  ) THEN
    ALTER TABLE signups ADD CONSTRAINT chk_signups_source_platform
      CHECK (source_platform IN ('website', 'miniapp'));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'chk_messages_platform' AND conrelid = 'messages'::regclass
  ) THEN
    ALTER TABLE messages ADD CONSTRAINT chk_messages_platform
      CHECK (platform IN ('website', 'miniapp', 'system'));
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint
    WHERE conname = 'fk_bookings_signup' AND conrelid = 'bookings'::regclass
  ) THEN
    ALTER TABLE bookings ADD CONSTRAINT fk_bookings_signup
      FOREIGN KEY (signup_id) REFERENCES signups(id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_event_business
  ON messages(event_key, business_type, business_id);
CREATE INDEX IF NOT EXISTS idx_messages_unread_id
  ON messages(is_read, id DESC);
CREATE INDEX IF NOT EXISTS idx_messages_platform_create_time
  ON messages(platform, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_signups_source_create_time
  ON signups(source_platform, create_time DESC);

-- 最后一阶段：字段全部回填且约束就绪后，再收紧非空并提供兼容默认值。
ALTER TABLE signups ALTER COLUMN source_platform SET NOT NULL;
ALTER TABLE messages ALTER COLUMN platform SET NOT NULL;
ALTER TABLE messages ALTER COLUMN event_key SET NOT NULL;
ALTER TABLE signups ALTER COLUMN source_platform SET DEFAULT 'website';
ALTER TABLE messages ALTER COLUMN platform SET DEFAULT 'system';
ALTER TABLE messages ALTER COLUMN event_key SET DEFAULT 'system.legacy';

-- 兼容仍未显式传入业务身份的旧消息写入路径，并避免默认消息互相触发唯一冲突。
CREATE OR REPLACE FUNCTION normalize_legacy_message_identity()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
  IF btrim(COALESCE(NEW.business_type, '')) = '' THEN
    NEW.business_type := 'message';
  END IF;
  IF btrim(COALESCE(NEW.business_id, '')) = '' THEN
    NEW.business_id := NEW.id::text;
  END IF;
  RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_messages_normalize_legacy_identity ON messages;
CREATE TRIGGER trg_messages_normalize_legacy_identity
BEFORE INSERT ON messages
FOR EACH ROW EXECUTE FUNCTION normalize_legacy_message_identity();

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
  authors JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_source_works_authors_array CHECK (jsonb_typeof(authors) = 'array'),
  editors JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_source_works_editors_array CHECK (jsonb_typeof(editors) = 'array'),
  translators JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_source_works_translators_array CHECK (jsonb_typeof(translators) = 'array'),
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
  aliases JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_cards_aliases_array CHECK (jsonb_typeof(aliases) = 'array'),
  domain TEXT NOT NULL DEFAULT '',
  subdomain TEXT NOT NULL DEFAULT '',
  card_kind TEXT NOT NULL CHECK (card_kind IN ('concept','claim','axis','stage','relation','profile','practice','warning')),
  summary TEXT NOT NULL DEFAULT '',
  definition TEXT NOT NULL DEFAULT '',
  core_claim TEXT NOT NULL DEFAULT '',
  mechanism TEXT NOT NULL DEFAULT '',
  applicable_context TEXT NOT NULL DEFAULT '',
  non_applicable_context TEXT NOT NULL DEFAULT '',
  observable_signals JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_cards_observable_signals_array CHECK (jsonb_typeof(observable_signals) = 'array'),
  common_triggers JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_cards_common_triggers_array CHECK (jsonb_typeof(common_triggers) = 'array'),
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
-- Replacement transaction order: supersede the old published card before publishing the new version.
-- The partial unique index intentionally forbids two simultaneously published versions of one canonical key.

CREATE TABLE IF NOT EXISTS theory_practices (
  id BIGSERIAL PRIMARY KEY,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  goal TEXT NOT NULL,
  estimated_minutes INTEGER NOT NULL DEFAULT 0 CHECK (estimated_minutes >= 0),
  steps JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_practices_steps_array CHECK (jsonb_typeof(steps) = 'array'),
  reflection_prompts JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_practices_reflection_prompts_array CHECK (jsonb_typeof(reflection_prompts) = 'array'),
  expected_feedback JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_practices_expected_feedback_array CHECK (jsonb_typeof(expected_feedback) = 'array'),
  stop_conditions JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_practices_stop_conditions_array CHECK (jsonb_typeof(stop_conditions) = 'array'),
  professional_escalation JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_practices_professional_escalation_array CHECK (jsonb_typeof(professional_escalation) = 'array'),
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

CREATE OR REPLACE FUNCTION lock_theory_libraries(library_ids BIGINT[])
RETURNS VOID AS $$
DECLARE
  candidate BIGINT;
BEGIN
  FOR candidate IN
    SELECT DISTINCT value
    FROM unnest(library_ids) AS value
    WHERE value IS NOT NULL
    ORDER BY value
  LOOP
    PERFORM pg_advisory_xact_lock(hashtextextended('nine-xing:theory-library:' || candidate::text, 0));
  END LOOP;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION lock_theory_ownership_scope()
RETURNS TRIGGER AS $$
DECLARE
  scope_ids BIGINT[];
  new_row JSONB := to_jsonb(NEW);
  old_row JSONB := to_jsonb(OLD);
BEGIN
  IF TG_TABLE_NAME = 'theory_source_works' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM (
      SELECT (new_row->>'library_id')::BIGINT AS library_id
      UNION ALL SELECT (old_row->>'library_id')::BIGINT
      UNION ALL SELECT library_id FROM theory_source_works WHERE id IN ((new_row->>'canonical_work_id')::BIGINT, (old_row->>'canonical_work_id')::BIGINT)
    ) scope;
  ELSIF TG_TABLE_NAME = 'theory_source_files' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM (
      SELECT work.library_id FROM theory_source_works work WHERE work.id IN ((new_row->>'work_id')::BIGINT, (old_row->>'work_id')::BIGINT)
      UNION ALL
      SELECT duplicate_work.library_id
      FROM theory_source_files duplicate
      JOIN theory_source_works duplicate_work ON duplicate_work.id = duplicate.work_id
      WHERE duplicate.id IN ((new_row->>'duplicate_of_file_id')::BIGINT, (old_row->>'duplicate_of_file_id')::BIGINT)
    ) scope;
  ELSIF TG_TABLE_NAME = 'theory_cards' THEN
    scope_ids := ARRAY[(new_row->>'library_id')::BIGINT, (old_row->>'library_id')::BIGINT];
  ELSIF TG_TABLE_NAME = 'theory_card_relations' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM theory_cards
    WHERE id IN ((new_row->>'from_card_id')::BIGINT, (new_row->>'to_card_id')::BIGINT,
      (old_row->>'from_card_id')::BIGINT, (old_row->>'to_card_id')::BIGINT);
  ELSIF TG_TABLE_NAME = 'theory_card_sources' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM (
      SELECT library_id FROM theory_cards WHERE id IN ((new_row->>'card_id')::BIGINT, (old_row->>'card_id')::BIGINT)
      UNION ALL SELECT library_id FROM theory_source_works WHERE id IN ((new_row->>'work_id')::BIGINT, (old_row->>'work_id')::BIGINT)
      UNION ALL
      SELECT work.library_id
      FROM theory_source_files file
      JOIN theory_source_works work ON work.id = file.work_id
      WHERE file.id IN ((new_row->>'file_id')::BIGINT, (old_row->>'file_id')::BIGINT)
    ) scope;
  ELSIF TG_TABLE_NAME = 'theory_practices' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM theory_cards
    WHERE id IN ((new_row->>'card_id')::BIGINT, (old_row->>'card_id')::BIGINT);
  ELSIF TG_TABLE_NAME = 'theory_chunks' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM (
      SELECT (new_row->>'library_id')::BIGINT AS library_id
      UNION ALL SELECT (old_row->>'library_id')::BIGINT
      UNION ALL SELECT library_id FROM theory_cards WHERE id IN ((new_row->>'card_id')::BIGINT, (old_row->>'card_id')::BIGINT)
      UNION ALL
      SELECT card.library_id
      FROM theory_practices practice
      JOIN theory_cards card ON card.id = practice.card_id
      WHERE practice.id IN ((new_row->>'practice_id')::BIGINT, (old_row->>'practice_id')::BIGINT)
    ) scope;
  ELSIF TG_TABLE_NAME = 'theory_library_releases' THEN
    scope_ids := ARRAY[(new_row->>'library_id')::BIGINT, (old_row->>'library_id')::BIGINT];
  ELSIF TG_TABLE_NAME = 'theory_release_cards' THEN
    SELECT array_agg(DISTINCT library_id) INTO scope_ids
    FROM (
      SELECT library_id FROM theory_library_releases WHERE id IN ((new_row->>'release_id')::BIGINT, (old_row->>'release_id')::BIGINT)
      UNION ALL SELECT library_id FROM theory_cards WHERE id IN ((new_row->>'card_id')::BIGINT, (old_row->>'card_id')::BIGINT)
      UNION ALL SELECT library_id FROM theory_chunks WHERE id IN ((new_row->>'chunk_id')::BIGINT, (old_row->>'chunk_id')::BIGINT)
    ) scope;
  ELSE
    RETURN NEW;
  END IF;
  PERFORM lock_theory_libraries(scope_ids);
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION validate_theory_library_ownership()
RETURNS TRIGGER AS $$
DECLARE
  target_id BIGINT := (to_jsonb(NEW)->>'id')::BIGINT;
  target_library_id BIGINT := (to_jsonb(NEW)->>'library_id')::BIGINT;
  target_card_id BIGINT := (to_jsonb(NEW)->>'card_id')::BIGINT;
BEGIN
  IF TG_TABLE_NAME = 'theory_source_works' THEN
    IF EXISTS (
      SELECT 1 FROM theory_source_works work
      JOIN theory_source_works canonical ON canonical.id = work.canonical_work_id
      WHERE (work.id = target_id OR canonical.id = target_id)
        AND work.library_id IS DISTINCT FROM canonical.library_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: canonical work must belong to the same library';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_card_sources source
      JOIN theory_cards card ON card.id = source.card_id
      JOIN theory_source_works work ON work.id = source.work_id
      LEFT JOIN theory_source_files file ON file.id = source.file_id
      WHERE source.work_id = target_id
        AND (card.library_id IS DISTINCT FROM work.library_id OR (source.file_id IS NOT NULL AND file.work_id IS DISTINCT FROM source.work_id))
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: card source card, work, and file must share ownership';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_source_files file
      JOIN theory_source_works work ON work.id = file.work_id
      JOIN theory_source_files duplicate ON duplicate.id = file.duplicate_of_file_id
      JOIN theory_source_works duplicate_work ON duplicate_work.id = duplicate.work_id
      WHERE (work.id = target_id OR duplicate_work.id = target_id)
        AND work.library_id IS DISTINCT FROM duplicate_work.library_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: duplicate source file must belong to the same library';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_source_files' THEN
    IF EXISTS (
      SELECT 1 FROM theory_source_files file
      JOIN theory_source_works work ON work.id = file.work_id
      JOIN theory_source_files duplicate ON duplicate.id = file.duplicate_of_file_id
      JOIN theory_source_works duplicate_work ON duplicate_work.id = duplicate.work_id
      WHERE (file.id = target_id OR duplicate.id = target_id)
        AND work.library_id IS DISTINCT FROM duplicate_work.library_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: duplicate source file must belong to the same library';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_card_sources source
      JOIN theory_cards card ON card.id = source.card_id
      JOIN theory_source_works work ON work.id = source.work_id
      LEFT JOIN theory_source_files file ON file.id = source.file_id
      WHERE source.file_id = target_id
        AND (card.library_id IS DISTINCT FROM work.library_id OR file.work_id IS DISTINCT FROM source.work_id)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: card source card, work, and file must share ownership';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_cards' THEN
    IF EXISTS (
      SELECT 1 FROM theory_card_relations relation
      JOIN theory_cards from_card ON from_card.id = relation.from_card_id
      JOIN theory_cards to_card ON to_card.id = relation.to_card_id
      WHERE (relation.from_card_id = target_id OR relation.to_card_id = target_id)
        AND from_card.library_id IS DISTINCT FROM to_card.library_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: relation cards must belong to the same library';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_card_sources source
      JOIN theory_source_works work ON work.id = source.work_id
      LEFT JOIN theory_source_files file ON file.id = source.file_id
      WHERE source.card_id = target_id
        AND (target_library_id IS DISTINCT FROM work.library_id OR (source.file_id IS NOT NULL AND file.work_id IS DISTINCT FROM source.work_id))
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: card source card, work, and file must share ownership';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_chunks chunk
      LEFT JOIN theory_practices practice ON practice.id = chunk.practice_id
      WHERE chunk.card_id = target_id
        AND (chunk.library_id IS DISTINCT FROM target_library_id OR (chunk.practice_id IS NOT NULL AND practice.card_id IS DISTINCT FROM chunk.card_id))
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: chunk library, card, and practice ownership mismatch';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_release_cards mapping
      JOIN theory_library_releases release ON release.id = mapping.release_id
      JOIN theory_chunks chunk ON chunk.id = mapping.chunk_id
      WHERE mapping.card_id = target_id
        AND (release.library_id IS DISTINCT FROM target_library_id OR release.library_id IS DISTINCT FROM chunk.library_id OR chunk.card_id IS DISTINCT FROM mapping.card_id)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: release card mapping ownership mismatch';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_card_relations' THEN
    IF EXISTS (
      SELECT 1 FROM theory_card_relations relation
      JOIN theory_cards from_card ON from_card.id = relation.from_card_id
      JOIN theory_cards to_card ON to_card.id = relation.to_card_id
      WHERE relation.id = target_id AND from_card.library_id IS DISTINCT FROM to_card.library_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: relation cards must belong to the same library';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_card_sources' THEN
    IF EXISTS (
      SELECT 1 FROM theory_card_sources source
      JOIN theory_cards card ON card.id = source.card_id
      JOIN theory_source_works work ON work.id = source.work_id
      LEFT JOIN theory_source_files file ON file.id = source.file_id
      WHERE source.id = target_id
        AND (card.library_id IS DISTINCT FROM work.library_id OR (source.file_id IS NOT NULL AND file.work_id IS DISTINCT FROM source.work_id))
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: card source card, work, and file must share ownership';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_practices' THEN
    IF EXISTS (
      SELECT 1 FROM theory_chunks chunk
      WHERE chunk.practice_id = target_id AND chunk.card_id IS DISTINCT FROM target_card_id
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: chunk library, card, and practice ownership mismatch';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_chunks' THEN
    IF EXISTS (
      SELECT 1 FROM theory_chunks chunk
      JOIN theory_cards card ON card.id = chunk.card_id
      LEFT JOIN theory_practices practice ON practice.id = chunk.practice_id
      WHERE chunk.id = target_id
        AND (chunk.library_id IS DISTINCT FROM card.library_id OR (chunk.practice_id IS NOT NULL AND practice.card_id IS DISTINCT FROM chunk.card_id))
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: chunk library, card, and practice ownership mismatch';
    END IF;
    IF EXISTS (
      SELECT 1 FROM theory_release_cards mapping
      JOIN theory_library_releases release ON release.id = mapping.release_id
      JOIN theory_cards card ON card.id = mapping.card_id
      WHERE mapping.chunk_id = target_id
        AND (release.library_id IS DISTINCT FROM card.library_id OR release.library_id IS DISTINCT FROM target_library_id OR target_card_id IS DISTINCT FROM mapping.card_id)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: release card mapping ownership mismatch';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_library_releases' THEN
    IF EXISTS (
      SELECT 1 FROM theory_release_cards mapping
      JOIN theory_cards card ON card.id = mapping.card_id
      JOIN theory_chunks chunk ON chunk.id = mapping.chunk_id
      WHERE mapping.release_id = target_id
        AND (target_library_id IS DISTINCT FROM card.library_id OR target_library_id IS DISTINCT FROM chunk.library_id OR chunk.card_id IS DISTINCT FROM mapping.card_id)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: release card mapping ownership mismatch';
    END IF;
  ELSIF TG_TABLE_NAME = 'theory_release_cards' THEN
    IF EXISTS (
      SELECT 1 FROM theory_release_cards mapping
      JOIN theory_library_releases release ON release.id = mapping.release_id
      JOIN theory_cards card ON card.id = mapping.card_id
      JOIN theory_chunks chunk ON chunk.id = mapping.chunk_id
      WHERE mapping.id = target_id
        AND (release.library_id IS DISTINCT FROM card.library_id OR release.library_id IS DISTINCT FROM chunk.library_id OR chunk.card_id IS DISTINCT FROM mapping.card_id)
    ) THEN
      RAISE EXCEPTION USING ERRCODE = '23514', MESSAGE = 'theory ownership constraint: release card mapping ownership mismatch';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Lock triggers use only the related library IDs, so unrelated libraries remain concurrent.
DROP TRIGGER IF EXISTS theory_source_works_ownership_lock ON theory_source_works;
CREATE TRIGGER theory_source_works_ownership_lock BEFORE INSERT OR UPDATE ON theory_source_works
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_source_files_ownership_lock ON theory_source_files;
CREATE TRIGGER theory_source_files_ownership_lock BEFORE INSERT OR UPDATE ON theory_source_files
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_cards_ownership_lock ON theory_cards;
CREATE TRIGGER theory_cards_ownership_lock BEFORE INSERT OR UPDATE ON theory_cards
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_card_relations_ownership_lock ON theory_card_relations;
CREATE TRIGGER theory_card_relations_ownership_lock BEFORE INSERT OR UPDATE ON theory_card_relations
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_card_sources_ownership_lock ON theory_card_sources;
CREATE TRIGGER theory_card_sources_ownership_lock BEFORE INSERT OR UPDATE ON theory_card_sources
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();

-- Constraint triggers are recreated per target table so unrelated same-name triggers cannot suppress them.
DROP TRIGGER IF EXISTS theory_card_relations_ownership ON theory_card_relations;
CREATE CONSTRAINT TRIGGER theory_card_relations_ownership AFTER INSERT OR UPDATE ON theory_card_relations
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();
DROP TRIGGER IF EXISTS theory_card_sources_file_work_match ON theory_card_sources;
CREATE CONSTRAINT TRIGGER theory_card_sources_file_work_match AFTER INSERT OR UPDATE ON theory_card_sources
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();
DROP TRIGGER IF EXISTS theory_source_files_card_source_work_match ON theory_source_files;
CREATE CONSTRAINT TRIGGER theory_source_files_card_source_work_match AFTER INSERT OR UPDATE ON theory_source_files
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();
DROP TRIGGER IF EXISTS theory_cards_ownership_dependents ON theory_cards;
CREATE CONSTRAINT TRIGGER theory_cards_ownership_dependents AFTER INSERT OR UPDATE ON theory_cards
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();
DROP TRIGGER IF EXISTS theory_source_works_ownership_dependents ON theory_source_works;
CREATE CONSTRAINT TRIGGER theory_source_works_ownership_dependents AFTER INSERT OR UPDATE ON theory_source_works
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();

CREATE TABLE IF NOT EXISTS theory_chunks (
  id BIGSERIAL PRIMARY KEY,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES theory_cards(id) ON DELETE CASCADE,
  practice_id BIGINT REFERENCES theory_practices(id) ON DELETE SET NULL,
  chunk_key TEXT NOT NULL,
  chunk_kind TEXT NOT NULL CHECK (chunk_kind IN ('card','practice')),
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  keywords JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_chunks_keywords_array CHECK (jsonb_typeof(keywords) = 'array'),
  tags JSONB NOT NULL DEFAULT '[]'::jsonb CONSTRAINT ck_theory_chunks_tags_array CHECK (jsonb_typeof(tags) = 'array'),
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

-- 数据包同步审计：离线 pending 模板不构成审核，正式审核仅记录数据库用户。
CREATE TABLE IF NOT EXISTS theory_package_imports (
  id BIGSERIAL PRIMARY KEY,
  package_id TEXT NOT NULL,
  content_digest TEXT NOT NULL,
  package_digest TEXT NOT NULL,
  schema_version TEXT NOT NULL,
  library_id BIGINT NOT NULL REFERENCES theory_libraries(id) ON DELETE RESTRICT,
  target_database TEXT NOT NULL,
  desired_release_version INTEGER NOT NULL CHECK (desired_release_version > 0),
  state TEXT NOT NULL DEFAULT 'staged' CHECK (state IN ('staged','promoted')),
  payload JSONB NOT NULL CONSTRAINT ck_theory_package_imports_payload_object CHECK (jsonb_typeof(payload) = 'object'),
  payload_sha256 TEXT NOT NULL CONSTRAINT ck_theory_package_imports_payload_sha256 CHECK (payload_sha256 ~ '^[0-9a-f]{64}$'),
  payload_receipt_sha256 TEXT NOT NULL CONSTRAINT ck_theory_package_imports_payload_receipt_sha256 CHECK (payload_receipt_sha256 ~ '^[0-9a-f]{64}$'),
  payload_hash_contract TEXT NOT NULL DEFAULT 'postgres-jsonb-text-sha256-v1' CONSTRAINT ck_theory_package_imports_payload_hash_contract CHECK (payload_hash_contract = 'postgres-jsonb-text-sha256-v1'),
  object_fingerprints JSONB NOT NULL DEFAULT '{}'::jsonb CONSTRAINT ck_theory_package_imports_fingerprints_object CHECK (jsonb_typeof(object_fingerprints) = 'object'),
  staged_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  staged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  promoted_at TIMESTAMPTZ,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (package_id)
);

CREATE TABLE IF NOT EXISTS theory_package_reviews (
  id BIGSERIAL PRIMARY KEY,
  import_id BIGINT NOT NULL REFERENCES theory_package_imports(id) ON DELETE CASCADE,
  review_type TEXT NOT NULL CHECK (review_type IN ('source-verification','theory-review','safety-review')),
  content_digest TEXT NOT NULL,
  decision TEXT NOT NULL CHECK (decision IN ('approved','rejected')),
  reviewer_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  reviewer_role TEXT NOT NULL,
  notes TEXT NOT NULL DEFAULT '',
  reviewed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (import_id, review_type, content_digest)
);

CREATE TABLE IF NOT EXISTS theory_package_promotions (
  id BIGSERIAL PRIMARY KEY,
  import_id BIGINT NOT NULL REFERENCES theory_package_imports(id) ON DELETE RESTRICT,
  content_digest TEXT NOT NULL,
  release_id BIGINT NOT NULL REFERENCES theory_library_releases(id) ON DELETE RESTRICT,
  promoted_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  promoted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (import_id, content_digest)
);

CREATE INDEX IF NOT EXISTS idx_theory_package_imports_library ON theory_package_imports(library_id, state);
CREATE INDEX IF NOT EXISTS idx_theory_package_reviews_import ON theory_package_reviews(import_id, review_type);
CREATE INDEX IF NOT EXISTS idx_theory_package_promotions_release ON theory_package_promotions(release_id);

CREATE OR REPLACE FUNCTION theory_package_jsonb_sha256(value JSONB)
RETURNS TEXT
LANGUAGE SQL IMMUTABLE STRICT
SET search_path = pg_catalog
AS $$
  SELECT encode(sha256(convert_to(value::text, 'UTF8')), 'hex')
$$;

CREATE OR REPLACE FUNCTION theory_package_receipt_sha256(
  package_id TEXT,
  content_digest TEXT,
  package_digest TEXT,
  payload_sha256 TEXT,
  schema_version TEXT,
  library_id BIGINT,
  target_database TEXT,
  desired_release_version INTEGER
)
RETURNS TEXT
LANGUAGE SQL IMMUTABLE STRICT
SET search_path = pg_catalog
AS $$
  SELECT public.theory_package_jsonb_sha256(jsonb_build_object(
    'packageId', package_id,
    'contentDigest', content_digest,
    'packageDigest', package_digest,
    'payloadSha256', payload_sha256,
    'schemaVersion', schema_version,
    'libraryId', library_id,
    'targetDatabase', target_database,
    'desiredReleaseVersion', desired_release_version
  ))
$$;

CREATE OR REPLACE FUNCTION theory_package_database_snapshot(p_library_id BIGINT)
RETURNS JSONB
LANGUAGE SQL STABLE
SET search_path = pg_catalog
AS $$
  SELECT jsonb_build_object(
    'schemaVersion', 'xinzhili.database-snapshot.v1',
    'cards', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'canonical_key'), '[]'::jsonb)
      FROM (
        SELECT to_jsonb(c)-ARRAY['id','library_id','status','reviewed_by','reviewed_at','published_at','created_by','updated_by','create_time','update_time'] AS item
        FROM public.theory_cards c
        WHERE c.library_id=p_library_id
      ) rows
    ),
    'practices', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'card_canonical_key'), '[]'::jsonb)
      FROM (
        SELECT (to_jsonb(p)-ARRAY['id','card_id','status','create_time','update_time'])||jsonb_build_object('card_canonical_key',c.canonical_key) AS item
        FROM public.theory_practices p
        JOIN public.theory_cards c ON c.id=p.card_id
        WHERE c.library_id=p_library_id
      ) rows
    ),
    'sourceWorks', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'canonical_key'), '[]'::jsonb)
      FROM (
        SELECT (to_jsonb(w)-ARRAY['id','library_id','canonical_work_id','status','create_time','update_time'])||jsonb_build_object('canonical_work_key',canonical.canonical_key) AS item
        FROM public.theory_source_works w
        LEFT JOIN public.theory_source_works canonical ON canonical.id=w.canonical_work_id
        WHERE w.library_id=p_library_id
      ) rows
    ),
    'sourceFiles', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'work_canonical_key',item->>'relative_path',item->>'sha256'), '[]'::jsonb)
      FROM (
        SELECT (to_jsonb(f)-ARRAY['id','work_id','duplicate_of_file_id','create_time','update_time'])||jsonb_build_object('work_canonical_key',w.canonical_key,'duplicate_sha256',duplicate.sha256) AS item
        FROM public.theory_source_files f
        JOIN public.theory_source_works w ON w.id=f.work_id
        LEFT JOIN public.theory_source_files duplicate ON duplicate.id=f.duplicate_of_file_id
        WHERE w.library_id=p_library_id
      ) rows
    ),
    'cardSources', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'card_canonical_key',item->>'work_canonical_key',item->>'file_sha256',item->>'location_label'), '[]'::jsonb)
      FROM (
        SELECT (to_jsonb(s)-ARRAY['id','card_id','work_id','file_id','verified_by','verified_at','create_time','update_time'])||jsonb_build_object('card_canonical_key',c.canonical_key,'work_canonical_key',w.canonical_key,'file_sha256',f.sha256) AS item
        FROM public.theory_card_sources s
        JOIN public.theory_cards c ON c.id=s.card_id
        JOIN public.theory_source_works w ON w.id=s.work_id
        LEFT JOIN public.theory_source_files f ON f.id=s.file_id
        WHERE c.library_id=p_library_id
      ) rows
    ),
    'relations', (
      SELECT COALESCE(jsonb_agg(item ORDER BY item->>'from_canonical_key',item->>'to_canonical_key',item->>'relation_type'), '[]'::jsonb)
      FROM (
        SELECT (to_jsonb(r)-ARRAY['id','from_card_id','to_card_id','status','created_by','reviewed_by','create_time','update_time'])||jsonb_build_object('from_canonical_key',source.canonical_key,'to_canonical_key',target.canonical_key) AS item
        FROM public.theory_card_relations r
        JOIN public.theory_cards source ON source.id=r.from_card_id
        JOIN public.theory_cards target ON target.id=r.to_card_id
        WHERE source.library_id=p_library_id
      ) rows
    )
  )
$$;

-- Drop the previous contract trigger before migrating existing rows; otherwise its
-- old immutability rules can block the one-time hash-contract upgrade.
DROP TRIGGER IF EXISTS theory_package_imports_immutable ON theory_package_imports;

-- Existing imports from the pre-PostgreSQL hash contract are migrated fail-closed.
ALTER TABLE theory_package_imports ADD COLUMN IF NOT EXISTS payload_sha256 TEXT;
ALTER TABLE theory_package_imports ADD COLUMN IF NOT EXISTS payload_receipt_sha256 TEXT;
ALTER TABLE theory_package_imports ADD COLUMN IF NOT EXISTS payload_hash_contract TEXT;
UPDATE theory_package_imports SET payload_sha256=repeat('0',64) WHERE payload_sha256 IS NULL;
UPDATE theory_package_imports SET payload_receipt_sha256=repeat('0',64) WHERE payload_receipt_sha256 IS NULL;
UPDATE theory_package_imports
SET payload_sha256 = CASE WHEN state='promoted' THEN public.theory_package_jsonb_sha256(payload) ELSE repeat('0',64) END,
    payload_receipt_sha256 = CASE WHEN state='promoted' THEN public.theory_package_receipt_sha256(
      package_id, content_digest, package_digest, public.theory_package_jsonb_sha256(payload), schema_version,
      library_id, target_database, desired_release_version
    ) ELSE repeat('0',64) END,
    payload_hash_contract = 'postgres-jsonb-text-sha256-v1',
    object_fingerprints = CASE WHEN state='promoted' THEN
      jsonb_set(
        jsonb_set(
          object_fingerprints || jsonb_build_object('payloadSha256', public.theory_package_jsonb_sha256(payload)),
          '{sha256}', to_jsonb(public.theory_package_jsonb_sha256(object_fingerprints->'snapshot'))
        ),
        '{schemaVersion}', to_jsonb('xinzhili.database-snapshot.v1'::text)
      )
    ELSE object_fingerprints - 'payloadSha256' END
WHERE payload_hash_contract IS DISTINCT FROM 'postgres-jsonb-text-sha256-v1';
ALTER TABLE theory_package_imports ALTER COLUMN payload_sha256 SET NOT NULL;
ALTER TABLE theory_package_imports ALTER COLUMN payload_receipt_sha256 SET NOT NULL;
ALTER TABLE theory_package_imports ALTER COLUMN payload_hash_contract SET DEFAULT 'postgres-jsonb-text-sha256-v1';
ALTER TABLE theory_package_imports ALTER COLUMN payload_hash_contract SET NOT NULL;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='theory_package_imports'::regclass AND conname='ck_theory_package_imports_payload_sha256') THEN
    ALTER TABLE theory_package_imports ADD CONSTRAINT ck_theory_package_imports_payload_sha256 CHECK (payload_sha256 ~ '^[0-9a-f]{64}$');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='theory_package_imports'::regclass AND conname='ck_theory_package_imports_payload_receipt_sha256') THEN
    ALTER TABLE theory_package_imports ADD CONSTRAINT ck_theory_package_imports_payload_receipt_sha256 CHECK (payload_receipt_sha256 ~ '^[0-9a-f]{64}$');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid='theory_package_imports'::regclass AND conname='ck_theory_package_imports_payload_hash_contract') THEN
    ALTER TABLE theory_package_imports ADD CONSTRAINT ck_theory_package_imports_payload_hash_contract CHECK (payload_hash_contract = 'postgres-jsonb-text-sha256-v1');
  END IF;
END $$;

CREATE OR REPLACE FUNCTION protect_theory_package_import_contract()
RETURNS TRIGGER AS $$
DECLARE
  expected_payload_sha256 TEXT;
  expected_receipt_sha256 TEXT;
  valid_fingerprint BOOLEAN;
  legacy_repair BOOLEAN := FALSE;
BEGIN
  expected_payload_sha256 := public.theory_package_jsonb_sha256(NEW.payload);
  expected_receipt_sha256 := public.theory_package_receipt_sha256(
    NEW.package_id, NEW.content_digest, NEW.package_digest, expected_payload_sha256,
    NEW.schema_version, NEW.library_id, NEW.target_database, NEW.desired_release_version
  );
  valid_fingerprint :=
    jsonb_typeof(NEW.object_fingerprints) = 'object'
    AND NEW.object_fingerprints ?& ARRAY['schemaVersion','sha256','payloadSha256','snapshot']
    AND (NEW.object_fingerprints - ARRAY['schemaVersion','sha256','payloadSha256','snapshot']::TEXT[]) = '{}'::jsonb
    AND NEW.object_fingerprints->>'schemaVersion' = 'xinzhili.database-snapshot.v1'
    AND NEW.object_fingerprints->>'payloadSha256' = NEW.payload_sha256
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot') = 'object'
    AND (NEW.object_fingerprints->'snapshot') ?& ARRAY['schemaVersion','cards','practices','sourceWorks','sourceFiles','cardSources','relations']
    AND ((NEW.object_fingerprints->'snapshot') - ARRAY['schemaVersion','cards','practices','sourceWorks','sourceFiles','cardSources','relations']::TEXT[]) = '{}'::jsonb
    AND NEW.object_fingerprints->'snapshot'->>'schemaVersion' = 'xinzhili.database-snapshot.v1'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'cards') = 'array'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'practices') = 'array'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'sourceWorks') = 'array'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'sourceFiles') = 'array'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'cardSources') = 'array'
    AND jsonb_typeof(NEW.object_fingerprints->'snapshot'->'relations') = 'array'
    AND NEW.object_fingerprints->'snapshot' = public.theory_package_database_snapshot(NEW.library_id)
    AND NEW.object_fingerprints->>'sha256' = public.theory_package_jsonb_sha256(NEW.object_fingerprints->'snapshot');

  IF NEW.payload_hash_contract <> 'postgres-jsonb-text-sha256-v1'
    OR NEW.payload_sha256 <> expected_payload_sha256
    OR NEW.payload_receipt_sha256 <> expected_receipt_sha256
    OR valid_fingerprint IS NOT TRUE THEN
    RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='theory package import PostgreSQL hash contract is invalid';
  END IF;

  IF TG_OP = 'INSERT' THEN
    RETURN NEW;
  END IF;

  -- one-way legacy fingerprint repair: only a database-recomputed payload,
  -- receipt, and complete fingerprint may replace the two zero hashes.
  legacy_repair := (
    OLD.state = 'staged'
    AND NEW.state = 'staged'
    AND NEW.promoted_at IS NOT DISTINCT FROM OLD.promoted_at
    AND OLD.payload_hash_contract = 'postgres-jsonb-text-sha256-v1'
    AND OLD.payload_sha256 = repeat('0',64)
    AND OLD.payload_receipt_sha256 = repeat('0',64)
    AND NOT (OLD.object_fingerprints ? 'payloadSha256')
    AND OLD.object_fingerprints->'snapshot' = NEW.object_fingerprints->'snapshot'
  ) IS TRUE;

  IF NEW.package_id IS DISTINCT FROM OLD.package_id
    OR NEW.content_digest IS DISTINCT FROM OLD.content_digest
    OR NEW.package_digest IS DISTINCT FROM OLD.package_digest
    OR NEW.schema_version IS DISTINCT FROM OLD.schema_version
    OR NEW.library_id IS DISTINCT FROM OLD.library_id
    OR NEW.target_database IS DISTINCT FROM OLD.target_database
    OR NEW.desired_release_version IS DISTINCT FROM OLD.desired_release_version
    OR NEW.payload IS DISTINCT FROM OLD.payload
    OR NEW.payload_hash_contract IS DISTINCT FROM OLD.payload_hash_contract
    OR NEW.staged_by IS DISTINCT FROM OLD.staged_by
    OR NEW.staged_at IS DISTINCT FROM OLD.staged_at
    OR NEW.create_time IS DISTINCT FROM OLD.create_time THEN
    RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='theory package import staged contract is immutable';
  END IF;
  IF (NEW.payload_sha256 IS DISTINCT FROM OLD.payload_sha256
      OR NEW.payload_receipt_sha256 IS DISTINCT FROM OLD.payload_receipt_sha256
      OR NEW.object_fingerprints IS DISTINCT FROM OLD.object_fingerprints)
    AND NOT legacy_repair THEN
    RAISE EXCEPTION USING ERRCODE='23514', MESSAGE='theory package import fingerprint contract is immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql SET search_path = pg_catalog;

CREATE TRIGGER theory_package_imports_immutable
  BEFORE INSERT OR UPDATE ON theory_package_imports
  FOR EACH ROW EXECUTE FUNCTION protect_theory_package_import_contract();

DROP TRIGGER IF EXISTS theory_practices_ownership_lock ON theory_practices;
CREATE TRIGGER theory_practices_ownership_lock BEFORE INSERT OR UPDATE ON theory_practices
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_practices_ownership_dependents ON theory_practices;
CREATE CONSTRAINT TRIGGER theory_practices_ownership_dependents
  AFTER INSERT OR UPDATE ON theory_practices
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();

DROP TRIGGER IF EXISTS theory_chunks_ownership_lock ON theory_chunks;
CREATE TRIGGER theory_chunks_ownership_lock BEFORE INSERT OR UPDATE ON theory_chunks
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_chunks_ownership ON theory_chunks;
CREATE CONSTRAINT TRIGGER theory_chunks_ownership
  AFTER INSERT OR UPDATE ON theory_chunks
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();

DROP TRIGGER IF EXISTS theory_library_releases_ownership_lock ON theory_library_releases;
CREATE TRIGGER theory_library_releases_ownership_lock BEFORE INSERT OR UPDATE ON theory_library_releases
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_library_releases_ownership_dependents ON theory_library_releases;
CREATE CONSTRAINT TRIGGER theory_library_releases_ownership_dependents
  AFTER INSERT OR UPDATE ON theory_library_releases
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();

DROP TRIGGER IF EXISTS theory_release_cards_ownership_lock ON theory_release_cards;
CREATE TRIGGER theory_release_cards_ownership_lock BEFORE INSERT OR UPDATE ON theory_release_cards
  FOR EACH ROW EXECUTE FUNCTION lock_theory_ownership_scope();
DROP TRIGGER IF EXISTS theory_release_cards_ownership ON theory_release_cards;
CREATE CONSTRAINT TRIGGER theory_release_cards_ownership
  AFTER INSERT OR UPDATE ON theory_release_cards
  DEFERRABLE INITIALLY DEFERRED
  FOR EACH ROW EXECUTE FUNCTION validate_theory_library_ownership();

-- Existing installations created before the array contracts need explicit, idempotent upgrades.
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_source_works'::regclass AND conname = 'ck_theory_source_works_authors_array') THEN
    ALTER TABLE theory_source_works ADD CONSTRAINT ck_theory_source_works_authors_array CHECK (jsonb_typeof(authors) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_source_works'::regclass AND conname = 'ck_theory_source_works_editors_array') THEN
    ALTER TABLE theory_source_works ADD CONSTRAINT ck_theory_source_works_editors_array CHECK (jsonb_typeof(editors) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_source_works'::regclass AND conname = 'ck_theory_source_works_translators_array') THEN
    ALTER TABLE theory_source_works ADD CONSTRAINT ck_theory_source_works_translators_array CHECK (jsonb_typeof(translators) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_cards'::regclass AND conname = 'ck_theory_cards_aliases_array') THEN
    ALTER TABLE theory_cards ADD CONSTRAINT ck_theory_cards_aliases_array CHECK (jsonb_typeof(aliases) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_cards'::regclass AND conname = 'ck_theory_cards_observable_signals_array') THEN
    ALTER TABLE theory_cards ADD CONSTRAINT ck_theory_cards_observable_signals_array CHECK (jsonb_typeof(observable_signals) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_cards'::regclass AND conname = 'ck_theory_cards_common_triggers_array') THEN
    ALTER TABLE theory_cards ADD CONSTRAINT ck_theory_cards_common_triggers_array CHECK (jsonb_typeof(common_triggers) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_practices'::regclass AND conname = 'ck_theory_practices_steps_array') THEN
    ALTER TABLE theory_practices ADD CONSTRAINT ck_theory_practices_steps_array CHECK (jsonb_typeof(steps) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_practices'::regclass AND conname = 'ck_theory_practices_reflection_prompts_array') THEN
    ALTER TABLE theory_practices ADD CONSTRAINT ck_theory_practices_reflection_prompts_array CHECK (jsonb_typeof(reflection_prompts) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_practices'::regclass AND conname = 'ck_theory_practices_expected_feedback_array') THEN
    ALTER TABLE theory_practices ADD CONSTRAINT ck_theory_practices_expected_feedback_array CHECK (jsonb_typeof(expected_feedback) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_practices'::regclass AND conname = 'ck_theory_practices_stop_conditions_array') THEN
    ALTER TABLE theory_practices ADD CONSTRAINT ck_theory_practices_stop_conditions_array CHECK (jsonb_typeof(stop_conditions) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_practices'::regclass AND conname = 'ck_theory_practices_professional_escalation_array') THEN
    ALTER TABLE theory_practices ADD CONSTRAINT ck_theory_practices_professional_escalation_array CHECK (jsonb_typeof(professional_escalation) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_chunks'::regclass AND conname = 'ck_theory_chunks_keywords_array') THEN
    ALTER TABLE theory_chunks ADD CONSTRAINT ck_theory_chunks_keywords_array CHECK (jsonb_typeof(keywords) = 'array');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conrelid = 'theory_chunks'::regclass AND conname = 'ck_theory_chunks_tags_array') THEN
    ALTER TABLE theory_chunks ADD CONSTRAINT ck_theory_chunks_tags_array CHECK (jsonb_typeof(tags) = 'array');
  END IF;
END $$;

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
DECLARE
  pg_trgm_schema TEXT;
BEGIN
  BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_trgm;
  EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_trgm 不可用，跳过理论库 trigram 索引初始化：%', SQLERRM;
    RETURN;
  END;

  SELECT namespace.nspname INTO pg_trgm_schema
  FROM pg_extension extension
  JOIN pg_namespace namespace ON namespace.oid = extension.extnamespace
  WHERE extension.extname = 'pg_trgm';

  IF pg_trgm_schema IS NOT NULL THEN
    BEGIN
      EXECUTE format($trgm$
        CREATE INDEX IF NOT EXISTS idx_theory_chunks_lexical_trgm
          ON theory_chunks USING gin ((title || ' ' || content || ' ' || keywords::text || ' ' || tags::text) %I.gin_trgm_ops)
      $trgm$, pg_trgm_schema);
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '理论块 trigram 索引不可用，回退到 ILIKE：%', SQLERRM;
    END;
    BEGIN
      EXECUTE format($trgm$
        CREATE INDEX IF NOT EXISTS idx_theory_cards_lexical_trgm
          ON theory_cards USING gin ((canonical_name || ' ' || aliases::text) %I.gin_trgm_ops)
      $trgm$, pg_trgm_schema);
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '理论卡 trigram 索引不可用，回退到 ILIKE：%', SQLERRM;
    END;
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
    -- theory embedding column is independently optional: type/search_path failures degrade safely.
    BEGIN
      EXECUTE 'ALTER TABLE theory_chunk_embeddings ADD COLUMN IF NOT EXISTS embedding vector(1536)';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '理论库 vector 列不可用，保留纯元数据模式：%', SQLERRM;
    END;

    -- theory HNSW index is independently optional: old pgvector/operator class failures do not abort startup.
    BEGIN
      EXECUTE 'CREATE INDEX IF NOT EXISTS idx_theory_chunk_embeddings_hnsw ON theory_chunk_embeddings USING hnsw (embedding vector_cosine_ops)';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '理论库 HNSW 索引不可用，跳过向量索引：%', SQLERRM;
    END;

    -- 兼容旧知识库向量列；任何旧版 pgvector 差异不影响后续 schema。
    BEGIN
      EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedding vector(1536)';
      EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedding_model TEXT NOT NULL DEFAULT ''''';
      EXECUTE 'ALTER TABLE rag_documents ADD COLUMN IF NOT EXISTS embedded_at TIMESTAMPTZ';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '知识库 vector 列不可用，保留关键词模式：%', SQLERRM;
    END;

    BEGIN
      EXECUTE 'CREATE INDEX IF NOT EXISTS idx_rag_documents_embedding ON rag_documents USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)';
    EXCEPTION WHEN OTHERS THEN
      RAISE NOTICE '知识库 ivfflat 索引不可用，跳过向量索引：%', SQLERRM;
    END;
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
  scene       TEXT NOT NULL DEFAULT 'chat',
  context_summary TEXT NOT NULL DEFAULT '',
  context_summary_through_message_id BIGINT NOT NULL DEFAULT 0,
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary TEXT NOT NULL DEFAULT '';
ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS context_summary_through_message_id BIGINT NOT NULL DEFAULT 0;
ALTER TABLE app_chat_sessions ADD COLUMN IF NOT EXISTS scene TEXT NOT NULL DEFAULT 'chat';

CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_user ON app_chat_sessions(app_user_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_card ON app_chat_sessions(card_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_user_card_scene
  ON app_chat_sessions(app_user_id, card_id, scene, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_app_chat_sessions_scene
  ON app_chat_sessions(app_user_id, card_id, scene, updated_at DESC);

CREATE TABLE IF NOT EXISTS app_xinzhili_mode_preferences (
  app_user_id BIGINT PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
  requested_mode TEXT NOT NULL CHECK (requested_mode IN ('normal', 'argument', 'comfort', 'deep_listening')),
  revision BIGINT NOT NULL CHECK (revision > 0),
  update_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_chat_messages (
  id         BIGSERIAL PRIMARY KEY,
  session_id BIGINT NOT NULL REFERENCES app_chat_sessions(id) ON DELETE CASCADE,
  role       TEXT NOT NULL,           -- 'user' | 'assistant'
  content    TEXT NOT NULL DEFAULT '',
  sources    JSONB NOT NULL DEFAULT '[]'::jsonb,
  favorite   BOOLEAN NOT NULL DEFAULT false,  -- 是否被用户收藏
  feedback   TEXT NOT NULL DEFAULT '',        -- 'helpful' | 'inaccurate' | 'continue' | ''
  delivery_status TEXT,
  delivered_text TEXT,
  xinzhili_mode TEXT,
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS favorite BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS feedback TEXT NOT NULL DEFAULT '';
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS message_type TEXT NOT NULL DEFAULT 'text';
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS audio_duration_ms INTEGER NOT NULL DEFAULT 0;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS transcript TEXT NOT NULL DEFAULT '';
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS delivery_status TEXT;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS delivered_text TEXT;
ALTER TABLE app_chat_messages ADD COLUMN IF NOT EXISTS xinzhili_mode TEXT;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'app_chat_messages_delivery_status_check' AND conrelid = 'app_chat_messages'::regclass) THEN
    ALTER TABLE app_chat_messages ADD CONSTRAINT app_chat_messages_delivery_status_check
      CHECK (delivery_status IS NULL OR delivery_status IN ('generated', 'synthesizing', 'sent', 'played', 'tts_failed', 'interrupted', 'unconfirmed'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'app_chat_messages_xinzhili_mode_check' AND conrelid = 'app_chat_messages'::regclass) THEN
    ALTER TABLE app_chat_messages ADD CONSTRAINT app_chat_messages_xinzhili_mode_check
      CHECK (xinzhili_mode IS NULL OR xinzhili_mode IN ('normal', 'argument', 'comfort', 'deep_listening'));
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'app_chat_messages_delivered_text_check' AND conrelid = 'app_chat_messages'::regclass) THEN
    ALTER TABLE app_chat_messages ADD CONSTRAINT app_chat_messages_delivered_text_check
      CHECK (delivered_text IS NULL OR left(content, char_length(delivered_text)) = delivered_text);
  END IF;
END $$;

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

ALTER TABLE app_daily_quiz_questions ADD COLUMN IF NOT EXISTS type_weights JSONB;
UPDATE app_daily_quiz_questions SET type_weights = '{}'::jsonb WHERE type_weights IS NULL;
ALTER TABLE app_daily_quiz_questions ALTER COLUMN type_weights SET DEFAULT '{}'::jsonb;
ALTER TABLE app_daily_quiz_questions ALTER COLUMN type_weights SET NOT NULL;

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

-- ============ 老师课堂（系列、音视频课件、上传、权益与学习进度）=====
CREATE TABLE IF NOT EXISTS classroom_series (
  id BIGSERIAL PRIMARY KEY,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  cover_url TEXT NOT NULL DEFAULT '',
  cover_asset_id BIGINT REFERENCES upload_assets(id) ON DELETE SET NULL,
  manual_cover_object_key TEXT NOT NULL DEFAULT '',
  cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9' CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1')),
  teacher_key TEXT NOT NULL DEFAULT '',
  teacher_name_snapshot TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','offline')),
  playback_blocked BOOLEAN NOT NULL DEFAULT false,
  access_level TEXT NOT NULL DEFAULT 'public' CHECK (access_level IN ('public','login','member','paid')),
  price_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  published_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (access_level = 'paid' AND price_cents > 0 OR access_level <> 'paid' AND price_cents = 0)
);

ALTER TABLE classroom_series ADD COLUMN IF NOT EXISTS manual_cover_object_key TEXT NOT NULL DEFAULT '';
ALTER TABLE classroom_series ADD COLUMN IF NOT EXISTS cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9' CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1'));
ALTER TABLE classroom_series DROP CONSTRAINT IF EXISTS classroom_series_cover_aspect_ratio_check;
ALTER TABLE classroom_series ADD CONSTRAINT classroom_series_cover_aspect_ratio_check CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1'));

CREATE TABLE IF NOT EXISTS classroom_contents (
  id BIGSERIAL PRIMARY KEY,
  series_id BIGINT REFERENCES classroom_series(id) ON DELETE RESTRICT,
  show_as_standalone BOOLEAN NOT NULL DEFAULT false,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL CHECK (content_type IN ('video','audio')),
  media_asset_id BIGINT,
  cover_url TEXT NOT NULL DEFAULT '',
  manual_cover_object_key TEXT NOT NULL DEFAULT '',
  cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9' CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1')),
  duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
  teacher_key TEXT NOT NULL DEFAULT '',
  teacher_name_snapshot TEXT NOT NULL DEFAULT '',
  recorded_at TIMESTAMPTZ,
  badge TEXT NOT NULL DEFAULT '',
  tags JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(tags) = 'array'),
  episode_no INTEGER NOT NULL DEFAULT 0 CHECK (episode_no >= 0),
  sort_order INTEGER NOT NULL DEFAULT 0 CHECK (sort_order >= 0),
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','processing','ready','published','offline','archived','failed')),
  playback_blocked BOOLEAN NOT NULL DEFAULT false,
  access_level TEXT NOT NULL DEFAULT 'inherit' CHECK (access_level IN ('inherit','public','login','member','paid')),
  price_cents INTEGER NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  published_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  updated_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (series_id IS NOT NULL OR access_level <> 'inherit'),
  CHECK (series_id IS NOT NULL OR show_as_standalone = false),
  CHECK (access_level = 'paid' AND price_cents > 0 OR access_level <> 'paid' AND price_cents = 0)
);

ALTER TABLE classroom_contents ADD COLUMN IF NOT EXISTS manual_cover_object_key TEXT NOT NULL DEFAULT '';
ALTER TABLE classroom_contents ADD COLUMN IF NOT EXISTS cover_aspect_ratio TEXT NOT NULL DEFAULT '16:9' CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1'));
ALTER TABLE classroom_contents DROP CONSTRAINT IF EXISTS classroom_contents_cover_aspect_ratio_check;
ALTER TABLE classroom_contents ADD CONSTRAINT classroom_contents_cover_aspect_ratio_check CHECK (cover_aspect_ratio IN ('16:9','9:16','1:1'));
ALTER TABLE classroom_contents DROP CONSTRAINT IF EXISTS classroom_contents_status_check;
ALTER TABLE classroom_contents ADD CONSTRAINT classroom_contents_status_check CHECK (status IN ('draft','processing','ready','published','offline','archived','failed'));

CREATE TABLE IF NOT EXISTS classroom_media_assets (
  id BIGSERIAL PRIMARY KEY,
  bucket TEXT NOT NULL,
  object_key TEXT NOT NULL,
  etag TEXT NOT NULL DEFAULT '',
  checksum TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL CHECK (content_type IN ('video','audio')),
  size_bytes BIGINT NOT NULL DEFAULT 0 CHECK (size_bytes >= 0),
  duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (duration_seconds >= 0),
  width INTEGER NOT NULL DEFAULT 0 CHECK (width >= 0),
  height INTEGER NOT NULL DEFAULT 0 CHECK (height >= 0),
  cover_object_key TEXT NOT NULL DEFAULT '',
  storage_status TEXT NOT NULL DEFAULT 'pending' CHECK (storage_status IN ('pending','uploaded','processing','ready','failed','deleted')),
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (bucket, object_key),
  CHECK (storage_status <> 'ready' OR (object_key <> '' AND etag <> '' AND checksum <> '' AND size_bytes > 0))
);

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'fk_classroom_contents_media_asset') THEN
    ALTER TABLE classroom_contents
      ADD CONSTRAINT fk_classroom_contents_media_asset
      FOREIGN KEY (media_asset_id) REFERENCES classroom_media_assets(id) ON DELETE SET NULL;
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS classroom_upload_tasks (
  id BIGSERIAL PRIMARY KEY,
  content_id BIGINT NOT NULL REFERENCES classroom_contents(id) ON DELETE CASCADE,
  creator_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  original_filename TEXT NOT NULL DEFAULT '',
  oss_upload_id TEXT NOT NULL,
  object_key TEXT NOT NULL,
  expected_size BIGINT NOT NULL CHECK (expected_size > 0),
  checksum TEXT NOT NULL DEFAULT '',
  completed_parts INTEGER NOT NULL DEFAULT 0 CHECK (completed_parts >= 0),
  completed_bytes BIGINT NOT NULL DEFAULT 0 CHECK (completed_bytes >= 0 AND completed_bytes <= expected_size),
  part_size BIGINT NOT NULL CHECK (part_size > 0),
  max_parts INTEGER NOT NULL CHECK (max_parts > 0),
  status TEXT NOT NULL DEFAULT 'initiated' CHECK (status IN ('initiating','initiated','uploading','completing','cleaning','completed','aborted','expired','failed')),
  expires_at TIMESTAMPTZ NOT NULL,
  attempt_count INTEGER NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
  cleanup_status TEXT NOT NULL DEFAULT 'pending' CHECK (cleanup_status IN ('pending','retained','cleaned','failed')),
  media_asset_id BIGINT REFERENCES classroom_media_assets(id) ON DELETE SET NULL,
  failure_reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (content_id),
  UNIQUE (oss_upload_id)
);

ALTER TABLE classroom_upload_tasks ADD COLUMN IF NOT EXISTS original_filename TEXT NOT NULL DEFAULT '';
ALTER TABLE classroom_upload_tasks ADD COLUMN IF NOT EXISTS completed_parts INTEGER NOT NULL DEFAULT 0;
ALTER TABLE classroom_upload_tasks ADD COLUMN IF NOT EXISTS completed_bytes BIGINT NOT NULL DEFAULT 0;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'classroom_upload_tasks_progress_parts_check') THEN
    ALTER TABLE classroom_upload_tasks ADD CONSTRAINT classroom_upload_tasks_progress_parts_check CHECK (completed_parts >= 0 AND completed_parts <= max_parts);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'classroom_upload_tasks_progress_bytes_check') THEN
    ALTER TABLE classroom_upload_tasks ADD CONSTRAINT classroom_upload_tasks_progress_bytes_check CHECK (completed_bytes >= 0 AND completed_bytes <= expected_size);
  END IF;
END $$;

-- Upgrade databases created before the completing claim state was introduced.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'classroom_upload_tasks_status_check') THEN
    ALTER TABLE classroom_upload_tasks DROP CONSTRAINT IF EXISTS classroom_upload_tasks_status_check;
  END IF;
  ALTER TABLE classroom_upload_tasks
    ADD CONSTRAINT classroom_upload_tasks_status_check CHECK (status IN ('initiating','initiated','uploading','completing','cleaning','completed','aborted','expired','failed'));
END $$;

CREATE TABLE IF NOT EXISTS classroom_entitlements (
  id BIGSERIAL PRIMARY KEY,
  wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  series_id BIGINT REFERENCES classroom_series(id) ON DELETE RESTRICT,
  content_id BIGINT REFERENCES classroom_contents(id) ON DELETE RESTRICT,
  order_id BIGINT REFERENCES orders(id) ON DELETE SET NULL,
  source TEXT NOT NULL CHECK (source IN ('purchase','manual')),
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK ((series_id IS NOT NULL)::int + (content_id IS NOT NULL)::int = 1),
  CHECK (expires_at IS NULL OR expires_at > created_at)
);

CREATE TABLE IF NOT EXISTS classroom_progress (
  wx_user_id BIGINT NOT NULL REFERENCES wx_users(id) ON DELETE CASCADE,
  content_id BIGINT NOT NULL REFERENCES classroom_contents(id) ON DELETE CASCADE,
  position_seconds INTEGER NOT NULL DEFAULT 0 CHECK (position_seconds >= 0),
  completed BOOLEAN NOT NULL DEFAULT false,
  last_played_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (wx_user_id, content_id)
);

CREATE INDEX IF NOT EXISTS idx_classroom_series_public_list ON classroom_series(status, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_classroom_series_access ON classroom_series(access_level, status);
CREATE INDEX IF NOT EXISTS idx_classroom_contents_series ON classroom_contents(series_id, status, episode_no, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_classroom_contents_standalone ON classroom_contents(show_as_standalone, status, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_classroom_contents_media ON classroom_contents(media_asset_id);
CREATE INDEX IF NOT EXISTS idx_classroom_upload_tasks_status ON classroom_upload_tasks(status, expires_at);
CREATE INDEX IF NOT EXISTS idx_classroom_upload_tasks_creator ON classroom_upload_tasks(creator_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_classroom_entitlements_user ON classroom_entitlements(wx_user_id, revoked_at, expires_at);
-- Remove the first migration's active-target indexes: their predicate ignored expires_at
-- and therefore permanently blocked renewal rows after an entitlement expired.
DROP INDEX IF EXISTS uq_classroom_entitlement_active_series;
DROP INDEX IF EXISTS uq_classroom_entitlement_active_content;
CREATE UNIQUE INDEX IF NOT EXISTS uq_classroom_entitlements_order ON classroom_entitlements(order_id) WHERE order_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_classroom_progress_recent ON classroom_progress(wx_user_id, last_played_at DESC);

-- ----- 企业培训推广：媒体、案例、方案、授权、线索和基础归因 -----
CREATE TABLE IF NOT EXISTS promotion_media_assets (
  id BIGSERIAL PRIMARY KEY,
  asset_key TEXT NOT NULL UNIQUE,
  kind TEXT NOT NULL CHECK (kind IN ('image', 'video', 'audio', 'document')),
  source_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  object_key TEXT NOT NULL UNIQUE CHECK (object_key LIKE 'promotion/%'),
  sha256 TEXT NOT NULL CHECK (length(sha256) = 64),
  byte_size BIGINT NOT NULL DEFAULT 0 CHECK (byte_size >= 0),
  original_filename TEXT NOT NULL DEFAULT '',
  content_type TEXT NOT NULL DEFAULT '',
  probe_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  derived_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  state TEXT NOT NULL DEFAULT 'reserved' CONSTRAINT promotion_media_assets_state_check CHECK (state IN ('reserved','uploading','uploaded','probing','transcoding','validating','qa_pending','ready','quarantined','rejected','failed')),
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  ready_qa_review_id BIGINT,
  ready_attempt_id BIGINT,
  qa_result TEXT NOT NULL DEFAULT 'pending' CHECK (qa_result IN ('pending','passed','failed')),
  qa_approved_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  qa_approved_at TIMESTAMPTZ,
  qa_note TEXT NOT NULL DEFAULT '',
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  updated_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (source_asset_id IS NULL OR source_asset_id <> id),
  CONSTRAINT promotion_media_assets_ready_snapshot_check CHECK (state <> 'ready' OR (qa_result = 'passed' AND ready_qa_review_id IS NOT NULL AND ready_attempt_id IS NOT NULL AND qa_approved_by IS NOT NULL AND qa_approved_at IS NOT NULL))
);
ALTER TABLE promotion_media_assets ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE promotion_media_assets ADD COLUMN IF NOT EXISTS ready_qa_review_id BIGINT;
ALTER TABLE promotion_media_assets ADD COLUMN IF NOT EXISTS ready_attempt_id BIGINT;
ALTER TABLE promotion_media_assets DROP CONSTRAINT IF EXISTS promotion_media_assets_state_check;
ALTER TABLE promotion_media_assets ADD CONSTRAINT promotion_media_assets_state_check
  CHECK (state IN ('reserved','uploading','uploaded','probing','transcoding','validating','qa_pending','ready','quarantined','rejected','failed'));
ALTER TABLE promotion_media_assets DROP CONSTRAINT IF EXISTS promotion_media_assets_ready_snapshot_check;
ALTER TABLE promotion_media_assets ADD CONSTRAINT promotion_media_assets_ready_snapshot_check
  CHECK (state <> 'ready' OR (qa_result = 'passed' AND ready_qa_review_id IS NOT NULL AND ready_attempt_id IS NOT NULL AND qa_approved_by IS NOT NULL AND qa_approved_at IS NOT NULL));
CREATE INDEX IF NOT EXISTS idx_promotion_media_assets_source ON promotion_media_assets(source_asset_id);
CREATE INDEX IF NOT EXISTS idx_promotion_media_assets_state ON promotion_media_assets(state, id);

CREATE TABLE IF NOT EXISTS promotion_media_upload_tasks (
  id BIGSERIAL PRIMARY KEY,
  upload_key TEXT NOT NULL UNIQUE,
  asset_id BIGINT NOT NULL REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  object_key TEXT NOT NULL UNIQUE CHECK (object_key LIKE 'promotion/%'),
  provider_upload_id TEXT NOT NULL DEFAULT '',
  expected_sha256 TEXT NOT NULL CHECK (length(expected_sha256) = 64),
  expected_size BIGINT NOT NULL CHECK (expected_size >= 0),
  part_size BIGINT NOT NULL CHECK (part_size > 0),
  state TEXT NOT NULL DEFAULT 'reserved' CHECK (state IN ('reserved','uploading','completing','completed','aborted','expired','failed')),
  completed_parts JSONB NOT NULL DEFAULT '[]'::jsonb,
  expires_at TIMESTAMPTZ NOT NULL,
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_promotion_media_upload_tasks_asset ON promotion_media_upload_tasks(asset_id, id DESC);

CREATE TABLE IF NOT EXISTS promotion_media_processing_attempts (
  id BIGSERIAL PRIMARY KEY,
  asset_id BIGINT NOT NULL REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  attempt_number INT NOT NULL CHECK (attempt_number > 0),
  state TEXT NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','probing','transcoding','validating','succeeded','quarantined','failed','cancelled')),
  lease_owner TEXT NOT NULL DEFAULT '',
  lease_expires_at TIMESTAMPTZ,
  input_object_key TEXT NOT NULL DEFAULT '',
  output_object_key TEXT NOT NULL DEFAULT '',
  output_sha256 TEXT CHECK (output_sha256 IS NULL OR length(output_sha256) = 64),
  probe_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  derived_metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_code TEXT NOT NULL DEFAULT '',
  error_detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  error_log_excerpt TEXT NOT NULL DEFAULT '',
  retry_after TIMESTAMPTZ,
  started_at TIMESTAMPTZ,
  finished_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(asset_id, attempt_number),
  CHECK ((lease_owner = '' AND lease_expires_at IS NULL) OR (lease_owner <> '' AND lease_expires_at IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_promotion_media_attempts_lease ON promotion_media_processing_attempts(state, lease_expires_at, id);

CREATE TABLE IF NOT EXISTS promotion_media_qa_reviews (
  id BIGSERIAL PRIMARY KEY,
  asset_id BIGINT NOT NULL REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  asset_version BIGINT NOT NULL CHECK (asset_version > 0),
  attempt_id BIGINT NOT NULL REFERENCES promotion_media_processing_attempts(id) ON DELETE RESTRICT,
  output_sha256 TEXT NOT NULL CHECK (length(output_sha256) = 64),
  qa_result TEXT NOT NULL CHECK (qa_result IN ('passed','failed')),
  approved_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  approved_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  qa_note TEXT NOT NULL DEFAULT '',
  approval_txid BIGINT NOT NULL DEFAULT txid_current()
);
ALTER TABLE promotion_media_qa_reviews ADD COLUMN IF NOT EXISTS asset_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE promotion_media_qa_reviews ADD COLUMN IF NOT EXISTS attempt_id BIGINT;
ALTER TABLE promotion_media_qa_reviews ADD COLUMN IF NOT EXISTS output_sha256 TEXT;
DROP TRIGGER IF EXISTS trg_promotion_media_qa_reviews_append_only ON promotion_media_qa_reviews;
UPDATE promotion_media_qa_reviews review
SET asset_version = asset.version,
    output_sha256 = asset.sha256,
    attempt_id = COALESCE(review.attempt_id, (
      SELECT attempt.id FROM promotion_media_processing_attempts attempt
      WHERE attempt.asset_id=review.asset_id
      ORDER BY attempt.attempt_number DESC LIMIT 1
    ))
FROM promotion_media_assets asset
WHERE asset.id=review.asset_id
  AND (review.output_sha256 IS NULL OR review.attempt_id IS NULL);
ALTER TABLE promotion_media_qa_reviews ALTER COLUMN asset_version SET NOT NULL;
ALTER TABLE promotion_media_qa_reviews ALTER COLUMN attempt_id SET NOT NULL;
ALTER TABLE promotion_media_qa_reviews ALTER COLUMN output_sha256 SET NOT NULL;
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_promotion_media_qa_attempt' AND conrelid='promotion_media_qa_reviews'::regclass) THEN
    ALTER TABLE promotion_media_qa_reviews ADD CONSTRAINT fk_promotion_media_qa_attempt
      FOREIGN KEY (attempt_id) REFERENCES promotion_media_processing_attempts(id) ON DELETE RESTRICT;
  END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_promotion_media_qa_reviews_asset ON promotion_media_qa_reviews(asset_id, id DESC);

CREATE OR REPLACE FUNCTION promotion_media_attempt_identity_guard()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM promotion_media_qa_reviews review WHERE review.attempt_id=OLD.id) THEN
    RAISE EXCEPTION 'QA-bound promotion media processing attempt identity is immutable';
  END IF;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_promotion_media_attempt_identity_guard ON promotion_media_processing_attempts;
CREATE TRIGGER trg_promotion_media_attempt_identity_guard
BEFORE UPDATE OF asset_id, state, output_object_key, output_sha256, finished_at ON promotion_media_processing_attempts
FOR EACH ROW EXECUTE FUNCTION promotion_media_attempt_identity_guard();

CREATE OR REPLACE FUNCTION promotion_media_qa_reviews_stamp()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  SELECT asset.version, asset.sha256
  INTO NEW.asset_version, NEW.output_sha256
  FROM promotion_media_assets asset
  WHERE asset.id = NEW.asset_id;
  IF NOT FOUND THEN
    RAISE EXCEPTION 'QA review asset does not exist';
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM promotion_media_processing_attempts attempt
    WHERE attempt.id = NEW.attempt_id
      AND attempt.asset_id = NEW.asset_id
      AND attempt.state = 'succeeded'
      AND attempt.finished_at IS NOT NULL
      AND btrim(attempt.output_object_key) <> ''
      AND attempt.output_sha256 IS NOT NULL
      AND attempt.output_sha256 = NEW.output_sha256
  ) THEN
    RAISE EXCEPTION 'QA review attempt does not match the current asset content';
  END IF;
  NEW.approval_txid := txid_current();
  NEW.approved_at := now();
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_promotion_media_qa_reviews_stamp ON promotion_media_qa_reviews;
CREATE TRIGGER trg_promotion_media_qa_reviews_stamp
BEFORE INSERT ON promotion_media_qa_reviews
FOR EACH ROW EXECUTE FUNCTION promotion_media_qa_reviews_stamp();

CREATE OR REPLACE FUNCTION promotion_media_qa_reviews_append_only()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'promotion media QA reviews are append-only';
END;
$$;
DROP TRIGGER IF EXISTS trg_promotion_media_qa_reviews_append_only ON promotion_media_qa_reviews;
CREATE TRIGGER trg_promotion_media_qa_reviews_append_only
BEFORE UPDATE OR DELETE ON promotion_media_qa_reviews
FOR EACH ROW EXECUTE FUNCTION promotion_media_qa_reviews_append_only();

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_promotion_media_ready_review' AND conrelid='promotion_media_assets'::regclass) THEN
    ALTER TABLE promotion_media_assets
      ADD CONSTRAINT fk_promotion_media_ready_review FOREIGN KEY (ready_qa_review_id) REFERENCES promotion_media_qa_reviews(id) ON DELETE RESTRICT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_promotion_media_ready_attempt' AND conrelid='promotion_media_assets'::regclass) THEN
    ALTER TABLE promotion_media_assets
      ADD CONSTRAINT fk_promotion_media_ready_attempt FOREIGN KEY (ready_attempt_id) REFERENCES promotion_media_processing_attempts(id) ON DELETE RESTRICT;
  END IF;
END $$;

CREATE OR REPLACE FUNCTION promotion_media_asset_identity_guard()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
  identity_changed BOOLEAN;
BEGIN
  identity_changed := OLD.asset_key IS DISTINCT FROM NEW.asset_key
    OR OLD.kind IS DISTINCT FROM NEW.kind
    OR OLD.object_key IS DISTINCT FROM NEW.object_key
    OR OLD.sha256 IS DISTINCT FROM NEW.sha256
    OR OLD.byte_size IS DISTINCT FROM NEW.byte_size
    OR OLD.source_asset_id IS DISTINCT FROM NEW.source_asset_id
    OR OLD.original_filename IS DISTINCT FROM NEW.original_filename
    OR OLD.content_type IS DISTINCT FROM NEW.content_type
    OR OLD.probe_metadata IS DISTINCT FROM NEW.probe_metadata
    OR OLD.derived_metadata IS DISTINCT FROM NEW.derived_metadata;
  IF identity_changed THEN
    IF OLD.state = 'ready' THEN
      RAISE EXCEPTION 'ready promotion media identity is immutable';
    END IF;
    NEW.version := OLD.version + 1;
    NEW.qa_result := 'pending';
    NEW.ready_qa_review_id := NULL;
    NEW.ready_attempt_id := NULL;
    NEW.qa_approved_by := NULL;
    NEW.qa_approved_at := NULL;
    NEW.qa_note := '';
    IF NEW.state IN ('qa_pending','ready') THEN
      NEW.state := 'uploaded';
    END IF;
  ELSIF NEW.version IS DISTINCT FROM OLD.version THEN
    RAISE EXCEPTION 'promotion media version is database managed';
  END IF;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_promotion_media_asset_identity_guard ON promotion_media_assets;
CREATE TRIGGER trg_promotion_media_asset_identity_guard
BEFORE UPDATE OF asset_key, kind, object_key, sha256, byte_size, source_asset_id, original_filename, content_type, probe_metadata, derived_metadata, version ON promotion_media_assets
FOR EACH ROW EXECUTE FUNCTION promotion_media_asset_identity_guard();

CREATE OR REPLACE FUNCTION promotion_media_ready_requires_current_qa()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.state = 'ready' THEN
    IF TG_OP = 'INSERT' THEN
      RAISE EXCEPTION 'promotion media must enter ready from qa_pending';
    ELSIF OLD.state IS DISTINCT FROM 'ready' AND OLD.state IS DISTINCT FROM 'qa_pending' THEN
      RAISE EXCEPTION 'promotion media must enter ready from qa_pending';
    END IF;
  END IF;
  IF NEW.state = 'ready' AND (
      TG_OP = 'INSERT'
      OR OLD.state IS DISTINCT FROM NEW.state
      OR OLD.qa_result IS DISTINCT FROM NEW.qa_result
      OR OLD.qa_approved_by IS DISTINCT FROM NEW.qa_approved_by
      OR OLD.qa_approved_at IS DISTINCT FROM NEW.qa_approved_at
      OR OLD.qa_note IS DISTINCT FROM NEW.qa_note
    ) THEN
    IF NOT EXISTS (
      SELECT 1
      FROM promotion_media_qa_reviews review
      JOIN promotion_media_processing_attempts attempt ON attempt.id=review.attempt_id
      WHERE review.asset_id = NEW.id
        AND review.id = NEW.ready_qa_review_id
        AND review.asset_version = NEW.version
        AND review.attempt_id = NEW.ready_attempt_id
        AND review.output_sha256 = NEW.sha256
        AND review.qa_result = 'passed'
        AND review.approved_by = NEW.qa_approved_by
        AND review.approved_at = NEW.qa_approved_at
        AND review.qa_note = NEW.qa_note
        AND review.approval_txid = txid_current()
        AND attempt.asset_id = NEW.id
        AND attempt.state = 'succeeded'
        AND attempt.finished_at IS NOT NULL
        AND btrim(attempt.output_object_key) <> ''
        AND attempt.output_sha256 = NEW.sha256
    ) THEN
      RAISE EXCEPTION 'ready transition requires a passing QA review in the current transaction';
    END IF;
  END IF;
  RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_promotion_media_ready_requires_current_qa ON promotion_media_assets;
CREATE TRIGGER trg_promotion_media_ready_requires_current_qa
BEFORE INSERT OR UPDATE OF state, ready_qa_review_id, ready_attempt_id, qa_result, qa_approved_by, qa_approved_at, qa_note ON promotion_media_assets
FOR EACH ROW EXECUTE FUNCTION promotion_media_ready_requires_current_qa();

CREATE TABLE IF NOT EXISTS enterprise_trainers (
  id BIGSERIAL PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  title TEXT NOT NULL DEFAULT '',
  avatar_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  short_bio TEXT NOT NULL DEFAULT '',
  full_bio TEXT NOT NULL DEFAULT '',
  specialties JSONB NOT NULL DEFAULT '[]'::jsonb,
  credentials JSONB NOT NULL DEFAULT '[]'::jsonb,
  service_industries JSONB NOT NULL DEFAULT '[]'::jsonb,
  experience_summary TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','review','published','offline')),
  sort_order INT NOT NULL DEFAULT 0,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  updated_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS training_topics (
  id BIGSERIAL PRIMARY KEY,
  key TEXT NOT NULL UNIQUE CHECK (key IN ('team-communication', 'leadership', 'cohesion', 'culture', 'employee-growth')),
  title TEXT NOT NULL,
  sort_order INT NOT NULL DEFAULT 0,
  enabled BOOLEAN NOT NULL DEFAULT true,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS training_cases (
  id BIGSERIAL PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  cover_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  company_display_name TEXT NOT NULL DEFAULT '',
  company_internal_name_encrypted BYTEA,
  industry TEXT NOT NULL DEFAULT '',
  city TEXT NOT NULL DEFAULT '',
  participant_range TEXT NOT NULL DEFAULT '',
  training_date DATE,
  duration_label TEXT NOT NULL DEFAULT '',
  business_challenges JSONB NOT NULL DEFAULT '[]'::jsonb,
  training_goals JSONB NOT NULL DEFAULT '[]'::jsonb,
  training_modules JSONB NOT NULL DEFAULT '[]'::jsonb,
  training_methods JSONB NOT NULL DEFAULT '[]'::jsonb,
  trainer_id BIGINT NOT NULL REFERENCES enterprise_trainers(id) ON DELETE RESTRICT,
  trainer_name_snapshot TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','review','published','offline')),
  authorization_status TEXT NOT NULL DEFAULT 'pending' CHECK (authorization_status IN ('pending','approved','expired','revoked')),
  featured BOOLEAN NOT NULL DEFAULT false,
  sort_order INT NOT NULL DEFAULT 0,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  published_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  updated_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_training_cases_public ON training_cases(status, featured DESC, sort_order, id DESC);

CREATE TABLE IF NOT EXISTS enterprise_solutions (
  id BIGSERIAL PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  title TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  cover_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  audiences JSONB NOT NULL DEFAULT '[]'::jsonb,
  problems JSONB NOT NULL DEFAULT '[]'::jsonb,
  goals JSONB NOT NULL DEFAULT '[]'::jsonb,
  modules JSONB NOT NULL DEFAULT '[]'::jsonb,
  delivery_methods JSONB NOT NULL DEFAULT '[]'::jsonb,
  recommended_participants TEXT NOT NULL DEFAULT '',
  recommended_duration TEXT NOT NULL DEFAULT '',
  customizable_items JSONB NOT NULL DEFAULT '[]'::jsonb,
  trainer_id BIGINT NOT NULL REFERENCES enterprise_trainers(id) ON DELETE RESTRICT,
  trainer_name_snapshot TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','review','published','offline')),
  featured BOOLEAN NOT NULL DEFAULT false,
  sort_order INT NOT NULL DEFAULT 0,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  published_at TIMESTAMPTZ,
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  updated_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS publication_consents (
  id BIGSERIAL PRIMARY KEY,
  subject_type TEXT NOT NULL CHECK (subject_type IN ('company','person','media_asset','testimonial','document_screen')),
  subject_reference TEXT NOT NULL,
  display_alias TEXT NOT NULL DEFAULT '',
  channels JSONB NOT NULL DEFAULT '[]'::jsonb,
  usage_scopes JSONB NOT NULL DEFAULT '[]'::jsonb,
  evidence_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  contract_reference TEXT NOT NULL DEFAULT '',
  effective_at TIMESTAMPTZ,
  expires_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','expired','revoked')),
  reviewed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  reviewed_at TIMESTAMPTZ,
  revocation_reason TEXT NOT NULL DEFAULT '',
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(subject_type, subject_reference, version),
  CONSTRAINT uq_publication_consents_id_subject UNIQUE(id, subject_type),
  CHECK (expires_at IS NULL OR effective_at IS NULL OR expires_at > effective_at)
);

CREATE TABLE IF NOT EXISTS training_case_media (
  id BIGSERIAL PRIMARY KEY,
  case_id BIGINT NOT NULL REFERENCES training_cases(id) ON DELETE RESTRICT,
  media_asset_id BIGINT NOT NULL REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  role TEXT NOT NULL CHECK (role IN ('promo','highlight','topic_clip','gallery')),
  position INT NOT NULL CHECK (position >= 0),
  caption TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published','offline')),
  UNIQUE(case_id, role, position),
  UNIQUE(case_id, media_asset_id, role)
);

CREATE TABLE IF NOT EXISTS training_case_solutions (
  case_id BIGINT NOT NULL REFERENCES training_cases(id) ON DELETE RESTRICT,
  solution_id BIGINT NOT NULL REFERENCES enterprise_solutions(id) ON DELETE RESTRICT,
  position INT NOT NULL CHECK (position >= 0),
  PRIMARY KEY(case_id, solution_id),
  UNIQUE(case_id, position)
);

CREATE TABLE IF NOT EXISTS training_case_topics (
  case_id BIGINT NOT NULL REFERENCES training_cases(id) ON DELETE RESTRICT,
  topic_id BIGINT NOT NULL REFERENCES training_topics(id) ON DELETE RESTRICT,
  position INT NOT NULL CHECK (position >= 0),
  PRIMARY KEY(case_id, topic_id),
  UNIQUE(case_id, position)
);

CREATE TABLE IF NOT EXISTS training_case_testimonials (
  id BIGSERIAL PRIMARY KEY,
  case_id BIGINT NOT NULL REFERENCES training_cases(id) ON DELETE RESTRICT,
  quote TEXT NOT NULL,
  speaker_display TEXT NOT NULL DEFAULT '',
  speaker_role TEXT NOT NULL DEFAULT '',
  provenance TEXT NOT NULL,
  consent_id BIGINT REFERENCES publication_consents(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','review','approved','rejected','offline')),
  position INT NOT NULL CHECK (position >= 0),
  UNIQUE(case_id, position)
);

CREATE TABLE IF NOT EXISTS training_case_claims (
  id BIGSERIAL PRIMARY KEY,
  case_id BIGINT NOT NULL REFERENCES training_cases(id) ON DELETE RESTRICT,
  claim_type TEXT NOT NULL CHECK (claim_type IN ('fact','client_quote','editorial_summary')),
  statement TEXT NOT NULL,
  source_reference TEXT NOT NULL,
  reviewed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  reviewed_at TIMESTAMPTZ,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','rejected')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS training_case_consent_links (
  id BIGSERIAL PRIMARY KEY,
  case_id BIGINT REFERENCES training_cases(id) ON DELETE RESTRICT,
  consent_id BIGINT NOT NULL REFERENCES publication_consents(id) ON DELETE RESTRICT,
  media_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  testimonial_id BIGINT REFERENCES training_case_testimonials(id) ON DELETE RESTRICT,
  subject_type TEXT NOT NULL CHECK (subject_type IN ('company','person','media_asset','testimonial','document_screen')),
  subject_id BIGINT NOT NULL,
  use_scope TEXT NOT NULL,
  requirement_key TEXT NOT NULL,
  required BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(consent_id, subject_type, subject_id, use_scope),
  CONSTRAINT fk_training_case_consent_subject FOREIGN KEY (consent_id, subject_type) REFERENCES publication_consents(id, subject_type) ON DELETE RESTRICT,
  CHECK (case_id IS NOT NULL OR media_asset_id IS NOT NULL OR testimonial_id IS NOT NULL),
  CHECK (subject_type <> 'media_asset' OR media_asset_id = subject_id),
  CONSTRAINT chk_training_case_consent_testimonial_subject CHECK (subject_type <> 'testimonial' OR testimonial_id = subject_id)
);
ALTER TABLE training_case_consent_links ALTER COLUMN case_id DROP NOT NULL;
ALTER TABLE training_case_consent_links ADD COLUMN IF NOT EXISTS subject_type TEXT;
ALTER TABLE training_case_consent_links ADD COLUMN IF NOT EXISTS subject_id BIGINT;
ALTER TABLE training_case_consent_links ADD COLUMN IF NOT EXISTS use_scope TEXT;
UPDATE training_case_consent_links link
SET subject_type = consent.subject_type,
    subject_id = CASE
      WHEN consent.subject_type = 'media_asset' AND link.media_asset_id IS NOT NULL THEN link.media_asset_id
      WHEN consent.subject_type = 'testimonial' AND link.testimonial_id IS NOT NULL THEN link.testimonial_id
      ELSE link.consent_id
    END,
    use_scope = COALESCE(NULLIF(link.requirement_key, ''), 'publication')
FROM publication_consents consent
WHERE consent.id = link.consent_id
  AND (link.subject_type IS NULL OR link.subject_id IS NULL OR link.use_scope IS NULL);
ALTER TABLE training_case_consent_links ALTER COLUMN subject_type SET NOT NULL;
ALTER TABLE training_case_consent_links ALTER COLUMN subject_id SET NOT NULL;
ALTER TABLE training_case_consent_links ALTER COLUMN use_scope SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_training_case_consent_links_subject
  ON training_case_consent_links(consent_id, subject_type, subject_id, use_scope);
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='uq_publication_consents_id_subject' AND conrelid='publication_consents'::regclass) THEN
    ALTER TABLE publication_consents ADD CONSTRAINT uq_publication_consents_id_subject UNIQUE(id, subject_type);
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='fk_training_case_consent_subject' AND conrelid='training_case_consent_links'::regclass) THEN
    ALTER TABLE training_case_consent_links ADD CONSTRAINT fk_training_case_consent_subject
      FOREIGN KEY (consent_id, subject_type) REFERENCES publication_consents(id, subject_type) ON DELETE RESTRICT;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname='chk_training_case_consent_testimonial_subject' AND conrelid='training_case_consent_links'::regclass) THEN
    ALTER TABLE training_case_consent_links ADD CONSTRAINT chk_training_case_consent_testimonial_subject
      CHECK (subject_type <> 'testimonial' OR testimonial_id = subject_id);
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS enterprise_promotion_settings (
  key TEXT PRIMARY KEY,
  draft_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  published_config JSONB NOT NULL DEFAULT '{}'::jsonb,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  published_at TIMESTAMPTZ,
  updated_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS promotion_share_tokens (
  id BIGSERIAL PRIMARY KEY,
  token_hash TEXT NOT NULL UNIQUE,
  channel TEXT NOT NULL DEFAULT '',
  target_page TEXT NOT NULL,
  case_id BIGINT REFERENCES training_cases(id) ON DELETE RESTRICT,
  solution_id BIGINT REFERENCES enterprise_solutions(id) ON DELETE RESTRICT,
  created_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','revoked','expired')),
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS promotion_sessions (
  id BIGSERIAL PRIMARY KEY,
  session_key TEXT NOT NULL UNIQUE,
  first_touch JSONB NOT NULL DEFAULT '{}'::jsonb,
  last_touch JSONB NOT NULL DEFAULT '{}'::jsonb,
  share_token_id BIGINT REFERENCES promotion_share_tokens(id) ON DELETE RESTRICT,
  channel TEXT NOT NULL DEFAULT '',
  first_visited_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  last_visited_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at TIMESTAMPTZ NOT NULL DEFAULT (now() + interval '90 days')
);

CREATE TABLE IF NOT EXISTS promotion_events (
  id BIGSERIAL PRIMARY KEY,
  session_id BIGINT NOT NULL REFERENCES promotion_sessions(id) ON DELETE RESTRICT,
  event_type TEXT NOT NULL,
  case_id BIGINT REFERENCES training_cases(id) ON DELETE RESTRICT,
  solution_id BIGINT REFERENCES enterprise_solutions(id) ON DELETE RESTRICT,
  media_asset_id BIGINT REFERENCES promotion_media_assets(id) ON DELETE RESTRICT,
  page_path TEXT NOT NULL DEFAULT '',
  event_data JSONB NOT NULL DEFAULT '{}'::jsonb,
  idempotency_key TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  received_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE promotion_events DROP CONSTRAINT IF EXISTS promotion_events_idempotency_key_key;
ALTER TABLE promotion_events DROP CONSTRAINT IF EXISTS promotion_events_session_id_idempotency_key_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_promotion_events_idempotency_key
  ON promotion_events(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_promotion_events_session
  ON promotion_events(session_id, occurred_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_promotion_events_funnel ON promotion_events(event_type, occurred_at DESC);

CREATE TABLE IF NOT EXISTS enterprise_consultations (
  id BIGSERIAL PRIMARY KEY,
  consultation_reference_hash TEXT NOT NULL UNIQUE,
  request_idempotency_hash TEXT NOT NULL UNIQUE,
  company_name_encrypted BYTEA NOT NULL,
  industry TEXT NOT NULL DEFAULT '',
  city TEXT NOT NULL DEFAULT '',
  participant_range TEXT NOT NULL DEFAULT '',
  requirements_encrypted BYTEA,
  preferred_training_time TEXT NOT NULL DEFAULT '',
  contact_name_encrypted BYTEA NOT NULL,
  phone_encrypted BYTEA NOT NULL,
  phone_lookup_hash TEXT NOT NULL DEFAULT '',
  wechat_encrypted BYTEA,
  note_encrypted BYTEA,
  source_page TEXT NOT NULL DEFAULT '',
  case_id BIGINT REFERENCES training_cases(id) ON DELETE RESTRICT,
  solution_id BIGINT REFERENCES enterprise_solutions(id) ON DELETE RESTRICT,
  first_touch_session_id BIGINT REFERENCES promotion_sessions(id) ON DELETE RESTRICT,
  last_touch_session_id BIGINT REFERENCES promotion_sessions(id) ON DELETE RESTRICT,
  share_token_id BIGINT REFERENCES promotion_share_tokens(id) ON DELETE RESTRICT,
  channel TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new','contacted','qualified','proposal','won','lost','spam')),
  assignee_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0),
  privacy_notice_version TEXT NOT NULL,
  consented_at TIMESTAMPTZ NOT NULL,
  consent_source TEXT NOT NULL,
  consent_ip_hash TEXT NOT NULL DEFAULT '',
  consent_user_agent_hash TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_enterprise_consultations_queue ON enterprise_consultations(status, created_at DESC);

CREATE TABLE IF NOT EXISTS enterprise_consultation_notes (
  id BIGSERIAL PRIMARY KEY,
  consultation_id BIGINT NOT NULL REFERENCES enterprise_consultations(id) ON DELETE RESTRICT,
  note_encrypted BYTEA NOT NULL,
  created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS consultation_privacy_requests (
  id BIGSERIAL PRIMARY KEY,
  consultation_id BIGINT NOT NULL REFERENCES enterprise_consultations(id) ON DELETE RESTRICT,
  request_type TEXT NOT NULL CHECK (request_type IN ('access','correction','deletion')),
  verified_phone_hash TEXT NOT NULL,
  correction_payload_encrypted BYTEA,
  status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','verified','approved','rejected','completed')),
  verification_expires_at TIMESTAMPTZ,
  reviewed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  reviewed_at TIMESTAMPTZ,
  completed_by BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  completed_at TIMESTAMPTZ,
  decision_basis TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS enterprise_promotion_audit_logs (
  id BIGSERIAL PRIMARY KEY,
  entity_type TEXT NOT NULL,
  entity_id BIGINT NOT NULL,
  action TEXT NOT NULL,
  actor_id BIGINT REFERENCES users(id) ON DELETE RESTRICT,
  detail JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_enterprise_promotion_audit_entity ON enterprise_promotion_audit_logs(entity_type, entity_id, created_at DESC);
