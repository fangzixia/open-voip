CREATE TABLE oc_agent_state_projection (
  id BIGINT PRIMARY KEY,
  agent_id UUID NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_oc_agent_state_projection_time ON oc_agent_state_projection(agent_id, created_at, id);
