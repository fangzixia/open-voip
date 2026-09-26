DROP INDEX IF EXISTS idx_oc_users_email_ci;
ALTER TABLE oc_users DROP COLUMN IF EXISTS email;
ALTER TABLE oc_users ADD COLUMN employee_no VARCHAR(64) NOT NULL DEFAULT '';
UPDATE oc_users SET display_name = username WHERE display_name IS NULL OR display_name = '';
CREATE UNIQUE INDEX idx_oc_users_employee_no ON oc_users (employee_no) WHERE employee_no <> '';
COMMENT ON COLUMN oc_users.username IS '登录名';
COMMENT ON COLUMN oc_users.display_name IS '用户名';
COMMENT ON COLUMN oc_users.employee_no IS '工号';
