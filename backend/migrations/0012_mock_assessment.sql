ALTER TABLE mock_sessions ADD COLUMN needs_review boolean NOT NULL DEFAULT false;
CREATE INDEX mock_sessions_teacher_queue ON mock_sessions(started_at) WHERE needs_review;
-- Cloned teacher-approved source documents reference immutable S3 bytes.
ALTER TABLE content_documents DROP CONSTRAINT content_documents_object_key_key;
CREATE INDEX content_documents_asset ON content_documents(object_key);
