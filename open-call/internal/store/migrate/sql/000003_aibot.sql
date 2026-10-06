CREATE TABLE IF NOT EXISTS oc_aibot_usage (
    id VARCHAR(36) PRIMARY KEY,
    call_id VARCHAR(36) NOT NULL,
    agent_id VARCHAR(36),
    queue_id VARCHAR(36),
    input_audio_ms BIGINT NOT NULL DEFAULT 0,
    output_audio_ms BIGINT NOT NULL DEFAULT 0,
    started_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_oc_aibot_usage_call ON oc_aibot_usage (call_id);

CREATE TABLE IF NOT EXISTS oc_aibot_queue_profiles (
    id VARCHAR(36) PRIMARY KEY,
    queue_id VARCHAR(36) NOT NULL UNIQUE,
    system_prompt TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
