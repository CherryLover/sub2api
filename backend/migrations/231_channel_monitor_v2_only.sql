-- 231_channel_monitor_v2_only.sql
--
-- 单管理员内部部署裁剪：渠道监控 V1（主动探测：checker / challenge / 调度器 /
-- 请求模板 / 配额抓取）已整体从代码中删除，只保留 V2 被动聚合。
--
-- 后果与本迁移要解决的问题：
--   1) 生产库 channel_monitor_mode 目前是 'v1'，而 channel_monitors 是 0 行 —— 也就是
--      开着一个会真实消耗上游额度的探测器空转，同时零成本的 V2 因为模式没切而不聚合。
--      新代码已经不再读这个键（V2 只由 channel_monitor_enabled 控制），但把值改成
--      'v2' 仍然有意义：万一回滚到上一版镜像，旧代码读到 'v2' 会直接走被动聚合，
--      而不是重新把主动探测打开。
--   2) channel_monitor_default_interval_seconds / channel_monitor_show_quota 只被 V1
--      的调度器与用户端配额展示读取，新代码里已经没有任何读写方，留着只会让后台
--      "系统设置"接口回显永远不生效的值。
--
-- 幂等：INSERT ... ON CONFLICT DO UPDATE 保证老库（有行，值为 v1）被改写、
-- 新库（无行，种子里也不再写这个键）被补上，重复执行结果一致。
-- 注意 229_force_registration_disabled.sql 的教训：只写 UPDATE 对新库是空操作。
INSERT INTO settings (key, value)
VALUES ('channel_monitor_mode', 'v2')
ON CONFLICT (key) DO UPDATE SET value = 'v2', updated_at = NOW();

-- V1 专属、已无消费方的设置键。
-- 只删 settings 行，不动 channel_monitors / channel_monitor_histories /
-- channel_monitor_request_templates / channel_monitor_daily_rollups 等表，
-- 表结构清理留给后续批次，因此本迁移可安全回滚。
DELETE FROM settings WHERE key IN (
    'channel_monitor_default_interval_seconds',
    'channel_monitor_show_quota'
);
