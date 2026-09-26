CREATE TABLE IF NOT EXISTS oc_switch_event_cursor (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_event_id BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO oc_switch_event_cursor (id, last_event_id) VALUES (1, 0) ON CONFLICT (id) DO NOTHING;
