CREATE TABLE oc_switch_event_inbox (
  event_id BIGINT PRIMARY KEY,
  application_id TEXT NOT NULL,
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oc_switch_event_outbox (
  event_id BIGINT PRIMARY KEY REFERENCES oc_switch_event_inbox(event_id) ON DELETE CASCADE,
  payload TEXT NOT NULL,
  delivered_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_switch_event_outbox_pending ON oc_switch_event_outbox(event_id) WHERE delivered_at IS NULL;
