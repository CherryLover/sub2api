-- Add configurable reasoning-effort billing multipliers to slim pricing tables.
-- The slim branch never shipped the upstream legacy single-effort pricing field,
-- so there is intentionally no legacy backfill here.
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE channel_account_stats_model_pricing
    ADD COLUMN IF NOT EXISTS reasoning_effort_multipliers JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN channel_model_pricing.reasoning_effort_multipliers IS
    'Custom billing multipliers by reasoning effort; omitted efforts use 1x';

COMMENT ON COLUMN channel_account_stats_model_pricing.reasoning_effort_multipliers IS
    'Custom account statistics billing multipliers by reasoning effort; omitted efforts use 1x';
