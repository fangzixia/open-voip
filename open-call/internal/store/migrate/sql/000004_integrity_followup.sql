-- Complete local references that are safe within the open-call data boundary.

DELETE FROM oc_agent_state_log x WHERE NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id);
DELETE FROM oc_guest_sessions x WHERE NOT EXISTS (SELECT 1 FROM oc_queues q WHERE q.id=x.queue_id);
UPDATE oc_cdr x SET queue_id=NULL WHERE queue_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM oc_queues q WHERE q.id=x.queue_id);
UPDATE oc_cdr x SET agent_id=NULL WHERE agent_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id);
DELETE FROM oc_qa_marks x WHERE NOT EXISTS (SELECT 1 FROM oc_cdr c WHERE c.call_id=x.call_id);
UPDATE oc_qa_marks x SET created_by=NULL WHERE created_by IS NOT NULL AND NOT EXISTS (SELECT 1 FROM oc_users u WHERE u.id=x.created_by);
DELETE FROM oc_call_wrap_ups x WHERE NOT EXISTS (SELECT 1 FROM oc_cdr c WHERE c.call_id=x.call_id) OR NOT EXISTS (SELECT 1 FROM oc_agents a WHERE a.id=x.agent_id);

ALTER TABLE oc_agent_state_log DROP CONSTRAINT IF EXISTS fk_oc_agent_state_log_agent;
ALTER TABLE oc_agent_state_log ADD CONSTRAINT fk_oc_agent_state_log_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE CASCADE;
ALTER TABLE oc_guest_sessions DROP CONSTRAINT IF EXISTS fk_oc_guest_sessions_queue;
ALTER TABLE oc_guest_sessions ADD CONSTRAINT fk_oc_guest_sessions_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE RESTRICT;
ALTER TABLE oc_cdr DROP CONSTRAINT IF EXISTS fk_oc_cdr_queue;
ALTER TABLE oc_cdr ADD CONSTRAINT fk_oc_cdr_queue FOREIGN KEY(queue_id) REFERENCES oc_queues(id) ON DELETE SET NULL;
ALTER TABLE oc_cdr DROP CONSTRAINT IF EXISTS fk_oc_cdr_agent;
ALTER TABLE oc_cdr ADD CONSTRAINT fk_oc_cdr_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE SET NULL;
ALTER TABLE oc_qa_marks DROP CONSTRAINT IF EXISTS fk_oc_qa_marks_call;
ALTER TABLE oc_qa_marks ADD CONSTRAINT fk_oc_qa_marks_call FOREIGN KEY(call_id) REFERENCES oc_cdr(call_id) ON DELETE CASCADE;
ALTER TABLE oc_qa_marks DROP CONSTRAINT IF EXISTS fk_oc_qa_marks_user;
ALTER TABLE oc_qa_marks ADD CONSTRAINT fk_oc_qa_marks_user FOREIGN KEY(created_by) REFERENCES oc_users(id) ON DELETE SET NULL;
ALTER TABLE oc_call_wrap_ups DROP CONSTRAINT IF EXISTS fk_oc_call_wrap_ups_call;
ALTER TABLE oc_call_wrap_ups ADD CONSTRAINT fk_oc_call_wrap_ups_call FOREIGN KEY(call_id) REFERENCES oc_cdr(call_id) ON DELETE CASCADE;
ALTER TABLE oc_call_wrap_ups DROP CONSTRAINT IF EXISTS fk_oc_call_wrap_ups_agent;
ALTER TABLE oc_call_wrap_ups ADD CONSTRAINT fk_oc_call_wrap_ups_agent FOREIGN KEY(agent_id) REFERENCES oc_agents(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_oc_cdr_agent_started ON oc_cdr(agent_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_oc_cdr_direction_started ON oc_cdr(direction, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_oc_cdr_result_started ON oc_cdr(result, started_at DESC);
