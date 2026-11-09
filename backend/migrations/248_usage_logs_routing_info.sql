-- Persist how a successful request picked its upstream account (selection
-- method, eligible candidates, queue wait and failover attempts).
-- Historical rows intentionally stay NULL; the column is nullable so adding it
-- is a metadata-only change and does not rewrite usage_logs.
ALTER TABLE usage_logs
    ADD COLUMN IF NOT EXISTS routing_info JSONB;
