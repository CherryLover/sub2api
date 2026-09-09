-- 241: 收敛 groups 的模型白名单列，修复 240 未落地却已被记账的实例。
--
-- 240 只在「models_list_config 存在且 model_allowlist 不存在」时执行重命名，
-- 而迁移一旦写入 schema_migrations 就会按「文件名 + checksum」整份跳过，不再重跑。
-- 因此只要数据库在 240 之后回到了旧结构，关联 groups 的查询都会失败：
--     pq: column groups.model_allowlist does not exist
--
-- 本迁移可重放，并且用 regclass 解析表（跟随 search_path）：
--   1) 只有旧列              -> 重命名，数据原样保留；
--   2) 两列并存              -> 新列仍是默认空值时，把旧列里的配置回填过来；
--   3) 两列都没有            -> 按默认值补建新列。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'groups'::regclass
          AND attname = 'models_list_config'
          AND NOT attisdropped
    ) THEN
        IF NOT EXISTS (
            SELECT 1 FROM pg_attribute
            WHERE attrelid = 'groups'::regclass
              AND attname = 'model_allowlist'
              AND NOT attisdropped
        ) THEN
            ALTER TABLE groups RENAME COLUMN models_list_config TO model_allowlist;
        ELSE
            EXECUTE $backfill$
                UPDATE groups
                   SET model_allowlist = models_list_config
                 WHERE COALESCE(model_allowlist, '{}'::jsonb) = '{}'::jsonb
                   AND COALESCE(models_list_config, '{}'::jsonb) <> '{}'::jsonb
            $backfill$;
        END IF;
    END IF;
END
$$;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE groups SET model_allowlist = '{}'::jsonb WHERE model_allowlist IS NULL;

ALTER TABLE groups ALTER COLUMN model_allowlist SET DEFAULT '{}'::jsonb;
ALTER TABLE groups ALTER COLUMN model_allowlist SET NOT NULL;

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';
