-- 229_force_registration_disabled.sql
--
-- 单管理员内部部署裁剪：邀请码/优惠码门禁与第三方 OAuth 登录已删除。
-- 老库若曾配置 registration_enabled=true 并依赖邀请码限制注册，升级后
-- 该门禁不复存在，会退化为完全开放注册。此迁移一次性强制关闭开放注册，
-- 需要多用户时由管理员在系统设置中显式重新开启。
-- 幂等：重复执行只会把该键再次置为 'false'。
UPDATE settings SET value = 'false', updated_at = NOW() WHERE key = 'registration_enabled';
