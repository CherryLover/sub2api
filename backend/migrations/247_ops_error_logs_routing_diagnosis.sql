-- Persist the per-request routing diagnosis for requests that ended without a
-- usable account (or after failover was exhausted): which accounts the
-- scheduler considered, which gate rejected each one, and every upstream
-- attempt. Historical rows intentionally stay NULL.
ALTER TABLE ops_error_logs
    ADD COLUMN IF NOT EXISTS routing_diagnosis JSONB;
