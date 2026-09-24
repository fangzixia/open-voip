-- open-call production hardening: identity lifecycle, durable jobs and local referential integrity.

ALTER TABLE oc_users ADD COLUMN IF NOT EXISTS auth_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE oc_users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS oc_auth_sessions (
  id UUID PRIMARY KEY,
  user_id UUID NOT NULL,
  refresh_jti VARCHAR(64) NOT NULL,
  user_agent VARCHAR(256),
  remote_ip VARCHAR(64),
  created_at TIMESTAMPTZ NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL,
  last_seen_at TIMESTAMPTZ NOT NULL,
  revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_oc_auth_sessions_user_id ON oc_auth_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_oc_auth_sessions_expires_at ON oc_auth_sessions(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_auth_sessions_refresh_jti ON oc_auth_sessions(refresh_jti);

ALTER TABLE oc_guest_sessions ADD COLUMN IF NOT EXISTS priority BIGINT NOT NULL DEFAULT 0;
ALTER TABLE oc_guest_sessions ADD COLUMN IF NOT EXISTS consumed_at TIMESTAMPTZ;

ALTER TABLE oc_webhook_deliveries ADD COLUMN IF NOT EXISTS event_id UUID;
ALTER TABLE oc_webhook_deliveries ADD COLUMN IF NOT EXISTS next_attempt_at TIMESTAMPTZ;
ALTER TABLE oc_webhook_deliveries ADD COLUMN IF NOT EXISTS locked_at TIMESTAMPTZ;
ALTER TABLE oc_webhook_deliveries ADD COLUMN IF NOT EXISTS dead_letter_at TIMESTAMPTZ;
UPDATE oc_webhook_deliveries SET event_id = id WHERE event_id IS NULL;
ALTER TABLE oc_webhook_deliveries ALTER COLUMN event_id SET NOT NULL;
UPDATE oc_webhook_deliveries SET next_attempt_at = COALESCE(updated_at, created_at, NOW()) WHERE next_attempt_at IS NULL;
ALTER TABLE oc_webhook_deliveries ALTER COLUMN next_attempt_at SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_webhook_delivery_event_subscription ON oc_webhook_deliveries(subscription_id, event_id);
CREATE INDEX IF NOT EXISTS idx_oc_webhook_delivery_due ON oc_webhook_deliveries(status, next_attempt_at);

ALTER TABLE oc_audit_logs ADD COLUMN IF NOT EXISTS request_id VARCHAR(128);
ALTER TABLE oc_audit_logs ADD COLUMN IF NOT EXISTS remote_ip VARCHAR(64);
ALTER TABLE oc_audit_logs ADD COLUMN IF NOT EXISTS outcome VARCHAR(16) NOT NULL DEFAULT 'success';
CREATE INDEX IF NOT EXISTS idx_oc_audit_logs_resource ON oc_audit_logs(resource);
CREATE INDEX IF NOT EXISTS idx_oc_audit_logs_outcome ON oc_audit_logs(outcome);

ALTER TABLE oc_recordings ADD COLUMN IF NOT EXISTS purge_error TEXT;
ALTER TABLE oc_recordings ADD COLUMN IF NOT EXISTS purge_retry_at TIMESTAMPTZ;

ALTER TABLE oc_call_wrap_ups ADD COLUMN IF NOT EXISTS disposition_code VARCHAR(64);
ALTER TABLE oc_call_wrap_ups ADD COLUMN IF NOT EXISTS tags_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE oc_call_wrap_ups ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

ALTER TABLE oc_qa_marks ADD COLUMN IF NOT EXISTS score BIGINT;

COMMENT ON COLUMN oc_users.auth_version IS '认证版本，密码、角色或禁用状态变化时递增';
COMMENT ON COLUMN oc_users.must_change_password IS '是否要求下次登录后修改临时密码';
COMMENT ON TABLE oc_auth_sessions IS '用户登录会话，可按设备单独撤销';
COMMENT ON COLUMN oc_auth_sessions.id IS '登录会话主键 UUID';
COMMENT ON COLUMN oc_auth_sessions.user_id IS '关联用户 ID';
COMMENT ON COLUMN oc_auth_sessions.refresh_jti IS '当前刷新令牌唯一标识';
COMMENT ON COLUMN oc_auth_sessions.user_agent IS '登录客户端 User-Agent';
COMMENT ON COLUMN oc_auth_sessions.remote_ip IS '登录来源 IP';
COMMENT ON COLUMN oc_auth_sessions.created_at IS '会话创建时间';
COMMENT ON COLUMN oc_auth_sessions.expires_at IS '会话过期时间';
COMMENT ON COLUMN oc_auth_sessions.last_seen_at IS '最后刷新时间';
COMMENT ON COLUMN oc_auth_sessions.revoked_at IS '会话撤销时间';
COMMENT ON COLUMN oc_guest_sessions.priority IS '由受信任签发方设置的队列优先级';
COMMENT ON COLUMN oc_guest_sessions.consumed_at IS '访客令牌首次成功入队时间';
COMMENT ON COLUMN oc_webhook_deliveries.event_id IS '业务事件唯一标识';
COMMENT ON COLUMN oc_webhook_deliveries.next_attempt_at IS '下次计划投递时间';
COMMENT ON COLUMN oc_webhook_deliveries.locked_at IS '后台任务租约获取时间';
COMMENT ON COLUMN oc_webhook_deliveries.dead_letter_at IS '进入死信队列时间';
COMMENT ON COLUMN oc_audit_logs.request_id IS '关联 HTTP 请求 ID';
COMMENT ON COLUMN oc_audit_logs.remote_ip IS '操作来源 IP';
COMMENT ON COLUMN oc_audit_logs.outcome IS '操作结果 success 或 failure';
COMMENT ON COLUMN oc_recordings.purge_error IS '最近一次文件清理失败原因';
COMMENT ON COLUMN oc_recordings.purge_retry_at IS '录音文件下次清理重试时间';
COMMENT ON COLUMN oc_call_wrap_ups.disposition_code IS '通话处置码';
COMMENT ON COLUMN oc_call_wrap_ups.tags_json IS '通话标签 JSON 数组';
COMMENT ON COLUMN oc_call_wrap_ups.completed_at IS '事后处理完成时间';
COMMENT ON COLUMN oc_qa_marks.score IS '质检评分，范围 0 至 100';

DELETE FROM oc_agent_skills x WHERE NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id) OR NOT EXISTS (SELECT 1 FROM oc_skills s WHERE s.id=x.skill_id);
DELETE FROM oc_queue_agents x WHERE NOT EXISTS (SELECT 1 FROM oc_queues q WHERE q.id=x.queue_id) OR NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id);
DELETE FROM oc_queue_skills x WHERE NOT EXISTS (SELECT 1 FROM oc_queues q WHERE q.id=x.queue_id) OR NOT EXISTS (SELECT 1 FROM oc_skills s WHERE s.id=x.skill_id);
DELETE FROM oc_agent_session_queues x WHERE NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id) OR NOT EXISTS (SELECT 1 FROM oc_queues q WHERE q.id=x.queue_id);
DELETE FROM oc_agent_sessions x WHERE NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id);
DELETE FROM oc_ivr_published_snapshots x WHERE NOT EXISTS (SELECT 1 FROM oc_ivr_flows f WHERE f.id=x.flow_id);
DELETE FROM oc_webhook_deliveries x WHERE NOT EXISTS (SELECT 1 FROM oc_webhook_subscriptions s WHERE s.id=x.subscription_id);
DELETE FROM oc_auth_sessions x WHERE NOT EXISTS (SELECT 1 FROM oc_users u WHERE u.id=x.user_id);
UPDATE oc_queues q SET overflow_queue_id=NULL WHERE overflow_queue_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM oc_queues q2 WHERE q2.id=q.overflow_queue_id);
UPDATE oc_queues q SET ivr_flow_id=NULL WHERE ivr_flow_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM oc_ivr_flows f WHERE f.id=q.ivr_flow_id);

ALTER TABLE oc_agents DROP CONSTRAINT IF EXISTS fk_oc_agents_user;
ALTER TABLE oc_agents ADD CONSTRAINT fk_oc_agents_user FOREIGN KEY(user_id) REFERENCES oc_users(id) ON DELETE CASCADE;
ALTER TABLE oc_agent_skills DROP CONSTRAINT IF EXISTS fk_oc_agent_skills_agent;
ALTER TABLE oc_agent_skills ADD CONSTRAINT fk_oc_agent_skills_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE CASCADE;
ALTER TABLE oc_agent_skills DROP CONSTRAINT IF EXISTS fk_oc_agent_skills_skill;
ALTER TABLE oc_agent_skills ADD CONSTRAINT fk_oc_agent_skills_skill FOREIGN KEY(skill_id) REFERENCES oc_skills(id) ON DELETE CASCADE;
ALTER TABLE oc_queue_agents DROP CONSTRAINT IF EXISTS fk_oc_queue_agents_queue;
ALTER TABLE oc_queue_agents ADD CONSTRAINT fk_oc_queue_agents_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE CASCADE;
ALTER TABLE oc_queue_agents DROP CONSTRAINT IF EXISTS fk_oc_queue_agents_agent;
ALTER TABLE oc_queue_agents ADD CONSTRAINT fk_oc_queue_agents_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE CASCADE;
ALTER TABLE oc_queue_skills DROP CONSTRAINT IF EXISTS fk_oc_queue_skills_queue;
ALTER TABLE oc_queue_skills ADD CONSTRAINT fk_oc_queue_skills_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE CASCADE;
ALTER TABLE oc_queue_skills DROP CONSTRAINT IF EXISTS fk_oc_queue_skills_skill;
ALTER TABLE oc_queue_skills ADD CONSTRAINT fk_oc_queue_skills_skill FOREIGN KEY(skill_id) REFERENCES oc_skills(id) ON DELETE CASCADE;
ALTER TABLE oc_agent_sessions DROP CONSTRAINT IF EXISTS fk_oc_agent_sessions_agent;
ALTER TABLE oc_agent_sessions ADD CONSTRAINT fk_oc_agent_sessions_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE CASCADE;
ALTER TABLE oc_agent_session_queues DROP CONSTRAINT IF EXISTS fk_oc_agent_session_queues_agent;
ALTER TABLE oc_agent_session_queues ADD CONSTRAINT fk_oc_agent_session_queues_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE CASCADE;
ALTER TABLE oc_agent_session_queues DROP CONSTRAINT IF EXISTS fk_oc_agent_session_queues_queue;
ALTER TABLE oc_agent_session_queues ADD CONSTRAINT fk_oc_agent_session_queues_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE CASCADE;
ALTER TABLE oc_did_routes DROP CONSTRAINT IF EXISTS fk_oc_did_routes_queue;
ALTER TABLE oc_did_routes ADD CONSTRAINT fk_oc_did_routes_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE RESTRICT;
ALTER TABLE oc_ivr_published_snapshots DROP CONSTRAINT IF EXISTS fk_oc_ivr_snapshots_flow;
ALTER TABLE oc_ivr_published_snapshots ADD CONSTRAINT fk_oc_ivr_snapshots_flow FOREIGN KEY(flow_id) REFERENCES oc_ivr_flows(id) ON DELETE CASCADE;
ALTER TABLE oc_webhook_deliveries DROP CONSTRAINT IF EXISTS fk_oc_webhook_deliveries_subscription;
ALTER TABLE oc_webhook_deliveries ADD CONSTRAINT fk_oc_webhook_deliveries_subscription FOREIGN KEY(subscription_id) REFERENCES oc_webhook_subscriptions(id) ON DELETE CASCADE;
ALTER TABLE oc_auth_sessions DROP CONSTRAINT IF EXISTS fk_oc_auth_sessions_user;
ALTER TABLE oc_auth_sessions ADD CONSTRAINT fk_oc_auth_sessions_user FOREIGN KEY(user_id) REFERENCES oc_users(id) ON DELETE CASCADE;
ALTER TABLE oc_queues DROP CONSTRAINT IF EXISTS fk_oc_queues_overflow_queue;
ALTER TABLE oc_queues ADD CONSTRAINT fk_oc_queues_overflow_queue FOREIGN KEY(overflow_queue_id) REFERENCES oc_queues(id) ON DELETE SET NULL;
ALTER TABLE oc_queues DROP CONSTRAINT IF EXISTS fk_oc_queues_ivr_flow;
ALTER TABLE oc_queues ADD CONSTRAINT fk_oc_queues_ivr_flow FOREIGN KEY(ivr_flow_id) REFERENCES oc_ivr_flows(id) ON DELETE SET NULL;

DROP INDEX IF EXISTS idx_ivr_flow_version;
CREATE UNIQUE INDEX IF NOT EXISTS idx_ivr_flow_version ON oc_ivr_published_snapshots(flow_id, version);
CREATE UNIQUE INDEX IF NOT EXISTS idx_oc_queues_name ON oc_queues(name);

ALTER TABLE oc_users DROP CONSTRAINT IF EXISTS chk_oc_users_role;
ALTER TABLE oc_users ADD CONSTRAINT chk_oc_users_role CHECK (role IN ('admin','supervisor','agent'));
ALTER TABLE oc_agents DROP CONSTRAINT IF EXISTS chk_oc_agents_terminal_type;
ALTER TABLE oc_agents ADD CONSTRAINT chk_oc_agents_terminal_type CHECK (terminal_type IN ('webrtc','sip'));
ALTER TABLE oc_agent_sessions DROP CONSTRAINT IF EXISTS chk_oc_agent_sessions_state;
ALTER TABLE oc_agent_sessions ADD CONSTRAINT chk_oc_agent_sessions_state CHECK (state IN ('offline','idle','busy','ringing','on_call','acw'));
ALTER TABLE oc_queues DROP CONSTRAINT IF EXISTS chk_oc_queues_strategy;
ALTER TABLE oc_queues ADD CONSTRAINT chk_oc_queues_strategy CHECK (dispatch_strategy IN ('longest_idle','round_robin'));
ALTER TABLE oc_queues DROP CONSTRAINT IF EXISTS chk_oc_queues_recording_policy;
ALTER TABLE oc_queues ADD CONSTRAINT chk_oc_queues_recording_policy CHECK (recording_policy IN ('off','audio','video_composite'));
ALTER TABLE oc_guest_sessions DROP CONSTRAINT IF EXISTS chk_oc_guest_media;
ALTER TABLE oc_guest_sessions ADD CONSTRAINT chk_oc_guest_media CHECK (allowed_media IN ('audio','video'));
ALTER TABLE oc_guest_sessions DROP CONSTRAINT IF EXISTS chk_oc_guest_priority;
ALTER TABLE oc_guest_sessions ADD CONSTRAINT chk_oc_guest_priority CHECK (priority BETWEEN 0 AND 10);
ALTER TABLE oc_webhook_deliveries DROP CONSTRAINT IF EXISTS chk_oc_webhook_status;
ALTER TABLE oc_webhook_deliveries ADD CONSTRAINT chk_oc_webhook_status CHECK (status IN ('pending','processing','success','dead_letter'));
ALTER TABLE oc_qa_marks DROP CONSTRAINT IF EXISTS chk_oc_qa_offset;
ALTER TABLE oc_qa_marks ADD CONSTRAINT chk_oc_qa_offset CHECK (offset_sec >= 0);
ALTER TABLE oc_qa_marks DROP CONSTRAINT IF EXISTS chk_oc_qa_score;
ALTER TABLE oc_qa_marks ADD CONSTRAINT chk_oc_qa_score CHECK (score IS NULL OR score BETWEEN 0 AND 100);
