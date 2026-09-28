-- Record administrator urgency separately from notification delivery.
ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS urgency VARCHAR(16) NOT NULL DEFAULT 'observe',
    ADD COLUMN IF NOT EXISTS delivery VARCHAR(32) NOT NULL DEFAULT 'none';

UPDATE ops_alert_events AS e
SET urgency = CASE
    WHEN r.metric_type IN (
        'account_window_used_percent', 'account_quota_used_percent', 'account_balance',
        'account_today_cost', 'account_expires_in_days', 'apikey_daily_used_percent'
    ) THEN 'observe'
    WHEN r.metric_type IN ('success_rate', 'error_rate') THEN 'immediate'
    WHEN r.metric_type = 'group_available_accounts'
         AND r.threshold <= 0
         AND r.operator IN ('<=', '==') THEN 'immediate'
    WHEN r.metric_type IN (
        'group_available_accounts', 'account_rate_limited_count', 'account_error_count',
        'account_error_ratio', 'account_temp_unscheduled_count', 'overload_account_count',
        'proxy_expired_count', 'proxy_expiring_soon_count', 'upstream_error_rate',
        'group_rate_limit_ratio', 'group_available_ratio', 'cpu_usage_percent',
        'memory_usage_percent', 'concurrency_queue_depth'
    ) THEN 'observe'
    ELSE 'silent'
END
FROM ops_alert_rules AS r
WHERE e.rule_id = r.id;

UPDATE ops_alert_events AS e
SET delivery = CASE
    WHEN e.urgency = 'immediate' AND COALESCE(r.notify_email, true) THEN 'bark_realtime'
    WHEN e.urgency IN ('immediate', 'observe') THEN 'in_app'
    ELSE 'none'
END
FROM ops_alert_rules AS r
WHERE e.rule_id = r.id;

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_urgency_fired_at
    ON ops_alert_events (urgency, fired_at DESC);
