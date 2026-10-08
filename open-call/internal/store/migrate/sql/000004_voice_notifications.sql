CREATE TABLE oc_voice_notifications (
 id VARCHAR(36) PRIMARY KEY,
 initiator_id VARCHAR(36) NOT NULL,
 request_key TEXT NOT NULL,
 destination TEXT NOT NULL,
 trunk_id TEXT NOT NULL DEFAULT '',
 asset_id TEXT NOT NULL,
 state TEXT NOT NULL DEFAULT 'queued',
 leg_id VARCHAR(36) NOT NULL DEFAULT '',
 playback_id VARCHAR(36) NOT NULL DEFAULT '',
 last_error TEXT NOT NULL DEFAULT '',
 deadline TIMESTAMPTZ NOT NULL,
 locked_until TIMESTAMPTZ,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 UNIQUE (initiator_id, request_key)
);
CREATE INDEX oc_voice_notifications_pending ON oc_voice_notifications (state, locked_until);
