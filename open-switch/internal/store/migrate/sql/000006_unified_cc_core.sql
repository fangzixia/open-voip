-- 破坏性统一：旧 call_center/external 运行数据不兼容，直接重建 Switch 运行库。
DROP TABLE IF EXISTS os_platform_outbox CASCADE;
DROP TABLE IF EXISTS os_call_events CASCADE;
DROP TABLE IF EXISTS os_call_legs CASCADE;
DROP TABLE IF EXISTS os_calls CASCADE;
DROP TABLE IF EXISTS os_bridges CASCADE;
DROP TABLE IF EXISTS os_commands CASCADE;
DROP TABLE IF EXISTS os_routing_sessions CASCADE;
DROP TABLE IF EXISTS os_queue_entries CASCADE;
DROP TABLE IF EXISTS os_acd_attempts CASCADE;
DROP TABLE IF EXISTS os_ivr_sessions CASCADE;
DROP TABLE IF EXISTS os_agent_state_log CASCADE;
DROP TABLE IF EXISTS os_agent_session_queues CASCADE;
DROP TABLE IF EXISTS os_agent_sessions CASCADE;
DROP TABLE IF EXISTS os_agent_skills CASCADE;
DROP TABLE IF EXISTS os_queue_agents CASCADE;
DROP TABLE IF EXISTS os_queue_skills CASCADE;
DROP TABLE IF EXISTS os_did_routes CASCADE;
DROP TABLE IF EXISTS os_ivr_published_snapshots CASCADE;
DROP TABLE IF EXISTS os_agents CASCADE;
DROP TABLE IF EXISTS os_skills CASCADE;
DROP TABLE IF EXISTS os_queues CASCADE;
DROP TABLE IF EXISTS os_active_config CASCADE;
DROP TABLE IF EXISTS os_config_versions CASCADE;
DROP TABLE IF EXISTS os_cdr CASCADE;
DROP TABLE IF EXISTS os_recordings CASCADE;

CREATE TABLE os_config_versions (
  application_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('validated','active','superseded')),
  checksum TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ,
  PRIMARY KEY (application_id, version),
  UNIQUE (application_id, checksum)
);

CREATE TABLE os_active_config (
  application_id TEXT PRIMARY KEY,
  version BIGINT NOT NULL,
  activated_at TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (application_id, version) REFERENCES os_config_versions(application_id, version)
);

CREATE TABLE os_queues (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  name TEXT NOT NULL,
  video_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  max_wait_sec INTEGER NOT NULL DEFAULT 300 CHECK (max_wait_sec > 0),
  dispatch_strategy TEXT NOT NULL CHECK (dispatch_strategy IN ('longest_idle','round_robin')),
  recording_policy TEXT NOT NULL CHECK (recording_policy IN ('off','audio','video_composite')),
  overflow_action TEXT NOT NULL DEFAULT 'hangup' CHECK (overflow_action IN ('hangup','voicemail','queue')),
  overflow_queue_id UUID,
  ivr_flow_id UUID,
  wait_prompt TEXT NOT NULL DEFAULT '',
  announce_recording BOOLEAN NOT NULL DEFAULT FALSE,
  priority_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  business_hours_json TEXT NOT NULL DEFAULT 'always',
  after_hours_action TEXT NOT NULL DEFAULT 'hangup' CHECK (after_hours_action IN ('hangup','voicemail','queue')),
  force_hangup_on_checkout BOOLEAN NOT NULL DEFAULT TRUE,
  listen_announce BOOLEAN NOT NULL DEFAULT FALSE,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions(application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_skills (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  name TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, name),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions(application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_agents (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  user_ref TEXT NOT NULL,
  extension TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  video_capable BOOLEAN NOT NULL DEFAULT FALSE,
  terminal_type TEXT NOT NULL CHECK (terminal_type IN ('webrtc','sip')),
  sip_username TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, extension),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions(application_id, version) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_os_agents_sip_username ON os_agents(application_id, config_version, sip_username) WHERE sip_username <> '';

CREATE TABLE os_queue_skills (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, queue_id, skill_id),
  FOREIGN KEY (application_id, config_version, queue_id) REFERENCES os_queues(application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, skill_id) REFERENCES os_skills(application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_queue_agents (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, queue_id, agent_id),
  FOREIGN KEY (application_id, config_version, queue_id) REFERENCES os_queues(application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, agent_id) REFERENCES os_agents(application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_agent_skills (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  agent_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, agent_id, skill_id),
  FOREIGN KEY (application_id, config_version, agent_id) REFERENCES os_agents(application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, skill_id) REFERENCES os_skills(application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_did_routes (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  trunk_id TEXT NOT NULL DEFAULT '*',
  normalized_did TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('queue','ivr','reject')),
  target_id UUID,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, trunk_id, normalized_did),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions(application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_ivr_published_snapshots (
  application_id TEXT NOT NULL,
  flow_id UUID NOT NULL,
  version INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, flow_id),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions(application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_agent_sessions (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  agent_id UUID NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('idle','busy','ringing','on_call','acw')),
  busy_reason TEXT NOT NULL DEFAULT '',
  current_call_id UUID,
  pending_checkout BOOLEAN NOT NULL DEFAULT FALSE,
  checked_in_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (application_id, id),
  UNIQUE (application_id, agent_id)
);

CREATE TABLE os_agent_session_queues (
  application_id TEXT NOT NULL,
  agent_id UUID NOT NULL,
  queue_id UUID NOT NULL,
  PRIMARY KEY (application_id, agent_id, queue_id)
);

CREATE TABLE os_agent_state_log (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  agent_id UUID NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  call_id UUID,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_agent_state_log_agent_time ON os_agent_state_log(application_id, agent_id, created_at DESC);

CREATE TABLE os_calls (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  business_ref TEXT NOT NULL DEFAULT '',
  metadata TEXT NOT NULL DEFAULT '{}',
  direction TEXT NOT NULL,
  session_type TEXT NOT NULL,
  state TEXT NOT NULL,
  version BIGINT NOT NULL DEFAULT 1,
  config_version BIGINT,
  queue_id UUID,
  parent_call_id UUID,
  priority BIGINT NOT NULL DEFAULT 0,
  caller TEXT NOT NULL DEFAULT '',
  callee TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  offered_agent TEXT NOT NULL DEFAULT '',
  answered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);
CREATE INDEX idx_os_calls_app_state ON os_calls(application_id, state, updated_at);
CREATE INDEX idx_os_calls_queue_state ON os_calls(application_id, queue_id, state);

CREATE TABLE os_call_legs (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls(id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'webrtc' CHECK (type IN ('sip','webrtc','playback')),
  role TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'new',
  agent_id UUID,
  participant_ref TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_call_legs_call ON os_call_legs(application_id, call_id, created_at);

CREATE TABLE os_bridges (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls(id) ON DELETE CASCADE,
  mode TEXT NOT NULL CHECK (mode IN ('pair','conference')),
  state TEXT NOT NULL,
  participants TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);

CREATE TABLE os_routing_sessions (
  call_id UUID PRIMARY KEY REFERENCES os_calls(id) ON DELETE CASCADE,
  application_id TEXT NOT NULL,
  state TEXT NOT NULL,
  did_route_id UUID,
  queue_id UUID,
  config_version BIGINT NOT NULL,
  ivr_flow_id UUID,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);

CREATE TABLE os_queue_entries (
  call_id UUID PRIMARY KEY REFERENCES os_calls(id) ON DELETE CASCADE,
  application_id TEXT NOT NULL,
  queue_id UUID NOT NULL,
  priority BIGINT NOT NULL,
  enqueued_at TIMESTAMPTZ NOT NULL,
  deadline_at TIMESTAMPTZ NOT NULL,
  state TEXT NOT NULL
);
CREATE INDEX idx_os_queue_entries_pick ON os_queue_entries(application_id, queue_id, state, priority DESC, enqueued_at);

CREATE TABLE os_acd_attempts (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls(id) ON DELETE CASCADE,
  queue_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  attempt INTEGER NOT NULL,
  state TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  failure_reason TEXT NOT NULL DEFAULT '',
  UNIQUE (call_id, attempt)
);

CREATE TABLE os_ivr_sessions (
  call_id UUID PRIMARY KEY REFERENCES os_calls(id) ON DELETE CASCADE,
  application_id TEXT NOT NULL,
  flow_id UUID NOT NULL,
  flow_version INTEGER NOT NULL,
  node_id TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT '{}',
  deadline_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE os_commands (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  type TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('accepted','running','succeeded','failed','unknown')),
  result TEXT NOT NULL DEFAULT '{}',
  error_code TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (application_id, idempotency_key)
);

CREATE TABLE os_call_events (
  id BIGSERIAL PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID,
  seq BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 0,
  command_id UUID,
  agent_id TEXT NOT NULL DEFAULT '',
  target_only BOOLEAN NOT NULL DEFAULT FALSE,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (call_id, seq)
);
CREATE INDEX idx_os_call_events_app_id ON os_call_events(application_id, id);

CREATE TABLE os_cdr (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL UNIQUE,
  direction TEXT NOT NULL,
  queue_id UUID,
  agent_id UUID,
  caller TEXT NOT NULL DEFAULT '',
  callee TEXT NOT NULL DEFAULT '',
  session_type TEXT NOT NULL,
  result TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  answered_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  duration_sec INTEGER NOT NULL DEFAULT 0,
  wait_sec INTEGER NOT NULL DEFAULT 0,
  video_started_at TIMESTAMPTZ,
  video_upgrade_ok BOOLEAN NOT NULL DEFAULT FALSE,
  screen_share_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE os_recordings (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL,
  file_path TEXT NOT NULL,
  media_type TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  retain_until TIMESTAMPTZ,
  file_size BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_recordings_call ON os_recordings(application_id, call_id);
