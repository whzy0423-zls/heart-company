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

ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS contact_type TEXT NOT NULL DEFAULT 'phone';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS follow_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS owner TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS next_follow_time TIMESTAMPTZ;
ALTER TABLE signups ADD COLUMN IF NOT EXISTS follow_note TEXT NOT NULL DEFAULT '';
ALTER TABLE signups ADD COLUMN IF NOT EXISTS update_time TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE site_visits ADD COLUMN IF NOT EXISTS visitor_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_role_menus_role ON role_menus(role_id);
CREATE INDEX IF NOT EXISTS idx_menus_pid ON menus(pid);
CREATE INDEX IF NOT EXISTS idx_signups_create_time ON signups(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_signups_follow_status ON signups(follow_status);
CREATE INDEX IF NOT EXISTS idx_signup_followups_signup ON signup_followups(signup_id, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_messages_type_read ON messages(type, is_read, create_time DESC);
CREATE INDEX IF NOT EXISTS idx_game_results_create_time ON game_results(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_game_results_type ON game_results(result_type);
CREATE INDEX IF NOT EXISTS idx_upload_assets_create_time ON upload_assets(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_site_visits_create_time ON site_visits(create_time DESC);
CREATE INDEX IF NOT EXISTS idx_site_visits_visitor_id ON site_visits(visitor_id);
