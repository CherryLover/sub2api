-- 230_drop_email_subsystem_settings.sql
--
-- 单管理员内部部署裁剪（B5 方案 A）：整个邮件体系已从代码中移除
-- （SMTP 发送器、邮件队列、通知邮件模板、余额/配额/订阅到期提醒、
-- ops 告警与定时报表邮件、忘记密码、邮箱验证码绑定）。
-- 这些设置键在新版本里已无任何读写方，留在库里只会：
--   1) 让后台"系统设置"接口继续回显一批永远不生效的值；
--   2) 把 SMTP 口令继续留存在数据库中。
-- 故一次性清理。幂等：重复执行只是再删一次不存在的行。
--
-- 注意：本迁移只删 settings 表中的键，不动 users / ops_alert_rules 等业务表，
-- 因此可以安全回滚到旧镜像（旧代码会按各自的默认值重新初始化这些键，
-- 只是 SMTP 需要管理员重新填写）。
DELETE FROM settings WHERE key IN (
    -- SMTP 发送器配置
    'smtp_host',
    'smtp_port',
    'smtp_username',
    'smtp_password',
    'smtp_from',
    'smtp_from_name',
    'smtp_use_tls',
    -- 邮箱验证与自助找回密码
    'email_verify_enabled',
    'password_reset_enabled',
    'frontend_url',
    -- 余额不足提醒
    'balance_low_notify_enabled',
    'balance_low_notify_threshold',
    'balance_low_notify_recharge_url',
    -- 订阅到期提醒
    'subscription_expiry_notify_enabled',
    -- 上游账号额度提醒
    'account_quota_notify_enabled',
    'account_quota_notify_emails',
    -- ops 告警邮件 / 定时报表邮件配置
    'ops_email_notification_config',
    -- 退订链接签名密钥
    'notification_email_unsubscribe_secret'
);

-- 通知邮件的模板覆盖、退订偏好、投递去重标记与语言偏好都以固定前缀存放，
-- 随模板渲染链路一并失去消费方，按前缀清理。
DELETE FROM settings WHERE key LIKE 'notification_email_template:%';
DELETE FROM settings WHERE key LIKE 'notification_email_preference:%';
DELETE FROM settings WHERE key LIKE 'notification_email_delivery:%';
DELETE FROM settings WHERE key LIKE 'notification_email_locale:%';
