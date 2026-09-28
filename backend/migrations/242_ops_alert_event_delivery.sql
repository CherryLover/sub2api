-- Record administrator urgency separately from notification delivery.
ALTER TABLE ops_alert_events
    ADD COLUMN IF NOT EXISTS urgency VARCHAR(16) NOT NULL DEFAULT 'observe',
    ADD COLUMN IF NOT EXISTS delivery VARCHAR(32) NOT NULL DEFAULT 'none';

CREATE INDEX IF NOT EXISTS idx_ops_alert_events_urgency_fired_at
    ON ops_alert_events (urgency, fired_at DESC);
