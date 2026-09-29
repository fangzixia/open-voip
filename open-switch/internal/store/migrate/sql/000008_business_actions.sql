CREATE TABLE os_business_actions (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls(id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  action TEXT NOT NULL,
  outcomes TEXT NOT NULL,
  deadline_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending','completed','expired')),
  outcome TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_os_business_actions_call ON os_business_actions(application_id,call_id);
