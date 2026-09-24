ALTER TABLE oc_agents ADD COLUMN IF NOT EXISTS terminal_type TEXT NOT NULL DEFAULT 'webrtc';
ALTER TABLE oc_agents ADD COLUMN IF NOT EXISTS sip_username TEXT NOT NULL DEFAULT '';
ALTER TABLE oc_agent_sessions ADD COLUMN IF NOT EXISTS current_call_id TEXT NOT NULL DEFAULT '';
ALTER TABLE oc_agent_sessions ADD COLUMN IF NOT EXISTS pending_checkout BOOLEAN NOT NULL DEFAULT FALSE;
COMMENT ON COLUMN oc_agents.terminal_type IS '坐席终端类型：webrtc 或 sip';
COMMENT ON COLUMN oc_agents.sip_username IS 'Switch 配置中的设备账号，不含凭证';
COMMENT ON COLUMN oc_agent_sessions.current_call_id IS '当前通话 ID，用于拒绝过期状态回调';
COMMENT ON COLUMN oc_agent_sessions.pending_checkout IS '结束当前通话后签出';
CREATE UNIQUE INDEX IF NOT EXISTS oc_agents_sip_username_unique ON oc_agents(sip_username) WHERE sip_username <> '';
