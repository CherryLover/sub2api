-- 238_drop_registration_email_suffix_whitelist_setting.sql
--
-- 单管理员内部部署裁剪（P0 收尾）：删掉「注册设置」卡片里那条邮箱域名白名单。
--
-- 背景：自助注册在批次 1/2 已经整体移除，白名单的判定函数
-- （IsRegistrationEmailSuffixAllowed / IsRegistrationEmailSuffixLimited）
-- 在生产代码里一个调用点都没有。后台设置页却还能填能存，站长照着填会以为
-- 「只有这些域名能开户」，实际后台建用户填任何邮箱都能建成功——是虚假的安全感。
-- 本次把前端卡片、后端读写与设置键一并删除。
--
-- 关于 237 里那条「刻意保留」的注释：
-- 当时保留它是因为 InitializeDefaultSettings 拿它当「是否已种过默认设置」的探测键，
-- 删掉会让每次启动都重跑一遍种子写入。本次已把探测键换成
-- allow_ungrouped_key_scheduling（分组隔离核心开关、没有任何迁移会 INSERT 它、
-- 默认值 false 是 fail-closed），所以这一行现在可以安全删除。
--
-- 幂等：DELETE 不存在的键无副作用，重复执行只是再删一次空集。
DELETE FROM settings WHERE key = 'registration_email_suffix_whitelist';

-- 兄弟键兜底：233 已经删过一次，这里再删一次，覆盖「233 之后又被写回」的情况。
DELETE FROM settings WHERE key = 'registration_email_domain_quota_enabled';
