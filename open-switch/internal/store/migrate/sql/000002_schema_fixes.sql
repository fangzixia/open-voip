DROP INDEX IF EXISTS idx_os_call_events_id;

ALTER TABLE os_agent_sessions DROP CONSTRAINT IF EXISTS os_agent_sessions_state_check;
ALTER TABLE os_agent_sessions ADD CONSTRAINT os_agent_sessions_state_check
  CHECK (state IN ('offline', 'idle', 'busy', 'ringing', 'on_call', 'acw'));
