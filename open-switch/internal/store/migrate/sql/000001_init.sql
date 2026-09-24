-- open-switch 运行时表（os_*）

CREATE TABLE IF NOT EXISTS os_calls (
  id UUID PRIMARY KEY,
  direction VARCHAR(16) NOT NULL,
  session_type VARCHAR(16) NOT NULL DEFAULT 'audio',
  state VARCHAR(32) NOT NULL,
  queue_id UUID,
  parent_call_id UUID,
  priority BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_os_calls_direction ON os_calls (direction);
CREATE INDEX IF NOT EXISTS idx_os_calls_state ON os_calls (state);
CREATE INDEX IF NOT EXISTS idx_os_calls_queue_id ON os_calls (queue_id);
CREATE INDEX IF NOT EXISTS idx_os_calls_parent_call_id ON os_calls (parent_call_id);

CREATE TABLE IF NOT EXISTS os_call_legs (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  role VARCHAR(32) NOT NULL,
  agent_id UUID,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_os_call_legs_call_id ON os_call_legs (call_id);
CREATE INDEX IF NOT EXISTS idx_os_call_legs_agent_id ON os_call_legs (agent_id);

COMMENT ON TABLE os_calls IS '呼叫中心会话主表，媒体 Room ID 与 id 一致';
COMMENT ON COLUMN os_calls.id IS '通话 ID 与媒体 Room 一致';
COMMENT ON COLUMN os_calls.direction IS '呼叫方向';
COMMENT ON COLUMN os_calls.session_type IS '会话媒介类型';
COMMENT ON COLUMN os_calls.state IS '呼叫状态机状态';
COMMENT ON COLUMN os_calls.queue_id IS '关联队列 ID';
COMMENT ON COLUMN os_calls.parent_call_id IS '父通话 ID';
COMMENT ON COLUMN os_calls.priority IS '优先级';
COMMENT ON COLUMN os_calls.created_at IS '创建时间';
COMMENT ON COLUMN os_calls.updated_at IS '更新时间';
COMMENT ON COLUMN os_calls.ended_at IS '结束时间';

COMMENT ON TABLE os_call_legs IS '通话媒体腿（客户、坐席、IVR 等）';
COMMENT ON COLUMN os_call_legs.id IS '通话腿 ID';
COMMENT ON COLUMN os_call_legs.call_id IS '所属通话 ID';
COMMENT ON COLUMN os_call_legs.role IS '腿角色';
COMMENT ON COLUMN os_call_legs.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_call_legs.created_at IS '创建时间';
