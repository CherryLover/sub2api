-- 234_drop_subscription_and_balance_semantics.sql
--
-- 单管理员内部部署裁剪（A4）：订阅体系与用户余额语义整体拆除，额度改为直接绑定
-- 在 API Key 上（api_keys.quota / quota_used）。本迁移只做「让存量数据在新代码下
-- 继续可用」的数据平滑，不删除任何业务表、不删除任何列，因此可以整体回滚。
--
-- 幂等：所有语句都是条件写入 / 幂等 DELETE，重复执行不产生额外影响。
--
-- ============================================================================
-- 1) 专属分组授权回填：防止存量 Key 因「订阅型分组绕过授权检查」被移除而失效
-- ============================================================================
-- 旧代码在 api_key_auth.go / api_key_service.go 里对 subscription_type='subscription'
-- 的分组直接放行，绕过 user_allowed_groups 授权检查。新代码取消了这条旁路，
-- 所有专属分组统一按 user_allowed_groups 判定。若不回填，绑定在「订阅型 + 专属」
-- 分组上的存量 Key 会立刻收到 403 GROUP_NOT_ALLOWED，转发直接中断。
--
-- 这里按「该用户已经有一把 Key 绑在该分组上」的既成事实回填授权，等价于把旧的
-- 隐式放行显式化。只补授权、不撤授权。
INSERT INTO user_allowed_groups (user_id, group_id, created_at)
SELECT DISTINCT k.user_id, k.group_id, NOW()
FROM api_keys k
JOIN groups g ON g.id = k.group_id
JOIN users u ON u.id = k.user_id
WHERE k.deleted_at IS NULL
  AND k.group_id IS NOT NULL
  AND g.deleted_at IS NULL
  AND g.is_exclusive = TRUE
  AND u.deleted_at IS NULL
ON CONFLICT (user_id, group_id) DO NOTHING;

-- ============================================================================
-- 2) 高峰倍率：保持拆除前后的实际计费口径完全一致
-- ============================================================================
-- 旧 PeakMultiplierAt() 的第一个条件是 !IsSubscriptionType() → 返回 1.0，
-- 也就是说非订阅分组上的 peak_rate_enabled 是「配置存在但完全不生效」。
-- 新代码把高峰倍率与订阅类型解绑（任何分组只要 enabled 就生效），
-- 因此必须把这些「本来就不生效」的开关显式关掉，否则升级瞬间会静默改价。
UPDATE groups
SET peak_rate_enabled = FALSE,
    updated_at = NOW()
WHERE peak_rate_enabled = TRUE
  AND subscription_type IS DISTINCT FROM 'subscription';

-- ============================================================================
-- 3) 分组计费类型归一：订阅型分组按标准分组对待
-- ============================================================================
-- 新代码不再读 groups.subscription_type / daily|weekly|monthly_limit_usd。
-- 这里把类型归一到 'standard'，让列语义与代码一致；列本身保留，A5 再删。
-- 限额列不清空：保留数据便于回滚与事后审计（新代码不读它们）。
UPDATE groups
SET subscription_type = 'standard',
    updated_at = NOW()
WHERE subscription_type IS DISTINCT FROM 'standard';

-- ============================================================================
-- 4) 清理已无读写方的设置键
-- ============================================================================
-- 新用户默认余额 / 默认订阅 / 注册来源默认授予（authSourceDefaults）随本批消失。
DELETE FROM settings WHERE key IN (
    'default_balance',
    'default_subscriptions',
    'auth_source_default_email_balance',
    'auth_source_default_email_concurrency',
    'auth_source_default_email_subscriptions',
    'auth_source_default_email_grant_on_signup',
    'auth_source_default_email_grant_on_first_bind',
    'auth_source_default_email_platform_quotas'
);

-- ============================================================================
-- 回滚说明
-- ============================================================================
-- 本迁移不 DROP 任何表 / 列 / 索引，user_subscriptions、users.balance、
-- groups.subscription_type 与三个限额列全部原样保留。回滚到旧镜像后：
--   * 订阅数据仍在，旧代码可直接继续读；
--   * groups.subscription_type 已被归一为 'standard'，若确实存在订阅型分组，
--     回滚后需要人工把它改回 'subscription'（迁移前的取值可在备份中查到）；
--   * peak_rate_enabled 被关掉的那些分组在旧代码下本来就不生效，回滚无影响；
--   * user_allowed_groups 多出来的授权行是"只增不减"的，旧代码同样接受；
--   * 被删的设置键会由旧代码在启动时按默认值重新初始化（default_balance 回到
--     config 的 default.user_balance，其余回到空/false）。
