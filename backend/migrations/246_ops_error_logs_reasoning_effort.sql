-- Persist the reasoning effort parsed from the inbound gateway request.
-- Historical rows intentionally stay NULL.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS reasoning_effort VARCHAR(20);
