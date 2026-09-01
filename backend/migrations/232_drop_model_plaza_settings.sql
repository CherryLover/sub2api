-- 232_drop_model_plaza_settings.sql
--
-- 单管理员内部部署裁剪（A2）：模型广场（公开的分组/模型定价橱窗）已整体删除，
-- 页面、路由、handler、service 与三个设置键的读写方在新版本里全部不存在。
-- 这三个键留在库里只会让后台「系统设置」接口继续回显一批永远不生效的值。
-- 幂等：重复执行只是再删一次不存在的行。
--
-- 注意：本迁移只删 settings 表中的键，不动任何业务表，可以安全回滚到旧镜像
-- （旧代码会按默认值 false/false/"" 重新初始化这三个键，广场重新变回默认关闭）。
DELETE FROM settings WHERE key IN (
    'model_plaza_enabled',
    'model_plaza_require_auth',
    'model_plaza_description'
);
