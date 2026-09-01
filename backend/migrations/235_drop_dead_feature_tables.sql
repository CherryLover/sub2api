-- 235_drop_dead_feature_tables.sql
--
-- 单管理员内部部署裁剪（A5 数据库压平）：批次 1-4 已经把下列功能的业务代码整体删除，
-- 本批把它们在 ent 里的实体定义一并删掉，这些表在新版本里不再有任何读写方。
--
--   支付/订单：payment_orders、payment_audit_logs、payment_provider_instances
--   兑换/促销：redeem_codes、promo_codes、promo_code_usages
--   推广返佣：user_affiliates、user_affiliate_ledger
--   订阅体系：subscription_plans、user_subscriptions
--   批量生图：batch_image_jobs、batch_image_items、batch_image_events
--   渠道监控 V1：channel_monitors、channel_monitor_histories、
--                channel_monitor_request_templates、channel_monitor_daily_rollups、
--                channel_monitor_aggregation_watermark
--   公告体系：announcements、announcement_reads
--   内容审计：content_moderation_logs
--
-- 幂等：全部使用 IF EXISTS，重复执行无副作用。
--
-- ⚠️ 不可回滚：这是本次裁剪里第一条真正丢数据的迁移。回滚到裁剪前的旧镜像后，
--    旧代码会因为这些表不存在而无法启动/报错。需要保留历史数据的环境，
--    请在升级前自行 pg_dump 这些表。
--
-- 关于 CASCADE：只有一条跨界外键——usage_logs.subscription_id REFERENCES
-- user_subscriptions(id)（migration 003）。CASCADE 只会删掉 usage_logs 上的这条
-- 外键约束，不会删掉 usage_logs.subscription_id 这一列，更不会动 usage_logs 的数据。
-- 该列目前仍由 repository 的裸 SQL 读写、并出现在用量 DTO 里，故刻意保留。
-- 其余外键都在本次要删的表之间（dead → dead）。全库没有视图依赖这些表。

-- 渠道监控 V1（V2 的 channel_monitor_v2_* 系列表不在此列，正在使用中）
DROP TABLE IF EXISTS channel_monitor_daily_rollups CASCADE;
DROP TABLE IF EXISTS channel_monitor_histories CASCADE;
DROP TABLE IF EXISTS channel_monitor_request_templates CASCADE;
DROP TABLE IF EXISTS channel_monitor_aggregation_watermark CASCADE;
DROP TABLE IF EXISTS channel_monitors CASCADE;

-- 批量生图（普通生图 / 异步生图不涉及这些表）
DROP TABLE IF EXISTS batch_image_events CASCADE;
DROP TABLE IF EXISTS batch_image_items CASCADE;
DROP TABLE IF EXISTS batch_image_jobs CASCADE;

-- 公告
DROP TABLE IF EXISTS announcement_reads CASCADE;
DROP TABLE IF EXISTS announcements CASCADE;

-- 内容审计（提示词审计走 prompt_audit_* 两张表，不在此列）
DROP TABLE IF EXISTS content_moderation_logs CASCADE;

-- 推广返佣
DROP TABLE IF EXISTS user_affiliate_ledger CASCADE;
DROP TABLE IF EXISTS user_affiliates CASCADE;

-- 兑换码 / 优惠码
DROP TABLE IF EXISTS promo_code_usages CASCADE;
DROP TABLE IF EXISTS promo_codes CASCADE;
DROP TABLE IF EXISTS redeem_codes CASCADE;

-- 支付
DROP TABLE IF EXISTS payment_audit_logs CASCADE;
DROP TABLE IF EXISTS payment_orders CASCADE;
DROP TABLE IF EXISTS payment_provider_instances CASCADE;

-- 订阅（usage_logs.subscription_id 的外键随 CASCADE 一起消失，列本身保留）
DROP TABLE IF EXISTS user_subscriptions CASCADE;
DROP TABLE IF EXISTS subscription_plans CASCADE;
