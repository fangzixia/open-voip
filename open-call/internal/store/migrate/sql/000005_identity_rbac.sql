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
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles(id) ON DELETE CASCADE,
  permission_code VARCHAR(96) NOT NULL REFERENCES oc_permissions(code) ON DELETE CASCADE,
  PRIMARY KEY (role_id, permission_code)
);
CREATE TABLE oc_user_roles (
  user_id UUID NOT NULL REFERENCES oc_users(id) ON DELETE CASCADE,
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles(id) ON DELETE CASCADE,
  PRIMARY KEY (user_id, role_id)
);
CREATE INDEX idx_oc_user_roles_role ON oc_user_roles(role_id);
CREATE TABLE oc_oidc_identities (
  issuer VARCHAR(512) NOT NULL,
  subject VARCHAR(255) NOT NULL,
  user_id UUID NOT NULL REFERENCES oc_users(id) ON DELETE CASCADE,
  PRIMARY KEY (issuer, subject),
  UNIQUE (user_id, issuer)
);
CREATE TABLE oc_oidc_group_roles (
  group_name VARCHAR(255) NOT NULL,
  role_id VARCHAR(64) NOT NULL REFERENCES oc_roles(id) ON DELETE CASCADE,
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
  user_id UUID NOT NULL REFERENCES oc_users(id) ON DELETE CASCADE,
  provider_refresh_token TEXT,
  provider_subject VARCHAR(255) NOT NULL,
  expires_at TIMESTAMPTZ NOT NULL
);
ALTER TABLE oc_auth_sessions ADD COLUMN provider VARCHAR(16) NOT NULL DEFAULT 'local';
ALTER TABLE oc_auth_sessions ADD COLUMN provider_refresh_token TEXT;
ALTER TABLE oc_auth_sessions ADD COLUMN provider_subject VARCHAR(255);

INSERT INTO oc_roles(id,name,built_in) VALUES
  ('admin','管理员',TRUE),('supervisor','班长',TRUE),('agent','坐席',TRUE);
INSERT INTO oc_permissions(code,description) VALUES
  ('users.read','查看用户'),('users.create','创建用户'),('users.update','修改用户'),('users.delete','删除用户'),
  ('users.credentials','管理密码'),('users.sessions','撤销会话'),('roles.read','查看角色'),('roles.write','管理角色'),
  ('identity.read','查看身份映射'),('identity.write','管理身份映射'),
  ('queues.read','查看队列'),('queues.write','管理队列'),('skills.read','查看技能'),('skills.write','管理技能'),
  ('agents.read','查看坐席'),('agents.self','操作本人坐席'),('agents.force_checkout','强制签出坐席'),
  ('calls.read','查看通话'),('calls.operate','操作通话'),('calls.listen','监听通话'),('calls.wrap_up','填写通话小结'),
  ('cdr.read','查看话单'),('cdr.export','导出话单'),('recordings.read','查看录音'),
  ('recordings.download','下载录音'),('recordings.qa','管理质检标记'),('recordings.purge','清理录音'),('reports.read','查看报表'),
  ('ivr.read','查看 IVR'),('ivr.write','管理 IVR'),('webhooks.read','查看 Webhook'),
  ('webhooks.write','管理 Webhook'),('audit.read','查看审计'),('dids.read','查看呼入号码'),
  ('dids.write','管理呼入号码'),('config.read','导出配置'),('config.write','导入配置'),
  ('guest.issue','签发访客会话'),('status.read','查看运行状态');
INSERT INTO oc_role_permissions(role_id, permission_code)
  SELECT 'admin', code FROM oc_permissions;
INSERT INTO oc_role_permissions(role_id, permission_code)
  SELECT 'supervisor', code FROM oc_permissions WHERE code IN
  ('queues.read','skills.read','agents.read','agents.self','agents.force_checkout','calls.read','calls.operate',
   'calls.listen','calls.wrap_up','cdr.read','recordings.read','recordings.download','recordings.qa','reports.read','ivr.read',
   'dids.read','guest.issue','status.read');
INSERT INTO oc_role_permissions(role_id, permission_code)
  SELECT 'agent', code FROM oc_permissions WHERE code IN
  ('queues.read','agents.read','agents.self','calls.read','calls.operate','calls.wrap_up','guest.issue');
INSERT INTO oc_user_roles(user_id,role_id)
  SELECT id, role FROM oc_users WHERE role IN ('admin','supervisor','agent');
