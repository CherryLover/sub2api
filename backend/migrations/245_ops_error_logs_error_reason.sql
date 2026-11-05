-- Persist the machine-readable reason behind "no available account" routing errors.
-- Historical rows intentionally stay NULL; new rows are populated by the gateway.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS error_reason TEXT;

-- Bytes of the client request body read by the gateway (NULL when unknown or
-- empty, e.g. WebSocket upgrades). Lets operators spot oversized requests.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS request_body_bytes BIGINT;
