-- 自旧版 baseline 升级：删除 Switch 镜像表；WS 表由 000002 早期版本或新 baseline 创建。
DROP TABLE IF EXISTS oc_queue_skills;
DROP TABLE IF EXISTS oc_queue_agents;
DROP TABLE IF EXISTS oc_queues;
DROP TABLE IF EXISTS oc_agent_skills;
DROP TABLE IF EXISTS oc_skills;
DROP TABLE IF EXISTS oc_did_routes;

CREATE TABLE IF NOT EXISTS oc_ws_event_seq (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  last_seq BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO oc_ws_event_seq (id, last_seq) VALUES (1, 0) ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS oc_ws_event_buffer (
  seq BIGINT PRIMARY KEY,
  call_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oc_ws_event_buffer_created ON oc_ws_event_buffer (created_at);
