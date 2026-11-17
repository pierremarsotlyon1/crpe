ALTER TABLE submissions ADD COLUMN correction jsonb;
ALTER TABLE submissions DROP CONSTRAINT submissions_status_check;
ALTER TABLE submissions ADD CONSTRAINT submissions_status_check CHECK(status IN ('pending_review','ai_reviewed','reviewed'));
CREATE TABLE correction_usage (
  submission_id uuid PRIMARY KEY REFERENCES submissions ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users ON DELETE CASCADE,
  reserved_at timestamptz NOT NULL DEFAULT now(),
  finished_at timestamptz,
  provider text NOT NULL,
  model text NOT NULL DEFAULT '',
  input_tokens bigint NOT NULL DEFAULT 0,
  output_tokens bigint NOT NULL DEFAULT 0,
  estimated_cost_usd numeric,
  outcome text NOT NULL DEFAULT 'reserved'
);
CREATE INDEX correction_usage_daily ON correction_usage(user_id,reserved_at);
