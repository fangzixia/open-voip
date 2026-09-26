CREATE TABLE IF NOT EXISTS os_call_events (
  id BIGSERIAL PRIMARY KEY,
  call_id TEXT NOT NULL,
  seq BIGINT NOT NULL,
  agent_id TEXT NOT NULL DEFAULT '',
  target_only BOOLEAN NOT NULL DEFAULT FALSE,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (call_id, seq)
);
CREATE INDEX IF NOT EXISTS idx_os_call_events_call_id_id ON os_call_events (call_id, id);
