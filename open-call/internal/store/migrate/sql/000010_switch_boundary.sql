-- 运行态坐席表不再由业务库持有；DID 表只保存待发布的业务配置草稿。
DROP TABLE IF EXISTS oc_agent_session_queues CASCADE;
DROP TABLE IF EXISTS oc_agent_sessions CASCADE;
DROP TABLE IF EXISTS oc_agent_state_log CASCADE;
DROP TABLE IF EXISTS oc_did_routes CASCADE;

CREATE TABLE oc_did_routes (
  id UUID PRIMARY KEY,
  trunk_id TEXT NOT NULL DEFAULT '*',
  d_id TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('queue','ivr','reject')),
  target_id UUID,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (trunk_id, d_id),
  CHECK ((target_type = 'reject' AND target_id IS NULL) OR (target_type IN ('queue','ivr') AND target_id IS NOT NULL))
);
