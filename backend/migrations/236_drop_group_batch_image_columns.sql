-- 236_drop_group_batch_image_columns.sql
--
-- 批量生图整删（批次 3）后，groups 上这三列在新版本里已无任何读写方，
-- ent schema 也已经在本批同步移除。
--
-- 幂等：DROP COLUMN IF EXISTS，重复执行无副作用。
--
-- ⚠️ 不可回滚：回滚到裁剪前的旧镜像后，ent 生成的 groups 查询会 SELECT 这三列
--    而报错，导致分组相关接口整体不可用。需要回滚时须先把列加回来
--    （默认值：allow_batch_image_generation=false、
--      batch_image_discount_multiplier=0.5、batch_image_hold_multiplier=0.6）。
--
-- 说明：groups 上的 auth cache 失效触发器（migration 193）不引用这三列，
-- 因此不需要同步改触发器函数。
ALTER TABLE groups
    DROP COLUMN IF EXISTS allow_batch_image_generation,
    DROP COLUMN IF EXISTS batch_image_discount_multiplier,
    DROP COLUMN IF EXISTS batch_image_hold_multiplier;
