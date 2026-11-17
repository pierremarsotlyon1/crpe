ALTER TABLE oral_sessions ADD COLUMN feedback_source text NOT NULL DEFAULT '' CHECK(feedback_source IN ('','teacher','ai'));
UPDATE oral_sessions SET feedback_source='teacher' WHERE status='reviewed';
DROP TABLE storage_cleanup;
