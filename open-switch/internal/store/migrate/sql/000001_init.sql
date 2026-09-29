-- open-switch 运行库（os_*）空库 schema，由历史迁移合并为单文件。
-- PostgreSQL；表名与列名英文，中文见 COMMENT。


CREATE TABLE os_config_versions (
  application_id TEXT NOT NULL,
  version BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('validated', 'active', 'superseded')),
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
  FOREIGN KEY (application_id, version) REFERENCES os_config_versions (application_id, version)
);

CREATE TABLE os_queues (
  application_id TEXT NOT NULL,
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
  PRIMARY KEY (application_id, config_version, id),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions (application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_skills (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  name TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, name),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions (application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_agents (
  application_id TEXT NOT NULL,
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
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, extension),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions (application_id, version) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_os_agents_sip_username ON os_agents (application_id, config_version, sip_username) WHERE sip_username <> '';

CREATE TABLE os_queue_skills (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, queue_id, skill_id),
  FOREIGN KEY (application_id, config_version, queue_id) REFERENCES os_queues (application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, skill_id) REFERENCES os_skills (application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_queue_agents (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  queue_id UUID NOT NULL,
  agent_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, queue_id, agent_id),
  FOREIGN KEY (application_id, config_version, queue_id) REFERENCES os_queues (application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, agent_id) REFERENCES os_agents (application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_agent_skills (
  application_id TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  agent_id UUID NOT NULL,
  skill_id UUID NOT NULL,
  PRIMARY KEY (application_id, config_version, agent_id, skill_id),
  FOREIGN KEY (application_id, config_version, agent_id) REFERENCES os_agents (application_id, config_version, id) ON DELETE CASCADE,
  FOREIGN KEY (application_id, config_version, skill_id) REFERENCES os_skills (application_id, config_version, id) ON DELETE CASCADE
);

CREATE TABLE os_did_routes (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  trunk_id TEXT NOT NULL DEFAULT '*',
  normalized_did TEXT NOT NULL,
  target_type TEXT NOT NULL CHECK (target_type IN ('queue', 'ivr', 'reject')),
  target_id UUID,
  config_version BIGINT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, id),
  UNIQUE (application_id, config_version, trunk_id, normalized_did),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions (application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_ivr_published_snapshots (
  application_id TEXT NOT NULL,
  flow_id UUID NOT NULL,
  version INTEGER NOT NULL,
  payload_json TEXT NOT NULL,
  config_version BIGINT NOT NULL,
  published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, config_version, flow_id),
  FOREIGN KEY (application_id, config_version) REFERENCES os_config_versions (application_id, version) ON DELETE CASCADE
);

CREATE TABLE os_agent_sessions (
  application_id TEXT NOT NULL,
  id UUID NOT NULL,
  agent_id UUID NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('idle', 'busy', 'ringing', 'on_call', 'acw')),
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
CREATE INDEX idx_os_agent_state_log_agent_time ON os_agent_state_log (application_id, agent_id, created_at DESC);

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
CREATE INDEX idx_os_calls_app_state ON os_calls (application_id, state, updated_at);
CREATE INDEX idx_os_calls_queue_state ON os_calls (application_id, queue_id, state);

CREATE TABLE os_call_legs (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
  type TEXT NOT NULL DEFAULT 'webrtc' CHECK (type IN ('sip', 'webrtc', 'playback')),
  role TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'new',
  agent_id UUID,
  participant_ref TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_os_call_legs_call ON os_call_legs (application_id, call_id, created_at);

CREATE TABLE os_bridges (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
  call_id UUID NOT NULL REFERENCES os_calls (id) ON DELETE CASCADE,
  mode TEXT NOT NULL CHECK (mode IN ('pair', 'conference')),
  state TEXT NOT NULL,
  participants TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL,
  ended_at TIMESTAMPTZ
);

CREATE TABLE os_routing_sessions (
  call_id UUID PRIMARY KEY REFERENCES os_calls (id) ON DELETE CASCADE,
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
  call_id UUID PRIMARY KEY REFERENCES os_calls (id) ON DELETE CASCADE,
  application_id TEXT NOT NULL,
  queue_id UUID NOT NULL,
  priority BIGINT NOT NULL,
  enqueued_at TIMESTAMPTZ NOT NULL,
  deadline_at TIMESTAMPTZ NOT NULL,
  state TEXT NOT NULL
);
CREATE INDEX idx_os_queue_entries_pick ON os_queue_entries (application_id, queue_id, state, priority DESC, enqueued_at);

CREATE TABLE os_acd_attempts (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
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
  status TEXT NOT NULL CHECK (status IN ('accepted', 'running', 'succeeded', 'failed', 'unknown')),
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
CREATE INDEX idx_os_call_events_app_id ON os_call_events (application_id, id);

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
CREATE INDEX idx_os_recordings_call ON os_recordings (application_id, call_id);

CREATE TABLE os_queue_dispatch_cursor (
  application_id TEXT NOT NULL,
  queue_id UUID NOT NULL,
  last_agent_id TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (application_id, queue_id)
);

CREATE TABLE os_business_actions (
  id UUID PRIMARY KEY,
  application_id TEXT NOT NULL,
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
CREATE INDEX idx_os_business_actions_call ON os_business_actions (application_id, call_id);

CREATE TABLE os_applications (
  id TEXT PRIMARY KEY,
  secret_hash TEXT NOT NULL,
  secret TEXT NOT NULL,
  events_callback_url TEXT NOT NULL DEFAULT '',
  event_retention_days INTEGER NOT NULL DEFAULT 14 CHECK (event_retention_days > 0),
  max_concurrent_calls INTEGER NOT NULL DEFAULT 100 CHECK (max_concurrent_calls > 0),
  enabled BOOLEAN NOT NULL DEFAULT TRUE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE os_integrator_event_deliveries (
  application_id TEXT NOT NULL,
  event_id BIGINT NOT NULL,
  status TEXT NOT NULL CHECK (status IN ('pending', 'success', 'failed')),
  attempts INTEGER NOT NULL DEFAULT 0,
  next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_error TEXT NOT NULL DEFAULT '',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (application_id, event_id)
);
CREATE INDEX idx_os_integrator_event_deliveries_retry
  ON os_integrator_event_deliveries (status, next_retry_at)
  WHERE status = 'pending';

-- open-switch 中文注释
COMMENT ON TABLE os_schema_migrations IS '交换服务已执行的数据库迁移版本';
COMMENT ON COLUMN os_schema_migrations.version IS '迁移版本号';
COMMENT ON COLUMN os_schema_migrations.name IS '迁移文件名称';
COMMENT ON COLUMN os_schema_migrations.applied_at IS '迁移执行时间';

COMMENT ON TABLE os_config_versions IS '按应用发布的配置包版本历史';
COMMENT ON COLUMN os_config_versions.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_config_versions.version IS '配置版本号';
COMMENT ON COLUMN os_config_versions.status IS '版本状态 validated active superseded';
COMMENT ON COLUMN os_config_versions.checksum IS '配置内容校验和';
COMMENT ON COLUMN os_config_versions.payload IS '完整配置 JSON';
COMMENT ON COLUMN os_config_versions.created_at IS '版本创建时间';
COMMENT ON COLUMN os_config_versions.activated_at IS '激活时间';

COMMENT ON TABLE os_active_config IS '各应用当前生效的配置版本指针';
COMMENT ON COLUMN os_active_config.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_active_config.version IS '当前生效配置版本号';
COMMENT ON COLUMN os_active_config.activated_at IS '切换生效时间';

COMMENT ON TABLE os_queues IS '已发布队列配置快照';
COMMENT ON COLUMN os_queues.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_queues.id IS '队列 ID';
COMMENT ON COLUMN os_queues.name IS '队列名称';
COMMENT ON COLUMN os_queues.video_enabled IS '是否视频队列';
COMMENT ON COLUMN os_queues.max_wait_sec IS '最大等待秒数';
COMMENT ON COLUMN os_queues.dispatch_strategy IS 'ACD 派单策略';
COMMENT ON COLUMN os_queues.recording_policy IS '录音策略';
COMMENT ON COLUMN os_queues.overflow_action IS '溢出动作';
COMMENT ON COLUMN os_queues.overflow_queue_id IS '溢出目标队列 ID';
COMMENT ON COLUMN os_queues.ivr_flow_id IS '绑定 IVR 流程 ID';
COMMENT ON COLUMN os_queues.wait_prompt IS '排队提示文案';
COMMENT ON COLUMN os_queues.announce_recording IS '是否播放录音告知';
COMMENT ON COLUMN os_queues.priority_enabled IS '是否启用优先级';
COMMENT ON COLUMN os_queues.business_hours_json IS '工作时间配置';
COMMENT ON COLUMN os_queues.after_hours_action IS '非工作时间动作';
COMMENT ON COLUMN os_queues.force_hangup_on_checkout IS '强制签出是否挂断';
COMMENT ON COLUMN os_queues.listen_announce IS '监听是否提示客户';
COMMENT ON COLUMN os_queues.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_queues.created_at IS '创建时间';
COMMENT ON COLUMN os_queues.updated_at IS '更新时间';

COMMENT ON TABLE os_skills IS '已发布技能组配置快照';
COMMENT ON COLUMN os_skills.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_skills.id IS '技能 ID';
COMMENT ON COLUMN os_skills.name IS '技能名称';
COMMENT ON COLUMN os_skills.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_skills.created_at IS '创建时间';

COMMENT ON TABLE os_agents IS '已发布坐席配置快照';
COMMENT ON COLUMN os_agents.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_agents.id IS '坐席 ID';
COMMENT ON COLUMN os_agents.user_ref IS '业务侧用户引用';
COMMENT ON COLUMN os_agents.extension IS '分机号';
COMMENT ON COLUMN os_agents.display_name IS '展示名';
COMMENT ON COLUMN os_agents.video_capable IS '是否支持视频';
COMMENT ON COLUMN os_agents.terminal_type IS '终端类型 webrtc 或 sip';
COMMENT ON COLUMN os_agents.sip_username IS 'SIP 设备账号';
COMMENT ON COLUMN os_agents.enabled IS '是否启用';
COMMENT ON COLUMN os_agents.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_agents.created_at IS '创建时间';
COMMENT ON COLUMN os_agents.updated_at IS '更新时间';

COMMENT ON TABLE os_queue_skills IS '队列与技能关联（配置快照）';
COMMENT ON COLUMN os_queue_skills.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_queue_skills.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_queue_skills.queue_id IS '队列 ID';
COMMENT ON COLUMN os_queue_skills.skill_id IS '技能 ID';

COMMENT ON TABLE os_queue_agents IS '队列与坐席关联（配置快照）';
COMMENT ON COLUMN os_queue_agents.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_queue_agents.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_queue_agents.queue_id IS '队列 ID';
COMMENT ON COLUMN os_queue_agents.agent_id IS '坐席 ID';

COMMENT ON TABLE os_agent_skills IS '坐席与技能关联（配置快照）';
COMMENT ON COLUMN os_agent_skills.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_agent_skills.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_agent_skills.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_agent_skills.skill_id IS '技能 ID';

COMMENT ON TABLE os_did_routes IS '已发布呼入号码路由';
COMMENT ON COLUMN os_did_routes.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_did_routes.id IS '路由 ID';
COMMENT ON COLUMN os_did_routes.trunk_id IS '中继标识';
COMMENT ON COLUMN os_did_routes.normalized_did IS '归一化 DID';
COMMENT ON COLUMN os_did_routes.target_type IS '目标类型 queue ivr reject';
COMMENT ON COLUMN os_did_routes.target_id IS '目标资源 ID';
COMMENT ON COLUMN os_did_routes.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_did_routes.created_at IS '创建时间';
COMMENT ON COLUMN os_did_routes.updated_at IS '更新时间';

COMMENT ON TABLE os_ivr_published_snapshots IS '已发布 IVR 流程快照';
COMMENT ON COLUMN os_ivr_published_snapshots.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_ivr_published_snapshots.flow_id IS 'IVR 流程 ID';
COMMENT ON COLUMN os_ivr_published_snapshots.version IS '流程内版本号';
COMMENT ON COLUMN os_ivr_published_snapshots.payload_json IS '流程定义 JSON';
COMMENT ON COLUMN os_ivr_published_snapshots.config_version IS '所属配置版本号';
COMMENT ON COLUMN os_ivr_published_snapshots.published_at IS '发布时间';

COMMENT ON TABLE os_agent_sessions IS '坐席签入与会话状态（运行态）';
COMMENT ON COLUMN os_agent_sessions.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_agent_sessions.id IS '签入会话 ID';
COMMENT ON COLUMN os_agent_sessions.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_agent_sessions.state IS '坐席状态';
COMMENT ON COLUMN os_agent_sessions.busy_reason IS '示忙原因';
COMMENT ON COLUMN os_agent_sessions.current_call_id IS '当前通话 ID';
COMMENT ON COLUMN os_agent_sessions.pending_checkout IS '通话结束后是否签出';
COMMENT ON COLUMN os_agent_sessions.checked_in_at IS '签入时间';
COMMENT ON COLUMN os_agent_sessions.updated_at IS '状态更新时间';

COMMENT ON TABLE os_agent_session_queues IS '签入坐席所选队列';
COMMENT ON COLUMN os_agent_session_queues.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_agent_session_queues.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_agent_session_queues.queue_id IS '队列 ID';

COMMENT ON TABLE os_agent_state_log IS '坐席状态变更审计日志';
COMMENT ON COLUMN os_agent_state_log.id IS '日志 ID';
COMMENT ON COLUMN os_agent_state_log.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_agent_state_log.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_agent_state_log.from_state IS '原状态';
COMMENT ON COLUMN os_agent_state_log.to_state IS '新状态';
COMMENT ON COLUMN os_agent_state_log.reason IS '变更原因';
COMMENT ON COLUMN os_agent_state_log.call_id IS '关联通话 ID';
COMMENT ON COLUMN os_agent_state_log.created_at IS '记录时间';

COMMENT ON TABLE os_calls IS '呼叫中心会话主表，媒体 Room ID 与 id 一致';
COMMENT ON COLUMN os_calls.id IS '通话 ID 与媒体 Room 一致';
COMMENT ON COLUMN os_calls.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_calls.business_ref IS '业务侧通话引用';
COMMENT ON COLUMN os_calls.metadata IS '扩展元数据 JSON';
COMMENT ON COLUMN os_calls.direction IS '呼叫方向';
COMMENT ON COLUMN os_calls.session_type IS '会话媒介类型';
COMMENT ON COLUMN os_calls.state IS '呼叫状态机状态';
COMMENT ON COLUMN os_calls.version IS '乐观锁版本';
COMMENT ON COLUMN os_calls.config_version IS '路由时使用的配置版本';
COMMENT ON COLUMN os_calls.queue_id IS '关联队列 ID';
COMMENT ON COLUMN os_calls.parent_call_id IS '父通话 ID';
COMMENT ON COLUMN os_calls.priority IS '优先级';
COMMENT ON COLUMN os_calls.caller IS '主叫号码或用户标识';
COMMENT ON COLUMN os_calls.callee IS '被叫号码或用户标识';
COMMENT ON COLUMN os_calls.agent_id IS '已接听坐席 ID';
COMMENT ON COLUMN os_calls.offered_agent IS '当前被邀约坐席 ID';
COMMENT ON COLUMN os_calls.answered_at IS '通话接通时间';
COMMENT ON COLUMN os_calls.created_at IS '创建时间';
COMMENT ON COLUMN os_calls.updated_at IS '更新时间';
COMMENT ON COLUMN os_calls.ended_at IS '结束时间';

COMMENT ON TABLE os_call_legs IS '通话媒体腿（客户、坐席、IVR 等）';
COMMENT ON COLUMN os_call_legs.id IS '通话腿 ID';
COMMENT ON COLUMN os_call_legs.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_call_legs.call_id IS '所属通话 ID';
COMMENT ON COLUMN os_call_legs.type IS '腿类型 sip webrtc playback';
COMMENT ON COLUMN os_call_legs.role IS '腿角色';
COMMENT ON COLUMN os_call_legs.state IS '腿状态';
COMMENT ON COLUMN os_call_legs.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_call_legs.participant_ref IS '参与者引用';
COMMENT ON COLUMN os_call_legs.created_at IS '创建时间';

COMMENT ON TABLE os_bridges IS '通话桥接（双人或会议）';
COMMENT ON COLUMN os_bridges.id IS '桥接 ID';
COMMENT ON COLUMN os_bridges.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_bridges.call_id IS '所属通话 ID';
COMMENT ON COLUMN os_bridges.mode IS '桥接模式 pair 或 conference';
COMMENT ON COLUMN os_bridges.state IS '桥接状态';
COMMENT ON COLUMN os_bridges.participants IS '参与者列表 JSON';
COMMENT ON COLUMN os_bridges.created_at IS '创建时间';
COMMENT ON COLUMN os_bridges.ended_at IS '结束时间';

COMMENT ON TABLE os_routing_sessions IS '呼入路由会话状态';
COMMENT ON COLUMN os_routing_sessions.call_id IS '通话 ID';
COMMENT ON COLUMN os_routing_sessions.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_routing_sessions.state IS '路由状态';
COMMENT ON COLUMN os_routing_sessions.did_route_id IS '匹配的 DID 路由 ID';
COMMENT ON COLUMN os_routing_sessions.queue_id IS '当前队列 ID';
COMMENT ON COLUMN os_routing_sessions.config_version IS '配置版本号';
COMMENT ON COLUMN os_routing_sessions.ivr_flow_id IS '当前 IVR 流程 ID';
COMMENT ON COLUMN os_routing_sessions.created_at IS '创建时间';
COMMENT ON COLUMN os_routing_sessions.updated_at IS '更新时间';
COMMENT ON COLUMN os_routing_sessions.ended_at IS '结束时间';

COMMENT ON TABLE os_queue_entries IS '排队中的通话条目';
COMMENT ON COLUMN os_queue_entries.call_id IS '通话 ID';
COMMENT ON COLUMN os_queue_entries.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_queue_entries.queue_id IS '队列 ID';
COMMENT ON COLUMN os_queue_entries.priority IS '排队优先级';
COMMENT ON COLUMN os_queue_entries.enqueued_at IS '入队时间';
COMMENT ON COLUMN os_queue_entries.deadline_at IS '最长等待截止时间';
COMMENT ON COLUMN os_queue_entries.state IS '排队条目状态';

COMMENT ON TABLE os_acd_attempts IS 'ACD 向坐席派单尝试记录';
COMMENT ON COLUMN os_acd_attempts.id IS '尝试记录 ID';
COMMENT ON COLUMN os_acd_attempts.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_acd_attempts.call_id IS '通话 ID';
COMMENT ON COLUMN os_acd_attempts.queue_id IS '队列 ID';
COMMENT ON COLUMN os_acd_attempts.agent_id IS '目标坐席 ID';
COMMENT ON COLUMN os_acd_attempts.attempt IS '第几次派单';
COMMENT ON COLUMN os_acd_attempts.state IS '尝试状态';
COMMENT ON COLUMN os_acd_attempts.started_at IS '开始时间';
COMMENT ON COLUMN os_acd_attempts.ended_at IS '结束时间';
COMMENT ON COLUMN os_acd_attempts.failure_reason IS '失败原因';

COMMENT ON TABLE os_ivr_sessions IS 'IVR 运行时节点状态';
COMMENT ON COLUMN os_ivr_sessions.call_id IS '通话 ID';
COMMENT ON COLUMN os_ivr_sessions.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_ivr_sessions.flow_id IS 'IVR 流程 ID';
COMMENT ON COLUMN os_ivr_sessions.flow_version IS '流程版本';
COMMENT ON COLUMN os_ivr_sessions.node_id IS '当前节点 ID';
COMMENT ON COLUMN os_ivr_sessions.state IS '节点运行时状态 JSON';
COMMENT ON COLUMN os_ivr_sessions.deadline_at IS '节点超时时间';
COMMENT ON COLUMN os_ivr_sessions.updated_at IS '更新时间';

COMMENT ON TABLE os_commands IS '异步控制命令与幂等记录';
COMMENT ON COLUMN os_commands.id IS '命令 ID';
COMMENT ON COLUMN os_commands.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_commands.call_id IS '关联通话 ID';
COMMENT ON COLUMN os_commands.idempotency_key IS '幂等键';
COMMENT ON COLUMN os_commands.request_hash IS '请求体哈希';
COMMENT ON COLUMN os_commands.type IS '命令类型';
COMMENT ON COLUMN os_commands.status IS '执行状态';
COMMENT ON COLUMN os_commands.result IS '执行结果 JSON';
COMMENT ON COLUMN os_commands.error_code IS '错误码';
COMMENT ON COLUMN os_commands.created_at IS '创建时间';
COMMENT ON COLUMN os_commands.updated_at IS '更新时间';

COMMENT ON TABLE os_call_events IS '通话与平台事件流（可回放）';
COMMENT ON COLUMN os_call_events.id IS '事件全局自增 ID';
COMMENT ON COLUMN os_call_events.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_call_events.call_id IS '关联通话 ID';
COMMENT ON COLUMN os_call_events.seq IS '通话内事件序号';
COMMENT ON COLUMN os_call_events.version IS '关联实体版本';
COMMENT ON COLUMN os_call_events.command_id IS '触发事件的命令 ID';
COMMENT ON COLUMN os_call_events.agent_id IS '目标坐席 ID';
COMMENT ON COLUMN os_call_events.target_only IS '是否仅投递给指定坐席';
COMMENT ON COLUMN os_call_events.type IS '事件类型';
COMMENT ON COLUMN os_call_events.payload IS '事件载荷 JSON';
COMMENT ON COLUMN os_call_events.created_at IS '创建时间';

COMMENT ON TABLE os_cdr IS '交换侧话单';
COMMENT ON COLUMN os_cdr.id IS '话单 ID';
COMMENT ON COLUMN os_cdr.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_cdr.call_id IS '通话 ID';
COMMENT ON COLUMN os_cdr.direction IS '呼叫方向';
COMMENT ON COLUMN os_cdr.queue_id IS '队列 ID';
COMMENT ON COLUMN os_cdr.agent_id IS '坐席 ID';
COMMENT ON COLUMN os_cdr.caller IS '主叫';
COMMENT ON COLUMN os_cdr.callee IS '被叫';
COMMENT ON COLUMN os_cdr.session_type IS '媒介类型';
COMMENT ON COLUMN os_cdr.result IS '通话结果';
COMMENT ON COLUMN os_cdr.started_at IS '开始时间';
COMMENT ON COLUMN os_cdr.answered_at IS '接通时间';
COMMENT ON COLUMN os_cdr.ended_at IS '结束时间';
COMMENT ON COLUMN os_cdr.duration_sec IS '通话时长秒';
COMMENT ON COLUMN os_cdr.wait_sec IS '等待秒数';
COMMENT ON COLUMN os_cdr.video_started_at IS '视频开始时间';
COMMENT ON COLUMN os_cdr.video_upgrade_ok IS '升视频是否成功';
COMMENT ON COLUMN os_cdr.screen_share_count IS '屏幕共享次数';
COMMENT ON COLUMN os_cdr.created_at IS '创建时间';

COMMENT ON TABLE os_recordings IS '录音文件元数据';
COMMENT ON COLUMN os_recordings.id IS '录音记录 ID';
COMMENT ON COLUMN os_recordings.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_recordings.call_id IS '通话 ID';
COMMENT ON COLUMN os_recordings.file_path IS '文件路径';
COMMENT ON COLUMN os_recordings.media_type IS '录制类型';
COMMENT ON COLUMN os_recordings.started_at IS '开始时间';
COMMENT ON COLUMN os_recordings.ended_at IS '结束时间';
COMMENT ON COLUMN os_recordings.retain_until IS '保留截止时间';
COMMENT ON COLUMN os_recordings.file_size IS '文件大小字节';
COMMENT ON COLUMN os_recordings.created_at IS '创建时间';

COMMENT ON TABLE os_queue_dispatch_cursor IS '按队列记录轮询派单游标（公平轮转）';
COMMENT ON COLUMN os_queue_dispatch_cursor.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_queue_dispatch_cursor.queue_id IS '队列 ID';
COMMENT ON COLUMN os_queue_dispatch_cursor.last_agent_id IS '上一轮派单到的坐席 ID';

COMMENT ON TABLE os_business_actions IS 'IVR 业务判断节点挂起的决策请求';
COMMENT ON COLUMN os_business_actions.id IS '决策请求 ID';
COMMENT ON COLUMN os_business_actions.application_id IS '租户应用 ID';
COMMENT ON COLUMN os_business_actions.call_id IS '通话 ID';
COMMENT ON COLUMN os_business_actions.node_id IS 'IVR 节点 ID';
COMMENT ON COLUMN os_business_actions.action IS '业务动作类型';
COMMENT ON COLUMN os_business_actions.outcomes IS '允许的结果分支 JSON';
COMMENT ON COLUMN os_business_actions.deadline_at IS '等待业务方响应的截止时间';
COMMENT ON COLUMN os_business_actions.status IS '状态 pending completed expired';
COMMENT ON COLUMN os_business_actions.outcome IS '业务方返回的分支结果';
COMMENT ON COLUMN os_business_actions.created_at IS '创建时间';
COMMENT ON COLUMN os_business_actions.updated_at IS '更新时间';

COMMENT ON TABLE os_applications IS '已登记的业务系统集成方（integrator）';
COMMENT ON COLUMN os_applications.id IS '租户 application_id';
COMMENT ON COLUMN os_applications.secret_hash IS 'integrator secret bcrypt 哈希';
COMMENT ON COLUMN os_applications.secret IS 'integrator secret 明文（供 callback Bearer）';
COMMENT ON COLUMN os_applications.events_callback_url IS '事件 HTTP 推送地址';
COMMENT ON COLUMN os_applications.event_retention_days IS 'os_call_events 保留天数';
COMMENT ON COLUMN os_applications.max_concurrent_calls IS '并发通话上限';
COMMENT ON COLUMN os_applications.enabled IS '是否启用';
COMMENT ON COLUMN os_applications.created_at IS '创建时间';
COMMENT ON COLUMN os_applications.updated_at IS '更新时间';

COMMENT ON TABLE os_integrator_event_deliveries IS 'integrator 事件 callback 投递状态';
COMMENT ON COLUMN os_integrator_event_deliveries.application_id IS '租户 application_id';
COMMENT ON COLUMN os_integrator_event_deliveries.event_id IS 'os_call_events.id';
COMMENT ON COLUMN os_integrator_event_deliveries.status IS 'pending success failed';
COMMENT ON COLUMN os_integrator_event_deliveries.attempts IS '已尝试次数';
COMMENT ON COLUMN os_integrator_event_deliveries.next_retry_at IS '下次重试时间';
COMMENT ON COLUMN os_integrator_event_deliveries.last_error IS '最近错误';
COMMENT ON COLUMN os_integrator_event_deliveries.updated_at IS '更新时间';
