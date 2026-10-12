-- Align recorded checksum with embedded SQL in current GHCR open-switch image.
-- Applied schema on production already matches; only metadata update.
UPDATE os_schema_migrations
SET checksum = '285012f419e5bb7a8e9123f0b6da503636f8ec325d9c6ed1d59993d2edf13a6c'
WHERE version = 2 AND name = '000002_schema_fixes.sql';
