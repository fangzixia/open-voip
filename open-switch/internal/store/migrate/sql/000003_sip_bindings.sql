CREATE TABLE IF NOT EXISTS os_sip_bindings (
  aor TEXT NOT NULL,
  contact_uri TEXT NOT NULL,
  call_id TEXT NOT NULL DEFAULT '',
  cseq BIGINT NOT NULL DEFAULT 0,
  addr TEXT NOT NULL DEFAULT '',
  expires_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (aor, contact_uri)
);
CREATE INDEX IF NOT EXISTS idx_os_sip_bindings_expires ON os_sip_bindings (expires_at);
