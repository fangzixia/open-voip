ALTER TABLE os_recordings
 ADD COLUMN recording_semantics varchar(64) NOT NULL DEFAULT 'legacy',
 ADD COLUMN channels integer NOT NULL DEFAULT 0,
 ADD COLUMN duration_samples bigint NOT NULL DEFAULT 0,
 ADD COLUMN status varchar(32) NOT NULL DEFAULT 'completed',
 ADD COLUMN failure_reason text NOT NULL DEFAULT '',
 ADD COLUMN sample_rate_hz integer NOT NULL DEFAULT 0,
 ADD COLUMN leg_paths jsonb NOT NULL DEFAULT '{}';
UPDATE os_recordings SET status='recording' WHERE ended_at IS NULL;
-- Active queue profiles are migrated by publishing a new configuration version
-- at startup. Historical immutable versions and their checksums are preserved.
