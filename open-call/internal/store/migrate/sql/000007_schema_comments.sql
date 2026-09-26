-- 为已部署的业务库补齐身份认证相关表和字段的中文数据库注释。
COMMENT ON COLUMN oc_users.email IS '用户邮箱，用于外部身份绑定与账号识别';
COMMENT ON COLUMN oc_jwt_revocations.jti IS '已撤销令牌的唯一标识';

COMMENT ON COLUMN oc_auth_sessions.provider IS '登录来源，local 表示本地认证';
COMMENT ON COLUMN oc_auth_sessions.provider_refresh_token IS '外部身份提供方的刷新令牌';
COMMENT ON COLUMN oc_auth_sessions.provider_subject IS '外部身份提供方的用户标识';

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

COMMENT ON TABLE oc_schema_migrations IS '业务库已执行的数据库迁移版本';
COMMENT ON COLUMN oc_schema_migrations.version IS '迁移版本号';
COMMENT ON COLUMN oc_schema_migrations.name IS '迁移文件名称';
COMMENT ON COLUMN oc_schema_migrations.applied_at IS '迁移执行时间';
