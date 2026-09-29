CREATE TABLE os_queue_dispatch_cursor (
 application_id TEXT NOT NULL,
 queue_id UUID NOT NULL,
 last_agent_id TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(application_id, queue_id)
);
