-- os_calls / 事件 / 派单游标上的坐席 ID 统一为 UUID，空串改为 NULL。
ALTER TABLE os_calls ALTER COLUMN agent_id DROP DEFAULT;
ALTER TABLE os_calls ALTER COLUMN offered_agent DROP DEFAULT;
UPDATE os_calls SET agent_id = NULL WHERE agent_id = '';
UPDATE os_calls SET offered_agent = NULL WHERE offered_agent = '';
ALTER TABLE os_calls ALTER COLUMN agent_id TYPE UUID USING agent_id::uuid;
ALTER TABLE os_calls ALTER COLUMN offered_agent TYPE UUID USING offered_agent::uuid;

ALTER TABLE os_call_events ALTER COLUMN agent_id DROP DEFAULT;
UPDATE os_call_events SET agent_id = NULL WHERE agent_id = '';
ALTER TABLE os_call_events ALTER COLUMN agent_id TYPE UUID USING agent_id::uuid;

ALTER TABLE os_queue_dispatch_cursor ALTER COLUMN last_agent_id DROP DEFAULT;
UPDATE os_queue_dispatch_cursor SET last_agent_id = NULL WHERE last_agent_id = '';
ALTER TABLE os_queue_dispatch_cursor ALTER COLUMN last_agent_id TYPE UUID USING last_agent_id::uuid;
