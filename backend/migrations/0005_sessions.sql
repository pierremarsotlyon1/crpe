ALTER TABLE content ADD COLUMN duration_minutes integer NOT NULL DEFAULT 15, ADD COLUMN prep_minutes integer NOT NULL DEFAULT 10, ADD COLUMN points numeric NOT NULL DEFAULT 1;
UPDATE content SET duration_minutes=180 WHERE kind='mock';
CREATE OR REPLACE VIEW published_content AS WITH RECURSIVE visible AS (
 SELECT * FROM content WHERE parent_id IS NULL AND status='validated' AND NOT archived
 UNION ALL SELECT c.* FROM content c JOIN visible v ON c.parent_id=v.id WHERE c.status='validated' AND NOT c.archived
) SELECT * FROM visible;
CREATE TABLE mock_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
 subject_id uuid NOT NULL REFERENCES content, started_at timestamptz NOT NULL DEFAULT now(),deadline timestamptz NOT NULL,
 questions jsonb NOT NULL, answers jsonb NOT NULL DEFAULT '{}',results jsonb NOT NULL DEFAULT '[]',
 completed boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX mock_active ON mock_sessions(user_id,subject_id) WHERE NOT completed;
CREATE TABLE oral_sessions (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(),user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
 subject_id uuid NOT NULL REFERENCES content, snapshot jsonb NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 status text NOT NULL DEFAULT 'active' CHECK(status IN('active','pending_review','reviewed')),
 transcript text NOT NULL DEFAULT '',answers text[] NOT NULL DEFAULT '{}',feedback jsonb NOT NULL DEFAULT '[]'
);
CREATE INDEX oral_user ON oral_sessions(user_id,created_at DESC);
