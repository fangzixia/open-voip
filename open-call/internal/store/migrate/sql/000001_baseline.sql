-- open-call 涓氬姟搴擄紙oc_*锛夌┖搴?schema锛岀敱鍘嗗彶杩佺Щ鍚堝苟涓哄崟鏂囦欢銆?
-- PostgreSQL锛涜〃鍚嶄笌鍒楀悕鑻辨枃锛屼腑鏂囪 COMMENT銆?

CREATE TABLE oc_users (
  id UUID PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  display_name VARCHAR(128),
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  auth_version BIGINT NOT NULL DEFAULT 1,
  must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
  employee_no VARCHAR(64) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_users_role CHECK (role IN ('admin', 'supervisor', 'agent'))
);
CREATE UNIQUE INDEX idx_oc_users_username ON oc_users (username);
CREATE INDEX idx_oc_users_role ON oc_users (role);
CREATE UNIQUE INDEX idx_oc_users_employee_no ON oc_users (employee_no) WHERE employee_no <> '';

CREATE TABLE oc_agents (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES oc_users (id) ON DELETE CASCADE,
  extension VARCHAR(16) NOT NULL,
  video_capable BOOLEAN NOT NULL DEFAULT FALSE,
  terminal_type TEXT NOT NULL DEFAULT 'webrtc',
  sip_username TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_agents_terminal_type CHECK (terminal_type IN ('webrtc', 'sip'))
);
CREATE UNIQUE INDEX idx_oc_agents_user_id ON oc_agents (user_id);
CREATE UNIQUE INDEX idx_oc_agents_extension ON oc_agents (extension);
CREATE UNIQUE INDEX oc_agents_sip_username_unique ON oc_agents (sip_username) WHERE sip_username <> '';

CREATE TABLE oc_ivr_flows (
  id UUID PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  draft_json TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE TABLE oc_cdr (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  direction VARCHAR(16),
  queue_id UUID,
  agent_id UUID REFERENCES oc_agents (id) ON DELETE SET NULL,
  caller VARCHAR(64),
  callee VARCHAR(64),
  session_type VARCHAR(16) NOT NULL,
  result VARCHAR(32) NOT NULL,
  started_at TIMESTAMPTZ,
  answered_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  duration_sec BIGINT NOT NULL DEFAULT 0,
  wait_sec BIGINT NOT NULL DEFAULT 0,
  video_started_at TIMESTAMPTZ,
  video_upgrade_ok BOOLEAN NOT NULL DEFAULT FALSE,
  screen_share_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_oc_cdr_call_id ON oc_cdr (call_id);
CREATE INDEX idx_oc_cdr_queue_id ON oc_cdr (queue_id);
CREATE INDEX idx_oc_cdr_agent_id ON oc_cdr (agent_id);
CREATE INDEX idx_oc_cdr_result ON oc_cdr (result);
CREATE INDEX idx_cdr_started_queue ON oc_cdr (started_at, queue_id);
CREATE INDEX idx_oc_cdr_agent_started ON oc_cdr (agent_id, started_at DESC);
CREATE INDEX idx_oc_cdr_direction_started ON oc_cdr (direction, started_at DESC);
CREATE INDEX idx_oc_cdr_result_started ON oc_cdr (result, started_at DESC);

CREATE TABLE oc_guest_sessions (
  id UUID PRIMARY KEY,
  queue_id UUID NOT NULL,
  call_id UUID,
  allowed_media VARCHAR(16) NOT NULL DEFAULT 'audio',
  priority BIGINT NOT NULL DEFAULT 0,
  token VARCHAR(128) NOT NULL,
  expires_at TIMESTAMPTZ,
  consumed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_guest_media CHECK (allowed_media IN ('audio', 'video')),
  CONSTRAINT chk_oc_guest_priority CHECK (priority BETWEEN 0 AND 10)
);
CREATE INDEX idx_oc_guest_sessions_queue_id ON oc_guest_sessions (queue_id);
CREATE INDEX idx_oc_guest_sessions_call_id ON oc_guest_sessions (call_id);
CREATE UNIQUE INDEX idx_oc_guest_sessions_token ON oc_guest_sessions (token);
CREATE INDEX idx_oc_guest_sessions_expires_at ON oc_guest_sessions (expires_at);

CREATE TABLE oc_ivr_published_snapshots (
  id UUID PRIMARY KEY,
  flow_id UUID NOT NULL REFERENCES oc_ivr_flows (id) ON DELETE CASCADE,
  version BIGINT NOT NULL,
  payload_json TEXT NOT NULL,
  published_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_ivr_flow_version ON oc_ivr_published_snapshots (flow_id, version);

CREATE TABLE oc_recordings (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  file_path VARCHAR(512) NOT NULL,
  media_type VARCHAR(32) NOT NULL,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  retain_until TIMESTAMPTZ,
  file_size BIGINT,
  purge_error TEXT,
  purge_retry_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_recordings_call_id ON oc_recordings (call_id);
CREATE INDEX idx_oc_recordings_retain_until ON oc_recordings (retain_until);

CREATE TABLE oc_qa_marks (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES oc_cdr (call_id) ON DELETE CASCADE,
  offset_sec BIGINT NOT NULL,
  label VARCHAR(256),
  score BIGINT,
  created_by UUID REFERENCES oc_users (id) ON DELETE SET NULL,
  created_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_qa_offset CHECK (offset_sec >= 0),
  CONSTRAINT chk_oc_qa_score CHECK (score IS NULL OR score BETWEEN 0 AND 100)
);
CREATE INDEX idx_oc_qa_marks_call_id ON oc_qa_marks (call_id);

CREATE TABLE oc_call_wrap_ups (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL REFERENCES oc_cdr (call_id) ON DELETE CASCADE,
  agent_id UUID NOT NULL REFERENCES oc_agents (id) ON DELETE RESTRICT,
  notes TEXT NOT NULL,
  disposition_code VARCHAR(64),
  tags_json TEXT NOT NULL DEFAULT '[]',
  completed_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_oc_call_wrap_ups_call_id ON oc_call_wrap_ups (call_id);
CREATE INDEX idx_oc_call_wrap_ups_agent_id ON oc_call_wrap_ups (agent_id);

CREATE TABLE oc_webhook_subscriptions (
  id UUID PRIMARY KEY,
  url VARCHAR(512) NOT NULL,
  event_types TEXT NOT NULL,
  secret VARCHAR(128),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ
);

CREATE TABLE oc_webhook_deliveries (
  id UUID PRIMARY KEY,
  subscription_id UUID NOT NULL REFERENCES oc_webhook_subscriptions (id) ON DELETE CASCADE,
  event_id UUID NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  payload TEXT NOT NULL,
  status VARCHAR(16) NOT NULL,
  attempts BIGINT NOT NULL DEFAULT 0,
  last_error TEXT,
  next_attempt_at TIMESTAMPTZ NOT NULL,
  locked_at TIMESTAMPTZ,
  dead_letter_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_webhook_status CHECK (status IN ('pending', 'processing', 'success', 'dead_letter'))
);
CREATE INDEX idx_oc_webhook_deliveries_subscription_id ON oc_webhook_deliveries (subscription_id);
CREATE INDEX idx_oc_webhook_deliveries_event_type ON oc_webhook_deliveries (event_type);
CREATE INDEX idx_oc_webhook_deliveries_status ON oc_webhook_deliveries (status);
CREATE UNIQUE INDEX idx_oc_webhook_delivery_event_subscription ON oc_webhook_deliveries (subscription_id, event_id);
CREATE INDEX idx_oc_webhook_delivery_due ON oc_webhook_deliveries (status, next_attempt_at);

CREATE TABLE oc_audit_logs (
  id UUID PRIMARY KEY,
  user_id UUID,
  action VARCHAR(64) NOT NULL,
  resource VARCHAR(256),
  detail_json TEXT,
  request_id VARCHAR(128),
  remote_ip VARCHAR(64),
  outcome VARCHAR(16) NOT NULL DEFAULT 'success',
  created_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_audit_logs_user_id ON oc_audit_logs (user_id);
CREATE INDEX idx_oc_audit_logs_action ON oc_audit_logs (action);
CREATE INDEX idx_oc_audit_logs_created_at ON oc_audit_logs (created_at);
CREATE INDEX idx_oc_audit_logs_resource ON oc_audit_logs (resource);
CREATE INDEX idx_oc_audit_logs_outcome ON oc_audit_logs (outcome);

CREATE TABLE oc_jwt_revocations (
  jti VARCHAR(64) PRIMARY KEY,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_jwt_revocations_expires_at ON oc_jwt_revocations (expires_at);

CREATE TABLE oc_auth_sessions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES oc_users (id) ON DELETE CASCADE,
  refresh_jti VARCHAR(64) NOT NULL,
  user_agent VARCHAR(256),
  remote_ip VARCHAR(64),
  provider VARCHAR(16) NOT NULL DEFAULT 'local',
  provider_refresh_token TEXT,
  provider_subject VARCHAR(255),
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_auth_sessions_user_id ON oc_auth_sessions (user_id);
CREATE INDEX idx_oc_auth_sessions_expires_at ON oc_auth_sessions (expires_at);
CREATE UNIQUE INDEX idx_oc_auth_sessions_refresh_jti ON oc_auth_sessions (refresh_jti);

CREATE TABLE oc_roles (
  id VARCHAR(64) PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  built_in BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oc_permissions (
  code VARCHAR(96) PRIMARY KEY,
  description VARCHAR(128) NOT NULL
);

CREATE TABLE oc_role_permissions (
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles (id) ON DELETE CASCADE,
  permission_code VARCHAR(96) NOT NULL REFERENCES oc_permissions (code) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_code)
);

CREATE TABLE oc_user_roles (
  user_id UUID NOT NULL REFERENCES oc_users (id) ON DELETE CASCADE,
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles (id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, role_id)
);
CREATE INDEX idx_oc_user_roles_role ON oc_user_roles (role_id);

CREATE TABLE oc_oidc_identities (
  issuer VARCHAR(512) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  user_id UUID NOT NULL REFERENCES oc_users (id) ON DELETE CASCADE,
  PRIMARY KEY (issuer, subject),
  UNIQUE (user_id, issuer)
);

CREATE TABLE oc_oidc_group_roles (
  group_name VARCHAR(255) NOT NULL,
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles (id) ON DELETE CASCADE,
  PRIMARY KEY (group_name, role_id)
);

CREATE TABLE oc_oidc_flows (
  state_hash CHAR(64) PRIMARY KEY,
  nonce VARCHAR(128) NOT NULL,
  verifier VARCHAR(128) NOT NULL,
  return_path VARCHAR(16) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE oc_oidc_tickets (
  ticket_hash CHAR(64) PRIMARY KEY,
  user_id UUID NOT NULL REFERENCES oc_users (id) ON DELETE CASCADE,
  provider_refresh_token TEXT,
  provider_subject VARCHAR(255) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE oc_switch_event_cursor (
  id INTEGER PRIMARY KEY CHECK (id = 1),
  last_event_id BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oc_agent_state_projection (
  id BIGINT PRIMARY KEY,
  agent_id UUID NOT NULL,
  from_state TEXT NOT NULL DEFAULT '',
  to_state TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_oc_agent_state_projection_time ON oc_agent_state_projection (agent_id, created_at, id);

CREATE TABLE oc_switch_event_inbox (
  event_id BIGINT PRIMARY KEY,
  received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oc_switch_event_outbox (
  event_id BIGINT PRIMARY KEY REFERENCES oc_switch_event_inbox (event_id) ON DELETE CASCADE,
  payload TEXT NOT NULL,
  delivered_at TIMESTAMPTZ
);
CREATE INDEX idx_oc_switch_event_outbox_pending ON oc_switch_event_outbox (event_id) WHERE delivered_at IS NULL;

-- open-call 涓枃娉ㄩ噴
COMMENT ON TABLE oc_schema_migrations IS '涓氬姟搴撳凡鎵ц鐨勬暟鎹簱杩佺Щ鐗堟湰';
COMMENT ON COLUMN oc_schema_migrations.version IS '杩佺Щ鐗堟湰鍙?;
COMMENT ON COLUMN oc_schema_migrations.name IS '杩佺Щ鏂囦欢鍚嶇О';
COMMENT ON COLUMN oc_schema_migrations.applied_at IS '杩佺Щ鎵ц鏃堕棿';

COMMENT ON TABLE oc_users IS '鍙櫥褰曠郴缁熺殑璐﹀彿锛堢鐞嗗憳銆佺彮闀挎垨鍧愬腑锛?;
COMMENT ON COLUMN oc_users.id IS '鐢ㄦ埛涓婚敭 UUID';
COMMENT ON COLUMN oc_users.username IS '鐧诲綍鍚?;
COMMENT ON COLUMN oc_users.password_hash IS 'argon2id 瀵嗙爜鍝堝笇';
COMMENT ON COLUMN oc_users.role IS '瑙掕壊 admin supervisor agent';
COMMENT ON COLUMN oc_users.display_name IS '鐢ㄦ埛鍚?;
COMMENT ON COLUMN oc_users.disabled IS '鏄惁绂佺敤璐﹀彿';
COMMENT ON COLUMN oc_users.auth_version IS '璁よ瘉鐗堟湰锛屽瘑鐮併€佽鑹叉垨绂佺敤鐘舵€佸彉鍖栨椂閫掑';
COMMENT ON COLUMN oc_users.must_change_password IS '鏄惁瑕佹眰涓嬫鐧诲綍鍚庝慨鏀逛复鏃跺瘑鐮?;
COMMENT ON COLUMN oc_users.employee_no IS '宸ュ彿';
COMMENT ON COLUMN oc_users.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN oc_users.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE oc_agents IS '鍛煎彨涓績鍧愬腑涓氬姟灞炴€э紝涓?User 涓€瀵逛竴鎵╁睍';
COMMENT ON COLUMN oc_agents.id IS '鍧愬腑涓婚敭 UUID';
COMMENT ON COLUMN oc_agents.user_id IS '鍏宠仈鐢ㄦ埛 ID';
COMMENT ON COLUMN oc_agents.extension IS '鍒嗘満鍙?;
COMMENT ON COLUMN oc_agents.video_capable IS '鏄惁鏀寔瑙嗛';
COMMENT ON COLUMN oc_agents.terminal_type IS '鍧愬腑缁堢绫诲瀷锛歸ebrtc 鎴?sip';
COMMENT ON COLUMN oc_agents.sip_username IS 'Switch 閰嶇疆涓殑璁惧璐﹀彿锛屼笉鍚嚟璇?;
COMMENT ON COLUMN oc_agents.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN oc_agents.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE oc_ivr_flows IS 'IVR 娴佺▼瀹氫箟锛堣崏绋夸笌鍏冩暟鎹級';
COMMENT ON COLUMN oc_ivr_flows.id IS 'IVR 娴佺▼ ID';
COMMENT ON COLUMN oc_ivr_flows.name IS '娴佺▼鍚嶇О';
COMMENT ON COLUMN oc_ivr_flows.draft_json IS '鑽夌 JSON 閰嶇疆';
COMMENT ON COLUMN oc_ivr_flows.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN oc_ivr_flows.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE oc_cdr IS '璇濆崟璁板綍';
COMMENT ON COLUMN oc_cdr.id IS '璇濆崟 ID';
COMMENT ON COLUMN oc_cdr.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN oc_cdr.direction IS '鍛煎彨鏂瑰悜';
COMMENT ON COLUMN oc_cdr.queue_id IS '闃熷垪 ID锛圫witch 闃熷垪锛屾棤鏈湴 FK锛?;
COMMENT ON COLUMN oc_cdr.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN oc_cdr.caller IS '涓诲彨';
COMMENT ON COLUMN oc_cdr.callee IS '琚彨';
COMMENT ON COLUMN oc_cdr.session_type IS '濯掍粙绫诲瀷';
COMMENT ON COLUMN oc_cdr.result IS '閫氳瘽缁撴灉';
COMMENT ON COLUMN oc_cdr.started_at IS '寮€濮嬫椂闂?;
COMMENT ON COLUMN oc_cdr.answered_at IS '鎺ラ€氭椂闂?;
COMMENT ON COLUMN oc_cdr.ended_at IS '缁撴潫鏃堕棿';
COMMENT ON COLUMN oc_cdr.duration_sec IS '閫氳瘽鏃堕暱绉?;
COMMENT ON COLUMN oc_cdr.wait_sec IS '绛夊緟绉掓暟';
COMMENT ON COLUMN oc_cdr.video_started_at IS '瑙嗛寮€濮嬫椂闂?;
COMMENT ON COLUMN oc_cdr.video_upgrade_ok IS '鍗囪棰戞槸鍚︽垚鍔?;
COMMENT ON COLUMN oc_cdr.screen_share_count IS '灞忓箷鍏变韩娆℃暟';
COMMENT ON COLUMN oc_cdr.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_guest_sessions IS '璁垮鍏ヤ細 token 涓庝細璇濆厓鏁版嵁';
COMMENT ON COLUMN oc_guest_sessions.id IS '璁垮浼氳瘽 ID';
COMMENT ON COLUMN oc_guest_sessions.queue_id IS '鐩爣闃熷垪 ID锛圫witch 闃熷垪锛屾棤鏈湴 FK锛?;
COMMENT ON COLUMN oc_guest_sessions.call_id IS '鍏宠仈閫氳瘽 ID';
COMMENT ON COLUMN oc_guest_sessions.allowed_media IS '鍏佽濯掍粙';
COMMENT ON COLUMN oc_guest_sessions.priority IS '鐢卞彈淇′换绛惧彂鏂硅缃殑闃熷垪浼樺厛绾?;
COMMENT ON COLUMN oc_guest_sessions.token IS '鍏ヤ細 token';
COMMENT ON COLUMN oc_guest_sessions.expires_at IS '杩囨湡鏃堕棿';
COMMENT ON COLUMN oc_guest_sessions.consumed_at IS '璁垮浠ょ墝棣栨鎴愬姛鍏ラ槦鏃堕棿';
COMMENT ON COLUMN oc_guest_sessions.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_ivr_published_snapshots IS '宸插彂甯?IVR 蹇収锛屽懠鍏ュ彧璇绘渶鏂?version';
COMMENT ON COLUMN oc_ivr_published_snapshots.id IS '蹇収 ID';
COMMENT ON COLUMN oc_ivr_published_snapshots.flow_id IS '娴佺▼ ID';
COMMENT ON COLUMN oc_ivr_published_snapshots.version IS '鍙戝竷鐗堟湰';
COMMENT ON COLUMN oc_ivr_published_snapshots.payload_json IS '鍙戝竷鍐呭 JSON';
COMMENT ON COLUMN oc_ivr_published_snapshots.published_at IS '鍙戝竷鏃堕棿';

COMMENT ON TABLE oc_recordings IS '褰曢煶/褰曞儚鏂囦欢鍏冩暟鎹?;
COMMENT ON COLUMN oc_recordings.id IS '褰曢煶璁板綍 ID';
COMMENT ON COLUMN oc_recordings.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN oc_recordings.file_path IS '鏂囦欢璺緞';
COMMENT ON COLUMN oc_recordings.media_type IS '褰曞埗绫诲瀷';
COMMENT ON COLUMN oc_recordings.started_at IS '寮€濮嬫椂闂?;
COMMENT ON COLUMN oc_recordings.ended_at IS '缁撴潫鏃堕棿';
COMMENT ON COLUMN oc_recordings.retain_until IS '淇濈暀鎴鏃堕棿';
COMMENT ON COLUMN oc_recordings.file_size IS '鏂囦欢澶у皬瀛楄妭';
COMMENT ON COLUMN oc_recordings.purge_error IS '鏈€杩戜竴娆℃枃浠舵竻鐞嗗け璐ュ師鍥?;
COMMENT ON COLUMN oc_recordings.purge_retry_at IS '褰曢煶鏂囦欢涓嬫娓呯悊閲嶈瘯鏃堕棿';
COMMENT ON COLUMN oc_recordings.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_qa_marks IS '璐ㄦ鏃堕棿鎴虫爣璁?;
COMMENT ON COLUMN oc_qa_marks.id IS '璐ㄦ鏍囪 ID';
COMMENT ON COLUMN oc_qa_marks.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN oc_qa_marks.offset_sec IS '鐩稿绉掓暟';
COMMENT ON COLUMN oc_qa_marks.label IS '鏍囪璇存槑';
COMMENT ON COLUMN oc_qa_marks.score IS '璐ㄦ璇勫垎锛岃寖鍥?0 鑷?100';
COMMENT ON COLUMN oc_qa_marks.created_by IS '鍒涘缓浜?;
COMMENT ON COLUMN oc_qa_marks.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_call_wrap_ups IS '閫氳瘽灏忕粨鏂囨湰';
COMMENT ON COLUMN oc_call_wrap_ups.id IS '灏忕粨 ID';
COMMENT ON COLUMN oc_call_wrap_ups.call_id IS '閫氳瘽 ID';
COMMENT ON COLUMN oc_call_wrap_ups.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN oc_call_wrap_ups.notes IS '灏忕粨鏂囨湰';
COMMENT ON COLUMN oc_call_wrap_ups.disposition_code IS '閫氳瘽澶勭疆鐮?;
COMMENT ON COLUMN oc_call_wrap_ups.tags_json IS '閫氳瘽鏍囩 JSON 鏁扮粍';
COMMENT ON COLUMN oc_call_wrap_ups.completed_at IS '浜嬪悗澶勭悊瀹屾垚鏃堕棿';
COMMENT ON COLUMN oc_call_wrap_ups.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_webhook_subscriptions IS 'Webhook 浜嬩欢璁㈤槄閰嶇疆';
COMMENT ON COLUMN oc_webhook_subscriptions.id IS 'Webhook 璁㈤槄 ID';
COMMENT ON COLUMN oc_webhook_subscriptions.url IS '鍥炶皟 URL';
COMMENT ON COLUMN oc_webhook_subscriptions.event_types IS '浜嬩欢绫诲瀷鍒楄〃 JSON';
COMMENT ON COLUMN oc_webhook_subscriptions.secret IS 'HMAC 瀵嗛挜';
COMMENT ON COLUMN oc_webhook_subscriptions.enabled IS '鏄惁鍚敤';
COMMENT ON COLUMN oc_webhook_subscriptions.created_at IS '鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_webhook_deliveries IS 'Webhook 鍗曟鎶曢€掕褰?;
COMMENT ON COLUMN oc_webhook_deliveries.id IS '鎶曢€掕褰?ID';
COMMENT ON COLUMN oc_webhook_deliveries.subscription_id IS '璁㈤槄 ID';
COMMENT ON COLUMN oc_webhook_deliveries.event_id IS '涓氬姟浜嬩欢鍞竴鏍囪瘑';
COMMENT ON COLUMN oc_webhook_deliveries.event_type IS '浜嬩欢绫诲瀷';
COMMENT ON COLUMN oc_webhook_deliveries.payload IS '杞借嵎 JSON';
COMMENT ON COLUMN oc_webhook_deliveries.status IS '鎶曢€掔姸鎬?;
COMMENT ON COLUMN oc_webhook_deliveries.attempts IS '灏濊瘯娆℃暟';
COMMENT ON COLUMN oc_webhook_deliveries.last_error IS '鏈€鍚庨敊璇?;
COMMENT ON COLUMN oc_webhook_deliveries.next_attempt_at IS '涓嬫璁″垝鎶曢€掓椂闂?;
COMMENT ON COLUMN oc_webhook_deliveries.locked_at IS '鍚庡彴浠诲姟绉熺害鑾峰彇鏃堕棿';
COMMENT ON COLUMN oc_webhook_deliveries.dead_letter_at IS '杩涘叆姝讳俊闃熷垪鏃堕棿';
COMMENT ON COLUMN oc_webhook_deliveries.created_at IS '鍒涘缓鏃堕棿';
COMMENT ON COLUMN oc_webhook_deliveries.updated_at IS '鏇存柊鏃堕棿';

COMMENT ON TABLE oc_audit_logs IS '绠＄悊鎿嶄綔涓庢晱鎰熻闂璁?;
COMMENT ON COLUMN oc_audit_logs.id IS '瀹¤鏃ュ織 ID';
COMMENT ON COLUMN oc_audit_logs.user_id IS '鎿嶄綔浜虹敤鎴?ID';
COMMENT ON COLUMN oc_audit_logs.action IS '鍔ㄤ綔绫诲瀷';
COMMENT ON COLUMN oc_audit_logs.resource IS '璧勬簮鎻忚堪';
COMMENT ON COLUMN oc_audit_logs.detail_json IS '璇︽儏 JSON';
COMMENT ON COLUMN oc_audit_logs.request_id IS '鍏宠仈 HTTP 璇锋眰 ID';
COMMENT ON COLUMN oc_audit_logs.remote_ip IS '鎿嶄綔鏉ユ簮 IP';
COMMENT ON COLUMN oc_audit_logs.outcome IS '鎿嶄綔缁撴灉 success 鎴?failure';
COMMENT ON COLUMN oc_audit_logs.created_at IS '鍙戠敓鏃堕棿';

COMMENT ON TABLE oc_jwt_revocations IS '宸叉挙閿€ JWT 鐨?jti 榛戝悕鍗?;
COMMENT ON COLUMN oc_jwt_revocations.jti IS '宸叉挙閿€浠ょ墝鐨勫敮涓€鏍囪瘑';
COMMENT ON COLUMN oc_jwt_revocations.expires_at IS '浠ょ墝杩囨湡鏃堕棿';
COMMENT ON COLUMN oc_jwt_revocations.revoked_at IS '鎾ら攢鏃堕棿';

COMMENT ON TABLE oc_auth_sessions IS '鐢ㄦ埛鐧诲綍浼氳瘽锛屽彲鎸夎澶囧崟鐙挙閿€';
COMMENT ON COLUMN oc_auth_sessions.id IS '鐧诲綍浼氳瘽涓婚敭 UUID';
COMMENT ON COLUMN oc_auth_sessions.user_id IS '鍏宠仈鐢ㄦ埛 ID';
COMMENT ON COLUMN oc_auth_sessions.refresh_jti IS '褰撳墠鍒锋柊浠ょ墝鍞竴鏍囪瘑';
COMMENT ON COLUMN oc_auth_sessions.user_agent IS '鐧诲綍瀹㈡埛绔?User-Agent';
COMMENT ON COLUMN oc_auth_sessions.remote_ip IS '鐧诲綍鏉ユ簮 IP';
COMMENT ON COLUMN oc_auth_sessions.provider IS '鐧诲綍鏉ユ簮锛宭ocal 琛ㄧず鏈湴璁よ瘉';
COMMENT ON COLUMN oc_auth_sessions.provider_refresh_token IS '澶栭儴韬唤鎻愪緵鏂圭殑鍒锋柊浠ょ墝';
COMMENT ON COLUMN oc_auth_sessions.provider_subject IS '澶栭儴韬唤鎻愪緵鏂圭殑鐢ㄦ埛鏍囪瘑';
COMMENT ON COLUMN oc_auth_sessions.created_at IS '浼氳瘽鍒涘缓鏃堕棿';
COMMENT ON COLUMN oc_auth_sessions.expires_at IS '浼氳瘽杩囨湡鏃堕棿';
COMMENT ON COLUMN oc_auth_sessions.last_seen_at IS '鏈€鍚庡埛鏂版椂闂?;
COMMENT ON COLUMN oc_auth_sessions.revoked_at IS '浼氳瘽鎾ら攢鏃堕棿';

COMMENT ON TABLE oc_roles IS '瑙掕壊瀹氫箟锛屽寘鎷郴缁熷唴缃鑹蹭笌鑷畾涔夎鑹?;
COMMENT ON COLUMN oc_roles.id IS '瑙掕壊鍞竴鏍囪瘑';
COMMENT ON COLUMN oc_roles.name IS '瑙掕壊鍚嶇О';
COMMENT ON COLUMN oc_roles.built_in IS '鏄惁涓虹郴缁熷唴缃鑹?;
COMMENT ON COLUMN oc_roles.created_at IS '瑙掕壊鍒涘缓鏃堕棿';

COMMENT ON TABLE oc_permissions IS '绯荤粺鏉冮檺瀹氫箟';
COMMENT ON COLUMN oc_permissions.code IS '鏉冮檺鍞竴缂栫爜';
COMMENT ON COLUMN oc_permissions.description IS '鏉冮檺涓枃璇存槑';

COMMENT ON TABLE oc_role_permissions IS '瑙掕壊涓庢潈闄愮殑鍏宠仈鍏崇郴';
COMMENT ON COLUMN oc_role_permissions.role_id IS '瑙掕壊鏍囪瘑';
COMMENT ON COLUMN oc_role_permissions.permission_code IS '鏉冮檺缂栫爜';

COMMENT ON TABLE oc_user_roles IS '鐢ㄦ埛涓庤鑹茬殑鍏宠仈鍏崇郴';
COMMENT ON COLUMN oc_user_roles.user_id IS '鐢ㄦ埛 ID';
COMMENT ON COLUMN oc_user_roles.role_id IS '瑙掕壊鏍囪瘑';

COMMENT ON TABLE oc_oidc_identities IS '澶栭儴 OIDC 韬唤涓庢湰鍦扮敤鎴风殑缁戝畾鍏崇郴';
COMMENT ON COLUMN oc_oidc_identities.issuer IS '韬唤鎻愪緵鏂圭鍙戣€呭湴鍧€';
COMMENT ON COLUMN oc_oidc_identities.subject IS '韬唤鎻愪緵鏂圭敤鎴锋爣璇?;
COMMENT ON COLUMN oc_oidc_identities.user_id IS '缁戝畾鐨勬湰鍦扮敤鎴?ID';

COMMENT ON TABLE oc_oidc_group_roles IS '澶栭儴韬唤缁勪笌鏈湴瑙掕壊鐨勬槧灏勫叧绯?;
COMMENT ON COLUMN oc_oidc_group_roles.group_name IS '澶栭儴韬唤缁勫悕绉?;
COMMENT ON COLUMN oc_oidc_group_roles.role_id IS '鏄犲皠鐨勬湰鍦拌鑹叉爣璇?;

COMMENT ON TABLE oc_oidc_flows IS '寰呭畬鎴愮殑 OIDC 鐧诲綍娴佺▼鐘舵€?;
COMMENT ON COLUMN oc_oidc_flows.state_hash IS '鐧诲綍鐘舵€佸弬鏁扮殑鍝堝笇鍊?;
COMMENT ON COLUMN oc_oidc_flows.nonce IS '鐢ㄤ簬鏍￠獙韬唤浠ょ墝鐨勯殢鏈哄€?;
COMMENT ON COLUMN oc_oidc_flows.verifier IS '鐢ㄤ簬 PKCE 鏍￠獙鐨勯獙璇佸€?;
COMMENT ON COLUMN oc_oidc_flows.return_path IS '鐧诲綍瀹屾垚鍚庣殑杩斿洖璺緞';
COMMENT ON COLUMN oc_oidc_flows.expires_at IS '鐧诲綍娴佺▼杩囨湡鏃堕棿';

COMMENT ON TABLE oc_oidc_tickets IS 'OIDC 鐧诲綍瀹屾垚鍚庣殑鐭湡鍏戞崲鍑嵁';
COMMENT ON COLUMN oc_oidc_tickets.ticket_hash IS '鍏戞崲鍑嵁鐨勫搱甯屽€?;
COMMENT ON COLUMN oc_oidc_tickets.user_id IS '瀵瑰簲鐨勬湰鍦扮敤鎴?ID';
COMMENT ON COLUMN oc_oidc_tickets.provider_refresh_token IS '寰呬氦浠樼殑韬唤鎻愪緵鏂瑰埛鏂颁护鐗?;
COMMENT ON COLUMN oc_oidc_tickets.provider_subject IS '韬唤鎻愪緵鏂圭敤鎴锋爣璇?;
COMMENT ON COLUMN oc_oidc_tickets.expires_at IS '鍏戞崲鍑嵁杩囨湡鏃堕棿';

COMMENT ON TABLE oc_switch_event_cursor IS '娑堣垂 Switch 浜嬩欢娴佺殑娓告爣锛堝崟琛岋級';
COMMENT ON COLUMN oc_switch_event_cursor.id IS '鍥哄畾涓?1 鐨勪富閿?;
COMMENT ON COLUMN oc_switch_event_cursor.last_event_id IS '宸茶瀵熷埌鐨勬渶澶т簨浠?ID锛堝璐︽按鍗帮級锛涗贡搴忓幓閲嶄緷璧?inbox锛屼笉浠ユ按鍗版嫆缁濇櫄鍒颁簨浠?;
COMMENT ON COLUMN oc_switch_event_cursor.updated_at IS '娓告爣鏇存柊鏃堕棿';

COMMENT ON TABLE oc_agent_state_projection IS '浠?Switch 浜嬩欢鎶曞奖鐨勫潗甯姸鎬佸彉鏇村巻鍙诧紙鍙锛?;
COMMENT ON COLUMN oc_agent_state_projection.id IS 'Switch 渚х姸鎬佹棩蹇椾簨浠?ID';
COMMENT ON COLUMN oc_agent_state_projection.agent_id IS '鍧愬腑 ID';
COMMENT ON COLUMN oc_agent_state_projection.from_state IS '鍘熺姸鎬?;
COMMENT ON COLUMN oc_agent_state_projection.to_state IS '鏂扮姸鎬?;
COMMENT ON COLUMN oc_agent_state_projection.reason IS '鍙樻洿鍘熷洜';
COMMENT ON COLUMN oc_agent_state_projection.created_at IS '浜嬩欢鍙戠敓鏃堕棿';

COMMENT ON TABLE oc_switch_event_inbox IS '宸叉帴鏀剁殑 Switch 浜嬩欢鍘婚噸鏀朵欢绠?;
COMMENT ON COLUMN oc_switch_event_inbox.event_id IS 'Switch 浜嬩欢鍏ㄥ眬 ID';
COMMENT ON COLUMN oc_switch_event_inbox.received_at IS '涓氬姟渚ф帴鏀舵椂闂?;

COMMENT ON TABLE oc_switch_event_outbox IS '寰呭悜 WebSocket/Webhook 鎶曢€掔殑浜嬩欢鍑虹珯闃熷垪';
COMMENT ON COLUMN oc_switch_event_outbox.event_id IS '鍏宠仈鏀朵欢绠变簨浠?ID';
COMMENT ON COLUMN oc_switch_event_outbox.payload IS '搴忓垪鍖栧悗鐨勪簨浠?JSON';
COMMENT ON COLUMN oc_switch_event_outbox.delivered_at IS '鎶曢€掑畬鎴愭椂闂达紝绌鸿〃绀哄緟鎶曢€?;

-- 鍐呯疆绉嶅瓙锛圧BAC + Switch 浜嬩欢娓告爣锛?

INSERT INTO oc_roles (id, name, built_in) VALUES
  ('admin', '绠＄悊鍛?, TRUE),
  ('supervisor', '鐝暱', TRUE),
  ('agent', '鍧愬腑', TRUE);

INSERT INTO oc_permissions (code, description) VALUES
  ('users.read', '鏌ョ湅鐢ㄦ埛'),
  ('users.create', '鍒涘缓鐢ㄦ埛'),
  ('users.update', '淇敼鐢ㄦ埛'),
  ('users.delete', '鍒犻櫎鐢ㄦ埛'),
  ('users.credentials', '绠＄悊瀵嗙爜'),
  ('users.sessions', '鎾ら攢浼氳瘽'),
  ('roles.read', '鏌ョ湅瑙掕壊'),
  ('roles.write', '绠＄悊瑙掕壊'),
  ('identity.read', '鏌ョ湅韬唤鏄犲皠'),
  ('identity.write', '绠＄悊韬唤鏄犲皠'),
  ('queues.read', '鏌ョ湅闃熷垪'),
  ('queues.write', '绠＄悊闃熷垪'),
  ('skills.read', '鏌ョ湅鎶€鑳?),
  ('skills.write', '绠＄悊鎶€鑳?),
  ('agents.read', '鏌ョ湅鍧愬腑'),
  ('agents.self', '鎿嶄綔鏈汉鍧愬腑'),
  ('agents.force_checkout', '寮哄埗绛惧嚭鍧愬腑'),
  ('calls.read', '鏌ョ湅閫氳瘽'),
  ('calls.operate', '鎿嶄綔閫氳瘽'),
  ('calls.listen', '鐩戝惉閫氳瘽'),
  ('calls.wrap_up', '濉啓閫氳瘽灏忕粨'),
  ('cdr.read', '鏌ョ湅璇濆崟'),
  ('cdr.export', '瀵煎嚭璇濆崟'),
  ('recordings.read', '鏌ョ湅褰曢煶'),
  ('recordings.download', '涓嬭浇褰曢煶'),
  ('recordings.qa', '绠＄悊璐ㄦ鏍囪'),
  ('recordings.purge', '娓呯悊褰曢煶'),
  ('reports.read', '鏌ョ湅鎶ヨ〃'),
  ('ivr.read', '鏌ョ湅 IVR'),
  ('ivr.write', '绠＄悊 IVR'),
  ('webhooks.read', '鏌ョ湅 Webhook'),
  ('webhooks.write', '绠＄悊 Webhook'),
  ('audit.read', '鏌ョ湅瀹¤'),
  ('dids.read', '鏌ョ湅鍛煎叆鍙风爜'),
  ('dids.write', '绠＄悊鍛煎叆鍙风爜'),
  ('config.read', '瀵煎嚭閰嶇疆'),
  ('config.write', '瀵煎叆閰嶇疆'),
  ('guest.issue', '绛惧彂璁垮浼氳瘽'),
  ('status.read', '鏌ョ湅杩愯鐘舵€?);

INSERT INTO oc_role_permissions (role_id, permission_code)
  SELECT 'admin', code FROM oc_permissions;

INSERT INTO oc_role_permissions (role_id, permission_code)
  SELECT 'supervisor', code FROM oc_permissions WHERE code IN (
    'queues.read', 'skills.read', 'agents.read', 'agents.self', 'agents.force_checkout',
    'calls.read', 'calls.operate', 'calls.listen', 'calls.wrap_up', 'cdr.read',
    'recordings.read', 'recordings.download', 'recordings.qa', 'reports.read', 'ivr.read',
    'dids.read', 'guest.issue', 'status.read'
  );

INSERT INTO oc_role_permissions (role_id, permission_code)
  SELECT 'agent', code FROM oc_permissions WHERE code IN (
    'queues.read', 'agents.read', 'agents.self', 'calls.read', 'calls.operate',
    'calls.wrap_up', 'guest.issue'
  );

INSERT INTO oc_switch_event_cursor (id, last_event_id) VALUES (1, 0);
CREATE TABLE IF NOT EXISTS oc_call_csat (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL UNIQUE,
  agent_id UUID,
  flow_id UUID,
  score SMALLINT NOT NULL CHECK (score >= 1 AND score <= 5),
  scored_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oc_call_csat_scored_at ON oc_call_csat (scored_at DESC);
COMMENT ON TABLE oc_call_csat IS '閫氳瘽婊℃剰搴﹁瘎鍒嗘姇褰?;
ALTER TABLE oc_cdr ADD COLUMN IF NOT EXISTS switch_event_version BIGINT NOT NULL DEFAULT 0;

ALTER TABLE oc_cdr DROP CONSTRAINT IF EXISTS oc_cdr_agent_id_fkey;

CREATE TABLE IF NOT EXISTS oc_agent_sync_tasks (
  id UUID PRIMARY KEY,
  agent_id UUID NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_agent_sync_tasks_agent ON oc_agent_sync_tasks (agent_id);
CREATE INDEX IF NOT EXISTS idx_oc_agent_sync_tasks_retry ON oc_agent_sync_tasks (next_retry_at);

CREATE TABLE IF NOT EXISTS oc_switch_event_projection_failures (
  event_id BIGINT PRIMARY KEY,
  failures INTEGER NOT NULL DEFAULT 0,
  dead_letter_at TIMESTAMPTZ,
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE oc_ws_event_seq (
  id SMALLINT PRIMARY KEY CHECK (id = 1),
  last_seq BIGINT NOT NULL DEFAULT 0,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
INSERT INTO oc_ws_event_seq (id, last_seq) VALUES (1, 0);

CREATE TABLE oc_ws_event_buffer (
  seq BIGINT PRIMARY KEY,
  call_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  payload JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_oc_ws_event_buffer_created ON oc_ws_event_buffer (created_at);
