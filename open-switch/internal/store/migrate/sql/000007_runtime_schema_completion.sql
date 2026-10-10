-- Complete the runtime columns omitted from the historical baseline. This
-- additive migration also applies to existing databases without rewriting data.
ALTER TABLE os_calls ADD COLUMN IF NOT EXISTS event_seq BIGINT NOT NULL DEFAULT 0;
ALTER TABLE os_queues ADD COLUMN IF NOT EXISTS post_call_ivr_flow_id UUID;
