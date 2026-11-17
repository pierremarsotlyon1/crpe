CREATE TABLE users (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email text NOT NULL UNIQUE,
  role text NOT NULL CHECK (role IN ('student','admin')),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE auth_links (
  token_hash bytea PRIMARY KEY, email text NOT NULL, client text NOT NULL CHECK (client IN ('web','mobile')),
  expires_at timestamptz NOT NULL
);
CREATE INDEX auth_links_email ON auth_links(email);
CREATE TABLE sessions (
  token_hash bytea PRIMARY KEY, user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user ON sessions(user_id);
CREATE TABLE rate_limits (
  key bytea NOT NULL, bucket bigint NOT NULL, count integer NOT NULL,
  PRIMARY KEY (bucket,key)
);
CREATE TABLE waitlist (
  email text PRIMARY KEY, confirmed_at timestamptz,
  confirm_hash bytea UNIQUE, confirm_expires timestamptz, unsubscribe_hash bytea UNIQUE
);
CREATE TABLE consents (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY, email text NOT NULL,
  event text NOT NULL CHECK(event IN ('waitlist_request','waitlist_confirm','waitlist_unsubscribe')),
  policy_version text NOT NULL, recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX consents_email ON consents(email);
CREATE TABLE exams (id text PRIMARY KEY, name text NOT NULL);
INSERT INTO exams VALUES ('crpe','CRPE');
CREATE TABLE content (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  exam_id text NOT NULL REFERENCES exams, kind text NOT NULL CHECK (kind IN ('chapter','lesson','exercise','mock','oral')),
  parent_id uuid REFERENCES content ON DELETE RESTRICT, title text NOT NULL, body text NOT NULL DEFAULT '',
  skill text NOT NULL DEFAULT '', position integer NOT NULL DEFAULT 0 CHECK(position >= 0),
  format text NOT NULL DEFAULT 'text' CHECK(format IN ('text','choice','numeric')), choices text[] NOT NULL DEFAULT '{}',
  criteria text[] NOT NULL DEFAULT '{}', video_url text NOT NULL DEFAULT '',
  answer text NOT NULL DEFAULT '', solution text NOT NULL DEFAULT '', tolerance double precision NOT NULL DEFAULT 0 CHECK(tolerance >= 0),
  status text NOT NULL DEFAULT 'draft' CHECK(status IN ('draft','validated')), archived boolean NOT NULL DEFAULT false
);
CREATE INDEX content_parent ON content(parent_id);
-- Publication of children depends on the entire chain of human validations.
CREATE VIEW published_content AS
WITH RECURSIVE visible AS (
  SELECT * FROM content WHERE parent_id IS NULL AND status='validated' AND NOT archived
  UNION ALL
  SELECT c.* FROM content c JOIN visible v ON c.parent_id=v.id WHERE c.status='validated' AND NOT c.archived
) SELECT * FROM visible;
CREATE TABLE attempts (
  user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE, client_id uuid NOT NULL,
  exercise_id uuid NOT NULL REFERENCES content, answer text NOT NULL, correct boolean NOT NULL,
  feedback text NOT NULL, skill text NOT NULL, performed_at timestamptz NOT NULL, next_review_at timestamptz NOT NULL,
  PRIMARY KEY(user_id,client_id)
);
CREATE INDEX attempts_schedule ON attempts(user_id,exercise_id,performed_at DESC,client_id);
CREATE INDEX attempts_skill ON attempts(user_id,skill,performed_at DESC);
CREATE TABLE submissions (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
  exercise_id uuid NOT NULL REFERENCES content, text_answer text NOT NULL DEFAULT '',
  object_key text NOT NULL DEFAULT '', content_type text NOT NULL DEFAULT '',
  status text NOT NULL DEFAULT 'pending_review' CHECK(status IN ('pending_review','reviewed')),
  feedback text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submissions_owner ON submissions (user_id,created_at DESC);
CREATE TABLE storage_cleanup (
  object_key text PRIMARY KEY, available_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE content_imports (
  user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE, client_id uuid NOT NULL,
  request_hash bytea NOT NULL, response jsonb NOT NULL, PRIMARY KEY(user_id,client_id)
);
ALTER TABLE submissions ADD COLUMN client_id uuid, ADD COLUMN request_hash bytea;
CREATE UNIQUE INDEX submissions_unique_request ON submissions(user_id,client_id);
ALTER TABLE content ADD COLUMN source_title text NOT NULL DEFAULT '', ADD COLUMN source_url text NOT NULL DEFAULT '', ADD COLUMN source_year integer NOT NULL DEFAULT 0;
CREATE TABLE content_documents (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), content_id uuid NOT NULL REFERENCES content,
 title text NOT NULL, size bigint NOT NULL, object_key text NOT NULL UNIQUE
);
CREATE INDEX content_documents_content ON content_documents(content_id);
-- Restore the view's projection after adding source fields.
CREATE OR REPLACE VIEW published_content AS WITH RECURSIVE visible AS (
 SELECT * FROM content WHERE parent_id IS NULL AND status='validated' AND NOT archived
 UNION ALL SELECT c.* FROM content c JOIN visible v ON c.parent_id=v.id WHERE c.status='validated' AND NOT c.archived
) SELECT * FROM visible;
