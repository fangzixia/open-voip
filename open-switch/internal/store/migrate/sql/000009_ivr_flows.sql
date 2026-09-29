CREATE TABLE os_ivr_flows (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  name TEXT NOT NULL,
  draft_json TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, id)
);

COMMENT ON TABLE os_ivr_flows IS 'IVR 流程草稿（按应用隔离）';
