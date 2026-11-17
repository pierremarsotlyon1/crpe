ALTER TABLE jobs ADD COLUMN updated_at timestamptz;
ALTER TABLE submissions ADD COLUMN updated_at timestamptz;
UPDATE jobs SET updated_at=coalesce(completed_at,created_at);
UPDATE submissions SET updated_at=created_at;
ALTER TABLE jobs ALTER COLUMN updated_at SET DEFAULT clock_timestamp(), ALTER COLUMN updated_at SET NOT NULL;
ALTER TABLE submissions ALTER COLUMN updated_at SET DEFAULT clock_timestamp(), ALTER COLUMN updated_at SET NOT NULL;
CREATE FUNCTION record_update_time() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.updated_at := clock_timestamp();
 RETURN NEW;
END;
$$;
CREATE TRIGGER jobs_transition_time BEFORE UPDATE ON jobs FOR EACH ROW EXECUTE FUNCTION record_update_time();
CREATE TRIGGER submissions_transition_time BEFORE UPDATE ON submissions FOR EACH ROW EXECUTE FUNCTION record_update_time();
CREATE VIEW submission_tracking AS
SELECT s.*, jsonb_build_object(
 'stage',CASE
  WHEN s.status='reviewed' THEN 'completed'
  WHEN s.correction->>'needsTeacherReview'='true' THEN 'teacher_review'
  WHEN s.status='ai_reviewed' AND s.correction IS NOT NULL THEN 'completed'
  WHEN j.id IS NULL THEN 'teacher_review'
  WHEN j.status='failed' THEN 'failed'
  WHEN j.status='pending' AND j.attempts=0 THEN 'queued'
  WHEN j.status='running' AND j.lease_until>now() THEN 'processing'
  WHEN j.status IN ('pending','running') THEN 'retrying'
  ELSE 'teacher_review'
 END,
 'attempts',coalesce(j.attempts,0),
 'updatedAt',greatest(s.updated_at,j.updated_at,
  CASE WHEN j.status='running' AND j.lease_until<=now() THEN j.lease_until END)
) AS correction_progress
FROM submissions s LEFT JOIN jobs j ON j.deduplication_key='correction/'||s.id::text AND j.kind='correct_submission';
