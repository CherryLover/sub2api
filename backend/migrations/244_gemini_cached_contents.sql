-- Selective backport of Wei-Shaw/sub2api #8006: durable owner bindings and mutation leases.
-- Gemini 显式缓存（cachedContents）的网关资源绑定。
-- public_id 是网关对客户端暴露的缓存 ID；upstream_name 是上游资源名，
-- 引用缓存的请求只能路由到 account_id 对应的账号。
CREATE TABLE IF NOT EXISTS gemini_cached_contents (
    id                BIGSERIAL PRIMARY KEY,
    public_id         VARCHAR(64)  NOT NULL,
    user_id           BIGINT       NOT NULL,
    api_key_id        BIGINT       NOT NULL,
    group_id          BIGINT       NOT NULL,
    account_id        BIGINT       NOT NULL,
    upstream_name     VARCHAR(512) NOT NULL,
    model             VARCHAR(255) NOT NULL,
    upstream_model    VARCHAR(512) NOT NULL,
    display_name      VARCHAR(512) NOT NULL DEFAULT '',
    total_token_count BIGINT       NOT NULL DEFAULT 0,
    expire_time       TIMESTAMPTZ  NOT NULL,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ  NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gemini_cached_contents_public_id
    ON gemini_cached_contents (public_id);

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_api_key_active
    ON gemini_cached_contents (api_key_id, id DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_account
    ON gemini_cached_contents (account_id);

-- Gemini 显式缓存创建时的渠道计费口径（渠道 ID / 请求模型 / 计费模型来源），
-- 延长有效期按同一口径计费。
ALTER TABLE gemini_cached_contents
    ADD COLUMN IF NOT EXISTS channel_usage JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Gemini 显式缓存修改有效期的进行中操作（修改意图）：mutation_token 标识持有者，
-- mutation_lease_until 之前同一缓存的其他修改被拒绝；expire_time 在提交前保持修改前的值，
-- 租约过期仍未清除的意图由恢复流程把上游有效期回滚到 expire_time。
ALTER TABLE gemini_cached_contents
    ADD COLUMN IF NOT EXISTS mutation_token       VARCHAR(64) NULL,
    ADD COLUMN IF NOT EXISTS mutation_lease_until TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_gemini_cached_contents_pending_mutation
    ON gemini_cached_contents (mutation_lease_until)
    WHERE mutation_token IS NOT NULL AND deleted_at IS NULL;
