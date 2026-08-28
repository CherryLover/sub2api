-- 237_drop_orphan_feature_settings.sql
--
-- 单管理员内部部署裁剪（A5 收尾）：把批次 1-3 删功能时漏清的 settings 行一次清掉。
-- 下面每个键（或键前缀）都逐条核对过——在当前后端源码里要么完全没有出现，
-- 要么只剩「门禁测试断言它不许再回流」这一种引用，没有任何生产读写方。
--
-- 幂等：DELETE 不存在的键无副作用。
--
-- 刻意保留、不在本迁移中删除的三个键：
--   * registration_enabled —— migration 229 特意把它强制置为 'false'，作为
--     「回滚到旧镜像后不会退化成完全开放注册」的安全兜底。删掉这一行会让旧代码
--     按默认值重新初始化，正好破坏 229 的意图。
--   * channel_monitor_mode —— migration 231 特意把它改成 'v2'（而不是删掉），
--     用途同上：万一回滚，旧代码读到 'v2' 会直接走被动聚合。等回滚窗口过去后
--     再单独清理。
--   * registration_email_suffix_whitelist —— InitializeDefaultSettings 用它判断
--     「是否已种过默认设置」，删掉会导致每次启动重跑种子写入（见 migration 233）。
--
-- 另外 channel_monitor_hide_throughput 与 openai_advanced_scheduler_enabled 看着像
-- 旧功能残留，实际仍被 V2 渠道监控与调度器读取，故不动。

-- ============================================================================
-- 1) 按前缀清理：以下前缀在生产代码里已经一条引用都没有
-- ============================================================================
-- 注册来源默认授予（批次 4 只清了 email 一组，linuxdo/oidc/wechat/dingtalk 还在）
DELETE FROM settings WHERE key LIKE 'auth_source_default\_%';
-- 支付渠道可见性与各 provider 配置（批次 1）
DELETE FROM settings WHERE key LIKE 'payment\_%';
-- 优惠码 / 推广返佣（批次 1）
DELETE FROM settings WHERE key LIKE 'promo\_%';
DELETE FROM settings WHERE key LIKE 'affiliate\_%';
-- 人机验证三家（批次 1 B4）
DELETE FROM settings WHERE key LIKE 'turnstile\_%';
DELETE FROM settings WHERE key LIKE 'tencent\_captcha\_%';
DELETE FROM settings WHERE key LIKE 'aliyun\_captcha\_%';
-- 登录条款 / 合规确认（批次 2 B1）
DELETE FROM settings WHERE key LIKE 'login\_agreement\_%';
-- 模型广场（批次 3；migration 232 已删三个已知键，这里兜底前缀）
DELETE FROM settings WHERE key LIKE 'model\_plaza\_%';
-- 第三方登录 provider 配置（批次 1）
DELETE FROM settings WHERE key LIKE 'oidc\_connect\_%';
DELETE FROM settings WHERE key LIKE 'wechat\_connect\_%';
-- 邮件体系（migration 230 已删已知键，这里兜底前缀）
DELETE FROM settings WHERE key LIKE 'smtp\_%';
DELETE FROM settings WHERE key LIKE 'notification\_email\_%';

-- ============================================================================
-- 2) 逐个清理：没有共同前缀的孤儿键
-- ============================================================================
DELETE FROM settings WHERE key IN (
    -- 支付遗留（大写键，不在 payment_ 前缀里）
    'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
    -- 邀请码 / 订阅购买入口（批次 1）
    'invitation_code_enabled',
    'purchase_subscription_enabled',
    'purchase_subscription_url',
    -- 第三方 OAuth 登录开关（批次 1）
    'linuxdo_oauth_enabled',
    'dingtalk_oauth_enabled',
    'wechat_oauth_enabled',
    'oidc_oauth_enabled',
    'github_oauth_enabled',
    'google_oauth_enabled',
    'force_email_on_third_party_signup',
    -- 站点外观与通用设置（批次 2 B2，站点名已收敛为固定常量 Sub2API）
    'site_name',
    'site_subtitle',
    'site_logo',
    'api_base_url',
    'contact_info',
    'home_content',
    'compact_home_enabled',
    'custom_menu_items',
    'hide_ccs_import_button',
    'table_default_page_size',
    'table_page_size_options',
    -- 内容安全审计（批次 3）
    'content_moderation_config',
    -- 渠道监控 V1 主动探测（批次 3）
    'channel_monitor_default_interval_seconds',
    'channel_monitor_show_quota'
);
