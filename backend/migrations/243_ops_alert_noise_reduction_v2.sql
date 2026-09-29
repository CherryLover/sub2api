-- Bark noise reduction v2:
-- the factory "very high error rate" rule used to page after only one minute.
-- Keep customized installations untouched; only upgrade the exact factory row
-- so realtime Bark means a sustained user-facing failure.
UPDATE ops_alert_rules
SET sustained_minutes = 5,
    updated_at = NOW()
WHERE name = '错误率极高'
  AND metric_type = 'error_rate'
  AND operator = '>'
  AND threshold = 20.0
  AND window_minutes = 1
  AND sustained_minutes = 1
  AND severity = 'P0'
  AND notify_email = true;
