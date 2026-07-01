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
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS seconds INT NOT NULL DEFAULT 15;
ALTER TABLE video_generations ADD COLUMN IF NOT EXISTS aspect_ratio TEXT NOT NULL DEFAULT '16:9';

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
  register_source TEXT NOT NULL DEFAULT 'sms',
  last_login_at   TIMESTAMPTZ,
  create_time     TIMESTAMPTZ NOT NULL DEFAULT now(),
  update_time     TIMESTAMPTZ NOT NULL DEFAULT now()
);

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

-- ----- App 问答会话：存储每张卡的对话历史 -----
CREATE TABLE IF NOT EXISTS app_chat_sessions (
  id          BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id     BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  title       TEXT NOT NULL DEFAULT '',
  updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
  paid_at         TIMESTAMPTZ
);

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
