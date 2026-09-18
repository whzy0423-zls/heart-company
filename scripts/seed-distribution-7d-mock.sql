-- 本地 7 天代理后台 mock 数据，可重复执行。
-- 依赖已存在代理：A100001 / B200001 / C300001
BEGIN;

-- 先清理旧的 7 天 mock，保证重复执行不会叠加。
DELETE FROM distribution_settlement_items
WHERE settlement_id IN (
  SELECT id FROM distribution_settlements WHERE payment_reference LIKE 'MOCK7D-%'
)
OR commission_id IN (
  SELECT c.id
  FROM distribution_commission_records c
  JOIN app_orders o ON o.id = c.order_id
  WHERE o.out_trade_no LIKE 'MOCK7D-%'
);

DELETE FROM distribution_settlements WHERE payment_reference LIKE 'MOCK7D-%';
DELETE FROM distribution_invite_events WHERE request_id LIKE 'MOCK7D-%';
DELETE FROM distribution_commission_records
WHERE order_id IN (SELECT id FROM app_orders WHERE out_trade_no LIKE 'MOCK7D-%');
DELETE FROM app_orders WHERE out_trade_no LIKE 'MOCK7D-%';
DELETE FROM distribution_user_relations
WHERE app_user_id IN (SELECT id FROM app_users WHERE register_source = 'local_distribution_mock_7d');
DELETE FROM app_users WHERE register_source = 'local_distribution_mock_7d';

WITH agent_seed AS (
  SELECT 'L1'::text AS bucket, id AS agent_id, agent_code, level FROM distribution_agents WHERE agent_code='A100001'
  UNION ALL SELECT 'L2', id, agent_code, level FROM distribution_agents WHERE agent_code='B200001'
  UNION ALL SELECT 'L3', id, agent_code, level FROM distribution_agents WHERE agent_code='C300001'
), missing AS (
  SELECT 1 WHERE (SELECT COUNT(*) FROM agent_seed) <> 3
), days AS (
  SELECT gs::int AS day_offset,
         (CURRENT_DATE - (6 - gs)::int) AS biz_date
  FROM generate_series(0, 6) AS gs
), slots AS (
  SELECT * FROM (VALUES
    (1, 'L1', 1999, 'monthly_vip', '月度会员'),
    (2, 'L2', 2999, 'monthly_vip_plus', '月度会员 Pro'),
    (3, 'L3', 3999, 'quarter_vip', '季度会员'),
    (4, 'L2', 6999, 'year_vip_trial', '年度会员体验'),
    (5, 'L3', 9999, 'year_vip', '年度会员'),
    (6, 'L1', 1599, 'mini_pack', '灵感加油包')
  ) AS v(slot_no, bucket, amount, product_id, title)
), rows AS (
  SELECT d.day_offset,
         d.biz_date,
         s.slot_no,
         s.bucket,
         a.agent_id,
         a.agent_code,
         a.level AS agent_level,
         s.amount,
         s.product_id,
         s.title,
         ('mock7d_' || to_char(d.biz_date, 'YYYYMMDD') || '_' || lpad(s.slot_no::text, 2, '0')) AS account,
         ('13977' || to_char(d.biz_date, 'MMDD') || lpad(s.slot_no::text, 2, '0')) AS phone,
         ('七天推广用户 D' || (d.day_offset + 1)::text || '-' || lpad(s.slot_no::text, 2, '0')) AS nickname,
         (d.biz_date + make_interval(hours => 9 + s.slot_no, mins => ((d.day_offset * 7 + s.slot_no * 3) % 60))) AS event_at
  FROM days d
  CROSS JOIN slots s
  JOIN agent_seed a ON a.bucket = s.bucket
), inserted_users AS (
  INSERT INTO app_users(
    phone, nickname, avatar, status, member_level, register_source,
    last_login_at, create_time, update_time, account, password_hash,
    user_code, invite_code
  )
  SELECT phone,
         nickname,
         '',
         'active',
         CASE WHEN amount >= 6999 THEN 'vip' ELSE 'free' END,
         'local_distribution_mock_7d',
         event_at + interval '20 minutes',
         event_at - interval '18 minutes',
         event_at + interval '22 minutes',
         account,
         '',
         'U' || upper(replace(account, 'mock7d_', 'M7D')),
         'I' || upper(replace(account, 'mock7d_', 'M7D'))
  FROM rows
  RETURNING id, account
), bound_relations AS (
  INSERT INTO distribution_user_relations(app_user_id, direct_agent_id, source, bound_at, updated_at)
  SELECT u.id, r.agent_id, 'agent_link', r.event_at - interval '12 minutes', r.event_at - interval '12 minutes'
  FROM inserted_users u
  JOIN rows r ON r.account = u.account
  RETURNING app_user_id, direct_agent_id, bound_at
), inserted_orders AS (
  INSERT INTO app_orders(
    out_trade_no, app_user_id, product_id, title, amount, status, transaction_id,
    create_time, update_time, paid_at, activation_at, membership_expires_at,
    payment_provider, pay_channel, provider_trade_no, provider_status,
    duration_days, purchase_mode, member_level_before, member_started_at_before, member_expires_at_before
  )
  SELECT 'MOCK7D-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'),
         u.id,
         r.product_id,
         r.title,
         r.amount,
         'paid',
         'TX-MOCK7D-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'),
         r.event_at - interval '8 minutes',
         r.event_at + interval '2 minutes',
         r.event_at,
         r.event_at,
         r.event_at + CASE WHEN r.amount >= 6999 THEN interval '365 days' WHEN r.amount >= 3999 THEN interval '90 days' ELSE interval '30 days' END,
         'mock',
         'mock_pay',
         'PTN-MOCK7D-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'),
         'SUCCESS',
         CASE WHEN r.amount >= 6999 THEN 365 WHEN r.amount >= 3999 THEN 90 ELSE 30 END,
         'online',
         'free',
         NULL,
         NULL
  FROM inserted_users u
  JOIN rows r ON r.account = u.account
  RETURNING id, out_trade_no, app_user_id, amount, paid_at
), inserted_commissions AS (
  INSERT INTO distribution_commission_records(
    order_id, agent_id, app_user_id, agent_level, order_amount, rate_bps, commission_amount,
    rule_version, chain_snapshot, status, created_at, updated_at
  )
  SELECT o.id,
         r.agent_id,
         o.app_user_id,
         r.agent_level,
         o.amount,
         CASE r.agent_level WHEN 1 THEN 3000 WHEN 2 THEN 2500 ELSE 2000 END,
         FLOOR(o.amount * (CASE r.agent_level WHEN 1 THEN 3000 WHEN 2 THEN 2500 ELSE 2000 END) / 10000.0)::bigint,
         1,
         json_build_object(
           'mock', true,
           'rootAgentCode', 'A100001',
           'directAgentCode', r.agent_code,
           'level', r.agent_level,
           'generatedFor', '7d-agent-dashboard'
         )::text,
         CASE WHEN r.biz_date >= CURRENT_DATE - 2 THEN 'pending' ELSE 'settled' END,
         o.paid_at + interval '1 minute',
         o.paid_at + interval '1 minute'
  FROM inserted_orders o
  JOIN inserted_users u ON u.id = o.app_user_id
  JOIN rows r ON r.account = u.account
  RETURNING id, agent_id, commission_amount, status, created_at
), invite_clicks AS (
  INSERT INTO distribution_invite_events(agent_id, app_user_id, agent_code, event_type, request_id, ip_hash, user_agent_hash, created_at)
  SELECT r.agent_id, NULL, r.agent_code, 'click', 'MOCK7D-CLICK-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'), 'ip_mock_hash', 'ua_mock_hash', r.event_at - interval '30 minutes'
  FROM rows r
  RETURNING id
), invite_registers AS (
  INSERT INTO distribution_invite_events(agent_id, app_user_id, agent_code, event_type, request_id, ip_hash, user_agent_hash, created_at)
  SELECT r.agent_id, u.id, r.agent_code, 'register', 'MOCK7D-REGISTER-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'), 'ip_mock_hash', 'ua_mock_hash', r.event_at - interval '16 minutes'
  FROM rows r
  JOIN inserted_users u ON u.account = r.account
  RETURNING id
), invite_binds AS (
  INSERT INTO distribution_invite_events(agent_id, app_user_id, agent_code, event_type, request_id, ip_hash, user_agent_hash, created_at)
  SELECT r.agent_id, u.id, r.agent_code, 'bind', 'MOCK7D-BIND-' || to_char(r.biz_date, 'YYYYMMDD') || '-' || lpad(r.slot_no::text, 2, '0'), 'ip_mock_hash', 'ua_mock_hash', r.event_at - interval '12 minutes'
  FROM rows r
  JOIN inserted_users u ON u.account = r.account
  RETURNING id
), l1_settlement AS (
  INSERT INTO distribution_settlements(agent_id, period_start, period_end, amount, status, paid_at, payment_reference, action_reason, created_at)
  SELECT a.agent_id,
         CURRENT_DATE - 6,
         CURRENT_DATE - 3,
         COALESCE(SUM(c.commission_amount), 0),
         'paid',
         CURRENT_DATE - 2 + time '16:30',
         'MOCK7D-SETTLE-L1-' || to_char(CURRENT_DATE, 'YYYYMMDD'),
         '本地 7 天 mock 已打款结算',
         CURRENT_DATE - 2 + time '16:00'
  FROM agent_seed a
  LEFT JOIN inserted_commissions c ON c.agent_id = a.agent_id AND c.status='settled'
  WHERE a.bucket='L1'
  GROUP BY a.agent_id
  HAVING COALESCE(SUM(c.commission_amount), 0) > 0
  RETURNING id, agent_id
), l1_settlement_items AS (
  INSERT INTO distribution_settlement_items(settlement_id, commission_id, amount)
  SELECT s.id, c.id, c.commission_amount
  FROM l1_settlement s
  JOIN inserted_commissions c ON c.agent_id=s.agent_id AND c.status='settled'
  RETURNING id
)
SELECT
  (SELECT COUNT(*) FROM inserted_users) AS users_inserted,
  (SELECT COUNT(*) FROM bound_relations) AS relations_inserted,
  (SELECT COUNT(*) FROM inserted_orders) AS orders_inserted,
  (SELECT COUNT(*) FROM inserted_commissions) AS commissions_inserted,
  (SELECT COUNT(*) FROM invite_clicks) + (SELECT COUNT(*) FROM invite_registers) + (SELECT COUNT(*) FROM invite_binds) AS invite_events_inserted,
  (SELECT COUNT(*) FROM l1_settlement) AS settlements_inserted;

COMMIT;
