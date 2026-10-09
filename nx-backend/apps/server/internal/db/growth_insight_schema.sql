CREATE TABLE IF NOT EXISTS app_growth_insight_consents (
  app_user_id BIGINT PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
  enabled BOOLEAN NOT NULL DEFAULT false,
  disclosure_version TEXT NOT NULL DEFAULT '',
  revision BIGINT NOT NULL DEFAULT 1,
  next_version INT NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'disabled',
  published_report_id BIGINT,
  published_at TIMESTAMPTZ,
  latest_report_id BIGINT,
  last_success_at TIMESTAMPTZ,
  last_fingerprint TEXT NOT NULL DEFAULT '',
  last_scanned_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS app_growth_insight_jobs (
  app_user_id BIGINT PRIMARY KEY REFERENCES app_users(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  revision BIGINT NOT NULL,
  fingerprint TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  due_at TIMESTAMPTZ NOT NULL,
  lease_until TIMESTAMPTZ,
  claim_token TEXT NOT NULL DEFAULT '',
  attempts INT NOT NULL DEFAULT 0,
  error_code TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_growth_insight_jobs_due ON app_growth_insight_jobs(status,due_at);

-- Attempt budgets use UTC calendar days, independently of the rolling successful-analysis cadence.
-- Withdrawal does not erase this minimal ledger. Account deletion removes owner linkage, not spent global capacity.
CREATE TABLE IF NOT EXISTS app_growth_insight_attempts (
  claim_token TEXT PRIMARY KEY,
  app_user_id BIGINT REFERENCES app_users(id) ON DELETE SET NULL,
  budget_day DATE NOT NULL,
  reserved_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_growth_insight_attempts_day_user ON app_growth_insight_attempts(budget_day,app_user_id);

CREATE TABLE IF NOT EXISTS app_growth_insight_reports (
  id BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  version INT NOT NULL,
  fingerprint TEXT NOT NULL,
  generated_at TIMESTAMPTZ NOT NULL,
  source_through TIMESTAMPTZ NOT NULL,
  evidence_count INT NOT NULL,
  payload JSONB NOT NULL,
  UNIQUE(app_user_id,version)
);

CREATE TABLE IF NOT EXISTS app_growth_insight_actions (
  id BIGSERIAL PRIMARY KEY,
  report_id BIGINT NOT NULL REFERENCES app_growth_insight_reports(id) ON DELETE CASCADE,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  title TEXT NOT NULL CHECK(char_length(title) BETWEEN 1 AND 80),
  detail TEXT NOT NULL CHECK(char_length(detail) BETWEEN 1 AND 500),
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','attempted','skipped')),
  outcome TEXT NOT NULL DEFAULT 'unknown' CHECK(outcome IN ('unknown','helpful','not_helpful','worse')),
  note TEXT NOT NULL DEFAULT '' CHECK(char_length(note)<=500),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_growth_insight_actions_report ON app_growth_insight_actions(report_id);

CREATE TABLE IF NOT EXISTS app_growth_insight_corrections (
  id BIGSERIAL PRIMARY KEY,
  app_user_id BIGINT NOT NULL REFERENCES app_users(id) ON DELETE CASCADE,
  card_id BIGINT NOT NULL REFERENCES app_user_cards(id) ON DELETE CASCADE,
  report_id BIGINT NOT NULL REFERENCES app_growth_insight_reports(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK(kind IN ('inaccurate','missing_context','other')),
  note TEXT NOT NULL CHECK(char_length(note) BETWEEN 1 AND 500),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(app_user_id,report_id,kind,note)
);

-- The consent row is the publication barrier shared by writers and deletion triggers.
CREATE OR REPLACE FUNCTION growth_insight_invalidate(uid BIGINT) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF NOT EXISTS(SELECT 1 FROM app_users WHERE id=uid) THEN RETURN; END IF;
  UPDATE app_growth_insight_consents SET revision=revision+1,
    published_report_id=NULL,latest_report_id=NULL,published_at=NULL,
    last_fingerprint='',
    status=CASE WHEN enabled THEN 'pending' ELSE 'disabled' END,updated_at=now()
    WHERE app_user_id=uid;
  DELETE FROM app_growth_insight_jobs WHERE app_user_id=uid;
  DELETE FROM app_growth_insight_reports WHERE app_user_id=uid;
END $$;

CREATE OR REPLACE FUNCTION growth_insight_message_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE uid BIGINT;
BEGIN
  IF TG_OP='INSERT' THEN
    IF NEW.role='user' THEN
      SELECT s.app_user_id INTO uid FROM app_chat_sessions s JOIN app_user_cards c ON c.id=s.card_id
      WHERE s.id=NEW.session_id AND s.scene='chat' AND s.skill_version_id IS NULL
        AND c.card_type='primary' AND c.status='active' AND c.app_user_id=s.app_user_id;
      UPDATE app_growth_insight_consents SET revision=revision+1 WHERE app_user_id=uid AND enabled;
    END IF;
    RETURN NEW;
  END IF;
  IF OLD.role='user' THEN
    SELECT s.app_user_id INTO uid FROM app_chat_sessions s JOIN app_user_cards c ON c.id=s.card_id
      WHERE s.id=OLD.session_id AND s.scene='chat' AND s.skill_version_id IS NULL
        AND c.card_type='primary' AND c.app_user_id=s.app_user_id;
    IF uid IS NOT NULL THEN PERFORM growth_insight_invalidate(uid); END IF;
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS growth_insight_message_insert ON app_chat_messages;
CREATE TRIGGER growth_insight_message_insert AFTER INSERT ON app_chat_messages FOR EACH ROW EXECUTE FUNCTION growth_insight_message_change();
DROP TRIGGER IF EXISTS growth_insight_message_delete ON app_chat_messages;
CREATE TRIGGER growth_insight_message_delete BEFORE DELETE ON app_chat_messages FOR EACH ROW EXECUTE FUNCTION growth_insight_message_change();
DROP TRIGGER IF EXISTS growth_insight_message_update ON app_chat_messages;
CREATE TRIGGER growth_insight_message_update BEFORE UPDATE OF content,transcript,role,session_id ON app_chat_messages FOR EACH ROW
WHEN ((OLD.content,OLD.transcript,OLD.role,OLD.session_id) IS DISTINCT FROM (NEW.content,NEW.transcript,NEW.role,NEW.session_id)) EXECUTE FUNCTION growth_insight_message_change();

CREATE OR REPLACE FUNCTION growth_insight_session_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.scene='chat' AND OLD.skill_version_id IS NULL
    AND EXISTS(SELECT 1 FROM app_user_cards WHERE id=OLD.card_id AND app_user_id=OLD.app_user_id AND card_type='primary' AND status='active')
    THEN PERFORM growth_insight_invalidate(OLD.app_user_id); END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS growth_insight_session_delete ON app_chat_sessions;
CREATE TRIGGER growth_insight_session_delete BEFORE DELETE ON app_chat_sessions FOR EACH ROW EXECUTE FUNCTION growth_insight_session_change();
DROP TRIGGER IF EXISTS growth_insight_session_update ON app_chat_sessions;
CREATE TRIGGER growth_insight_session_update BEFORE UPDATE OF card_id,app_user_id,scene,skill_version_id ON app_chat_sessions FOR EACH ROW
WHEN ((OLD.card_id,OLD.app_user_id,OLD.scene,OLD.skill_version_id) IS DISTINCT FROM (NEW.card_id,NEW.app_user_id,NEW.scene,NEW.skill_version_id)) EXECUTE FUNCTION growth_insight_session_change();

CREATE OR REPLACE FUNCTION growth_insight_card_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF OLD.card_type='primary' THEN PERFORM growth_insight_invalidate(OLD.app_user_id); END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  IF NEW.card_type='primary' AND NEW.app_user_id<>OLD.app_user_id THEN PERFORM growth_insight_invalidate(NEW.app_user_id); END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS growth_insight_card_delete ON app_user_cards;
CREATE TRIGGER growth_insight_card_delete BEFORE DELETE ON app_user_cards FOR EACH ROW EXECUTE FUNCTION growth_insight_card_change();
DROP TRIGGER IF EXISTS growth_insight_card_update ON app_user_cards;
CREATE TRIGGER growth_insight_card_update BEFORE UPDATE OF status,card_type,app_user_id,submission_id,enneagram,profile,revision ON app_user_cards FOR EACH ROW
WHEN ((OLD.status,OLD.card_type,OLD.app_user_id,OLD.submission_id,OLD.enneagram,OLD.profile,OLD.revision) IS DISTINCT FROM (NEW.status,NEW.card_type,NEW.app_user_id,NEW.submission_id,NEW.enneagram,NEW.profile,NEW.revision)) EXECUTE FUNCTION growth_insight_card_change();

CREATE OR REPLACE FUNCTION growth_insight_assessment_change() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE uid BIGINT;
BEGIN
  FOR uid IN SELECT app_user_id FROM app_user_cards WHERE submission_id=OLD.id AND card_type='primary' LOOP
    PERFORM growth_insight_invalidate(uid);
  END LOOP;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS growth_insight_assessment_delete ON app_quiz_submissions;
CREATE TRIGGER growth_insight_assessment_delete BEFORE DELETE ON app_quiz_submissions FOR EACH ROW EXECUTE FUNCTION growth_insight_assessment_change();
DROP TRIGGER IF EXISTS growth_insight_assessment_update ON app_quiz_submissions;
CREATE TRIGGER growth_insight_assessment_update BEFORE UPDATE OF result,primary_type,wing_type,answers ON app_quiz_submissions FOR EACH ROW
WHEN ((OLD.result,OLD.primary_type,OLD.wing_type,OLD.answers) IS DISTINCT FROM (NEW.result,NEW.primary_type,NEW.wing_type,NEW.answers)) EXECUTE FUNCTION growth_insight_assessment_change();

CREATE OR REPLACE FUNCTION growth_insight_account_change() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF TG_OP='DELETE' OR NEW.status<>'active' THEN
    UPDATE app_growth_insight_consents SET enabled=false WHERE app_user_id=OLD.id;
    PERFORM growth_insight_invalidate(OLD.id);
  END IF;
  -- Account deletion anonymizes and disables app_users; ordinary suspension keeps per-user spend.
  IF TG_OP='UPDATE' AND NEW.status='disabled'
    AND (to_jsonb(NEW)->>'phone') LIKE 'deleted-'||OLD.id::text||'-%' THEN
    UPDATE app_growth_insight_attempts SET app_user_id=NULL WHERE app_user_id=OLD.id;
  END IF;
  IF TG_OP='DELETE' THEN RETURN OLD; END IF;
  RETURN NEW;
END $$;
DROP TRIGGER IF EXISTS growth_insight_account_delete ON app_users;
CREATE TRIGGER growth_insight_account_delete BEFORE DELETE ON app_users FOR EACH ROW EXECUTE FUNCTION growth_insight_account_change();
DROP TRIGGER IF EXISTS growth_insight_account_update ON app_users;
CREATE TRIGGER growth_insight_account_update BEFORE UPDATE OF status ON app_users FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status) EXECUTE FUNCTION growth_insight_account_change();
