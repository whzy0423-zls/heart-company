-- Add the SVIP-only followup capability without rewriting prices, quotas,
-- custom feature text, or an explicit administrator false. The canonical
-- svip row is the runtime switch inherited by every SVIP billing cycle.
UPDATE app_plans
SET feature_flags = feature_flags || jsonb_build_object(
      'problemFollowup', plan_level = 'svip'
    ),
    update_time = now()
WHERE NOT (feature_flags ? 'problemFollowup');

UPDATE app_plans
SET features = features || '["问题解决跟进（30 分钟未回复，跟进一次）"]'::jsonb,
    update_time = now()
WHERE plan_level = 'svip'
  AND feature_flags->'problemFollowup' = 'true'::jsonb
  AND COALESCE((SELECT CASE WHEN feature_flags ? 'problemFollowup'
      THEN feature_flags->'problemFollowup' = 'true'::jsonb ELSE true END
      FROM app_plans WHERE code = 'svip'), true)
  AND jsonb_array_length(features) < 8
  AND NOT EXISTS (
    SELECT 1 FROM jsonb_array_elements_text(features) AS feature(value)
    WHERE feature.value LIKE '%问题解决跟进%'
  );
