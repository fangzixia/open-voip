-- open-call 业务库（oc_*）空库 schema，由历史迁移合并为单文件。
-- PostgreSQL；表名与列名英文，中文见 COMMENT。

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

CREATE TABLE oc_skills (
  id UUID PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_oc_skills_name ON oc_skills (name);

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

CREATE TABLE oc_agent_skills (
  agent_id UUID NOT NULL REFERENCES oc_agents (id) ON DELETE CASCADE,
  skill_id UUID NOT NULL REFERENCES oc_skills (id) ON DELETE CASCADE,
  PRIMARY KEY (agent_id, skill_id)
);
CREATE INDEX idx_oc_agent_skills_skill_id ON oc_agent_skills (skill_id);

CREATE TABLE oc_ivr_flows (
  id UUID PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  draft_json TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE TABLE oc_queues (
  id UUID PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  video_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  max_wait_sec BIGINT NOT NULL DEFAULT 300,
  dispatch_strategy VARCHAR(32) NOT NULL DEFAULT 'longest_idle',
  recording_policy VARCHAR(32) NOT NULL DEFAULT 'off',
  overflow_action VARCHAR(32),
  overflow_queue_id UUID REFERENCES oc_queues (id) ON DELETE SET NULL,
  ivr_flow_id UUID REFERENCES oc_ivr_flows (id) ON DELETE SET NULL,
  wait_prompt VARCHAR(256),
  announce_recording BOOLEAN NOT NULL DEFAULT FALSE,
  priority_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  business_hours_json TEXT,
  after_hours_action VARCHAR(32),
  force_hangup_on_checkout BOOLEAN NOT NULL DEFAULT TRUE,
  listen_announce BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ,
  CONSTRAINT chk_oc_queues_strategy CHECK (dispatch_strategy IN ('longest_idle', 'round_robin')),
  CONSTRAINT chk_oc_queues_recording_policy CHECK (recording_policy IN ('off', 'audio', 'video_composite'))
);
CREATE UNIQUE INDEX idx_oc_queues_name ON oc_queues (name);
CREATE INDEX idx_oc_queues_ivr_flow_id ON oc_queues (ivr_flow_id);

CREATE TABLE oc_queue_agents (
  queue_id UUID NOT NULL REFERENCES oc_queues (id) ON DELETE CASCADE,
  agent_id UUID NOT NULL REFERENCES oc_agents (id) ON DELETE CASCADE,
  PRIMARY KEY (queue_id, agent_id)
);
CREATE INDEX idx_oc_queue_agents_agent_id ON oc_queue_agents (agent_id);

CREATE TABLE oc_queue_skills (
  queue_id UUID NOT NULL REFERENCES oc_queues (id) ON DELETE CASCADE,
  skill_id UUID NOT NULL REFERENCES oc_skills (id) ON DELETE CASCADE,
  PRIMARY KEY (queue_id, skill_id)
);
CREATE INDEX idx_oc_queue_skills_skill_id ON oc_queue_skills (skill_id);

CREATE TABLE oc_did_routes (
  id UUID PRIMARY KEY,
  trunk_id TEXT NOT NULL DEFAULT '*',
  d_id TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('queue', 'ivr', 'reject')),
  target_id UUID,
  created_at TIMESTAMPTZ NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL,
  UNIQUE (trunk_id, d_id),
  CHECK (
    (target_type = 'reject' AND target_id IS NULL)
    OR (target_type IN ('queue', 'ivr') AND target_id IS NOT NULL)
  )
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

-- open-call 中文注释
COMMENT ON TABLE oc_schema_migrations IS '业务库已执行的数据库迁移版本';
COMMENT ON COLUMN oc_schema_migrations.version IS '迁移版本号';
COMMENT ON COLUMN oc_schema_migrations.name IS '迁移文件名称';
COMMENT ON COLUMN oc_schema_migrations.applied_at IS '迁移执行时间';

COMMENT ON TABLE oc_users IS '可登录系统的账号（管理员、班长或坐席）';
COMMENT ON COLUMN oc_users.id IS '用户主键 UUID';
COMMENT ON COLUMN oc_users.username IS '登录名';
COMMENT ON COLUMN oc_users.password_hash IS 'argon2id 密码哈希';
COMMENT ON COLUMN oc_users.role IS '角色 admin supervisor agent';
COMMENT ON COLUMN oc_users.display_name IS '用户名';
COMMENT ON COLUMN oc_users.disabled IS '是否禁用账号';
COMMENT ON COLUMN oc_users.auth_version IS '认证版本，密码、角色或禁用状态变化时递增';
COMMENT ON COLUMN oc_users.must_change_password IS '是否要求下次登录后修改临时密码';
COMMENT ON COLUMN oc_users.employee_no IS '工号';
COMMENT ON COLUMN oc_users.created_at IS '创建时间';
COMMENT ON COLUMN oc_users.updated_at IS '更新时间';

COMMENT ON TABLE oc_skills IS '技能组，用于队列路由匹配';
COMMENT ON COLUMN oc_skills.id IS '技能组 ID';
COMMENT ON COLUMN oc_skills.name IS '技能名称';
COMMENT ON COLUMN oc_skills.created_at IS '创建时间';

COMMENT ON TABLE oc_agents IS '呼叫中心坐席业务属性，与 User 一对一扩展';
COMMENT ON COLUMN oc_agents.id IS '坐席主键 UUID';
COMMENT ON COLUMN oc_agents.user_id IS '关联用户 ID';
COMMENT ON COLUMN oc_agents.extension IS '分机号';
COMMENT ON COLUMN oc_agents.video_capable IS '是否支持视频';
COMMENT ON COLUMN oc_agents.terminal_type IS '坐席终端类型：webrtc 或 sip';
COMMENT ON COLUMN oc_agents.sip_username IS 'Switch 配置中的设备账号，不含凭证';
COMMENT ON COLUMN oc_agents.created_at IS '创建时间';
COMMENT ON COLUMN oc_agents.updated_at IS '更新时间';

COMMENT ON TABLE oc_agent_skills IS '坐席与技能多对多关联';
COMMENT ON COLUMN oc_agent_skills.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_agent_skills.skill_id IS '技能 ID';

COMMENT ON TABLE oc_ivr_flows IS 'IVR 流程定义（草稿与元数据）';
COMMENT ON COLUMN oc_ivr_flows.id IS 'IVR 流程 ID';
COMMENT ON COLUMN oc_ivr_flows.name IS '流程名称';
COMMENT ON COLUMN oc_ivr_flows.draft_json IS '草稿 JSON 配置';
COMMENT ON COLUMN oc_ivr_flows.created_at IS '创建时间';
COMMENT ON COLUMN oc_ivr_flows.updated_at IS '更新时间';

COMMENT ON TABLE oc_queues IS '遗留队列镜像表；运行时队列配置以 Switch 为准，open-call 不再直写本表';
COMMENT ON COLUMN oc_queues.id IS '队列 ID';
COMMENT ON COLUMN oc_queues.name IS '队列名称';
COMMENT ON COLUMN oc_queues.video_enabled IS '是否视频队列';
COMMENT ON COLUMN oc_queues.max_wait_sec IS '最大等待秒数';
COMMENT ON COLUMN oc_queues.dispatch_strategy IS 'ACD 策略';
COMMENT ON COLUMN oc_queues.recording_policy IS '录音策略';
COMMENT ON COLUMN oc_queues.overflow_action IS '溢出动作';
COMMENT ON COLUMN oc_queues.overflow_queue_id IS '溢出目标队列 ID';
COMMENT ON COLUMN oc_queues.ivr_flow_id IS '绑定 IVR 流程 ID';
COMMENT ON COLUMN oc_queues.wait_prompt IS '排队提示文案';
COMMENT ON COLUMN oc_queues.announce_recording IS '是否播放录音告知';
COMMENT ON COLUMN oc_queues.priority_enabled IS '是否优先级队列';
COMMENT ON COLUMN oc_queues.business_hours_json IS '工作时间 JSON';
COMMENT ON COLUMN oc_queues.after_hours_action IS '非工作时间动作';
COMMENT ON COLUMN oc_queues.force_hangup_on_checkout IS '强制签出是否挂断';
COMMENT ON COLUMN oc_queues.listen_announce IS '监听是否提示客户';
COMMENT ON COLUMN oc_queues.created_at IS '创建时间';
COMMENT ON COLUMN oc_queues.updated_at IS '更新时间';

COMMENT ON TABLE oc_queue_agents IS '遗留队列坐席绑定镜像；运行时绑定以 Switch 为准';
COMMENT ON COLUMN oc_queue_agents.queue_id IS '队列 ID';
COMMENT ON COLUMN oc_queue_agents.agent_id IS '坐席 ID';

COMMENT ON TABLE oc_queue_skills IS '遗留队列技能绑定镜像；运行时绑定以 Switch 为准';
COMMENT ON COLUMN oc_queue_skills.queue_id IS '队列 ID';
COMMENT ON COLUMN oc_queue_skills.skill_id IS '技能 ID';

COMMENT ON TABLE oc_did_routes IS '遗留 DID 草稿表；运行时 DID 以 Switch 为准';
COMMENT ON COLUMN oc_did_routes.id IS 'DID 路由 ID';
COMMENT ON COLUMN oc_did_routes.trunk_id IS '中继标识，* 表示默认中继';
COMMENT ON COLUMN oc_did_routes.d_id IS '归一化后的 DID 号码';
COMMENT ON COLUMN oc_did_routes.target_type IS '路由目标类型：queue、ivr 或 reject';
COMMENT ON COLUMN oc_did_routes.target_id IS '目标队列或 IVR 流程 ID，reject 时为空';
COMMENT ON COLUMN oc_did_routes.created_at IS '创建时间';
COMMENT ON COLUMN oc_did_routes.updated_at IS '更新时间';

COMMENT ON TABLE oc_cdr IS '话单记录';
COMMENT ON COLUMN oc_cdr.id IS '话单 ID';
COMMENT ON COLUMN oc_cdr.call_id IS '通话 ID';
COMMENT ON COLUMN oc_cdr.direction IS '呼叫方向';
COMMENT ON COLUMN oc_cdr.queue_id IS '队列 ID（Switch 队列，无本地 FK）';
COMMENT ON COLUMN oc_cdr.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_cdr.caller IS '主叫';
COMMENT ON COLUMN oc_cdr.callee IS '被叫';
COMMENT ON COLUMN oc_cdr.session_type IS '媒介类型';
COMMENT ON COLUMN oc_cdr.result IS '通话结果';
COMMENT ON COLUMN oc_cdr.started_at IS '开始时间';
COMMENT ON COLUMN oc_cdr.answered_at IS '接通时间';
COMMENT ON COLUMN oc_cdr.ended_at IS '结束时间';
COMMENT ON COLUMN oc_cdr.duration_sec IS '通话时长秒';
COMMENT ON COLUMN oc_cdr.wait_sec IS '等待秒数';
COMMENT ON COLUMN oc_cdr.video_started_at IS '视频开始时间';
COMMENT ON COLUMN oc_cdr.video_upgrade_ok IS '升视频是否成功';
COMMENT ON COLUMN oc_cdr.screen_share_count IS '屏幕共享次数';
COMMENT ON COLUMN oc_cdr.created_at IS '创建时间';

COMMENT ON TABLE oc_guest_sessions IS '访客入会 token 与会话元数据';
COMMENT ON COLUMN oc_guest_sessions.id IS '访客会话 ID';
COMMENT ON COLUMN oc_guest_sessions.queue_id IS '目标队列 ID（Switch 队列，无本地 FK）';
COMMENT ON COLUMN oc_guest_sessions.call_id IS '关联通话 ID';
COMMENT ON COLUMN oc_guest_sessions.allowed_media IS '允许媒介';
COMMENT ON COLUMN oc_guest_sessions.priority IS '由受信任签发方设置的队列优先级';
COMMENT ON COLUMN oc_guest_sessions.token IS '入会 token';
COMMENT ON COLUMN oc_guest_sessions.expires_at IS '过期时间';
COMMENT ON COLUMN oc_guest_sessions.consumed_at IS '访客令牌首次成功入队时间';
COMMENT ON COLUMN oc_guest_sessions.created_at IS '创建时间';

COMMENT ON TABLE oc_ivr_published_snapshots IS '已发布 IVR 快照，呼入只读最新 version';
COMMENT ON COLUMN oc_ivr_published_snapshots.id IS '快照 ID';
COMMENT ON COLUMN oc_ivr_published_snapshots.flow_id IS '流程 ID';
COMMENT ON COLUMN oc_ivr_published_snapshots.version IS '发布版本';
COMMENT ON COLUMN oc_ivr_published_snapshots.payload_json IS '发布内容 JSON';
COMMENT ON COLUMN oc_ivr_published_snapshots.published_at IS '发布时间';

COMMENT ON TABLE oc_recordings IS '录音/录像文件元数据';
COMMENT ON COLUMN oc_recordings.id IS '录音记录 ID';
COMMENT ON COLUMN oc_recordings.call_id IS '通话 ID';
COMMENT ON COLUMN oc_recordings.file_path IS '文件路径';
COMMENT ON COLUMN oc_recordings.media_type IS '录制类型';
COMMENT ON COLUMN oc_recordings.started_at IS '开始时间';
COMMENT ON COLUMN oc_recordings.ended_at IS '结束时间';
COMMENT ON COLUMN oc_recordings.retain_until IS '保留截止时间';
COMMENT ON COLUMN oc_recordings.file_size IS '文件大小字节';
COMMENT ON COLUMN oc_recordings.purge_error IS '最近一次文件清理失败原因';
COMMENT ON COLUMN oc_recordings.purge_retry_at IS '录音文件下次清理重试时间';
COMMENT ON COLUMN oc_recordings.created_at IS '创建时间';

COMMENT ON TABLE oc_qa_marks IS '质检时间戳标记';
COMMENT ON COLUMN oc_qa_marks.id IS '质检标记 ID';
COMMENT ON COLUMN oc_qa_marks.call_id IS '通话 ID';
COMMENT ON COLUMN oc_qa_marks.offset_sec IS '相对秒数';
COMMENT ON COLUMN oc_qa_marks.label IS '标记说明';
COMMENT ON COLUMN oc_qa_marks.score IS '质检评分，范围 0 至 100';
COMMENT ON COLUMN oc_qa_marks.created_by IS '创建人';
COMMENT ON COLUMN oc_qa_marks.created_at IS '创建时间';

COMMENT ON TABLE oc_call_wrap_ups IS '通话小结文本';
COMMENT ON COLUMN oc_call_wrap_ups.id IS '小结 ID';
COMMENT ON COLUMN oc_call_wrap_ups.call_id IS '通话 ID';
COMMENT ON COLUMN oc_call_wrap_ups.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_call_wrap_ups.notes IS '小结文本';
COMMENT ON COLUMN oc_call_wrap_ups.disposition_code IS '通话处置码';
COMMENT ON COLUMN oc_call_wrap_ups.tags_json IS '通话标签 JSON 数组';
COMMENT ON COLUMN oc_call_wrap_ups.completed_at IS '事后处理完成时间';
COMMENT ON COLUMN oc_call_wrap_ups.created_at IS '创建时间';

COMMENT ON TABLE oc_webhook_subscriptions IS 'Webhook 事件订阅配置';
COMMENT ON COLUMN oc_webhook_subscriptions.id IS 'Webhook 订阅 ID';
COMMENT ON COLUMN oc_webhook_subscriptions.url IS '回调 URL';
COMMENT ON COLUMN oc_webhook_subscriptions.event_types IS '事件类型列表 JSON';
COMMENT ON COLUMN oc_webhook_subscriptions.secret IS 'HMAC 密钥';
COMMENT ON COLUMN oc_webhook_subscriptions.enabled IS '是否启用';
COMMENT ON COLUMN oc_webhook_subscriptions.created_at IS '创建时间';

COMMENT ON TABLE oc_webhook_deliveries IS 'Webhook 单次投递记录';
COMMENT ON COLUMN oc_webhook_deliveries.id IS '投递记录 ID';
COMMENT ON COLUMN oc_webhook_deliveries.subscription_id IS '订阅 ID';
COMMENT ON COLUMN oc_webhook_deliveries.event_id IS '业务事件唯一标识';
COMMENT ON COLUMN oc_webhook_deliveries.event_type IS '事件类型';
COMMENT ON COLUMN oc_webhook_deliveries.payload IS '载荷 JSON';
COMMENT ON COLUMN oc_webhook_deliveries.status IS '投递状态';
COMMENT ON COLUMN oc_webhook_deliveries.attempts IS '尝试次数';
COMMENT ON COLUMN oc_webhook_deliveries.last_error IS '最后错误';
COMMENT ON COLUMN oc_webhook_deliveries.next_attempt_at IS '下次计划投递时间';
COMMENT ON COLUMN oc_webhook_deliveries.locked_at IS '后台任务租约获取时间';
COMMENT ON COLUMN oc_webhook_deliveries.dead_letter_at IS '进入死信队列时间';
COMMENT ON COLUMN oc_webhook_deliveries.created_at IS '创建时间';
COMMENT ON COLUMN oc_webhook_deliveries.updated_at IS '更新时间';

COMMENT ON TABLE oc_audit_logs IS '管理操作与敏感访问审计';
COMMENT ON COLUMN oc_audit_logs.id IS '审计日志 ID';
COMMENT ON COLUMN oc_audit_logs.user_id IS '操作人用户 ID';
COMMENT ON COLUMN oc_audit_logs.action IS '动作类型';
COMMENT ON COLUMN oc_audit_logs.resource IS '资源描述';
COMMENT ON COLUMN oc_audit_logs.detail_json IS '详情 JSON';
COMMENT ON COLUMN oc_audit_logs.request_id IS '关联 HTTP 请求 ID';
COMMENT ON COLUMN oc_audit_logs.remote_ip IS '操作来源 IP';
COMMENT ON COLUMN oc_audit_logs.outcome IS '操作结果 success 或 failure';
COMMENT ON COLUMN oc_audit_logs.created_at IS '发生时间';

COMMENT ON TABLE oc_jwt_revocations IS '已撤销 JWT 的 jti 黑名单';
COMMENT ON COLUMN oc_jwt_revocations.jti IS '已撤销令牌的唯一标识';
COMMENT ON COLUMN oc_jwt_revocations.expires_at IS '令牌过期时间';
COMMENT ON COLUMN oc_jwt_revocations.revoked_at IS '撤销时间';

COMMENT ON TABLE oc_auth_sessions IS '用户登录会话，可按设备单独撤销';
COMMENT ON COLUMN oc_auth_sessions.id IS '登录会话主键 UUID';
COMMENT ON COLUMN oc_auth_sessions.user_id IS '关联用户 ID';
COMMENT ON COLUMN oc_auth_sessions.refresh_jti IS '当前刷新令牌唯一标识';
COMMENT ON COLUMN oc_auth_sessions.user_agent IS '登录客户端 User-Agent';
COMMENT ON COLUMN oc_auth_sessions.remote_ip IS '登录来源 IP';
COMMENT ON COLUMN oc_auth_sessions.provider IS '登录来源，local 表示本地认证';
COMMENT ON COLUMN oc_auth_sessions.provider_refresh_token IS '外部身份提供方的刷新令牌';
COMMENT ON COLUMN oc_auth_sessions.provider_subject IS '外部身份提供方的用户标识';
COMMENT ON COLUMN oc_auth_sessions.created_at IS '会话创建时间';
COMMENT ON COLUMN oc_auth_sessions.expires_at IS '会话过期时间';
COMMENT ON COLUMN oc_auth_sessions.last_seen_at IS '最后刷新时间';
COMMENT ON COLUMN oc_auth_sessions.revoked_at IS '会话撤销时间';

COMMENT ON TABLE oc_roles IS '角色定义，包括系统内置角色与自定义角色';
COMMENT ON COLUMN oc_roles.id IS '角色唯一标识';
COMMENT ON COLUMN oc_roles.name IS '角色名称';
COMMENT ON COLUMN oc_roles.built_in IS '是否为系统内置角色';
COMMENT ON COLUMN oc_roles.created_at IS '角色创建时间';

COMMENT ON TABLE oc_permissions IS '系统权限定义';
COMMENT ON COLUMN oc_permissions.code IS '权限唯一编码';
COMMENT ON COLUMN oc_permissions.description IS '权限中文说明';

COMMENT ON TABLE oc_role_permissions IS '角色与权限的关联关系';
COMMENT ON COLUMN oc_role_permissions.role_id IS '角色标识';
COMMENT ON COLUMN oc_role_permissions.permission_code IS '权限编码';

COMMENT ON TABLE oc_user_roles IS '用户与角色的关联关系';
COMMENT ON COLUMN oc_user_roles.user_id IS '用户 ID';
COMMENT ON COLUMN oc_user_roles.role_id IS '角色标识';

COMMENT ON TABLE oc_oidc_identities IS '外部 OIDC 身份与本地用户的绑定关系';
COMMENT ON COLUMN oc_oidc_identities.issuer IS '身份提供方签发者地址';
COMMENT ON COLUMN oc_oidc_identities.subject IS '身份提供方用户标识';
COMMENT ON COLUMN oc_oidc_identities.user_id IS '绑定的本地用户 ID';

COMMENT ON TABLE oc_oidc_group_roles IS '外部身份组与本地角色的映射关系';
COMMENT ON COLUMN oc_oidc_group_roles.group_name IS '外部身份组名称';
COMMENT ON COLUMN oc_oidc_group_roles.role_id IS '映射的本地角色标识';

COMMENT ON TABLE oc_oidc_flows IS '待完成的 OIDC 登录流程状态';
COMMENT ON COLUMN oc_oidc_flows.state_hash IS '登录状态参数的哈希值';
COMMENT ON COLUMN oc_oidc_flows.nonce IS '用于校验身份令牌的随机值';
COMMENT ON COLUMN oc_oidc_flows.verifier IS '用于 PKCE 校验的验证值';
COMMENT ON COLUMN oc_oidc_flows.return_path IS '登录完成后的返回路径';
COMMENT ON COLUMN oc_oidc_flows.expires_at IS '登录流程过期时间';

COMMENT ON TABLE oc_oidc_tickets IS 'OIDC 登录完成后的短期兑换凭据';
COMMENT ON COLUMN oc_oidc_tickets.ticket_hash IS '兑换凭据的哈希值';
COMMENT ON COLUMN oc_oidc_tickets.user_id IS '对应的本地用户 ID';
COMMENT ON COLUMN oc_oidc_tickets.provider_refresh_token IS '待交付的身份提供方刷新令牌';
COMMENT ON COLUMN oc_oidc_tickets.provider_subject IS '身份提供方用户标识';
COMMENT ON COLUMN oc_oidc_tickets.expires_at IS '兑换凭据过期时间';

COMMENT ON TABLE oc_switch_event_cursor IS '消费 Switch 事件流的游标（单行）';
COMMENT ON COLUMN oc_switch_event_cursor.id IS '固定为 1 的主键';
COMMENT ON COLUMN oc_switch_event_cursor.last_event_id IS '已观察到的最大事件 ID（对账水印）；乱序去重依赖 inbox，不以水印拒绝晚到事件';
COMMENT ON COLUMN oc_switch_event_cursor.updated_at IS '游标更新时间';

COMMENT ON TABLE oc_agent_state_projection IS '从 Switch 事件投影的坐席状态变更历史（只读）';
COMMENT ON COLUMN oc_agent_state_projection.id IS 'Switch 侧状态日志事件 ID';
COMMENT ON COLUMN oc_agent_state_projection.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_agent_state_projection.from_state IS '原状态';
COMMENT ON COLUMN oc_agent_state_projection.to_state IS '新状态';
COMMENT ON COLUMN oc_agent_state_projection.reason IS '变更原因';
COMMENT ON COLUMN oc_agent_state_projection.created_at IS '事件发生时间';

COMMENT ON TABLE oc_switch_event_inbox IS '已接收的 Switch 事件去重收件箱';
COMMENT ON COLUMN oc_switch_event_inbox.event_id IS 'Switch 事件全局 ID';
COMMENT ON COLUMN oc_switch_event_inbox.received_at IS '业务侧接收时间';

COMMENT ON TABLE oc_switch_event_outbox IS '待向 WebSocket/Webhook 投递的事件出站队列';
COMMENT ON COLUMN oc_switch_event_outbox.event_id IS '关联收件箱事件 ID';
COMMENT ON COLUMN oc_switch_event_outbox.payload IS '序列化后的事件 JSON';
COMMENT ON COLUMN oc_switch_event_outbox.delivered_at IS '投递完成时间，空表示待投递';

-- 内置种子（RBAC + Switch 事件游标）

INSERT INTO oc_roles (id, name, built_in) VALUES
  ('admin', '管理员', TRUE),
  ('supervisor', '班长', TRUE),
  ('agent', '坐席', TRUE);

INSERT INTO oc_permissions (code, description) VALUES
  ('users.read', '查看用户'),
  ('users.create', '创建用户'),
  ('users.update', '修改用户'),
  ('users.delete', '删除用户'),
  ('users.credentials', '管理密码'),
  ('users.sessions', '撤销会话'),
  ('roles.read', '查看角色'),
  ('roles.write', '管理角色'),
  ('identity.read', '查看身份映射'),
  ('identity.write', '管理身份映射'),
  ('queues.read', '查看队列'),
  ('queues.write', '管理队列'),
  ('skills.read', '查看技能'),
  ('skills.write', '管理技能'),
  ('agents.read', '查看坐席'),
  ('agents.self', '操作本人坐席'),
  ('agents.force_checkout', '强制签出坐席'),
  ('calls.read', '查看通话'),
  ('calls.operate', '操作通话'),
  ('calls.listen', '监听通话'),
  ('calls.wrap_up', '填写通话小结'),
  ('cdr.read', '查看话单'),
  ('cdr.export', '导出话单'),
  ('recordings.read', '查看录音'),
  ('recordings.download', '下载录音'),
  ('recordings.qa', '管理质检标记'),
  ('recordings.purge', '清理录音'),
  ('reports.read', '查看报表'),
  ('ivr.read', '查看 IVR'),
  ('ivr.write', '管理 IVR'),
  ('webhooks.read', '查看 Webhook'),
  ('webhooks.write', '管理 Webhook'),
  ('audit.read', '查看审计'),
  ('dids.read', '查看呼入号码'),
  ('dids.write', '管理呼入号码'),
  ('config.read', '导出配置'),
  ('config.write', '导入配置'),
  ('guest.issue', '签发访客会话'),
  ('status.read', '查看运行状态');

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
