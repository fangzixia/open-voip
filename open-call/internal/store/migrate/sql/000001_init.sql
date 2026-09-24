-- open-call 业务库（oc_*），不含 Switch 运行时 calls/legs

CREATE TABLE IF NOT EXISTS oc_users (
  id UUID PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  role VARCHAR(32) NOT NULL,
  display_name VARCHAR(128),
  disabled BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_users_username ON oc_users (username);
CREATE INDEX IF NOT EXISTS idx_oc_users_role ON oc_users (role);

CREATE TABLE IF NOT EXISTS oc_skills (
  id UUID PRIMARY KEY,
  name VARCHAR(64) NOT NULL,
  created_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_skills_name ON oc_skills (name);

CREATE TABLE IF NOT EXISTS oc_agents (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  extension VARCHAR(16) NOT NULL,
  video_capable BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_agents_user_id ON oc_agents (user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_agents_extension ON oc_agents (extension);

CREATE TABLE IF NOT EXISTS oc_agent_skills (
  agent_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (agent_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_oc_agent_skills_skill_id ON oc_agent_skills (skill_id);

CREATE TABLE IF NOT EXISTS oc_queues (
  id UUID PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  video_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  max_wait_sec BIGINT NOT NULL DEFAULT 300,
  dispatch_strategy VARCHAR(32) NOT NULL DEFAULT 'longest_idle',
  recording_policy VARCHAR(32) NOT NULL DEFAULT 'off',
  overflow_action VARCHAR(32),
  overflow_queue_id UUID,
  ivr_flow_id UUID,
  wait_prompt VARCHAR(256),
  announce_recording BOOLEAN NOT NULL DEFAULT FALSE,
  priority_enabled BOOLEAN NOT NULL DEFAULT FALSE,
  business_hours_json TEXT,
  after_hours_action VARCHAR(32),
  force_hangup_on_checkout BOOLEAN NOT NULL DEFAULT TRUE,
  listen_announce BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_queues_ivr_flow_id ON oc_queues (ivr_flow_id);

CREATE TABLE IF NOT EXISTS oc_queue_agents (
  queue_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  PRIMARY KEY (queue_id, agent_id)
);

CREATE INDEX IF NOT EXISTS idx_oc_queue_agents_agent_id ON oc_queue_agents (agent_id);

CREATE TABLE IF NOT EXISTS oc_queue_skills (
  queue_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (queue_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_oc_queue_skills_skill_id ON oc_queue_skills (skill_id);

CREATE TABLE IF NOT EXISTS oc_did_routes (
  id UUID PRIMARY KEY,
  d_id VARCHAR(32) NOT NULL,
  queue_id UUID NOT NULL,
  display_name VARCHAR(128),
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_did_routes_d_id ON oc_did_routes (d_id);
CREATE INDEX IF NOT EXISTS idx_oc_did_routes_queue_id ON oc_did_routes (queue_id);

CREATE TABLE IF NOT EXISTS oc_agent_sessions (
  id UUID PRIMARY KEY,
  agent_id UUID NOT NULL,
  state VARCHAR(32) NOT NULL,
  busy_reason VARCHAR(64),
  checked_in_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_agent_sessions_agent_id ON oc_agent_sessions (agent_id);
CREATE INDEX IF NOT EXISTS idx_oc_agent_sessions_state ON oc_agent_sessions (state);

CREATE TABLE IF NOT EXISTS oc_agent_session_queues (
  agent_id UUID NOT NULL,
  queue_id UUID NOT NULL,
  PRIMARY KEY (agent_id, queue_id)
);

CREATE INDEX IF NOT EXISTS idx_oc_agent_session_queues_queue_id ON oc_agent_session_queues (queue_id);

CREATE TABLE IF NOT EXISTS oc_agent_state_log (
  id UUID PRIMARY KEY,
  agent_id UUID NOT NULL,
  from_state VARCHAR(32),
  to_state VARCHAR(32) NOT NULL,
  reason VARCHAR(128),
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_agent_state_log_agent_id ON oc_agent_state_log (agent_id);
CREATE INDEX IF NOT EXISTS idx_oc_agent_state_log_created_at ON oc_agent_state_log (created_at);

CREATE TABLE IF NOT EXISTS oc_cdr (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  direction VARCHAR(16),
  queue_id UUID,
  agent_id UUID,
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

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_cdr_call_id ON oc_cdr (call_id);
CREATE INDEX IF NOT EXISTS idx_oc_cdr_queue_id ON oc_cdr (queue_id);
CREATE INDEX IF NOT EXISTS idx_oc_cdr_agent_id ON oc_cdr (agent_id);
CREATE INDEX IF NOT EXISTS idx_oc_cdr_result ON oc_cdr (result);
CREATE INDEX IF NOT EXISTS idx_cdr_started_queue ON oc_cdr (started_at, queue_id);

CREATE TABLE IF NOT EXISTS oc_guest_sessions (
  id UUID PRIMARY KEY,
  queue_id UUID NOT NULL,
  call_id UUID,
  allowed_media VARCHAR(16) NOT NULL DEFAULT 'audio',
  token VARCHAR(128) NOT NULL,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_guest_sessions_queue_id ON oc_guest_sessions (queue_id);
CREATE INDEX IF NOT EXISTS idx_oc_guest_sessions_call_id ON oc_guest_sessions (call_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_guest_sessions_token ON oc_guest_sessions (token);
CREATE INDEX IF NOT EXISTS idx_oc_guest_sessions_expires_at ON oc_guest_sessions (expires_at);

CREATE TABLE IF NOT EXISTS oc_ivr_flows (
  id UUID PRIMARY KEY,
  name VARCHAR(128) NOT NULL,
  draft_json TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oc_ivr_published_snapshots (
  id UUID PRIMARY KEY,
  flow_id UUID NOT NULL,
  version BIGINT NOT NULL,
  payload_json TEXT NOT NULL,
  published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_ivr_flow_version ON oc_ivr_published_snapshots (flow_id, version DESC);

CREATE TABLE IF NOT EXISTS oc_recordings (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  file_path VARCHAR(512) NOT NULL,
  media_type VARCHAR(32) NOT NULL,
  started_at TIMESTAMPTZ,
  ended_at TIMESTAMPTZ,
  retain_until TIMESTAMPTZ,
  file_size BIGINT,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_recordings_call_id ON oc_recordings (call_id);
CREATE INDEX IF NOT EXISTS idx_oc_recordings_retain_until ON oc_recordings (retain_until);

CREATE TABLE IF NOT EXISTS oc_qa_marks (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  offset_sec BIGINT NOT NULL,
  label VARCHAR(256),
  created_by UUID,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_qa_marks_call_id ON oc_qa_marks (call_id);

CREATE TABLE IF NOT EXISTS oc_call_wrap_ups (
  id UUID PRIMARY KEY,
  call_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  notes TEXT NOT NULL,
  created_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_call_wrap_ups_call_id ON oc_call_wrap_ups (call_id);
CREATE INDEX IF NOT EXISTS idx_oc_call_wrap_ups_agent_id ON oc_call_wrap_ups (agent_id);

CREATE TABLE IF NOT EXISTS oc_webhook_subscriptions (
  id UUID PRIMARY KEY,
  url VARCHAR(512) NOT NULL,
  event_types TEXT NOT NULL,
  secret VARCHAR(128),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS oc_webhook_deliveries (
  id UUID PRIMARY KEY,
  subscription_id UUID NOT NULL,
  event_type VARCHAR(64) NOT NULL,
  payload TEXT NOT NULL,
  status VARCHAR(16) NOT NULL,
  attempts BIGINT NOT NULL DEFAULT 0,
  last_error TEXT,
  created_at TIMESTAMPTZ,
  updated_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_webhook_deliveries_subscription_id ON oc_webhook_deliveries (subscription_id);
CREATE INDEX IF NOT EXISTS idx_oc_webhook_deliveries_event_type ON oc_webhook_deliveries (event_type);
CREATE INDEX IF NOT EXISTS idx_oc_webhook_deliveries_status ON oc_webhook_deliveries (status);

CREATE TABLE IF NOT EXISTS oc_audit_logs (
  id UUID PRIMARY KEY,
  user_id UUID,
  action VARCHAR(64) NOT NULL,
  resource VARCHAR(256),
  detail_json TEXT,
  created_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_audit_logs_user_id ON oc_audit_logs (user_id);
CREATE INDEX IF NOT EXISTS idx_oc_audit_logs_action ON oc_audit_logs (action);
CREATE INDEX IF NOT EXISTS idx_oc_audit_logs_created_at ON oc_audit_logs (created_at);

CREATE TABLE IF NOT EXISTS oc_jwt_revocations (
  jti VARCHAR(64) PRIMARY KEY,
  expires_at TIMESTAMPTZ,
  revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_oc_jwt_revocations_expires_at ON oc_jwt_revocations (expires_at);

-- 表与字段中文注释（PostgreSQL COMMENT）

COMMENT ON TABLE oc_users IS '可登录系统的账号（管理员、班长或坐席）';
COMMENT ON COLUMN oc_users.id IS '用户主键 UUID';
COMMENT ON COLUMN oc_users.username IS '登录用户名';
COMMENT ON COLUMN oc_users.password_hash IS 'argon2id 密码哈希';
COMMENT ON COLUMN oc_users.role IS '角色 admin supervisor agent';
COMMENT ON COLUMN oc_users.display_name IS '展示名';
COMMENT ON COLUMN oc_users.disabled IS '是否禁用账号';
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
COMMENT ON COLUMN oc_agents.created_at IS '创建时间';
COMMENT ON COLUMN oc_agents.updated_at IS '更新时间';

COMMENT ON TABLE oc_agent_skills IS '坐席与技能多对多关联';
COMMENT ON COLUMN oc_agent_skills.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_agent_skills.skill_id IS '技能 ID';

COMMENT ON TABLE oc_queues IS '呼入队列配置';
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

COMMENT ON TABLE oc_queue_agents IS '队列与可签入坐席的绑定关系';
COMMENT ON COLUMN oc_queue_agents.queue_id IS '队列 ID';
COMMENT ON COLUMN oc_queue_agents.agent_id IS '坐席 ID';

COMMENT ON TABLE oc_queue_skills IS '队列所需技能';
COMMENT ON COLUMN oc_queue_skills.queue_id IS '队列 ID';
COMMENT ON COLUMN oc_queue_skills.skill_id IS '技能 ID';

COMMENT ON TABLE oc_did_routes IS '外显/DID 号码到队列的路由';
COMMENT ON COLUMN oc_did_routes.id IS 'DID 路由 ID';
COMMENT ON COLUMN oc_did_routes.d_id IS 'DID 号码';
COMMENT ON COLUMN oc_did_routes.queue_id IS '目标队列 ID';
COMMENT ON COLUMN oc_did_routes.display_name IS '外显名称';
COMMENT ON COLUMN oc_did_routes.created_at IS '创建时间';
COMMENT ON COLUMN oc_did_routes.updated_at IS '更新时间';

COMMENT ON TABLE oc_agent_sessions IS '坐席签入会话（队列绑定、当前状态）';
COMMENT ON COLUMN oc_agent_sessions.id IS '签入会话 ID';
COMMENT ON COLUMN oc_agent_sessions.agent_id IS '坐席 ID 签入唯一';
COMMENT ON COLUMN oc_agent_sessions.state IS '坐席状态';
COMMENT ON COLUMN oc_agent_sessions.busy_reason IS '示忙原因';
COMMENT ON COLUMN oc_agent_sessions.checked_in_at IS '签入时间';
COMMENT ON COLUMN oc_agent_sessions.updated_at IS '状态更新时间';

COMMENT ON TABLE oc_agent_session_queues IS '签入会话所选队列';
COMMENT ON COLUMN oc_agent_session_queues.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_agent_session_queues.queue_id IS '签入队列 ID';

COMMENT ON TABLE oc_agent_state_log IS '坐席状态变更审计，供报表';
COMMENT ON COLUMN oc_agent_state_log.id IS '状态日志 ID';
COMMENT ON COLUMN oc_agent_state_log.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_agent_state_log.from_state IS '原状态';
COMMENT ON COLUMN oc_agent_state_log.to_state IS '新状态';
COMMENT ON COLUMN oc_agent_state_log.reason IS '变更原因';
COMMENT ON COLUMN oc_agent_state_log.created_at IS '记录时间';

COMMENT ON TABLE oc_cdr IS '话单记录';
COMMENT ON COLUMN oc_cdr.id IS '话单 ID';
COMMENT ON COLUMN oc_cdr.call_id IS '通话 ID';
COMMENT ON COLUMN oc_cdr.direction IS '呼叫方向';
COMMENT ON COLUMN oc_cdr.queue_id IS '队列 ID';
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
COMMENT ON COLUMN oc_guest_sessions.queue_id IS '目标队列 ID';
COMMENT ON COLUMN oc_guest_sessions.call_id IS '关联通话 ID';
COMMENT ON COLUMN oc_guest_sessions.allowed_media IS '允许媒介';
COMMENT ON COLUMN oc_guest_sessions.token IS '入会 token';
COMMENT ON COLUMN oc_guest_sessions.expires_at IS '过期时间';
COMMENT ON COLUMN oc_guest_sessions.created_at IS '创建时间';

COMMENT ON TABLE oc_ivr_flows IS 'IVR 流程定义（草稿与元数据）';
COMMENT ON COLUMN oc_ivr_flows.id IS 'IVR 流程 ID';
COMMENT ON COLUMN oc_ivr_flows.name IS '流程名称';
COMMENT ON COLUMN oc_ivr_flows.draft_json IS '草稿 JSON 配置';
COMMENT ON COLUMN oc_ivr_flows.created_at IS '创建时间';
COMMENT ON COLUMN oc_ivr_flows.updated_at IS '更新时间';

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
COMMENT ON COLUMN oc_recordings.created_at IS '创建时间';

COMMENT ON TABLE oc_qa_marks IS '质检时间戳标记';
COMMENT ON COLUMN oc_qa_marks.id IS '质检标记 ID';
COMMENT ON COLUMN oc_qa_marks.call_id IS '通话 ID';
COMMENT ON COLUMN oc_qa_marks.offset_sec IS '相对秒数';
COMMENT ON COLUMN oc_qa_marks.label IS '标记说明';
COMMENT ON COLUMN oc_qa_marks.created_by IS '创建人';
COMMENT ON COLUMN oc_qa_marks.created_at IS '创建时间';

COMMENT ON TABLE oc_call_wrap_ups IS '通话小结文本';
COMMENT ON COLUMN oc_call_wrap_ups.id IS '小结 ID';
COMMENT ON COLUMN oc_call_wrap_ups.call_id IS '通话 ID';
COMMENT ON COLUMN oc_call_wrap_ups.agent_id IS '坐席 ID';
COMMENT ON COLUMN oc_call_wrap_ups.notes IS '小结文本';
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
COMMENT ON COLUMN oc_webhook_deliveries.event_type IS '事件类型';
COMMENT ON COLUMN oc_webhook_deliveries.payload IS '载荷 JSON';
COMMENT ON COLUMN oc_webhook_deliveries.status IS '投递状态';
COMMENT ON COLUMN oc_webhook_deliveries.attempts IS '尝试次数';
COMMENT ON COLUMN oc_webhook_deliveries.last_error IS '最后错误';
COMMENT ON COLUMN oc_webhook_deliveries.created_at IS '创建时间';
COMMENT ON COLUMN oc_webhook_deliveries.updated_at IS '更新时间';

COMMENT ON TABLE oc_audit_logs IS '管理操作与敏感访问审计';
COMMENT ON COLUMN oc_audit_logs.id IS '审计日志 ID';
COMMENT ON COLUMN oc_audit_logs.user_id IS '操作人用户 ID';
COMMENT ON COLUMN oc_audit_logs.action IS '动作类型';
COMMENT ON COLUMN oc_audit_logs.resource IS '资源描述';
COMMENT ON COLUMN oc_audit_logs.detail_json IS '详情 JSON';
COMMENT ON COLUMN oc_audit_logs.created_at IS '发生时间';

COMMENT ON TABLE oc_jwt_revocations IS '已撤销 JWT 的 jti 黑名单';
COMMENT ON COLUMN oc_jwt_revocations.jti IS 'JWT jti';
COMMENT ON COLUMN oc_jwt_revocations.expires_at IS '令牌过期时间';
COMMENT ON COLUMN oc_jwt_revocations.revoked_at IS '撤销时间';
