-- open-switch 杩愯搴擄紙os_*锛夌┖搴?schema锛堝崟绉熸埛锛夈€?
-- PostgreSQL锛涜〃鍚嶄笌鍒楀悕鑻辨枃锛屼腑鏂囪 COMMENT銆?


CREATE TABLE os_config_versions (
  version BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('validated', 'active', 'superseded')),
  checksum TEXT NOT NULL,
  payload TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  activated_at TIMESTAMPTZ,
  PRIMARY KEY (version),
  UNIQUE (checksum)
);

CREATE TABLE os_active_config (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  version BIGINT NOT NULL,
  activated_at TIMESTAMPTZ NOT NULL,
  FOREIGN KEY (version) REFERENCES os_config_versions (version)
);

CREATE TABLE os_queues (
  id UUID NOT NULL,
  name TEXT NOT NULL,
  video_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  max_wait_sec INTEGER NOT NULL DEFAULT 300 CHECK (max_wait_sec > 0),
  dispatch_strategy TEXT NOT NULL CHECK (dispatch_strategy IN ('longest_idle', 'round_robin')),
  recording_policy TEXT NOT NULL CHECK (recording_policy IN ('off', 'audio', 'video_composite')),
  overflow_action TEXT NOT NULL DEFAULT 'hangup' CHECK (overflow_action IN ('hangup', 'voicemail', 'queue')),
  overflow_queue_id UUID,
  ivr_flow_id UUID,
  wait_prompt TEXT NOT NULL DEFAULT '',
  announce_recording BOOLEAN NOT NULL DEFAULT FALSE,
  priority_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  business_hours_json TEXT NOT NULL DEFAULT 'always',
  after_hours_action TEXT NOT NULL DEFAULT 'hangup' CHECK (after_hours_action IN ('hangup', 'voicemail', 'queue')),
  force_hangup_on_checkout BOOLEAN NOT NULL DEFAULT TRUE,
  listen_announce BOOLEAN NOT NULL DEFAULT FALSE,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (config_version, id),
  FOREIGN KEY (config_version) REFERENCES os_config_versions (version) ON DELETE CASCADE
);

CREATE TABLE os_skills (
  id UUID NOT NULL,
  name TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (config_version, id),
  UNIQUE (config_version, name),
  FOREIGN KEY (config_version) REFERENCES os_config_versions (version) ON DELETE CASCADE
);

CREATE TABLE os_agents (
  id UUID NOT NULL,
  user_ref TEXT NOT NULL,
  extension TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  video_capable BOOLEAN NOT NULL DEFAULT FALSE,
  terminal_type TEXT NOT NULL CHECK (terminal_type IN ('webrtc', 'sip')),
  sip_username TEXT NOT NULL DEFAULT '',
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (config_version, id),
  UNIQUE (config_version, extension),
  FOREIGN KEY (config_version) REFERENCES os_config_versions (version) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_os_agents_sip_username ON os_agents (config_version, sip_username) WHERE sip_username <> '';

CREATE TABLE os_queue_skills (
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (config_version, queue_id, skill_id),
  FOREIGN KEY (config_version, queue_id) REFERENCES os_queues (config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (config_version, skill_id) REFERENCES os_skills (config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_queue_agents (
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  PRIMARY KEY (config_version, queue_id, agent_id),
  FOREIGN KEY (config_version, queue_id) REFERENCES os_queues (config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (config_version, agent_id) REFERENCES os_agents (config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_agent_skills (
  config_version BIGINT NOT NULL,
  agent_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (config_version, agent_id, skill_id),
  FOREIGN KEY (config_version, agent_id) REFERENCES os_agents (config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (config_version, skill_id) REFERENCES os_skills (config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_did_routes (
  id UUID NOT NULL,
  trunk_id TEXT NOT NULL DEFAULT '*',
  normalized_did TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('queue', 'ivr', 'reject')),
  target_id UUID,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (config_version, id),
  UNIQUE (config_version, trunk_id, normalized_did),
  FOREIGN KEY (config_version) REFERENCES os_config_versions (version) ON DELETE CASCADE
);

CREATE TABLE os_ivr_published_snapshots (
  flow_id UUID NOT NULL,
  version INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (config_version, flow_id),
  FOREIGN KEY (config_version) REFERENCES os_config_versions (version) ON DELETE CASCADE
);

CREATE TABLE os_agent_sessions (
  id UUID NOT NULL,
  agent_id UUID NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('idle', 'busy', 'ringing', 'on_call', 'acw')),
  busy_reason TEXT NOT NULL DEFAULT '',
  current_call_id UUID,
  pending_checkout BOOLEAN NOT NULL DEFAULT FALSE,
  checked_in_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (id),
  UNIQUE (agent_id)
);

CREATE TABLE os_agent_session_queues (
  agent_id UUID NOT NULL,
  queue_id UUID NOT NULL,
  PRIMARY KEY (agent_id, queue_id)
);

CREATE TABLE os_agent_state_log (
  id UUID PRIMARY KEY,
  agent_id UUID NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  call_id UUID,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_agent_state_log_agent_time ON os_agent_state_log (agent_id, created_at DESC);

CREATE TABLE os_calls (
  id UUID PRIMARY KEY,
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
  agent_id UUID,
  offered_agent UUID,
  answered_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);
CREATE INDEX idx_os_calls_state ON os_calls (state, updated_at);
CREATE INDEX idx_os_calls_queue_state ON os_calls (queue_id, state);

CREATE TABLE os_call_legs (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'webrtc' CHECK (type IN ('sip', 'webrtc', 'playback')),
  role TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'new',
  agent_id UUID,
  participant_ref TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_call_legs_call ON os_call_legs (call_id, created_at);

CREATE TABLE os_bridges (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
  mode TEXT NOT NULL CHECK (mode IN ('pair', 'conference')),
  state TEXT NOT NULL,
  participants TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);

CREATE TABLE os_routing_sessions (
  call_id UUID PRIMARY KEY REFERENCES os_calls (id) ON DELETE CASCADE,
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
  call_id UUID PRIMARY KEY REFERENCES os_calls (id) ON DELETE CASCADE,
  queue_id UUID NOT NULL,
  priority BIGINT NOT NULL,
  enqueued_at TIMESTAMPTZ NOT NULL,
  deadline_at TIMESTAMPTZ NOT NULL,
  state TEXT NOT NULL
);
CREATE INDEX idx_os_queue_entries_pick ON os_queue_entries (queue_id, state, priority DESC, enqueued_at);

CREATE TABLE os_acd_attempts (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
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
  call_id UUID PRIMARY KEY REFERENCES os_calls (id) ON DELETE CASCADE,
  flow_id UUID NOT NULL,
  flow_version INTEGER NOT NULL,
  node_id TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT '{}',
  deadline_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE os_commands (
  id UUID PRIMARY KEY,
  call_id UUID,
  idempotency_key TEXT NOT NULL,
  request_hash TEXT NOT NULL,
  type TEXT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('accepted', 'running', 'succeeded', 'failed', 'unknown')),
  result TEXT NOT NULL DEFAULT '{}',
  error_code TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (idempotency_key)
);

CREATE TABLE os_call_events (
  id BIGSERIAL PRIMARY KEY,
  call_id UUID,
  seq BIGINT NOT NULL,
  version BIGINT NOT NULL DEFAULT 0,
  command_id UUID,
  agent_id UUID,
  target_only BOOLEAN NOT NULL DEFAULT FALSE,
  type TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (call_id, seq)
);
CREATE INDEX idx_os_call_events_id ON os_call_events (id);

CREATE TABLE os_cdr (
  id UUID PRIMARY KEY,
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
  call_id UUID NOT NULL,
  file_path TEXT NOT NULL,
  media_type TEXT NOT NULL,
  started_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ,
  retain_until TIMESTAMPTZ,
  file_size BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_recordings_call ON os_recordings (call_id);

CREATE TABLE os_queue_dispatch_cursor (
  queue_id UUID NOT NULL,
  last_agent_id UUID,
  PRIMARY KEY (queue_id)
);

CREATE TABLE os_business_actions (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
  node_id TEXT NOT NULL,
  action TEXT NOT NULL,
  outcomes TEXT NOT NULL,
  deadline_at TIMESTAMPTZ NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'completed', 'expired')),
  outcome TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_os_business_actions_call ON os_business_actions (call_id);

CREATE TABLE os_integrator_event_deliveries (
  event_id BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'success', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (event_id)
);
CREATE INDEX idx_os_integrator_event_deliveries_retry
  ON os_integrator_event_deliveries (status, next_retry_at)
  WHERE status = 'pending';

-- open-switch 涓枃娉ㄩ噴
COMMENT ON TABLE os_schema_migrations IS '浜ゆ崲鏈嶅姟宸叉墽琛岀殑鏁版嵁搴撹縼绉荤増鏈?;
COMMENT ON COLUMN os_schema_migrations.version IS '杩佺Щ鐗堟湰鍙?;
COMMENT ON COLUMN os_schema_migrations.name IS '杩佺Щ鏂囦欢鍚嶇О';
COMMENT ON COLUMN os_schema_migrations.applied_at IS '杩佺Щ鎵ц鏃堕棿';

COMMENT ON TABLE os_config_versions IS '閰嶇疆鍖呯増鏈巻鍙?;
COMMENT ON COLUMN os_config_versions.version IS '閰嶇疆鐗堟湰鍙?;
COMMENT ON COLUMN os_config_versions.status IS '鐗堟湰鐘舵€?validated active superseded';
COMMENT ON COLUMN os_config_versions.checksum IS '閰嶇疆鍐呭鏍￠獙鍜?;
COMMENT ON COLUMN os_config_versions.payload IS '瀹屾暣閰嶇疆 JSON';
COMMENT ON COLUMN os_config_versions.created_at IS '鐗堟湰鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_config_versions.activated_at IS '婵€娲绘椂闂?;

COMMENT ON TABLE os_active_config IS '褰撳墠鐢熸晥鐨勯厤缃増鏈寚閽堬紙鍗曡锛?;
COMMENT ON COLUMN os_active_config.id IS '鍥哄畾涓?1';
COMMENT ON COLUMN os_active_config.version IS '褰撳墠鐢熸晥閰嶇疆鐗堟湰鍙?;
COMMENT ON COLUMN os_active_config.activated_at IS '鍒囨崲鐢熸晥鏃堕棿';

COMMENT ON TABLE os_queues IS '宸插彂甯冮槦鍒楅厤缃揩鐓?;
COMMENT ON COLUMN os_queues.id IS '闃熷垪 ID';
COMMENT ON COLUMN os_queues.name IS '闃熷垪鍚嶇О';
COMMENT ON COLUMN os_queues.video_enabled IS '鏄惁瑙嗛闃熷垪';
COMMENT ON COLUMN os_queues.max_wait_sec IS '鏈€澶х瓑寰呯鏁?;
COMMENT ON COLUMN os_queues.dispatch_strategy IS 'ACD 娲惧崟绛栫暐';
COMMENT ON COLUMN os_queues.recording_policy IS '褰曢煶绛栫暐';
COMMENT ON COLUMN os_queues.overflow_action IS '婧㈠嚭鍔ㄤ綔';
COMMENT ON COLUMN os_queues.overflow_queue_id IS '婧㈠嚭鐩爣闃熷垪 ID';
COMMENT ON COLUMN os_queues.ivr_flow_id IS '缁戝畾 IVR 娴佺▼ ID';
COMMENT ON COLUMN os_queues.wait_prompt IS '鎺掗槦鎻愮ず鏂囨';
COMMENT ON COLUMN os_queues.announce_recording IS '鏄惁鎾斁褰曢煶鍛婄煡';
COMMENT ON COLUMN os_queues.priority_enabled IS '鏄惁鍚敤浼樺厛绾?;
COMMENT ON COLUMN os_queues.business_hours_json IS '宸ヤ綔鏃堕棿閰嶇疆';
COMMENT ON COLUMN os_queues.after_hours_action IS '闈炲伐浣滄椂闂村姩浣?;
COMMENT ON COLUMN os_queues.force_hangup_on_checkout IS '寮哄埗绛惧嚭鏄惁鎸傛柇';
COMMENT ON COLUMN os_queues.listen_announce IS '鐩戝惉鏄惁鎻愮ず瀹㈡埛';
COMMENT ON COLUMN os_queues.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_queues.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_queues.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_skills IS '宸插彂甯冩妧鑳界粍閰嶇疆蹇収';
COMMENT ON COLUMN os_skills.id IS '鎶€鑳?ID';
COMMENT ON COLUMN os_skills.name IS '鎶€鑳藉悕绉?;
COMMENT ON COLUMN os_skills.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_skills.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE os_agents IS '宸插彂甯冨潗甯厤缃揩鐓?;
COMMENT ON COLUMN os_agents.id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_agents.user_ref IS '涓氬姟渚х敤鎴峰紩鐢?;
COMMENT ON COLUMN os_agents.extension IS '鍒嗘満鍙?;
COMMENT ON COLUMN os_agents.display_name IS '灞曠ず鍚?;
COMMENT ON COLUMN os_agents.video_capable IS '鏄惁鏀寔瑙嗛';
COMMENT ON COLUMN os_agents.terminal_type IS '缁堢绫诲瀷 webrtc 鎴?sip';
COMMENT ON COLUMN os_agents.sip_username IS 'SIP 璁惧璐﹀彿';
COMMENT ON COLUMN os_agents.enabled IS '鏄惁鍚敤';
COMMENT ON COLUMN os_agents.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_agents.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_agents.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_queue_skills IS '闃熷垪涓庢妧鑳藉叧鑱旓紙閰嶇疆蹇収锛?;
COMMENT ON COLUMN os_queue_skills.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_queue_skills.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_queue_skills.skill_id IS '鎶€鑳?ID';

COMMENT ON TABLE os_queue_agents IS '闃熷垪涓庡潗甯叧鑱旓紙閰嶇疆蹇収锛?;
COMMENT ON COLUMN os_queue_agents.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_queue_agents.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_queue_agents.agent_id IS '鍧愬腑 ID';

COMMENT ON TABLE os_agent_skills IS '鍧愬腑涓庢妧鑳藉叧鑱旓紙閰嶇疆蹇収锛?;
COMMENT ON COLUMN os_agent_skills.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_agent_skills.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_agent_skills.skill_id IS '鎶€鑳?ID';

COMMENT ON TABLE os_did_routes IS '宸插彂甯冨懠鍏ュ彿鐮佽矾鐢?;
COMMENT ON COLUMN os_did_routes.id IS '璺敱 ID';
COMMENT ON COLUMN os_did_routes.trunk_id IS '涓户鏍囪瘑';
COMMENT ON COLUMN os_did_routes.normalized_did IS '褰掍竴鍖?DID';
COMMENT ON COLUMN os_did_routes.target_type IS '鐩爣绫诲瀷 queue ivr reject';
COMMENT ON COLUMN os_did_routes.target_id IS '鐩爣璧勬簮 ID';
COMMENT ON COLUMN os_did_routes.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_did_routes.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_did_routes.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_ivr_published_snapshots IS '宸插彂甯?IVR 娴佺▼蹇収';
COMMENT ON COLUMN os_ivr_published_snapshots.flow_id IS 'IVR 娴佺▼ ID';
COMMENT ON COLUMN os_ivr_published_snapshots.version IS '娴佺▼鍐呯増鏈彿';
COMMENT ON COLUMN os_ivr_published_snapshots.payload_json IS '娴佺▼瀹氫箟 JSON';
COMMENT ON COLUMN os_ivr_published_snapshots.config_version IS '鎵€灞為厤缃増鏈彿';
COMMENT ON COLUMN os_ivr_published_snapshots.published_at IS '鍙戝竷鏃堕棿';

COMMENT ON TABLE os_agent_sessions IS '鍧愬腑绛惧叆涓庝細璇濈姸鎬侊紙杩愯鎬侊級';
COMMENT ON COLUMN os_agent_sessions.id IS '绛惧叆浼氳瘽 ID';
COMMENT ON COLUMN os_agent_sessions.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_agent_sessions.state IS '鍧愬腑鐘舵€?;
COMMENT ON COLUMN os_agent_sessions.busy_reason IS '绀哄繖鍘熷洜';
COMMENT ON COLUMN os_agent_sessions.current_call_id IS '褰撳墠閫氳瘽 ID';
COMMENT ON COLUMN os_agent_sessions.pending_checkout IS '閫氳瘽缁撴潫鍚庢槸鍚︾鍑?;
COMMENT ON COLUMN os_agent_sessions.checked_in_at IS '绛惧叆鏃堕棿';
COMMENT ON COLUMN os_agent_sessions.updated_at IS '鐘舵€佹洿鏂版椂闂?;

COMMENT ON TABLE os_agent_session_queues IS '绛惧叆鍧愬腑鎵€閫夐槦鍒?;
COMMENT ON COLUMN os_agent_session_queues.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_agent_session_queues.queue_id IS '闃熷垪 ID';

COMMENT ON TABLE os_agent_state_log IS '鍧愬腑鐘舵€佸彉鏇村璁℃棩蹇?;
COMMENT ON COLUMN os_agent_state_log.id IS '鏃ュ織 ID';
COMMENT ON COLUMN os_agent_state_log.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_agent_state_log.from_state IS '鍘熺姸鎬?;
COMMENT ON COLUMN os_agent_state_log.to_state IS '鏂扮姸鎬?;
COMMENT ON COLUMN os_agent_state_log.reason IS '鍙樻洿鍘熷洜';
COMMENT ON COLUMN os_agent_state_log.call_id IS '鍏宠仈閫氳瘽 ID';
COMMENT ON COLUMN os_agent_state_log.created_at IS '璁板綍鏃堕棿';

COMMENT ON TABLE os_calls IS '鍛煎彨涓績浼氳瘽涓昏〃锛屽獟浣?Room ID 涓?id 涓€鑷?;
COMMENT ON COLUMN os_calls.id IS '閫氳瘽 ID 涓庡獟浣?Room 涓€鑷?;
COMMENT ON COLUMN os_calls.business_ref IS '涓氬姟渚ч€氳瘽寮曠敤';
COMMENT ON COLUMN os_calls.metadata IS '鎵╁睍鍏冩暟鎹?JSON';
COMMENT ON COLUMN os_calls.direction IS '鍛煎彨鏂瑰悜';
COMMENT ON COLUMN os_calls.session_type IS '浼氳瘽濯掍粙绫诲瀷';
COMMENT ON COLUMN os_calls.state IS '鍛煎彨鐘舵€佹満鐘舵€?;
COMMENT ON COLUMN os_calls.version IS '涔愯閿佺増鏈?;
COMMENT ON COLUMN os_calls.config_version IS '璺敱鏃朵娇鐢ㄧ殑閰嶇疆鐗堟湰';
COMMENT ON COLUMN os_calls.queue_id IS '鍏宠仈闃熷垪 ID';
COMMENT ON COLUMN os_calls.parent_call_id IS '鐖堕€氳瘽 ID';
COMMENT ON COLUMN os_calls.priority IS '浼樺厛绾?;
COMMENT ON COLUMN os_calls.caller IS '涓诲彨鍙风爜鎴栫敤鎴锋爣璇?;
COMMENT ON COLUMN os_calls.callee IS '琚彨鍙风爜鎴栫敤鎴锋爣璇?;
COMMENT ON COLUMN os_calls.agent_id IS '宸叉帴鍚潗甯?ID';
COMMENT ON COLUMN os_calls.offered_agent IS '褰撳墠琚個绾﹀潗甯?ID';
COMMENT ON COLUMN os_calls.answered_at IS '閫氳瘽鎺ラ€氭椂闂?;
COMMENT ON COLUMN os_calls.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_calls.updated_at IS '鏇存柊鏃堕棿';
COMMENT ON COLUMN os_calls.ended_at IS '缁撴潫鏃堕棿';

COMMENT ON TABLE os_call_legs IS '閫氳瘽濯掍綋鑵匡紙瀹㈡埛銆佸潗甯€両VR 绛夛級';
COMMENT ON COLUMN os_call_legs.id IS '閫氳瘽鑵?ID';
COMMENT ON COLUMN os_call_legs.call_id IS '鎵€灞為€氳瘽 ID';
COMMENT ON COLUMN os_call_legs.type IS '鑵跨被鍨?sip webrtc playback';
COMMENT ON COLUMN os_call_legs.role IS '鑵胯鑹?;
COMMENT ON COLUMN os_call_legs.state IS '鑵跨姸鎬?;
COMMENT ON COLUMN os_call_legs.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_call_legs.participant_ref IS '鍙備笌鑰呭紩鐢?;
COMMENT ON COLUMN os_call_legs.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE os_bridges IS '閫氳瘽妗ユ帴锛堝弻浜烘垨浼氳锛?;
COMMENT ON COLUMN os_bridges.id IS '妗ユ帴 ID';
COMMENT ON COLUMN os_bridges.call_id IS '鎵€灞為€氳瘽 ID';
COMMENT ON COLUMN os_bridges.mode IS '妗ユ帴妯″紡 pair 鎴?conference';
COMMENT ON COLUMN os_bridges.state IS '妗ユ帴鐘舵€?;
COMMENT ON COLUMN os_bridges.participants IS '鍙備笌鑰呭垪琛?JSON';
COMMENT ON COLUMN os_bridges.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_bridges.ended_at IS '缁撴潫鏃堕棿';

COMMENT ON TABLE os_routing_sessions IS '鍛煎叆璺敱浼氳瘽鐘舵€?;
COMMENT ON COLUMN os_routing_sessions.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_routing_sessions.state IS '璺敱鐘舵€?;
COMMENT ON COLUMN os_routing_sessions.did_route_id IS '鍖归厤鐨?DID 璺敱 ID';
COMMENT ON COLUMN os_routing_sessions.queue_id IS '褰撳墠闃熷垪 ID';
COMMENT ON COLUMN os_routing_sessions.config_version IS '閰嶇疆鐗堟湰鍙?;
COMMENT ON COLUMN os_routing_sessions.ivr_flow_id IS '褰撳墠 IVR 娴佺▼ ID';
COMMENT ON COLUMN os_routing_sessions.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_routing_sessions.updated_at IS '鏇存柊鏃堕棿';
COMMENT ON COLUMN os_routing_sessions.ended_at IS '缁撴潫鏃堕棿';

COMMENT ON TABLE os_queue_entries IS '鎺掗槦涓殑閫氳瘽鏉＄洰';
COMMENT ON COLUMN os_queue_entries.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_queue_entries.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_queue_entries.priority IS '鎺掗槦浼樺厛绾?;
COMMENT ON COLUMN os_queue_entries.enqueued_at IS '鍏ラ槦鏃堕棿';
COMMENT ON COLUMN os_queue_entries.deadline_at IS '鏈€闀跨瓑寰呮埅姝㈡椂闂?;
COMMENT ON COLUMN os_queue_entries.state IS '鎺掗槦鏉＄洰鐘舵€?;

COMMENT ON TABLE os_acd_attempts IS 'ACD 鍚戝潗甯淳鍗曞皾璇曡褰?;
COMMENT ON COLUMN os_acd_attempts.id IS '灏濊瘯璁板綍 ID';
COMMENT ON COLUMN os_acd_attempts.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_acd_attempts.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_acd_attempts.agent_id IS '鐩爣鍧愬腑 ID';
COMMENT ON COLUMN os_acd_attempts.attempt IS '绗嚑娆℃淳鍗?;
COMMENT ON COLUMN os_acd_attempts.state IS '灏濊瘯鐘舵€?;
COMMENT ON COLUMN os_acd_attempts.started_at IS '寮€濮嬫椂闂?;
COMMENT ON COLUMN os_acd_attempts.ended_at IS '缁撴潫鏃堕棿';
COMMENT ON COLUMN os_acd_attempts.failure_reason IS '澶辫触鍘熷洜';

COMMENT ON TABLE os_ivr_sessions IS 'IVR 杩愯鏃惰妭鐐圭姸鎬?;
COMMENT ON COLUMN os_ivr_sessions.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_ivr_sessions.flow_id IS 'IVR 娴佺▼ ID';
COMMENT ON COLUMN os_ivr_sessions.flow_version IS '娴佺▼鐗堟湰';
COMMENT ON COLUMN os_ivr_sessions.node_id IS '褰撳墠鑺傜偣 ID';
COMMENT ON COLUMN os_ivr_sessions.state IS '鑺傜偣杩愯鏃剁姸鎬?JSON';
COMMENT ON COLUMN os_ivr_sessions.deadline_at IS '鑺傜偣瓒呮椂鏃堕棿';
COMMENT ON COLUMN os_ivr_sessions.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_commands IS '寮傛鎺у埗鍛戒护涓庡箓绛夎褰?;
COMMENT ON COLUMN os_commands.id IS '鍛戒护 ID';
COMMENT ON COLUMN os_commands.call_id IS '鍏宠仈閫氳瘽 ID';
COMMENT ON COLUMN os_commands.idempotency_key IS '骞傜瓑閿?;
COMMENT ON COLUMN os_commands.request_hash IS '璇锋眰浣撳搱甯?;
COMMENT ON COLUMN os_commands.type IS '鍛戒护绫诲瀷';
COMMENT ON COLUMN os_commands.status IS '鎵ц鐘舵€?;
COMMENT ON COLUMN os_commands.result IS '鎵ц缁撴灉 JSON';
COMMENT ON COLUMN os_commands.error_code IS '閿欒鐮?;
COMMENT ON COLUMN os_commands.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_commands.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_call_events IS '閫氳瘽涓庡钩鍙颁簨浠舵祦锛堝彲鍥炴斁锛?;
COMMENT ON COLUMN os_call_events.id IS '浜嬩欢鍏ㄥ眬鑷 ID';
COMMENT ON COLUMN os_call_events.call_id IS '鍏宠仈閫氳瘽 ID';
COMMENT ON COLUMN os_call_events.seq IS '閫氳瘽鍐呬簨浠跺簭鍙?;
COMMENT ON COLUMN os_call_events.version IS '鍏宠仈瀹炰綋鐗堟湰';
COMMENT ON COLUMN os_call_events.command_id IS '瑙﹀彂浜嬩欢鐨勫懡浠?ID';
COMMENT ON COLUMN os_call_events.agent_id IS '鐩爣鍧愬腑 ID';
COMMENT ON COLUMN os_call_events.target_only IS '鏄惁浠呮姇閫掔粰鎸囧畾鍧愬腑';
COMMENT ON COLUMN os_call_events.type IS '浜嬩欢绫诲瀷';
COMMENT ON COLUMN os_call_events.payload IS '浜嬩欢杞借嵎 JSON';
COMMENT ON COLUMN os_call_events.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE os_cdr IS '浜ゆ崲渚ц瘽鍗?;
COMMENT ON COLUMN os_cdr.id IS '璇濆崟 ID';
COMMENT ON COLUMN os_cdr.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_cdr.direction IS '鍛煎彨鏂瑰悜';
COMMENT ON COLUMN os_cdr.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_cdr.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN os_cdr.caller IS '涓诲彨';
COMMENT ON COLUMN os_cdr.callee IS '琚彨';
COMMENT ON COLUMN os_cdr.session_type IS '濯掍粙绫诲瀷';
COMMENT ON COLUMN os_cdr.result IS '閫氳瘽缁撴灉';
COMMENT ON COLUMN os_cdr.started_at IS '寮€濮嬫椂闂?;
COMMENT ON COLUMN os_cdr.answered_at IS '鎺ラ€氭椂闂?;
COMMENT ON COLUMN os_cdr.ended_at IS '缁撴潫鏃堕棿';
COMMENT ON COLUMN os_cdr.duration_sec IS '閫氳瘽鏃堕暱绉?;
COMMENT ON COLUMN os_cdr.wait_sec IS '绛夊緟绉掓暟';
COMMENT ON COLUMN os_cdr.video_started_at IS '瑙嗛寮€濮嬫椂闂?;
COMMENT ON COLUMN os_cdr.video_upgrade_ok IS '鍗囪棰戞槸鍚︽垚鍔?;
COMMENT ON COLUMN os_cdr.screen_share_count IS '灞忓箷鍏变韩娆℃暟';
COMMENT ON COLUMN os_cdr.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE os_recordings IS '褰曢煶鏂囦欢鍏冩暟鎹?;
COMMENT ON COLUMN os_recordings.id IS '褰曢煶璁板綍 ID';
COMMENT ON COLUMN os_recordings.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_recordings.file_path IS '鏂囦欢璺緞';
COMMENT ON COLUMN os_recordings.media_type IS '褰曞埗绫诲瀷';
COMMENT ON COLUMN os_recordings.started_at IS '寮€濮嬫椂闂?;
COMMENT ON COLUMN os_recordings.ended_at IS '缁撴潫鏃堕棿';
COMMENT ON COLUMN os_recordings.retain_until IS '淇濈暀鎴鏃堕棿';
COMMENT ON COLUMN os_recordings.file_size IS '鏂囦欢澶у皬瀛楄妭';
COMMENT ON COLUMN os_recordings.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE os_queue_dispatch_cursor IS '鎸夐槦鍒楄褰曡疆璇㈡淳鍗曟父鏍囷紙鍏钩杞浆锛?;
COMMENT ON COLUMN os_queue_dispatch_cursor.queue_id IS '闃熷垪 ID';
COMMENT ON COLUMN os_queue_dispatch_cursor.last_agent_id IS '涓婁竴杞淳鍗曞埌鐨勫潗甯?ID';

COMMENT ON TABLE os_business_actions IS 'IVR 涓氬姟鍒ゆ柇鑺傜偣鎸傝捣鐨勫喅绛栬姹?;
COMMENT ON COLUMN os_business_actions.id IS '鍐崇瓥璇锋眰 ID';
COMMENT ON COLUMN os_business_actions.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN os_business_actions.node_id IS 'IVR 鑺傜偣 ID';
COMMENT ON COLUMN os_business_actions.action IS '涓氬姟鍔ㄤ綔绫诲瀷';
COMMENT ON COLUMN os_business_actions.outcomes IS '鍏佽鐨勭粨鏋滃垎鏀?JSON';
COMMENT ON COLUMN os_business_actions.deadline_at IS '绛夊緟涓氬姟鏂瑰搷搴旂殑鎴鏃堕棿';
COMMENT ON COLUMN os_business_actions.status IS '鐘舵€?pending completed expired';
COMMENT ON COLUMN os_business_actions.outcome IS '涓氬姟鏂硅繑鍥炵殑鍒嗘敮缁撴灉';
COMMENT ON COLUMN os_business_actions.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN os_business_actions.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE os_integrator_event_deliveries IS '浜嬩欢 callback 鎶曢€掔姸鎬?;
COMMENT ON COLUMN os_integrator_event_deliveries.event_id IS 'os_call_events.id';
COMMENT ON COLUMN os_integrator_event_deliveries.status IS 'pending success failed';
COMMENT ON COLUMN os_integrator_event_deliveries.attempts IS '宸插皾璇曟鏁?;
COMMENT ON COLUMN os_integrator_event_deliveries.next_retry_at IS '涓嬫閲嶈瘯鏃堕棿';
COMMENT ON COLUMN os_integrator_event_deliveries.last_error IS '鏈€杩戦敊璇?;
COMMENT ON COLUMN os_integrator_event_deliveries.updated_at IS '鏇存柊鏃堕棿';
ALTER TABLE os_queues ADD COLUMN IF NOT EXISTS post_call_ivr_flow_id UUID;
COMMENT ON COLUMN os_queues.post_call_ivr_flow_id IS '鍧愬腑杞弧鎰忓害 IVR 娴佺▼ ID';
ALTER TABLE os_calls ADD COLUMN IF NOT EXISTS event_seq BIGINT NOT NULL DEFAULT 0;
