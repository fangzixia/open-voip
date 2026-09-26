-- 为已部署的交换服务运行时表补齐中文数据库注释。
COMMENT ON COLUMN os_calls.caller IS '主叫号码或用户标识';
COMMENT ON COLUMN os_calls.callee IS '被叫号码或用户标识';
COMMENT ON COLUMN os_calls.agent_id IS '已接听坐席 ID';
COMMENT ON COLUMN os_calls.offered_agent IS '当前被邀约坐席 ID';
COMMENT ON COLUMN os_calls.answered_at IS '通话接通时间';

COMMENT ON COLUMN os_platform_outbox.id IS '平台回调任务主键';
COMMENT ON COLUMN os_platform_outbox.path IS '平台回调接口路径';
COMMENT ON COLUMN os_platform_outbox.payload IS '平台回调请求内容';
COMMENT ON COLUMN os_platform_outbox.created_at IS '回调任务创建时间';
COMMENT ON COLUMN os_platform_outbox.attempts IS '回调已尝试次数';
COMMENT ON COLUMN os_platform_outbox.last_error IS '最近一次回调失败原因';
COMMENT ON COLUMN os_platform_outbox.trace_id IS '关联链路追踪 ID';
COMMENT ON COLUMN os_platform_outbox.request_id IS '关联请求 ID';

COMMENT ON TABLE os_schema_migrations IS '交换服务已执行的数据库迁移版本';
COMMENT ON COLUMN os_schema_migrations.version IS '迁移版本号';
COMMENT ON COLUMN os_schema_migrations.name IS '迁移文件名称';
COMMENT ON COLUMN os_schema_migrations.applied_at IS '迁移执行时间';
