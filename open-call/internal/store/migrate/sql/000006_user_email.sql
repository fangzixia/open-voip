ALTER TABLE oc_users ADD COLUMN email VARCHAR(320) NOT NULL DEFAULT '';
CREATE UNIQUE INDEX idx_oc_users_email_ci ON oc_users (LOWER(email)) WHERE email <> '';
