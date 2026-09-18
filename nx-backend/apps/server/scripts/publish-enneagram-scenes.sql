-- Publish a frozen, independently routed snapshot of the ten reviewed active libraries.
-- Run with psql -v ON_ERROR_STOP=1. Safe to rerun after a successful publish.
BEGIN;
SELECT pg_advisory_xact_lock(hashtext('publish-enneagram-scenes-v1'));
DO $$
DECLARE
 lib BIGINT; rel BIGINT; catalog BIGINT; category BIGINT; skill BIGINT; ver BIGINT;
 new_card BIGINT; new_chunk BIGINT; item RECORD; part RECORD; n INTEGER;
 source_releases JSONB; instructions TEXT;
BEGIN
 IF EXISTS(SELECT 1 FROM app_skills WHERE key='enneagram-personality-library' AND latest_published_version_id IS NOT NULL) THEN
   RAISE NOTICE 'Nine-type scene skill already published'; RETURN;
 END IF;
 SELECT count(*), jsonb_object_agg(l.key,r.id) INTO n,source_releases
 FROM theory_libraries l JOIN theory_library_releases r ON r.library_id=l.id AND r.status='active'
 WHERE l.status='enabled' AND (l.key='enneagram-core' OR l.key ~ '^enneagram-type-0[1-9]$');
 IF n<>10 THEN RAISE EXCEPTION 'Expected ten enabled reviewed source libraries, found %',n; END IF;
 INSERT INTO theory_libraries(key,name,description,status,current_version)
 VALUES('enneagram-scenes-snapshot','九型专项对话知识快照','源自已发布的九型理论核心与九个型号库；按表单明确型号检索。','enabled',1) RETURNING id INTO lib;
 INSERT INTO theory_library_releases(library_id,version,status,retrieval_mode,index_version)
 VALUES(lib,1,'draft','lexical_only','enneagram-scenes-v1') RETURNING id INTO rel;
 FOR item IN
  SELECT DISTINCT c.*, l.key AS source_key FROM theory_cards c
  JOIN theory_libraries l ON l.id=c.library_id
  JOIN theory_release_cards m ON m.card_id=c.id
  WHERE m.release_id IN (SELECT value::bigint FROM jsonb_each_text(source_releases))
 LOOP
  new_card:=nextval('theory_cards_id_seq');
  INSERT INTO theory_cards SELECT (jsonb_populate_record(NULL::theory_cards,
   (to_jsonb(item)-'source_key') || jsonb_build_object('id',new_card,'library_id',lib,'canonical_key',item.source_key||':'||item.canonical_key))).*;
  FOR part IN
   SELECT DISTINCT c.* FROM theory_chunks c JOIN theory_release_cards m ON m.chunk_id=c.id
   WHERE c.card_id=item.id AND m.release_id IN (SELECT value::bigint FROM jsonb_each_text(source_releases)) AND c.status='enabled'
  LOOP
   new_chunk:=nextval('theory_chunks_id_seq');
   INSERT INTO theory_chunks SELECT (jsonb_populate_record(NULL::theory_chunks,
    to_jsonb(part)||jsonb_build_object('id',new_chunk,'library_id',lib,'card_id',new_card,'practice_id',NULL,
     'chunk_key',item.source_key||':'||part.chunk_key,'tags',part.tags||jsonb_build_array(item.source_key)))).*;
   INSERT INTO theory_release_cards(release_id,card_id,chunk_id) VALUES(rel,new_card,new_chunk);
  END LOOP;
 END LOOP;
 SELECT count(*) INTO n FROM theory_release_cards WHERE release_id=rel;
 IF n<180 THEN RAISE EXCEPTION 'Incomplete scene snapshot: % chunks',n; END IF;
 UPDATE theory_library_releases SET status='active',card_count=(SELECT count(DISTINCT card_id) FROM theory_release_cards WHERE release_id=rel),chunk_count=n,activated_at=now() WHERE id=rel;
 INSERT INTO app_skill_libraries(key,name,description,icon_key,status,sort_order)
 VALUES('enneagram-scenes','九型应用','从真实情景出发的独立专项对话','psychology','enabled',10) RETURNING id INTO catalog;
 INSERT INTO app_skill_categories(library_id,key,name,icon_key,color_token,status)
 VALUES(catalog,'relationships','情景沟通','forum','blue','enabled') RETURNING id INTO category;
 INSERT INTO app_skills(category_id,key,name,summary,description,icon_key,color_token,status)
 VALUES(category,'enneagram-personality-library','九型专项对话','结合人物关系、具体事件与期望结果，探索可执行的沟通方法。','每个场景独立保存上下文，使用已发布九型知识库。','psychology','blue','enabled') RETURNING id INTO skill;
 instructions:='你是九型情景沟通助手。本会话独立于人物卡的其他对话。持续参考初始表单中的人物关系、明确选择的型号、事件、目标和问题；用户在后续提供的新事实优先。仅使用提供的九型理论核心和所选型号知识片段，不猜型，不将性格当作诊断或确定性因果，不编造知识库引文。型号未知时只从事实与理论核心讨论。区分事实、可能机制、其他解释，给出可以直接说的话、下一步行动和观察信号。后续追问自然承接，无需每次重复完整结构。涉及胁迫、暴力或即时危险，优先人身安全和适当专业支持。';
 INSERT INTO app_skill_versions(skill_id,version,instructions,theory_release_id,safety_profile,content_hash,status,published_at,source_metadata)
 VALUES(skill,'1.0.0',instructions,rel,'sensitive-relationships-v1',
 encode(sha256(convert_to(instructions||source_releases::text,'UTF8')),'hex'),'published',now(),
 jsonb_build_object('sourceReleases',source_releases,'compilerPolicy','enneagram-scenes-snapshot-v1','reviewDecision','publish','reviewDecisionRef','existing-reviewed-active-enneagram-libraries','sourceNeeded',false)) RETURNING id INTO ver;
 UPDATE app_skills SET latest_published_version_id=ver WHERE id=skill;
 RAISE NOTICE 'Published skill %, version %, release %, chunks %',skill,ver,rel,n;
END $$;
COMMIT;
